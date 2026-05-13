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
	"flag"
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
	fs := flag.NewFlagSet("generate", flag.ExitOnError)
	seedScript := fs.String("seed-script", "", "reuse a prior script.json instead of calling Anthropic")
	dryRun := fs.Bool("dry-run", false, "generate script only, print JSON, skip TTS/video")
	_ = fs.Parse(args)

	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "missing idea argument")
		usage()
		return 2
	}
	idea := strings.Join(fs.Args(), " ")

	// Load .env if present
	loadDotEnv(".env")

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// --dry-run: script only, no preflight (no ffmpeg/venv needed), no API keys except Anthropic
	if *dryRun {
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
	result, err := pipeline.Run(ctx, cfg, idea, *seedScript)
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

	// Voice duration from alignment.json (last word's end)
	duration, err := readDurationFromAlignment(filepath.Join(sessionDir, "alignment.json"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "read alignment.json: %v\n", err)
		return 1
	}

	fontsDir, _ := filepath.Abs(filepath.Join(envDefault("ASSETS_DIR", "assets"), "fonts"))
	bgPath, _ := filepath.Abs(envDefault("BACKGROUND_MUSIC", "assets/music/bg.mp3"))
	outputPath := filepath.Join(sessionDir, fmt.Sprintf("short_%s_rebuild.mp4", filepath.Base(sessionDir)))

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	fmt.Fprintf(os.Stderr, "[rebuild] %d segments, %.2fs voice, output → %s\n", len(segs), duration, outputPath)
	start := time.Now()
	if err := assembly.Build(ctx, assembly.Inputs{
		Segments:   segs,
		VoicePath:  filepath.Join(sessionDir, "voice.mp3"),
		MusicPath:  bgPath,
		ASSPath:    filepath.Join(sessionDir, "captions.ass"),
		FontsDir:   fontsDir,
		Duration:   duration,
		OutputPath: outputPath,
		SessionDir: sessionDir,
	}); err != nil {
		fmt.Fprintf(os.Stderr, "FAILED: %v\n", err)
		return 1
	}
	fmt.Printf("=== DONE ===\nOutput: %s\nWall:   %s\n", outputPath, time.Since(start).Round(time.Second))
	return 0
}

// readDurationFromAlignment reads alignment.json and returns the last
// word's End timestamp.
func readDurationFromAlignment(path string) (float64, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	var parsed struct {
		Words []struct {
			End float64 `json:"end"`
		} `json:"words"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return 0, err
	}
	if len(parsed.Words) == 0 {
		return 0, fmt.Errorf("no words in alignment.json")
	}
	return parsed.Words[len(parsed.Words)-1].End, nil
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
