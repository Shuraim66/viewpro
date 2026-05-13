// Package beats detects speech onsets in a voice MP3 (via librosa
// subprocess) and turns them into a cut plan for b-roll switching.
package beats

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
)

// Result is what beats.py prints to stdout.
type Result struct {
	Duration float64   `json:"duration"`
	Onsets   []float64 `json:"onsets"`
}

// Detect shells out to .venv/bin/python3 python/beats.py.
// pythonPath defaults to ".venv/bin/python3" if empty.
func Detect(ctx context.Context, mp3Path, pythonPath, scriptPath string) (*Result, error) {
	if pythonPath == "" {
		pythonPath = ".venv/bin/python3"
	}
	if scriptPath == "" {
		scriptPath = "python/beats.py"
	}
	cmd := exec.CommandContext(ctx, pythonPath, scriptPath, mp3Path)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("beats.py: %w; stderr=%q", err, stderr.String())
	}
	var r Result
	if err := json.Unmarshal(stdout.Bytes(), &r); err != nil {
		return nil, fmt.Errorf("beats.py: parse stdout: %w; raw=%q", err, stdout.String())
	}
	return &r, nil
}
