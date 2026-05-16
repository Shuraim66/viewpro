// Package visuals searches Pexels for portrait stock video clips and
// caches the downloads on disk so repeat runs are nearly free.
package visuals

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	"viewpro/internal/httpx"
)

const searchEndpoint = "https://api.pexels.com/videos/search"

type Client struct {
	apiKey   string
	cacheDir string
	http     *http.Client
}

func New(apiKey, cacheDir string) *Client {
	return &Client{
		apiKey:   apiKey,
		cacheDir: cacheDir,
		http:     &http.Client{Timeout: 120 * time.Second},
	}
}

// Pexels API response shapes (only what we use)
type pexelsSearchResp struct {
	Videos []pexelsVideo `json:"videos"`
}

type pexelsVideo struct {
	ID         int               `json:"id"`
	Width      int               `json:"width"`
	Height     int               `json:"height"`
	Duration   int               `json:"duration"`
	URL        string            `json:"url"`
	Tags       []string          `json:"tags"`
	VideoFiles []pexelsVideoFile `json:"video_files"`
}

type pexelsVideoFile struct {
	ID       int    `json:"id"`
	Quality  string `json:"quality"` // "hd" | "sd" | "uhd"
	FileType string `json:"file_type"`
	Width    int    `json:"width"`
	Height   int    `json:"height"`
	Link     string `json:"link"`
}

// defaultBrollPool is the last-resort cycle used when both the original
// keyword and its simplification return zero portrait clips.
var defaultBrollPool = []string{
	"thinking",
	"person alone",
	"couple talking",
	"eye close up",
	"city night",
	"city day",
	"hands gesturing",
	"person looking down",
}

// openerPool is the curated "high-stop" fallback for the opening clip.
// Used when the script's first keyword doesn't yield a close-up/portrait
// clip that meets opener criteria.
var openerPool = []string{
	"person looking at camera",
	"close up eyes",
	"face close up portrait",
	"woman looking at camera close up",
	"man looking at camera",
}

// FetchForKeywords runs one search per keyword and returns one downloaded
// clip per keyword. The first keyword uses a stricter selection (close-up
// + vertical aspect ≥1.6x + duration ≥3s) so the opening frame is a
// high-attention shot, not a wide/landscape b-roll. Subsequent keywords
// use the default 3-level fallback.
func (c *Client) FetchForKeywords(ctx context.Context, keywords []string) ([]ClipSummary, error) {
	var out []ClipSummary
	for i, kw := range keywords {
		var clip *ClipSummary
		if i == 0 {
			clip = c.fetchOpenerWithFallback(ctx, kw)
		} else {
			clip = c.fetchWithFallback(ctx, kw)
		}
		if clip != nil {
			// Stamp the original script keyword (clip may have been
			// found via a simplified or pool fallback query).
			clip.KeywordRequested = kw
			out = append(out, *clip)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no clips obtained for any of %d keywords", len(keywords))
	}
	return out, nil
}

// fetchOpenerWithFallback tries (in order):
//  1. script's first keyword, with strict opener filter (close-up tags
//     in URL slug + height > 1.6×width + duration ≥3s)
//  2. curated opener pool (same strict filter)
//  3. drop strict filter and call the regular fallback chain
func (c *Client) fetchOpenerWithFallback(ctx context.Context, keyword string) *ClipSummary {
	if clip := c.tryQueryFiltered(ctx, keyword, openerVariant, openerFilter); clip != nil {
		fmt.Fprintf(os.Stderr, "[visuals] opener L1 hit: %q\n", keyword)
		return clip
	}
	fmt.Fprintf(os.Stderr, "[visuals] opener L1 miss for %q\n", keyword)
	for _, q := range openerPool {
		if clip := c.tryQueryFiltered(ctx, q, openerVariant, openerFilter); clip != nil {
			fmt.Fprintf(os.Stderr, "[visuals] opener L2 hit: %q → %q (pool)\n", keyword, q)
			return clip
		}
	}
	fmt.Fprintf(os.Stderr, "[visuals] opener strict failed for %q, using default fallback\n", keyword)
	return c.fetchWithFallback(ctx, keyword)
}

// videoFilter selects (file, ok) for a candidate Pexels video.
// Returns false if the video should be skipped entirely.
type videoFilter func(v pexelsVideo) (*pexelsVideoFile, bool)

const (
	defaultVariant = "default-min4"
	openerVariant  = "opener-strict-min3"
)

// defaultFilter: existing behavior — duration ≥4, any portrait file ≥1080px wide.
func defaultFilter(v pexelsVideo) (*pexelsVideoFile, bool) {
	if v.Duration < 4 {
		return nil, false
	}
	f := bestPortraitFile(v.VideoFiles)
	if f == nil {
		return nil, false
	}
	return f, true
}

// openerFilter: strict close-up portrait — height/width >1.6, duration ≥3,
// URL slug must contain close-up/portrait/face/head tokens.
func openerFilter(v pexelsVideo) (*pexelsVideoFile, bool) {
	if v.Duration < 3 {
		return nil, false
	}
	if !urlSuggestsCloseup(v.URL) {
		return nil, false
	}
	f := bestStrictPortraitFile(v.VideoFiles)
	if f == nil {
		return nil, false
	}
	return f, true
}

func urlSuggestsCloseup(rawURL string) bool {
	u := strings.ToLower(rawURL)
	for _, needle := range []string{"close-up", "closeup", "close up", "portrait", "face", "looking-at"} {
		if strings.Contains(u, needle) {
			return true
		}
	}
	return false
}

// bestStrictPortraitFile picks an HD file with strict portrait ratio
// (height > 1.6×width) — stricter than bestPortraitFile.
func bestStrictPortraitFile(files []pexelsVideoFile) *pexelsVideoFile {
	var candidates []pexelsVideoFile
	for _, f := range files {
		if f.FileType != "video/mp4" {
			continue
		}
		if f.Width < 1080 {
			continue
		}
		if float64(f.Height) <= 1.6*float64(f.Width) {
			continue
		}
		candidates = append(candidates, f)
	}
	if len(candidates) == 0 {
		return nil
	}
	sort.Slice(candidates, func(i, j int) bool {
		return abs(candidates[i].Width-1080) < abs(candidates[j].Width-1080)
	})
	return &candidates[0]
}

// fetchWithFallback tries (in order): original keyword → simplified
// variants (drop leading filler words) → default pool. Logs which level
// produced the clip.
func (c *Client) fetchWithFallback(ctx context.Context, query string) *ClipSummary {
	// Level 1: original
	if clip := c.tryQueryFiltered(ctx, query, defaultVariant, defaultFilter); clip != nil {
		return clip
	}
	fmt.Fprintf(os.Stderr, "[visuals] L1 miss for %q\n", query)

	// Level 2: simplified variants (last 2 words, then last 1)
	for _, simplified := range simplifyKeyword(query) {
		if simplified == query {
			continue
		}
		if clip := c.tryQueryFiltered(ctx, simplified, defaultVariant, defaultFilter); clip != nil {
			fmt.Fprintf(os.Stderr, "[visuals] L2 hit: %q → %q\n", query, simplified)
			return clip
		}
	}

	// Level 3: default pool
	for _, fallback := range defaultBrollPool {
		if clip := c.tryQueryFiltered(ctx, fallback, defaultVariant, defaultFilter); clip != nil {
			fmt.Fprintf(os.Stderr, "[visuals] L3 hit: %q → %q (pool)\n", query, fallback)
			return clip
		}
	}

	fmt.Fprintf(os.Stderr, "[visuals] all fallbacks exhausted for %q; skipping slot\n", query)
	return nil
}

// simplifyKeyword returns progressively-shorter variants of query.
// "nervous hand gestures" → ["hand gestures", "gestures"]
// "phone" → []  (already 1 word, no simplification)
func simplifyKeyword(query string) []string {
	words := strings.Fields(query)
	var out []string
	if len(words) >= 3 {
		out = append(out, strings.Join(words[len(words)-2:], " "))
	}
	if len(words) >= 2 {
		out = append(out, words[len(words)-1])
	}
	return out
}

// tryQueryFiltered performs one Pexels search and selects a clip using
// the given filter. variant distinguishes cache keys so the opener path
// doesn't pull a non-strict cached clip from a prior default search.
func (c *Client) tryQueryFiltered(ctx context.Context, query, variant string, filter videoFilter) *ClipSummary {
	if cached, ok := readCache(c.cacheDir, query, variant); ok && len(cached.Meta) > 0 {
		clip := cached.Meta[0]
		// Re-screen cached clips: the blocked-terms list may have grown
		// since this clip was cached, or the clip predates the filter.
		if term := blockedTermIn(clipMetadata(clip.URL, clip.Tags)); term != "" {
			fmt.Fprintf(os.Stderr, "[visuals] safety: cached clip for %q has blocked term %q, re-searching\n", query, term)
		} else {
			return &clip
		}
	}

	q := url.Values{}
	q.Set("query", query)
	q.Set("orientation", "portrait")
	q.Set("size", "medium")
	q.Set("per_page", "15")

	resp, err := httpx.Do(ctx, 3, time.Second, func() (*http.Response, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, searchEndpoint+"?"+q.Encode(), nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", c.apiKey)
		return c.http.Do(req)
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "[visuals] search error for %q: %v\n", query, err)
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		preview, _ := io.ReadAll(io.LimitReader(resp.Body, 200))
		fmt.Fprintf(os.Stderr, "[visuals] http %d for %q: %s\n", resp.StatusCode, query, string(preview))
		return nil
	}

	var search pexelsSearchResp
	if err := json.NewDecoder(resp.Body).Decode(&search); err != nil {
		fmt.Fprintf(os.Stderr, "[visuals] decode error for %q: %v\n", query, err)
		return nil
	}

	// Screen + rank all candidates before downloading anything, so the
	// chosen clip is the safest, best-niche-fit result — not just the
	// first one Pexels happened to return.
	type scored struct {
		v     pexelsVideo
		file  *pexelsVideoFile
		score int
	}
	var candidates []scored
	for _, v := range search.Videos {
		file, ok := filter(v)
		if !ok {
			continue
		}
		meta := clipMetadata(v.URL, v.Tags)
		if term := blockedTermIn(meta); term != "" {
			fmt.Fprintf(os.Stderr, "[visuals] safety: skip video %d for %q — blocked term %q (%s)\n",
				v.ID, query, term, v.URL)
			continue
		}
		candidates = append(candidates, scored{v, file, preferredScore(meta)})
	}
	// Stable sort by preferred-term score, descending. Stable keeps
	// Pexels' own relevance order as the tiebreaker within equal scores.
	sort.SliceStable(candidates, func(i, j int) bool {
		return candidates[i].score > candidates[j].score
	})

	for _, cand := range candidates {
		v, file := cand.v, cand.file
		dst := clipPath(c.cacheDir, v.ID)
		if _, err := os.Stat(dst); err != nil {
			if err := c.download(ctx, file.Link, dst); err != nil {
				fmt.Fprintf(os.Stderr, "[visuals] download error for %q: %v\n", query, err)
				continue // try the next-best candidate
			}
		}
		summary := ClipSummary{
			VideoID:  v.ID,
			Path:     dst,
			Width:    file.Width,
			Height:   file.Height,
			Duration: v.Duration,
			URL:      v.URL,
			Tags:     v.Tags,
		}
		_ = writeCache(c.cacheDir, query, variant, &cachedSearch{
			ClipPaths: []string{dst},
			Meta:      []ClipSummary{summary},
		})
		return &summary
	}
	return nil
}

// bestPortraitFile picks the highest-resolution portrait HD file ≤ 1920p.
// Pexels returns the same clip at multiple resolutions; we want the
// smallest one that's still ≥ 1080 wide to keep download size sane.
func bestPortraitFile(files []pexelsVideoFile) *pexelsVideoFile {
	var portrait []pexelsVideoFile
	for _, f := range files {
		if f.Height > f.Width && f.Width >= 1080 && f.FileType == "video/mp4" {
			portrait = append(portrait, f)
		}
	}
	if len(portrait) == 0 {
		return nil
	}
	// Prefer HD over UHD (smaller) and width closest to 1080
	sort.Slice(portrait, func(i, j int) bool {
		di := abs(portrait[i].Width - 1080)
		dj := abs(portrait[j].Width - 1080)
		return di < dj
	})
	return &portrait[0]
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func (c *Client) download(ctx context.Context, link, dst string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, link, nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("download: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download: http %d", resp.StatusCode)
	}
	tmp := dst + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, dst)
}
