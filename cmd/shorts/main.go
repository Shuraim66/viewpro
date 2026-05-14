// Command shorts generates a 1080x1920 YouTube Short from a one-line idea.
//
// Usage:
//
//	shorts generate "<idea>"                       # full pipeline
//	shorts generate "<idea>" --seed-script PATH    # skip step 1, reuse a prior script.json
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"viewpro/internal/assembly"
	"viewpro/internal/pipeline"
	"viewpro/internal/script"
	"viewpro/internal/subtitles"
	"viewpro/internal/voice"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "generate":
		os.Exit(cmdGenerate(os.Args[2:]))
	case "rebuild":
		os.Exit(cmdRebuild(os.Args[2:]))
	case "-h", "--help", "help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %q\n", os.Args[1])
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, `shorts — AI YouTube Shorts generator

Usage:
  shorts generate "<idea>" [--seed-script PATH] [--dry-run]
  shorts rebuild  <session-dir>       # re-render the MP4 from existing artifacts

Flags:
  --seed-script PATH   reuse a prior script.json instead of calling Anthropic
  --dry-run            generate script via Claude only, print JSON, skip TTS/video (~$0.002)

Examples:
  shorts generate "people who apologize too much"
  shorts generate "the spotlight effect" --dry-run
  shorts generate "the spotlight effect" --seed-script output/20260514-143022/script.json
  shorts rebuild output/20260514-010619
`)
}

func cmdGenerate(args []string) int {
	idea, seedScript, dryRun, err := parseGenerateArgs(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		usage()
		return 2
	}

	// Load .env if present
	loadDotEnv(".env")

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// --dry-run: script only, no preflight (no ffmpeg/venv needed), no API keys except Anthropic
	if dryRun {
		return cmdDryRun(ctx, idea)
	}

	// Full pipeline: sanity checks first
	if err := preflight(); err != nil {
		fmt.Fprintf(os.Stderr, "pre-flight failed: %v\n", err)
		fmt.Fprintln(os.Stderr, "run: bash scripts/preflight.sh")
		return 1
	}

	cfg := pipeline.Config{
		AnthropicKey:      mustEnv("ANTHROPIC_API_KEY"),
		ElevenLabsKey:     mustEnv("ELEVENLABS_API_KEY"),
		ElevenLabsVoiceID: mustEnv("ELEVENLABS_VOICE_ID"),
		PexelsKey:         mustEnv("PEXELS_API_KEY"),
		BackgroundMusic:   envDefault("BACKGROUND_MUSIC", "assets/music/bg.mp3"),
		OutputDir:         envDefault("OUTPUT_DIR", "output"),
		CacheDir:          envDefault("CACHE_DIR", "cache"),
		AssetsDir:         envDefault("ASSETS_DIR", "assets"),
	}

	start := time.Now()
	result, err := pipeline.Run(ctx, cfg, idea, seedScript)
	if err != nil {
		fmt.Fprintf(os.Stderr, "\nFAILED: %v\n", err)
		return 1
	}

	elapsed := time.Since(start)
	fmt.Println()
	fmt.Println("=== DONE ===")
	fmt.Printf("Output:   %s\n", result.OutputMP4)
	fmt.Printf("Duration: %.2fs\n", result.Duration)
	fmt.Printf("Wall:     %s\n", elapsed.Round(time.Second))
	fmt.Printf("Preview:  mpv %s\n", result.OutputMP4)
	printCostSummary(result.Costs)
	return 0
}

// parseGenerateArgs handles flags appearing anywhere in args, unlike
// stdlib flag.FlagSet which stops at the first non-flag and silently
// drops trailing flags. Pre-fix, `shorts generate "<idea>" --dry-run`
// missed the --dry-run and ran the full pipeline.
func parseGenerateArgs(args []string) (idea, seedScript string, dryRun bool, err error) {
	var positional []string
	i := 0
	for i < len(args) {
		a := args[i]
		switch {
		case a == "--dry-run":
			dryRun = true
			i++
		case a == "--seed-script":
			if i+1 >= len(args) {
				return "", "", false, fmt.Errorf("--seed-script requires a path")
			}
			seedScript = args[i+1]
			i += 2
		case strings.HasPrefix(a, "--seed-script="):
			seedScript = strings.TrimPrefix(a, "--seed-script=")
			i++
		case a == "--":
			positional = append(positional, args[i+1:]...)
			i = len(args)
		case strings.HasPrefix(a, "-"):
			return "", "", false, fmt.Errorf("unknown flag: %s", a)
		default:
			positional = append(positional, a)
			i++
		}
	}
	if len(positional) == 0 {
		return "", "", false, fmt.Errorf("missing idea argument")
	}
	return strings.Join(positional, " "), seedScript, dryRun, nil
}

// cmdDryRun runs only the Claude script generation step. Useful for
// iterating on hook quality without burning ElevenLabs credits.
func cmdDryRun(ctx context.Context, idea string) int {
	apiKey := mustEnv("ANTHROPIC_API_KEY")
	sr, err := script.New(apiKey).Generate(ctx, idea)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAILED: %v\n", err)
		return 1
	}

	data, _ := json.MarshalIndent(sr.Script, "", "  ")
	fmt.Println(string(data))

	// Cost summary for the Claude-only call
	costs := pipeline.Costs{
		ClaudeInputTokens:  sr.InputTokens,
		ClaudeOutputTokens: sr.OutputTokens,
	}
	costs.ClaudeUSD = float64(costs.ClaudeInputTokens)*(1.0/1_000_000) +
		float64(costs.ClaudeOutputTokens)*(5.0/1_000_000)
	costs.TotalUSD = costs.ClaudeUSD
	printCostSummary(costs)
	return 0
}

func printCostSummary(c pipeline.Costs) {
	fmt.Println()
	fmt.Println("=== Run cost summary ===")
	fmt.Printf("Claude (Haiku): $%.4f  (%d in, %d out)\n",
		c.ClaudeUSD, c.ClaudeInputTokens, c.ClaudeOutputTokens)
	if c.ElevenLabsChars > 0 {
		fmt.Printf("ElevenLabs (%d chars): $%.4f\n", c.ElevenLabsChars, c.ElevenLabsUSD)
	}
	fmt.Printf("Total: $%.4f\n", c.TotalUSD)
}

// cmdRebuild re-renders just the assembly step (ffmpeg) against an
// existing session dir. Useful for testing assembly changes without
// burning API credits. Output is written next to the original as
// short_<ts>_rebuild.mp4 so the original stays around for comparison.
func cmdRebuild(args []string) int {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "missing session dir")
		usage()
		return 2
	}
	sessionDir, err := filepath.Abs(args[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "bad path: %v\n", err)
		return 1
	}

	loadDotEnv(".env")
	if err := preflight(); err != nil {
		fmt.Fprintf(os.Stderr, "pre-flight failed: %v\n", err)
		return 1
	}

	// Load segments
	segs, err := assembly.LoadSegments(sessionDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load segments: %v\n", err)
		return 1
	}

	// Load words from alignment.json — both for duration AND so we can
	// regenerate captions.ass with the current subtitles code (picks up
	// any margin/style/code changes since the original run).
	words, duration, err := loadAlignment(filepath.Join(sessionDir, "alignment.json"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "read alignment.json: %v\n", err)
		return 1
	}

	// Regenerate captions.ass with current subtitles.Defaults().
	// The original .ass is overwritten — its content is fully derivable
	// from alignment.json + current code, so no data loss.
	assPath := filepath.Join(sessionDir, "captions.ass")
	if err := subtitles.Write(assPath, words, subtitles.Defaults()); err != nil {
		fmt.Fprintf(os.Stderr, "regen captions: %v\n", err)
		return 1
	}
	fmt.Fprintf(os.Stderr, "[rebuild] regenerated captions.ass (%d words)\n", len(words))

	fontsDir, _ := filepath.Abs(filepath.Join(envDefault("ASSETS_DIR", "assets"), "fonts"))
	bgPath, _ := filepath.Abs(envDefault("BACKGROUND_MUSIC", "assets/music/bg.mp3"))
	outputPath := filepath.Join(sessionDir, fmt.Sprintf("short_%s_rebuild.mp4", filepath.Base(sessionDir)))

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// Music auto-leveling against the existing voice.mp3
	voicePath := filepath.Join(sessionDir, "voice.mp3")
	voiceMean, err := assembly.ProbeMeanVolume(ctx, voicePath)
	if err != nil {
		voiceMean = -18
	}
	musicVol := assembly.MusicVolumeForVoice(voiceMean)
	fmt.Fprintf(os.Stderr, "[rebuild] %d segments, %.2fs voice, music vol=%.3f, output → %s\n", len(segs), duration, musicVol, outputPath)

	start := time.Now()
	if err := assembly.Build(ctx, assembly.Inputs{
		Segments:    segs,
		VoicePath:   voicePath,
		MusicPath:   bgPath,
		ASSPath:     assPath,
		FontsDir:    fontsDir,
		Duration:    duration,
		OutputPath:  outputPath,
		SessionDir:  sessionDir,
		MusicVolume: musicVol,
	}); err != nil {
		fmt.Fprintf(os.Stderr, "FAILED: %v\n", err)
		return 1
	}
	fmt.Printf("=== DONE ===\nOutput: %s\nWall:   %s\n", outputPath, time.Since(start).Round(time.Second))
	return 0
}

// loadAlignment reads alignment.json and returns the words plus duration
// (last word's End). Used by rebuild so it can regenerate captions.ass
// against current subtitles code.
func loadAlignment(path string) ([]voice.Word, float64, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, 0, err
	}
	var parsed struct {
		Words []voice.Word `json:"words"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, 0, err
	}
	if len(parsed.Words) == 0 {
		return nil, 0, fmt.Errorf("no words in alignment.json")
	}
	return parsed.Words, parsed.Words[len(parsed.Words)-1].End, nil
}

// preflight verifies that the system can run the pipeline.
func preflight() error {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return fmt.Errorf("ffmpeg not found in PATH")
	}
	if _, err := os.Stat(".venv/bin/python3"); err != nil {
		return fmt.Errorf(".venv/bin/python3 not found")
	}
	if _, err := os.Stat("python/beats.py"); err != nil {
		return fmt.Errorf("python/beats.py not found (are you in the project root?)")
	}
	music := envDefault("BACKGROUND_MUSIC", "assets/music/bg.mp3")
	if _, err := os.Stat(music); err != nil {
		return fmt.Errorf("background music %q not found — drop an mp3 there or set BACKGROUND_MUSIC in .env", music)
	}
	return nil
}

func mustEnv(name string) string {
	v := os.Getenv(name)
	if v == "" {
		fmt.Fprintf(os.Stderr, "ERROR: %s is empty (set it in .env)\n", name)
		os.Exit(1)
	}
	return v
}

func envDefault(name, def string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return def
}

// loadDotEnv reads KEY=VALUE lines from path into os.Setenv if not already set.
// Doesn't pull in godotenv — this is ~20 lines.
func loadDotEnv(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		eq := strings.IndexByte(line, '=')
		if eq < 0 {
			continue
		}
		key := strings.TrimSpace(line[:eq])
		val := strings.TrimSpace(line[eq+1:])
		// Strip optional surrounding quotes
		val = strings.Trim(val, `"'`)
		if os.Getenv(key) == "" {
			_ = os.Setenv(key, val)
		}
	}
}
