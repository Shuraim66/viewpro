// Package assembly runs FFmpeg to glue b-roll segments + voice + music +
// burned-in subtitles into the final 1080x1920 MP4.
//
// Always uses the concat filter (not the concat demuxer) because the
// demuxer corrupts PTS when the same source file is reused multiple
// times in a row — which our round-robin clip assignment routinely does.
// Symptom of that bug: freezes during reused-clip segments and
// captions reverting to t=0 because the subtitles filter sees PTS reset.
//
// Each input clip is loaded exactly once via -i; the filter graph references
// the same [k:v] label across multiple trim chains for reused clips.
// Per-input fps=30 normalizes heterogeneous Pexels framerates (24/25/30)
// before concat.
package assembly

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"viewpro/internal/beats"
)

// Inputs to Build.
type Inputs struct {
	Segments    []beats.Segment // ordered b-roll segments
	VoicePath   string          // absolute path to voice.mp3
	MusicPath   string          // absolute path to background music
	ASSPath     string          // absolute path to captions.ass
	FontsDir    string          // absolute path to assets/fonts (for libass)
	Duration    float64         // total voice duration (truncates output)
	OutputPath  string          // absolute path to short_<ts>.mp4
	SessionDir  string          // where to put segments.json
	MusicVolume float64         // 0.0–1.0; default 0.15
}

// Build renders the final MP4.
func Build(ctx context.Context, in Inputs) error {
	if in.MusicVolume == 0 {
		in.MusicVolume = 0.15
	}
	if len(in.Segments) == 0 {
		return fmt.Errorf("no segments to render")
	}

	// Persist segments as JSON for debugging / rebuild
	_ = writeSegmentsJSON(filepath.Join(in.SessionDir, "segments.json"), in.Segments)

	// Deduplicate clips → input index
	inputIdx := map[string]int{}
	uniqueClips := []string{}
	for _, s := range in.Segments {
		if _, ok := inputIdx[s.ClipPath]; !ok {
			inputIdx[s.ClipPath] = len(uniqueClips)
			uniqueClips = append(uniqueClips, s.ClipPath)
		}
	}

	// Probe each unique clip's duration for the defensive clamp
	clipDur := map[string]float64{}
	for _, p := range uniqueClips {
		d, err := probeDuration(ctx, p)
		if err != nil {
			return fmt.Errorf("ffprobe %s: %w", p, err)
		}
		clipDur[p] = d
	}

	// Build -i args: unique clips first, then voice, then music
	args := []string{"-y"}
	for _, p := range uniqueClips {
		args = append(args, "-i", p)
	}
	voiceIdx := len(uniqueClips)
	musicIdx := voiceIdx + 1
	args = append(args, "-i", in.VoicePath, "-i", in.MusicPath)

	// Build filter graph
	var parts []string

	// Per-segment chain:
	//   [k:v]trim=in:in+dur,setpts=PTS-STARTPTS,fps=30,scale...,crop...,setsar=1[v_i]
	// Order is load-bearing:
	//   - trim cuts the source window [in, in+dur] (in>0 skips intro frames)
	//   - setpts=PTS-STARTPTS resets to 0 (required by concat filter)
	//   - fps=30 resamples to uniform rate on the reset clock
	//   - scale/crop normalize geometry
	for i, s := range in.Segments {
		k := inputIdx[s.ClipPath]
		dur := s.OutPoint
		in := s.InPoint
		if maxDur := clipDur[s.ClipPath]; maxDur > 0 && in+dur > maxDur {
			// The source window [in, in+dur] overruns the clip. Keep the
			// on-screen duration (it's beat-synced) — pull InPoint back
			// before shortening the segment.
			in = maxDur - dur
			if in < 0 {
				fmt.Fprintf(os.Stderr, "[assembly] WARN segment %d: duration %.3f > clip duration %.3f, clamping\n", i, dur, maxDur)
				in, dur = 0, maxDur
			} else if s.InPoint > 0 {
				fmt.Fprintf(os.Stderr, "[assembly] WARN segment %d: intro-skip %.3f → %.3f to fit clip duration %.3f\n", i, s.InPoint, in, maxDur)
			}
		}
		parts = append(parts, fmt.Sprintf(
			"[%d:v]trim=%.3f:%.3f,setpts=PTS-STARTPTS,fps=30,scale=1080:1920:force_original_aspect_ratio=increase,crop=1080:1920,setsar=1[v%d]",
			k, in, in+dur, i,
		))
	}

	// Concat all per-segment outputs
	var refs strings.Builder
	for i := range in.Segments {
		fmt.Fprintf(&refs, "[v%d]", i)
	}
	parts = append(parts, fmt.Sprintf(
		"%sconcat=n=%d:v=1:a=0[concat]",
		refs.String(), len(in.Segments),
	))

	// Burn in subtitles on the concatenated video
	parts = append(parts, fmt.Sprintf(
		"[concat]subtitles=%s:fontsdir=%s[v]",
		ffmpegEscape(in.ASSPath), ffmpegEscape(in.FontsDir),
	))

	// Audio mix
	parts = append(parts, fmt.Sprintf("[%d:a]volume=1.0[voice]", voiceIdx))
	parts = append(parts, fmt.Sprintf("[%d:a]volume=%.2f[bg]", musicIdx, in.MusicVolume))
	parts = append(parts, "[voice][bg]amix=inputs=2:duration=first:dropout_transition=0[a]")

	filter := strings.Join(parts, ";")

	args = append(args,
		"-filter_complex", filter,
		"-map", "[v]", "-map", "[a]",
		"-r", "30",
		"-c:v", "libx264", "-preset", "veryfast", "-crf", "20", "-pix_fmt", "yuv420p",
		"-c:a", "aac", "-b:a", "192k",
		"-t", fmt.Sprintf("%.3f", in.Duration),
		"-movflags", "+faststart",
		in.OutputPath,
	)
	return runFFmpeg(ctx, args)
}

func runFFmpeg(ctx context.Context, args []string) error {
	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		s := stderr.String()
		if len(s) > 1500 {
			s = "…" + s[len(s)-1500:]
		}
		return fmt.Errorf("ffmpeg: %w\nstderr:\n%s", err, s)
	}
	return nil
}

func probeDuration(ctx context.Context, path string) (float64, error) {
	cmd := exec.CommandContext(ctx, "ffprobe",
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		path)
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return 0, err
	}
	return strconv.ParseFloat(strings.TrimSpace(out.String()), 64)
}

func writeSegmentsJSON(path string, segs []beats.Segment) error {
	data, err := json.MarshalIndent(segs, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// ParseLegacyConcatList parses an old broll_list.txt (concat-demuxer
// format) back into Segments. Used by `shorts rebuild` for sessions
// created before the assembly fix.
func ParseLegacyConcatList(path string) ([]beats.Segment, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var segs []beats.Segment
	var cur beats.Segment
	var cumTime float64
	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		switch {
		case strings.HasPrefix(line, "file '"):
			cur.ClipPath = strings.TrimSuffix(strings.TrimPrefix(line, "file '"), "'")
		case strings.HasPrefix(line, "outpoint "):
			d, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimPrefix(line, "outpoint ")), 64)
			if err != nil {
				return nil, fmt.Errorf("parse outpoint %q: %w", line, err)
			}
			cur.OutPoint = d
			cur.Start = cumTime
			cur.End = cumTime + d
			cumTime += d
			segs = append(segs, cur)
			cur = beats.Segment{}
		}
	}
	return segs, nil
}

// LoadSegments tries segments.json first, then falls back to legacy
// broll_list.txt. Returns the first one that parses cleanly.
func LoadSegments(sessionDir string) ([]beats.Segment, error) {
	if data, err := os.ReadFile(filepath.Join(sessionDir, "segments.json")); err == nil {
		var segs []beats.Segment
		if err := json.Unmarshal(data, &segs); err == nil {
			return segs, nil
		}
	}
	return ParseLegacyConcatList(filepath.Join(sessionDir, "broll_list.txt"))
}

// ffmpegEscape escapes characters with special meaning inside a -filter_complex value.
func ffmpegEscape(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '\\', ':', '\'', ',', '[', ']', ';':
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}
