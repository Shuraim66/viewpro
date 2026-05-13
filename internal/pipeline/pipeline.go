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
	BackgroundMusic   string // absolute path
	OutputDir         string // default "output"
	CacheDir          string // default "cache"
	AssetsDir         string // default "assets"
}

// Costs is a best-effort cost accounting for a single run. ElevenLabs
// rate is approximate (their pricing is credit-based).
type Costs struct {
	ClaudeInputTokens  int64
	ClaudeOutputTokens int64
	ClaudeUSD          float64

	ElevenLabsChars int
	ElevenLabsUSD   float64

	TotalUSD float64
}

// Pricing constants (USD)
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
}

// Run executes the pipeline. If seedScript is non-empty, step 1 is
// skipped and the script is read from that path instead — useful for
// iterating on later steps without re-spending Anthropic credits.
func Run(ctx context.Context, cfg Config, idea, seedScript string) (*Result, error) {
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

	costs := Costs{}

	// 1. Script
	logStep("script", "asking Haiku 4.5…")
	sr, err := loadOrGenerateScript(ctx, cfg, idea, seedScript)
	if err != nil {
		return nil, fmt.Errorf("script: %w", err)
	}
	sc := sr.Script
	costs.ClaudeInputTokens = sr.InputTokens
	costs.ClaudeOutputTokens = sr.OutputTokens
	writeJSON(filepath.Join(sessionDir, "script.json"), sc)
	logStep("script", fmt.Sprintf("phenomenon=%q keywords=%v", sc.PhenomenonName, sc.Keywords))

	// 2. Voice + alignment (includes silence trim)
	logStep("voice", "ElevenLabs TTS…")
	vr, err := voice.New(cfg.ElevenLabsKey, cfg.ElevenLabsVoiceID).
		Synthesize(ctx, sc.FullText(), sessionDir)
	if err != nil {
		return nil, fmt.Errorf("voice: %w", err)
	}
	costs.ElevenLabsChars = vr.Chars
	logStep("voice", fmt.Sprintf("trimmed=%.2fs words=%d chars=%d", vr.Duration, len(vr.Words), vr.Chars))

	// 3. Pexels b-roll
	logStep("visuals", fmt.Sprintf("Pexels search/download for %d keywords…", len(sc.Keywords)))
	clips, err := visuals.New(cfg.PexelsKey, absoluteOrDefault(cfg.CacheDir, "cache")).
		FetchForKeywords(ctx, sc.Keywords)
	if err != nil {
		return nil, fmt.Errorf("visuals: %w", err)
	}
	clipPaths := make([]string, len(clips))
	for i, c := range clips {
		clipPaths[i] = c.Path
	}
	logStep("visuals", fmt.Sprintf("got %d clips", len(clips)))

	// 4. Beats
	logStep("beats", "librosa onset detection…")
	br, err := beats.Detect(ctx, vr.MP3Path, "", "")
	if err != nil {
		return nil, fmt.Errorf("beats: %w", err)
	}
	logStep("beats", fmt.Sprintf("found %d onsets in %.2fs audio", len(br.Onsets), br.Duration))

	// 5. Cut plan
	cuts := beats.PlanCuts(br.Onsets, vr.Duration, 1.8, 3.5)
	segs := beats.AssignClips(cuts, clipPaths)
	if len(segs) == 0 {
		return nil, fmt.Errorf("cutplan produced 0 segments (cuts=%v clips=%d)", cuts, len(clips))
	}
	logStep("cutplan", fmt.Sprintf("%d segments", len(segs)))

	// 6. Subtitles
	logStep("subtitles", "rendering ASS…")
	assPath := filepath.Join(sessionDir, "captions.ass")
	if err := subtitles.Write(assPath, vr.Words, subtitles.Defaults()); err != nil {
		return nil, fmt.Errorf("subtitles: %w", err)
	}

	// 7. Music auto-leveling (probe voice loudness)
	voiceMean, err := assembly.ProbeMeanVolume(ctx, vr.MP3Path)
	if err != nil {
		logStep("assembly", fmt.Sprintf("WARN volumedetect failed (%v), using default music volume", err))
		voiceMean = -18 // neutral default
	}
	musicVol := assembly.MusicVolumeForVoice(voiceMean)
	logStep("assembly", fmt.Sprintf("voice mean=%.1fdB → music vol=%.3f", voiceMean, musicVol))

	// 8. Assembly
	fontsDir, _ := filepath.Abs(filepath.Join(absoluteOrDefault(cfg.AssetsDir, "assets"), "fonts"))
	slug := Slugify(sc.PhenomenonName)
	outputPath := filepath.Join(sessionDir, fmt.Sprintf("short_%s_%s.mp4", ts, slug))
	bgPath, _ := filepath.Abs(cfg.BackgroundMusic)
	if err := assembly.Build(ctx, assembly.Inputs{
		Segments:    segs,
		VoicePath:   vr.MP3Path,
		MusicPath:   bgPath,
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

	return &Result{
		SessionDir: sessionDir,
		OutputMP4:  outputPath,
		Duration:   vr.Duration,
		Costs:      costs,
	}, nil
}

func loadOrGenerateScript(ctx context.Context, cfg Config, idea, seedPath string) (*script.Result, error) {
	if seedPath != "" {
		data, err := os.ReadFile(seedPath)
		if err != nil {
			return nil, err
		}
		var s script.Script
		if err := json.Unmarshal(data, &s); err != nil {
			return nil, fmt.Errorf("parse seed script: %w", err)
		}
		// Seed path = no Anthropic call → 0 tokens
		return &script.Result{Script: &s}, nil
	}
	return script.New(cfg.AnthropicKey).Generate(ctx, idea)
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
