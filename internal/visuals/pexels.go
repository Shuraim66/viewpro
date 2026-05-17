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
	rejected map[int]bool // video IDs from cache/rejected_clips.json — never reused
}

func New(apiKey, cacheDir string) *Client {
	return &Client{
		apiKey:   apiKey,
		cacheDir: cacheDir,
		http:     &http.Client{Timeout: 120 * time.Second},
		rejected: LoadRejectedClips(cacheDir),
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

// maxClipsPerKeyword is how many distinct clips a single keyword may
// contribute to the b-roll pool. With ~4 keywords this yields ~12-16
// unique clips per Short — enough that a typical 28-32s Short (12-15
// segments) plays with little or no clip repetition.
const maxClipsPerKeyword = 4

// FetchForKeywords builds the b-roll pool. out[0] is always the opener:
// a strict close-up clip (vertical aspect ≥1.6x, duration ≥3s) pinned to
// segment 0 so the opening frame is a high-attention shot. The rest is a
// bulk pool — up to maxClipsPerKeyword clips per keyword — deduplicated
// by video ID. The cut planner shuffles the pool across the segments.
func (c *Client) FetchForKeywords(ctx context.Context, keywords []string) ([]ClipSummary, error) {
	if len(keywords) == 0 {
		return nil, fmt.Errorf("no keywords supplied")
	}
	if n := len(c.rejected); n > 0 {
		fmt.Fprintf(os.Stderr, "[visuals] rejected-clip cache: %d video ID(s) will be skipped\n", n)
	}

	out := []ClipSummary{}
	seen := map[int]bool{}
	add := func(clip ClipSummary, keyword string) {
		if clip.VideoID != 0 && seen[clip.VideoID] {
			return // already in the pool (opener ↔ bulk, or shared across keywords)
		}
		seen[clip.VideoID] = true
		// Stamp the original script keyword (clip may have been found
		// via a simplified or pool fallback query).
		clip.KeywordRequested = keyword
		out = append(out, clip)
	}

	// Opener — one strict clip, pinned to segment 0.
	if opener := c.fetchOpenerWithFallback(ctx, keywords[0]); opener != nil {
		add(*opener, keywords[0])
	}

	// Bulk pool — up to maxClipsPerKeyword clips per keyword.
	for _, kw := range keywords {
		for _, clip := range c.fetchClipsForKeyword(ctx, kw, maxClipsPerKeyword) {
			add(clip, kw)
		}
	}

	if len(out) == 0 {
		return nil, fmt.Errorf("no clips obtained for any of %d keywords", len(keywords))
	}
	fmt.Fprintf(os.Stderr, "[visuals] pool: %d unique clips from %d keywords\n", len(out), len(keywords))
	return out, nil
}

// fetchOpenerWithFallback tries (in order):
//  1. script's first keyword, with strict opener filter (close-up tags
//     in URL slug + height > 1.6×width + duration ≥3s)
//  2. curated opener pool (same strict filter)
//  3. drop strict filter and call the regular fallback chain
func (c *Client) fetchOpenerWithFallback(ctx context.Context, keyword string) *ClipSummary {
	if clips := c.tryQueryFiltered(ctx, keyword, openerVariant, openerFilter, 1); len(clips) > 0 {
		fmt.Fprintf(os.Stderr, "[visuals] opener L1 hit: %q\n", keyword)
		return &clips[0]
	}
	fmt.Fprintf(os.Stderr, "[visuals] opener L1 miss for %q\n", keyword)
	for _, q := range openerPool {
		if clips := c.tryQueryFiltered(ctx, q, openerVariant, openerFilter, 1); len(clips) > 0 {
			fmt.Fprintf(os.Stderr, "[visuals] opener L2 hit: %q → %q (pool)\n", keyword, q)
			return &clips[0]
		}
	}
	fmt.Fprintf(os.Stderr, "[visuals] opener strict failed for %q, using default fallback\n", keyword)
	if clips := c.fetchClipsForKeyword(ctx, keyword, 1); len(clips) > 0 {
		return &clips[0]
	}
	return nil
}

// videoFilter selects (file, ok) for a candidate Pexels video.
// Returns false if the video should be skipped entirely.
type videoFilter func(v pexelsVideo) (*pexelsVideoFile, bool)

const openerVariant = "opener-strict-min3"

// bulkVariant is the cache-key variant for a multi-clip default-filter
// fetch. n is encoded so fetches wanting different counts get separate
// cache entries, and they never collide with the opener's strict cache.
func bulkVariant(n int) string {
	return fmt.Sprintf("bulk-default-n%d", n)
}

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

// slugTitle turns a Pexels video URL into a human-readable title from
// its slug, dropping the trailing numeric ID — e.g.
// ".../video/man-looking-at-camera-12345/" → "man looking at camera".
// Falls back to the raw URL if no slug can be extracted. Used only for
// log messages.
func slugTitle(rawURL string) string {
	s := strings.Trim(rawURL, "/")
	if i := strings.LastIndex(s, "/"); i >= 0 {
		s = s[i+1:]
	}
	parts := strings.Split(s, "-")
	if n := len(parts); n > 0 && isAllDigits(parts[n-1]) {
		parts = parts[:n-1]
	}
	title := strings.Join(parts, " ")
	if title == "" {
		return rawURL
	}
	return title
}

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
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

// fetchClipsForKeyword returns up to n distinct clips for one keyword,
// trying (in order): the original keyword → simplified variants → the
// default pool. The keyword and simplified levels return as many clips
// as the search yields (capped at n); the pool fallback contributes a
// single clip, just to avoid an empty slot.
func (c *Client) fetchClipsForKeyword(ctx context.Context, keyword string, n int) []ClipSummary {
	// Level 1: original keyword
	if clips := c.tryQueryFiltered(ctx, keyword, bulkVariant(n), defaultFilter, n); len(clips) > 0 {
		return clips
	}
	fmt.Fprintf(os.Stderr, "[visuals] L1 miss for %q\n", keyword)

	// Level 2: simplified variants (last 2 words, then last 1)
	for _, simplified := range simplifyKeyword(keyword) {
		if simplified == keyword {
			continue
		}
		if clips := c.tryQueryFiltered(ctx, simplified, bulkVariant(n), defaultFilter, n); len(clips) > 0 {
			fmt.Fprintf(os.Stderr, "[visuals] L2 hit: %q → %q (%d clips)\n", keyword, simplified, len(clips))
			return clips
		}
	}

	// Level 3: default pool — one clip, just to fill the slot
	for _, fallback := range defaultBrollPool {
		if clips := c.tryQueryFiltered(ctx, fallback, bulkVariant(1), defaultFilter, 1); len(clips) > 0 {
			fmt.Fprintf(os.Stderr, "[visuals] L3 hit: %q → %q (pool)\n", keyword, fallback)
			return clips
		}
	}

	fmt.Fprintf(os.Stderr, "[visuals] all fallbacks exhausted for %q; skipping slot\n", keyword)
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

// tryQueryFiltered performs one Pexels search and returns up to n
// downloaded clips selected by the given filter, ranked safest- and
// best-niche-fit first. variant distinguishes cache keys (and, when it
// equals openerVariant, enables soft-intro deprioritization). Returns
// nil if no clip qualifies.
func (c *Client) tryQueryFiltered(ctx context.Context, query, variant string, filter videoFilter, n int) []ClipSummary {
	if n < 1 {
		n = 1
	}
	if cached, ok := readCache(c.cacheDir, query, variant); ok && len(cached.Meta) > 0 {
		// Re-screen cached clips: the blocked-terms list may have grown
		// since they were cached, or they predate the safety filter.
		var valid []ClipSummary
		for _, clip := range cached.Meta {
			if c.rejected[clip.VideoID] {
				fmt.Fprintf(os.Stderr, "[visuals] rejected-clip: cached video %d for %q dropped\n", clip.VideoID, query)
				continue
			}
			if term := blockedTermIn(clipMetadata(clip.URL, clip.Tags)); term != "" {
				fmt.Fprintf(os.Stderr, "[visuals] safety: cached clip for %q has blocked term %q, dropping\n", query, term)
				continue
			}
			valid = append(valid, clip)
		}
		if len(valid) > 0 {
			if len(valid) > n {
				valid = valid[:n]
			}
			return valid
		}
		// every cached clip is now blocked — fall through to re-search
	}

	q := url.Values{}
	q.Set("query", query)
	q.Set("orientation", "portrait")
	q.Set("size", "medium")
	q.Set("per_page", "25")

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
		v         pexelsVideo
		file      *pexelsVideoFile
		score     int
		softIntro bool
	}
	// The opener variant gets soft-intro deprioritization; segments 1+
	// (defaultVariant) don't — only segment 0's first frame is on screen
	// before the viewer decides to keep watching.
	isOpener := variant == openerVariant
	var candidates []scored
	softIntroSeen := false
	for _, v := range search.Videos {
		if c.rejected[v.ID] {
			fmt.Fprintf(os.Stderr, "[visuals] rejected-clip: skip video %d for %q (in %s)\n",
				v.ID, query, rejectedClipsFile)
			continue
		}
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
		score := preferredScore(meta)
		soft := false
		if isOpener {
			if term := softIntroTermIn(meta); term != "" {
				soft = true
				softIntroSeen = true
				score -= softIntroPenalty
				fmt.Fprintf(os.Stderr, "[visuals] seg 0: deprioritized %q — soft intro likely (%q)\n",
					slugTitle(v.URL), term)
			}
		}
		// Screen-recording risk applies to every segment, not just the
		// opener — a personal-data leak is unsafe wherever it appears.
		if term := screenRecordingTermIn(meta); term != "" {
			score -= screenRecordingPenalty
			fmt.Fprintf(os.Stderr, "[visuals] deprioritized %q — screen-recording risk (%q)\n",
				slugTitle(v.URL), term)
		}
		candidates = append(candidates, scored{v, file, score, soft})
	}
	// Stable sort by rank score, descending. Stable keeps Pexels' own
	// relevance order as the tiebreaker within equal scores.
	sort.SliceStable(candidates, func(i, j int) bool {
		return candidates[i].score > candidates[j].score
	})

	var out []ClipSummary
	for _, cand := range candidates {
		if len(out) >= n {
			break
		}
		v, file := cand.v, cand.file
		dst := clipPath(c.cacheDir, v.ID)
		if _, err := os.Stat(dst); err != nil {
			if err := c.download(ctx, file.Link, dst); err != nil {
				fmt.Fprintf(os.Stderr, "[visuals] download error for %q: %v\n", query, err)
				continue // try the next-best candidate
			}
		}
		if isOpener && softIntroSeen && !cand.softIntro && len(out) == 0 {
			fmt.Fprintf(os.Stderr, "[visuals] seg 0: selected %q (ranked higher than soft-intro alternative)\n",
				slugTitle(v.URL))
		}
		out = append(out, ClipSummary{
			VideoID:  v.ID,
			Path:     dst,
			Width:    file.Width,
			Height:   file.Height,
			Duration: v.Duration,
			URL:      v.URL,
			Tags:     v.Tags,
		})
	}
	if len(out) > 0 {
		paths := make([]string, len(out))
		for i := range out {
			paths[i] = out[i].Path
		}
		_ = writeCache(c.cacheDir, query, variant, &cachedSearch{
			ClipPaths: paths,
			Meta:      out,
		})
	}
	return out
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
