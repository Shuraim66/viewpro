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
	ID         int                 `json:"id"`
	Width      int                 `json:"width"`
	Height     int                 `json:"height"`
	Duration   int                 `json:"duration"`
	URL        string              `json:"url"`
	VideoFiles []pexelsVideoFile   `json:"video_files"`
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
// keyword and its simplification return zero portrait clips. Generic
// enough to fit any psychology Short.
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

// FetchForKeywords runs one search per keyword and returns one downloaded
// clip per keyword. Each keyword goes through a 3-level fallback chain
// so the pipeline doesn't lose a slot just because Pexels didn't have
// a matching clip for an oddly-phrased query.
func (c *Client) FetchForKeywords(ctx context.Context, keywords []string) ([]ClipSummary, error) {
	var out []ClipSummary
	for _, kw := range keywords {
		clip := c.fetchWithFallback(ctx, kw)
		if clip != nil {
			out = append(out, *clip)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no clips obtained for any of %d keywords", len(keywords))
	}
	return out, nil
}

// fetchWithFallback tries (in order): original keyword → simplified
// variants (drop leading filler words) → default pool. Logs which level
// produced the clip.
func (c *Client) fetchWithFallback(ctx context.Context, query string) *ClipSummary {
	// Level 1: original
	if clip := c.tryQuery(ctx, query); clip != nil {
		return clip
	}
	fmt.Fprintf(os.Stderr, "[visuals] L1 miss for %q\n", query)

	// Level 2: simplified variants (last 2 words, then last 1)
	for _, simplified := range simplifyKeyword(query) {
		if simplified == query {
			continue
		}
		if clip := c.tryQuery(ctx, simplified); clip != nil {
			fmt.Fprintf(os.Stderr, "[visuals] L2 hit: %q → %q\n", query, simplified)
			return clip
		}
	}

	// Level 3: default pool
	for _, fallback := range defaultBrollPool {
		if clip := c.tryQuery(ctx, fallback); clip != nil {
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

// tryQuery is the inner helper that performs one Pexels search + download.
// Returns nil on any error or no-results (errors logged at warn level).
func (c *Client) tryQuery(ctx context.Context, query string) *ClipSummary {
	// Cache hit
	if cached, ok := readCache(c.cacheDir, query); ok && len(cached.Meta) > 0 {
		clip := cached.Meta[0]
		return &clip
	}

	q := url.Values{}
	q.Set("query", query)
	q.Set("orientation", "portrait")
	q.Set("size", "medium")
	q.Set("per_page", "10")

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

	for _, v := range search.Videos {
		if v.Duration < 4 {
			continue
		}
		file := bestPortraitFile(v.VideoFiles)
		if file == nil {
			continue
		}
		dst := clipPath(c.cacheDir, v.ID)
		if _, err := os.Stat(dst); err != nil {
			if err := c.download(ctx, file.Link, dst); err != nil {
				fmt.Fprintf(os.Stderr, "[visuals] download error: %v\n", err)
				return nil
			}
		}
		summary := ClipSummary{
			VideoID:  v.ID,
			Path:     dst,
			Width:    file.Width,
			Height:   file.Height,
			Duration: v.Duration,
			URL:      v.URL,
		}
		_ = writeCache(c.cacheDir, query, &cachedSearch{
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
