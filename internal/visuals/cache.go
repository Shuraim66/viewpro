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

func searchKey(query string) string {
	sum := sha256.Sum256([]byte(query + "|portrait|min4"))
	return hex.EncodeToString(sum[:])[:12]
}

func cachePath(cacheDir, query string) string {
	return filepath.Join(cacheDir, "search_"+searchKey(query)+".json")
}

func readCache(cacheDir, query string) (*cachedSearch, bool) {
	path := cachePath(cacheDir, query)
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

func writeCache(cacheDir, query string, c *cachedSearch) error {
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return err
	}
	c.StoredAt = time.Now()
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(cachePath(cacheDir, query), data, 0o644)
}

func clipPath(cacheDir string, videoID int) string {
	return filepath.Join(cacheDir, fmt.Sprintf("pexels_%d.mp4", videoID))
}
