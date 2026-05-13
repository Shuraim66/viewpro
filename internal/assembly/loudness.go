package assembly

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"os/exec"
	"regexp"
	"strconv"
)

var meanVolumeRE = regexp.MustCompile(`mean_volume:\s*(-?[\d.]+)\s*dB`)

// ProbeMeanVolume reads the ffmpeg `volumedetect` output and returns
// the mean volume in dB (typically negative).
func ProbeMeanVolume(ctx context.Context, audioPath string) (float64, error) {
	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-i", audioPath,
		"-af", "volumedetect",
		"-f", "null", "-")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	// volumedetect writes to stderr; exit code is fine on null muxer.
	_ = cmd.Run()

	m := meanVolumeRE.FindStringSubmatch(stderr.String())
	if len(m) < 2 {
		return 0, fmt.Errorf("could not find mean_volume in ffmpeg output")
	}
	return strconv.ParseFloat(m[1], 64)
}

// MusicVolumeForVoice maps a voice mean volume (dB) into a background
// music volume in [0.08, 0.18]. Formula:
//
//	musicVol = clamp(0.08, 0.18, 0.15 + (voiceMean+18)*0.01)
//
// Louder voice → slightly louder music (we have more headroom).
// Quieter voice → quieter music (so it doesn't drown the voice).
//
// Reference points:
//
//	voiceMean = -18 dB →  0.15  (neutral)
//	voiceMean = -10 dB →  0.18  (clamped from 0.23)
//	voiceMean = -28 dB →  0.08  (clamped from 0.05)
func MusicVolumeForVoice(voiceMeanDB float64) float64 {
	v := 0.15 + (voiceMeanDB+18)*0.01
	return math.Max(0.08, math.Min(0.18, v))
}
