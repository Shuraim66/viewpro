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
	"strconv"
	"strings"
	"syscall"
	"time"

	"viewpro/internal/assembly"
	"viewpro/internal/pipeline"
	"viewpro/internal/script"
	"viewpro/internal/subtitles"
	"viewpro/internal/visuals"
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
	case "reject":
		os.Exit(cmdReject(os.Args[2:]))
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
  shorts generate "<idea>" [flags]
  shorts rebuild  <session-dir>       # re-render the MP4 from existing artifacts
  shorts reject   <video-id|session-dir> [reason]   # block clip(s) from future runs

Flags:
  --seed-script PATH       reuse a prior script.json instead of calling Anthropic
  --dry-run                generate script via Claude only, print JSON, skip TTS/video (~$0.002)
  --script-style STYLE     default|question|list|story  (omit for weighted random)
  --caption-preset PRESET  a|b|c                        (omit for weighted random)
  --target-dur SECONDS     spoken target duration       (omit for weighted random ~28-32s)
  --strict-duration        abort if voiceover exceeds target by >25%% (default: warn only)

Examples:
  shorts generate "people who apologize too much"
  shorts generate "the spotlight effect" --dry-run
  shorts generate "the spotlight effect" --script-style question --caption-preset c
  shorts generate "the spotlight effect" --seed-script output/20260514-143022/script.json
  shorts rebuild output/20260514-010619
  shorts reject pexels_6586070 "personal data visible in screen recording"
  shorts reject output/20260514-010619 "flagged by content review"
`)
}

func cmdGenerate(args []string) int {
	parsed, err := parseGenerateArgs(args)
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
	if parsed.dryRun {
		return cmdDryRun(ctx, parsed)
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
	result, err := pipeline.Run(ctx, cfg, parsed.idea, pipeline.RunOpts{
		SeedScript:     parsed.seedScript,
		ScriptStyle:    parsed.scriptStyle,
		CaptionPreset:  parsed.captionPreset,
		TargetDuration: parsed.targetDur,
		StrictDuration: parsed.strictDuration,
	})
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
	printAuditSummary(result.Audit)
	printCostSummary(result.Costs)
	return 0
}

// generateArgs is the parsed result of `shorts generate` flags. Empty
// scriptStyle / captionPreset / targetDur fields mean "let pipeline pick
// a weighted-random default" — the normal case.
type generateArgs struct {
	idea           string
	seedScript     string
	dryRun         bool
	scriptStyle    script.Style
	captionPreset  subtitles.Preset
	targetDur      float64
	strictDuration bool
}

// parseGenerateArgs handles flags appearing anywhere in args, unlike
// stdlib flag.FlagSet which stops at the first non-flag and silently
// drops trailing flags. Pre-fix, `shorts generate "<idea>" --dry-run`
// missed the --dry-run and ran the full pipeline.
func parseGenerateArgs(args []string) (generateArgs, error) {
	var out generateArgs
	var positional []string

	// takeValue returns the next arg for a flag like `--name VALUE` or
	// extracts it from `--name=VALUE`. Updates i in caller via return.
	takeValue := func(i int, name string) (string, int, error) {
		a := args[i]
		if strings.Contains(a, "=") {
			return strings.SplitN(a, "=", 2)[1], i + 1, nil
		}
		if i+1 >= len(args) {
			return "", 0, fmt.Errorf("%s requires a value", name)
		}
		return args[i+1], i + 2, nil
	}

	i := 0
	for i < len(args) {
		a := args[i]
		switch {
		case a == "--dry-run":
			out.dryRun = true
			i++
		case a == "--strict-duration":
			out.strictDuration = true
			i++
		case a == "--seed-script" || strings.HasPrefix(a, "--seed-script="):
			v, ni, err := takeValue(i, "--seed-script")
			if err != nil {
				return generateArgs{}, err
			}
			out.seedScript = v
			i = ni
		case a == "--script-style" || strings.HasPrefix(a, "--script-style="):
			v, ni, err := takeValue(i, "--script-style")
			if err != nil {
				return generateArgs{}, err
			}
			style, ok := script.ParseStyle(v)
			if !ok {
				return generateArgs{}, fmt.Errorf("--script-style: unknown %q (want default|question|list|story)", v)
			}
			out.scriptStyle = style
			i = ni
		case a == "--caption-preset" || strings.HasPrefix(a, "--caption-preset="):
			v, ni, err := takeValue(i, "--caption-preset")
			if err != nil {
				return generateArgs{}, err
			}
			preset, ok := subtitles.ParsePreset(v)
			if !ok {
				return generateArgs{}, fmt.Errorf("--caption-preset: unknown %q (want a|b|c)", v)
			}
			out.captionPreset = preset
			i = ni
		case a == "--target-dur" || strings.HasPrefix(a, "--target-dur="):
			v, ni, err := takeValue(i, "--target-dur")
			if err != nil {
				return generateArgs{}, err
			}
			f, err := strconv.ParseFloat(v, 64)
			if err != nil || f <= 0 {
				return generateArgs{}, fmt.Errorf("--target-dur: bad seconds value %q", v)
			}
			out.targetDur = f
			i = ni
		case a == "--":
			positional = append(positional, args[i+1:]...)
			i = len(args)
		case strings.HasPrefix(a, "-"):
			return generateArgs{}, fmt.Errorf("unknown flag: %s", a)
		default:
			positional = append(positional, a)
			i++
		}
	}
	if len(positional) == 0 {
		return generateArgs{}, fmt.Errorf("missing idea argument")
	}
	out.idea = strings.Join(positional, " ")
	return out, nil
}

// cmdDryRun runs only the Claude script generation step. Useful for
// iterating on hook quality without burning ElevenLabs credits.
//
// Picks a random style + target duration unless --script-style /
// --target-dur were supplied, so the printed JSON reflects what the
// full pipeline would have generated.
func cmdDryRun(ctx context.Context, args generateArgs) int {
	apiKey := mustEnv("ANTHROPIC_API_KEY")
	style := args.scriptStyle
	if style == "" {
		style = script.RandomStyle()
	}
	targetDur := args.targetDur
	if targetDur <= 0 {
		targetDur = pipeline.RandomTargetDuration()
	}
	fmt.Fprintf(os.Stderr, "[dry-run] style=%s target=%.1fs\n", style, targetDur)
	sr, err := script.New(apiKey).Generate(ctx, args.idea, script.GenerateOpts{
		Style:             style,
		TargetDurationSec: targetDur,
	})
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

// printAuditSummary prints the per-run variant decisions captured in
// audit.json. Useful at-a-glance proof that the pipeline rotated styles
// across uploads (the whole point of the variation batch).
func printAuditSummary(a pipeline.Audit) {
	fmt.Println()
	fmt.Println("=== Run audit ===")
	fmt.Printf("Style:    %s\n", a.ScriptStyle)
	fmt.Printf("Preset:   %s\n", a.CaptionPreset)
	fmt.Printf("Music:    %s\n", a.MusicFile)
	fmt.Printf("Target:   %.1fs  (actual %.2fs)\n", a.TargetDurSec, a.ActualDurSec)
	fmt.Printf("Overlay:  %q\n", a.HookOverlayText)
	editorial := "yes"
	if !a.EditorialVoiceFound {
		editorial = "NO (prompt instruction missed)"
	}
	fmt.Printf("Editorial voice: %s\n", editorial)
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

	// Read hook overlay text from script.json. Missing field (older
	// session before this feature) is handled by Script.HookOverlay()
	// which derives from the hook field.
	hookOverlay := readHookOverlay(filepath.Join(sessionDir, "script.json"))

	// Regenerate captions.ass with current subtitles.Defaults().
	// The original .ass is overwritten — its content is fully derivable
	// from alignment.json + script.json + current code, so no data loss.
	assPath := filepath.Join(sessionDir, "captions.ass")
	if err := subtitles.Write(assPath, words, hookOverlay, subtitles.Defaults()); err != nil {
		fmt.Fprintf(os.Stderr, "regen captions: %v\n", err)
		return 1
	}
	fmt.Fprintf(os.Stderr, "[rebuild] regenerated captions.ass (%d words, hook=%q)\n", len(words), hookOverlay)

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

// cmdReject adds clips to cache/rejected_clips.json so every future run
// skips them. Accepts either a Pexels video ID ("6586070" or
// "pexels_6586070") or a session directory — the latter rejects every
// clip used in that Short, which is the usual move after a content-
// safety flag on a published video.
func cmdReject(args []string) int {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: shorts reject <video-id|session-dir> [reason...]")
		return 2
	}
	loadDotEnv(".env")
	cacheDir := envDefault("CACHE_DIR", "cache")
	target := args[0]
	reason := strings.Join(args[1:], " ")

	// Session directory → reject every clip used in that Short.
	if info, err := os.Stat(target); err == nil && info.IsDir() {
		segs, err := assembly.LoadSegments(target)
		if err != nil {
			fmt.Fprintf(os.Stderr, "reject: load segments from %s: %v\n", target, err)
			return 1
		}
		ids := map[int]bool{}
		for _, s := range segs {
			if id, ok := videoIDFromPath(s.ClipPath); ok {
				ids[id] = true
			}
		}
		if len(ids) == 0 {
			fmt.Fprintf(os.Stderr, "reject: no Pexels clips found in %s\n", target)
			return 1
		}
		added := 0
		for id := range ids {
			ok, err := visuals.RejectClip(cacheDir, id, reason, target)
			if err != nil {
				fmt.Fprintf(os.Stderr, "reject: %v\n", err)
				return 1
			}
			if ok {
				added++
				fmt.Printf("rejected pexels_%d\n", id)
			}
		}
		fmt.Printf("done: %d new clip(s) rejected from %s (%d used in session)\n", added, target, len(ids))
		return 0
	}

	// Otherwise treat the argument as a single video ID.
	id, err := parseVideoID(target)
	if err != nil {
		fmt.Fprintf(os.Stderr, "reject: %q is neither a video ID nor a directory\n", target)
		return 2
	}
	ok, err := visuals.RejectClip(cacheDir, id, reason, "")
	if err != nil {
		fmt.Fprintf(os.Stderr, "reject: %v\n", err)
		return 1
	}
	if ok {
		fmt.Printf("rejected pexels_%d\n", id)
	} else {
		fmt.Printf("pexels_%d was already in the rejected list\n", id)
	}
	return 0
}

// parseVideoID accepts "6586070", "pexels_6586070", or "pexels_6586070.mp4".
func parseVideoID(s string) (int, error) {
	s = strings.TrimSuffix(s, ".mp4")
	s = strings.TrimPrefix(s, "pexels_")
	return strconv.Atoi(s)
}

// videoIDFromPath extracts the Pexels video ID from a cached clip path
// such as ".../cache/pexels_6586070.mp4".
func videoIDFromPath(p string) (int, bool) {
	id, err := parseVideoID(filepath.Base(p))
	return id, err == nil
}

// readHookOverlay loads script.json and returns the hook overlay text.
// Uses script.Script.HookOverlay() which falls back to deriving from the
// hook field if hook_overlay_text is missing (older sessions).
// Returns "" on any error so rebuild still works without the overlay.
func readHookOverlay(scriptPath string) string {
	data, err := os.ReadFile(scriptPath)
	if err != nil {
		return ""
	}
	var s script.Script
	if err := json.Unmarshal(data, &s); err != nil {
		return ""
	}
	return s.HookOverlay()
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
