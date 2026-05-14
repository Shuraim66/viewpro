// Package music picks a random audio track from a directory, or returns
// a single file unchanged. Used by the pipeline to add per-run variation
// (different music in each Short reduces "template" feel for YouTube's
// inauthentic-content checks).
package music

import (
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strings"
)

var audioExtensions = map[string]bool{
	".mp3":  true,
	".wav":  true,
	".m4a":  true,
	".aac":  true,
	".ogg":  true,
	".flac": true,
	".opus": true,
}

// Pick returns the absolute path of an audio file. If path is a single
// audio file, returns it as-is. If path is a directory, scans it for
// audio files (non-recursive) and returns a random one.
func Pick(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("music path %s: %w", abs, err)
	}
	if !info.IsDir() {
		return abs, nil
	}
	entries, err := os.ReadDir(abs)
	if err != nil {
		return "", err
	}
	var candidates []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if audioExtensions[strings.ToLower(filepath.Ext(e.Name()))] {
			candidates = append(candidates, filepath.Join(abs, e.Name()))
		}
	}
	if len(candidates) == 0 {
		return "", fmt.Errorf("no audio files found in %s", abs)
	}
	return candidates[rand.IntN(len(candidates))], nil
}

// List returns absolute paths of all audio files in a directory.
// Useful for debugging / showing what the rotation pool looks like.
// Returns nil for a single-file path.
func List(path string) []string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil
	}
	info, err := os.Stat(abs)
	if err != nil || !info.IsDir() {
		return nil
	}
	entries, err := os.ReadDir(abs)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if audioExtensions[strings.ToLower(filepath.Ext(e.Name()))] {
			out = append(out, filepath.Join(abs, e.Name()))
		}
	}
	return out
}
