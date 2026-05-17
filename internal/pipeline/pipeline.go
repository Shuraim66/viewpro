// Package pipeline orchestrates the 7-step process that turns an idea
// into a 1080x1920 MP4. Each step writes its output to a per-run
// session directory: output/<timestamp>/.
package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"viewpro/internal/assembly"
	"viewpro/internal/beats"
	"viewpro/internal/music"
	"viewpro/internal/script"
	"viewpro/internal/subtitles"
	"viewpro/internal/visuals"
	"viewpro/internal/voice"
)

type Config struct {
	AnthropicKey      string
	ElevenLabsKey     string
	ElevenLabsVoiceID string
	PexelsKey         string
	BackgroundMusic   string // file path OR directory (music.Pick handles both)
	OutputDir         string
	CacheDir          string
	AssetsDir         string
}

// RunOpts carries per-run overrides. Empty fields → pipeline picks a
// weighted-random default (the normal case — variation across uploads
// is the whole point). Explicit values are used as-is (CLI flags use this
// path for testing).
type RunOpts struct {
	SeedScript     string           // reuse a prior script.json (skips Anthropic call)
	ScriptStyle    script.Style     // "" → RandomStyle()
	CaptionPreset  subtitles.Preset // "" → RandomPreset()
	TargetDuration float64          // 0 → RandomTargetDuration()

	// StrictDuration: when true, abort the run if the synthesized voice
	// overshoots the target by more than DurationOvershootLimit. Default
	// false (overshoots just log a warning).
	StrictDuration bool
}

// DurationOvershootLimit is the fraction of target duration above which
// the pipeline emits a warning (or errors, if StrictDuration is set).
const DurationOvershootLimit = 0.25

// Costs is a best-effort cost accounting for a single run.
type Costs struct {
	ClaudeInputTokens  int64
	ClaudeOutputTokens int64
	ClaudeUSD          float64

	ElevenLabsChars int
	ElevenLabsUSD   float64

	TotalUSD float64
}

const (
	// Haiku 4.5: $1 / 1M input tokens, $5 / 1M output tokens
	claudeHaikuInputUSDPerToken  = 1.0 / 1_000_000
	claudeHaikuOutputUSDPerToken = 5.0 / 1_000_000
	// ElevenLabs Turbo v2.5: approx $0.06 / 1000 chars at Starter tier amortized
	elevenLabsTurboUSDPerChar = 0.06 / 1000
)

type Result struct {
	SessionDir string
	OutputMP4  string
	Duration   float64
	Costs      Costs
	Audit      Audit
}

// Run executes the pipeline.
func Run(ctx context.Context, cfg Config, idea string, opts RunOpts) (*Result, error) {
	// 0. Session dir
	ts := time.Now().Format("20060102-150405")
	sessionRel := filepath.Join(cfg.OutputDir, ts)
	if err := os.MkdirAll(sessionRel, 0o755); err != nil {
		return nil, fmt.Errorf("mkdir session: %w", err)
	}
	sessionDir, err := filepath.Abs(sessionRel)
	if err != nil {
		return nil, err
	}

	// 1. Pick variant choices (random unless overridden)
	style := opts.ScriptStyle
	if style == "" {
		style = script.RandomStyle()
	}
	preset := opts.CaptionPreset
	if preset == "" {
		preset = subtitles.RandomPreset()
	}
	targetDur := opts.TargetDuration
	if targetDur <= 0 {
		targetDur = RandomTargetDuration()
	}
	logStep("variant", fmt.Sprintf("style=%s preset=%s target=%.1fs", style, preset, targetDur))

	// Pick music
	musicPath, err := music.Pick(cfg.BackgroundMusic)
	if err != nil {
		return nil, fmt.Errorf("music: %w", err)
	}
	logStep("variant", fmt.Sprintf("music=%s", filepath.Base(musicPath)))

	costs := Costs{}

	// 2. Script
	logStep("script", fmt.Sprintf("asking Haiku 4.5 (%s, ~%.0fs)…", style, targetDur))
	sr, err := loadOrGenerateScript(ctx, cfg, idea, opts.SeedScript, script.GenerateOpts{
		Style:             style,
		TargetDurationSec: targetDur,
	})
	if err != nil {
		return nil, fmt.Errorf("script: %w", err)
	}
	sc := sr.Script
	costs.ClaudeInputTokens = sr.InputTokens
	costs.ClaudeOutputTokens = sr.OutputTokens
	writeJSON(filepath.Join(sessionDir, "script.json"), sc)
	editorialVoice := script.HasEditorialVoice(sc.Body)
	if !editorialVoice {
		logStep("script", "WARN editorial-voice pattern not found in body")
	}
	logStep("script", fmt.Sprintf("phenomenon=%q editorial=%v", sc.PhenomenonName, editorialVoice))

	// 3. Voice + alignment (with silence trim)
	logStep("voice", "ElevenLabs TTS…")
	vr, err := voice.New(cfg.ElevenLabsKey, cfg.ElevenLabsVoiceID).
		Synthesize(ctx, sc.FullText(), sessionDir)
	if err != nil {
		return nil, fmt.Errorf("voice: %w", err)
	}
	costs.ElevenLabsChars = vr.Chars
	logStep("voice", fmt.Sprintf("trimmed=%.2fs (target=%.1fs) words=%d", vr.Duration, targetDur, len(vr.Words)))

	overshoot := (vr.Duration - targetDur) / targetDur
	if overshoot > DurationOvershootLimit {
		msg := fmt.Sprintf("duration %.1fs exceeds target %.1fs by %.0f%% (limit %.0f%%)",
			vr.Duration, targetDur, overshoot*100, DurationOvershootLimit*100)
		if opts.StrictDuration {
			return nil, fmt.Errorf("strict-duration: %s", msg)
		}
		logStep("voice", "WARN "+msg)
	}

	// 4. Pexels b-roll
	logStep("visuals", fmt.Sprintf("Pexels search/download for %d keywords…", len(sc.Keywords)))
	clips, err := visuals.New(cfg.PexelsKey, absoluteOrDefault(cfg.CacheDir, "cache")).
		FetchForKeywords(ctx, sc.Keywords)
	if err != nil {
		return nil, fmt.Errorf("visuals: %w", err)
	}
	clipPaths := make([]string, len(clips))
	brollReview := make([]BrollReviewItem, len(clips))
	for i, c := range clips {
		clipPaths[i] = c.Path
		tags := c.Tags
		if tags == nil {
			tags = []string{}
		}
		brollReview[i] = BrollReviewItem{
			KeywordRequested: c.KeywordRequested,
			PexelsURL:        c.URL,
			SelectedPath:     c.Path,
			TagsReturned:     tags,
		}
	}
	logStep("visuals", fmt.Sprintf("got %d clips", len(clips)))

	// 5. Beats
	logStep("beats", "librosa onset detection…")
	br, err := beats.Detect(ctx, vr.MP3Path, "", "")
	if err != nil {
		return nil, fmt.Errorf("beats: %w", err)
	}
	logStep("beats", fmt.Sprintf("found %d onsets in %.2fs audio", len(br.Onsets), br.Duration))

	// 6. Cut plan
	cuts := beats.PlanCuts(br.Onsets, vr.Duration, 1.8, 3.5)
	segs := beats.AssignClips(cuts, clipPaths)
	if len(segs) == 0 {
		return nil, fmt.Errorf("cutplan produced 0 segments (cuts=%v clips=%d)", cuts, len(clips))
	}
	usage := beats.SummarizeClipUsage(segs)
	gap := "n/a"
	if usage.MinGap >= 0 {
		gap = fmt.Sprintf("%d segments", usage.MinGap)
	}
	logStep("cutplan", fmt.Sprintf("%d segments, %d unique clips, max repeats: %d, min gap: %s",
		usage.Segments, usage.UniqueClips, usage.MaxRepeats, gap))

	// 7. Subtitles (with chosen preset)
	logStep("subtitles", fmt.Sprintf("rendering ASS (preset %s)…", preset))
	assPath := filepath.Join(sessionDir, "captions.ass")
	hookOverlay := sc.HookOverlay()
	if err := subtitles.Write(assPath, vr.Words, hookOverlay, subtitles.OptionsForPreset(preset)); err != nil {
		return nil, fmt.Errorf("subtitles: %w", err)
	}

	// 8. Music auto-leveling (probe voice loudness)
	voiceMean, err := assembly.ProbeMeanVolume(ctx, vr.MP3Path)
	if err != nil {
		logStep("assembly", fmt.Sprintf("WARN volumedetect failed (%v), using default music volume", err))
		voiceMean = -18
	}
	musicVol := assembly.MusicVolumeForVoice(voiceMean)
	logStep("assembly", fmt.Sprintf("voice mean=%.1fdB → music vol=%.3f", voiceMean, musicVol))

	// 9. Assembly
	fontsDir, _ := filepath.Abs(filepath.Join(absoluteOrDefault(cfg.AssetsDir, "assets"), "fonts"))
	slug := Slugify(sc.PhenomenonName)
	outputPath := filepath.Join(sessionDir, fmt.Sprintf("short_%s_%s.mp4", ts, slug))
	if err := assembly.Build(ctx, assembly.Inputs{
		Segments:    segs,
		VoicePath:   vr.MP3Path,
		MusicPath:   musicPath,
		ASSPath:     assPath,
		FontsDir:    fontsDir,
		Duration:    vr.Duration,
		OutputPath:  outputPath,
		SessionDir:  sessionDir,
		MusicVolume: musicVol,
	}); err != nil {
		return nil, fmt.Errorf("assembly: %w", err)
	}
	logStep("assembly", fmt.Sprintf("output → %s", outputPath))

	// Compute costs
	costs.ClaudeUSD = float64(costs.ClaudeInputTokens)*claudeHaikuInputUSDPerToken +
		float64(costs.ClaudeOutputTokens)*claudeHaikuOutputUSDPerToken
	costs.ElevenLabsUSD = float64(costs.ElevenLabsChars) * elevenLabsTurboUSDPerChar
	costs.TotalUSD = costs.ClaudeUSD + costs.ElevenLabsUSD

	// 10. Audit log
	audit := Audit{
		Idea:                idea,
		Slug:                slug,
		CaptionPreset:       string(preset),
		MusicFile:           filepath.Base(musicPath),
		ScriptStyle:         string(style),
		TargetDurSec:        targetDur,
		ActualDurSec:        vr.Duration,
		HookOverlayText:     hookOverlay,
		EditorialVoiceFound: editorialVoice,
		BrollReview:         brollReview,
		ClaudeUSD:           costs.ClaudeUSD,
		ElevenLabsUSD:       costs.ElevenLabsUSD,
		TotalUSD:            costs.TotalUSD,
		OutputMP4:           outputPath,
	}
	writeJSON(filepath.Join(sessionDir, "audit.json"), audit)

	return &Result{
		SessionDir: sessionDir,
		OutputMP4:  outputPath,
		Duration:   vr.Duration,
		Costs:      costs,
		Audit:      audit,
	}, nil
}

func loadOrGenerateScript(ctx context.Context, cfg Config, idea, seedPath string, opts script.GenerateOpts) (*script.Result, error) {
	if seedPath != "" {
		data, err := os.ReadFile(seedPath)
		if err != nil {
			return nil, err
		}
		var s script.Script
		if err := json.Unmarshal(data, &s); err != nil {
			return nil, fmt.Errorf("parse seed script: %w", err)
		}
		return &script.Result{Script: &s}, nil
	}
	return script.New(cfg.AnthropicKey).Generate(ctx, idea, opts)
}

func writeJSON(path string, v any) {
	data, _ := json.MarshalIndent(v, "", "  ")
	_ = os.WriteFile(path, data, 0o644)
}

func logStep(step, msg string) {
	fmt.Fprintf(os.Stderr, "[%s] %s\n", step, msg)
}

func absoluteOrDefault(p, def string) string {
	if p == "" {
		p = def
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return p
	}
	return abs
}
