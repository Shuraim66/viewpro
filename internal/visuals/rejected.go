package visuals

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Persistent rejected-clip cache. When a Short trips a content-safety
// review (e.g. a screen-recording clip turned out to expose personal
// data), the offending Pexels video IDs are recorded here so every
// future run skips them automatically — no re-litigating known-bad
// clips. The cache spans sessions; it lives at cache/rejected_clips.json.

const rejectedClipsFile = "rejected_clips.json"

// RejectedClip records one Pexels video that must never be reused.
type RejectedClip struct {
	VideoID    int    `json:"video_id"`
	Reason     string `json:"reason,omitempty"`
	Session    string `json:"session,omitempty"` // session dir that surfaced the problem
	RejectedAt string `json:"rejected_at"`
}

type rejectedClipsCache struct {
	Rejected []RejectedClip `json:"rejected"`
}

// LoadRejectedClips reads cache/rejected_clips.json into a video-ID set.
// A missing file is not an error — it returns an empty set. A corrupt
// file is logged and treated as empty (better to fetch than to crash).
func LoadRejectedClips(cacheDir string) map[int]bool {
	set := map[int]bool{}
	data, err := os.ReadFile(filepath.Join(cacheDir, rejectedClipsFile))
	if err != nil {
		return set
	}
	var c rejectedClipsCache
	if err := json.Unmarshal(data, &c); err != nil {
		fmt.Fprintf(os.Stderr, "[visuals] WARN %s is corrupt (%v); ignoring\n", rejectedClipsFile, err)
		return set
	}
	for _, r := range c.Rejected {
		set[r.VideoID] = true
	}
	return set
}

// RejectClip adds a video ID to cache/rejected_clips.json, creating the
// file if needed. It is idempotent: a video already listed is left as
// is. Returns true if the clip was newly added.
func RejectClip(cacheDir string, videoID int, reason, session string) (bool, error) {
	path := filepath.Join(cacheDir, rejectedClipsFile)
	var c rejectedClipsCache
	if data, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(data, &c)
	}
	for _, r := range c.Rejected {
		if r.VideoID == videoID {
			return false, nil // already rejected
		}
	}
	c.Rejected = append(c.Rejected, RejectedClip{
		VideoID:    videoID,
		Reason:     reason,
		Session:    session,
		RejectedAt: time.Now().UTC().Format(time.RFC3339),
	})
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return false, err
	}
	data, err := json.MarshalIndent(&c, "", "  ")
	if err != nil {
		return false, err
	}
	return true, os.WriteFile(path, data, 0o644)
}
