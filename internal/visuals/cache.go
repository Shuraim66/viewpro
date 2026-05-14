package visuals

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const cacheTTL = 24 * time.Hour

type cachedSearch struct {
	StoredAt  time.Time     `json:"stored_at"`
	ClipPaths []string      `json:"clip_paths"` // absolute paths in CacheDir
	Meta      []ClipSummary `json:"meta"`
}

// ClipSummary is the metadata we keep about a downloaded clip.
type ClipSummary struct {
	VideoID  int    `json:"video_id"`
	Path     string `json:"path"` // absolute
	Width    int    `json:"width"`
	Height   int    `json:"height"`
	Duration int    `json:"duration"`
	URL      string `json:"url"`
}

// searchKey hashes (query, variant). variant distinguishes search criteria
// (e.g., "default" vs "opener-strict") so the opener fallback doesn't get
// a non-strict cached clip from an earlier default search.
func searchKey(query, variant string) string {
	sum := sha256.Sum256([]byte(query + "|portrait|" + variant))
	return hex.EncodeToString(sum[:])[:12]
}

func cachePath(cacheDir, query, variant string) string {
	return filepath.Join(cacheDir, "search_"+searchKey(query, variant)+".json")
}

func readCache(cacheDir, query, variant string) (*cachedSearch, bool) {
	path := cachePath(cacheDir, query, variant)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	var c cachedSearch
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, false
	}
	if time.Since(c.StoredAt) > cacheTTL {
		return nil, false
	}
	// Verify each clip still exists on disk
	for _, p := range c.ClipPaths {
		if _, err := os.Stat(p); err != nil {
			return nil, false
		}
	}
	return &c, true
}

func writeCache(cacheDir, query, variant string, c *cachedSearch) error {
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return err
	}
	c.StoredAt = time.Now()
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(cachePath(cacheDir, query, variant), data, 0o644)
}

func clipPath(cacheDir string, videoID int) string {
	return filepath.Join(cacheDir, fmt.Sprintf("pexels_%d.mp4", videoID))
}
