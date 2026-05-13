package voice

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

// SilenceBounds describes leading and trailing silence in an audio file.
// LeadEnd=0 means no leading silence; TailStart=Duration means no trailing silence.
type SilenceBounds struct {
	Duration  float64 // total source duration in seconds
	LeadEnd   float64 // first non-silent moment
	TailStart float64 // start of trailing silence (=Duration if none)
}

// DetectSilence runs `ffmpeg -af silencedetect` and parses the bounds.
// thresholdDB is the noise floor (e.g. -40); minDur is the minimum
// silence duration to detect (e.g. 0.05).
func DetectSilence(ctx context.Context, path string, thresholdDB, minDur float64) (*SilenceBounds, error) {
	// Total duration via ffprobe
	durCmd := exec.CommandContext(ctx, "ffprobe", "-v", "error",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		path)
	var durOut bytes.Buffer
	durCmd.Stdout = &durOut
	if err := durCmd.Run(); err != nil {
		return nil, fmt.Errorf("ffprobe duration: %w", err)
	}
	duration, err := strconv.ParseFloat(strings.TrimSpace(durOut.String()), 64)
	if err != nil {
		return nil, fmt.Errorf("parse duration %q: %w", durOut.String(), err)
	}

	af := fmt.Sprintf("silencedetect=noise=%.0fdB:d=%.3f", thresholdDB, minDur)
	cmd := exec.CommandContext(ctx, "ffmpeg", "-i", path, "-af", af, "-f", "null", "-")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	// ffmpeg writes silencedetect to stderr; exit code is fine on null muxer.
	_ = cmd.Run()

	return parseSilenceOutput(stderr.String(), duration), nil
}

var (
	silenceStartRE = regexp.MustCompile(`silence_start:\s*(-?[\d.]+)`)
	silenceEndRE   = regexp.MustCompile(`silence_end:\s*(-?[\d.]+)`)
)

func parseSilenceOutput(stderr string, duration float64) *SilenceBounds {
	starts := silenceStartRE.FindAllStringSubmatch(stderr, -1)
	ends := silenceEndRE.FindAllStringSubmatch(stderr, -1)

	b := &SilenceBounds{Duration: duration, LeadEnd: 0, TailStart: duration}

	// Leading silence: silence_start at ~0, paired with a silence_end
	if len(starts) > 0 && len(ends) > 0 {
		firstStart, _ := strconv.ParseFloat(starts[0][1], 64)
		firstEnd, _ := strconv.ParseFloat(ends[0][1], 64)
		if firstStart <= 0.01 {
			b.LeadEnd = firstEnd
		}
	}

	// Trailing silence: more starts than ends (unmatched final start = EOF)
	// OR last end ≈ duration meaning silence ran to EOF
	if len(starts) > len(ends) {
		last, _ := strconv.ParseFloat(starts[len(starts)-1][1], 64)
		b.TailStart = last
	} else if len(ends) > 0 {
		lastEnd, _ := strconv.ParseFloat(ends[len(ends)-1][1], 64)
		if math.Abs(lastEnd-duration) < 0.1 {
			lastStart, _ := strconv.ParseFloat(starts[len(starts)-1][1], 64)
			b.TailStart = lastStart
		}
	}

	return b
}

// TrimSilence trims leading + trailing silence from inPath to outPath.
// Returns the new (trimmed) duration in seconds and the leading shift
// that should be subtracted from word timestamps.
func TrimSilence(ctx context.Context, inPath, outPath string) (newDuration, leadingShift float64, err error) {
	bounds, err := DetectSilence(ctx, inPath, -40, 0.05)
	if err != nil {
		return 0, 0, err
	}

	lead := bounds.LeadEnd
	tail := bounds.TailStart
	if tail < 0 || tail > bounds.Duration {
		tail = bounds.Duration
	}
	duration := tail - lead
	if duration <= 0.1 {
		return 0, 0, fmt.Errorf("trimmed audio < 0.1s (lead=%.3f tail=%.3f total=%.3f)", lead, tail, bounds.Duration)
	}

	// Use atrim filter for sample-accurate trimming (more precise than -ss/-t)
	af := fmt.Sprintf("atrim=%.3f:%.3f,asetpts=PTS-STARTPTS", lead, tail)
	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-y", "-i", inPath,
		"-af", af,
		"-c:a", "libmp3lame", "-q:a", "2",
		outPath)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		s := stderr.String()
		if len(s) > 800 {
			s = "…" + s[len(s)-800:]
		}
		return 0, 0, fmt.Errorf("ffmpeg atrim: %w; stderr=%s", err, s)
	}

	return duration, lead, nil
}

// ShiftWords subtracts leadingShift from each word's timestamps and
// clamps them into [0, newDuration]. Drops words that end before 0.
func ShiftWords(words []Word, leadingShift, newDuration float64) []Word {
	out := make([]Word, 0, len(words))
	for _, w := range words {
		w.Start -= leadingShift
		w.End -= leadingShift
		if w.End <= 0 {
			continue
		}
		if w.Start < 0 {
			w.Start = 0
		}
		if w.End > newDuration {
			w.End = newDuration
		}
		out = append(out, w)
	}
	return out
}
