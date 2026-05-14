// Package subtitles emits an ASS subtitle file. Supports three caption
// presets to give per-Short visual variation (mitigates YouTube's
// "Inauthentic Content" templated-output detection):
//
//	Preset A: word-by-word karaoke, yellow active word, bottom (current)
//	Preset B: phrase blocks (~3 words), no per-word highlight, bottom
//	Preset C: word-by-word karaoke, green active word, top third
//
// Time format: H:MM:SS.cs (centiseconds, NOT milliseconds).
//
// ASS color format: &HAABBGGRR& (alpha-blue-green-red), opaque alpha is 00.
//
//	white  = &H00FFFFFF&
//	yellow = &H0000FFFF&
//	green  = &H0000FF00&
package subtitles

import (
	"fmt"
	"math"
	"math/rand/v2"
	"os"
	"strings"

	"viewpro/internal/voice"
)

// Preset identifies a caption style. Selection happens at the pipeline
// level; subtitles.Write just renders what the preset says.
type Preset string

const (
	PresetA Preset = "a" // karaoke-bottom-yellow (default look)
	PresetB Preset = "b" // phrase-blocks-bottom
	PresetC Preset = "c" // karaoke-top-green
)

// RenderMode is how Dialogue lines are emitted.
type RenderMode int

const (
	Karaoke      RenderMode = iota // per-word, active word colored
	PhraseBlocks                   // per-phrase, static, no color
)

// Options control the look of the captions.
type Options struct {
	FontName    string
	FontSize    int
	WindowSize  int        // words on each side of active word; ignored for PhraseBlocks
	MarginL     int        // px from left edge
	MarginR     int        // px from right edge
	MarginV     int        // px from anchored edge (bottom if Alignment∈{1,2,3}, top if {7,8,9})
	Alignment   int        // ASS numpad: 2=bottom-center, 8=top-center
	ActiveColor string     // ASS color for active word, "" = no highlight
	RenderMode  RenderMode // dispatch dispatch
	BlockSize   int        // words per phrase in PhraseBlocks mode
}

// OptionsForPreset returns the Options for a given preset.
func OptionsForPreset(p Preset) Options {
	base := Options{
		FontName:   "Montserrat ExtraBold",
		WindowSize: 1,
		MarginL:    160,
		MarginR:    160,
		BlockSize:  3,
	}
	switch p {
	case PresetB:
		base.FontSize = 64
		base.MarginV = 380
		base.Alignment = 2
		base.RenderMode = PhraseBlocks
		base.ActiveColor = "" // no highlight
	case PresetC:
		base.FontSize = 76
		base.MarginV = 320 // from top edge with Alignment 8
		base.Alignment = 8
		base.RenderMode = Karaoke
		base.ActiveColor = "&H0000FF00&" // green
	default: // PresetA
		base.FontSize = 70
		base.MarginV = 420
		base.Alignment = 2
		base.RenderMode = Karaoke
		base.ActiveColor = "&H0000FFFF&" // yellow
	}
	return base
}

// Defaults returns Options for Preset A (back-compat shim for callers
// that don't care about variation).
func Defaults() Options { return OptionsForPreset(PresetA) }

// RandomPreset picks a weighted-random preset: A 50%, B 30%, C 20%.
func RandomPreset() Preset {
	r := rand.IntN(100)
	switch {
	case r < 50:
		return PresetA
	case r < 80:
		return PresetB
	default:
		return PresetC
	}
}

// ParsePreset parses a flag value into a Preset. Empty/unknown → PresetA.
func ParsePreset(s string) (Preset, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "a", "":
		return PresetA, true
	case "b":
		return PresetB, true
	case "c":
		return PresetC, true
	default:
		return PresetA, false
	}
}

const header = `[Script Info]
ScriptType: v4.00+
PlayResX: 1080
PlayResY: 1920
WrapStyle: 2
ScaledBorderAndShadow: yes

[V4+ Styles]
Format: Name, Fontname, Fontsize, PrimaryColour, SecondaryColour, OutlineColour, BackColour, Bold, Italic, Underline, StrikeOut, ScaleX, ScaleY, Spacing, Angle, BorderStyle, Outline, Shadow, Alignment, MarginL, MarginR, MarginV, Encoding
Style: Default,%s,%d,&H00FFFFFF,&H000000FF,&H00000000,&H80000000,0,0,0,0,100,100,0,0,1,4,0,%d,%d,%d,%d,1
Style: HookBox,Montserrat ExtraBold,90,&HFF000000,&H000000FF,&H40000000,&H00000000,1,0,0,0,100,100,0,0,3,20,0,8,80,80,400,1
Style: Hook,Montserrat ExtraBold,90,&H00FFFFFF,&H000000FF,&H0000FFFF,&HCC000000,1,0,0,0,100,100,0,0,1,4,3,8,80,80,400,1

[Events]
Format: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text
`

// Hook overlay duration in seconds. Centered text appears at t=0,
// fades out over the last hookFadeOutMs as standard captions take over.
const (
	hookOverlayEnd = 1.50
	hookFadeOutMs  = 200
)

// breathHold is extra time the screen holds a sentence-ending word past
// its natural end, capped at the next word's start.
const breathHold = 0.15

// Write emits the ASS file. The active preset is encoded in opts
// (FontSize, MarginV, Alignment, ActiveColor, RenderMode); call
// OptionsForPreset(preset) to get the right Options.
func Write(path string, words []voice.Word, hookOverlayText string, opts Options) error {
	if len(words) == 0 {
		return fmt.Errorf("no words to render")
	}

	var b strings.Builder
	fmt.Fprintf(&b, header,
		opts.FontName, opts.FontSize,
		opts.Alignment, opts.MarginL, opts.MarginR, opts.MarginV)

	writeHookOverlay(&b, hookOverlayText)

	// When the hook overlay is present, suppress per-word/per-phrase
	// captions during its on-screen window so the two don't visually
	// collide. The first body caption starts at or after hookOverlayEnd.
	bodyWords := words
	if strings.TrimSpace(hookOverlayText) != "" {
		bodyWords = wordsAfter(words, hookOverlayEnd)
	}

	switch opts.RenderMode {
	case PhraseBlocks:
		writePhraseBlocks(&b, bodyWords, opts.BlockSize)
	default:
		writeKaraoke(&b, bodyWords, opts.WindowSize, opts.ActiveColor)
	}

	return os.WriteFile(path, []byte(b.String()), 0o644)
}

func writeHookOverlay(b *strings.Builder, text string) {
	if text == "" {
		return
	}
	clean := escapeASS(strings.ToUpper(strings.TrimSpace(text)))
	startT := formatASSTime(0)
	endT := formatASSTime(hookOverlayEnd)
	fade := fmt.Sprintf("{\\q0\\fad(0,%d)}", hookFadeOutMs)
	// Layer 0: backdrop box (renders below text)
	fmt.Fprintf(b, "Dialogue: 0,%s,%s,HookBox,,0,0,0,,%s%s\n", startT, endT, fade, clean)
	// Layer 1: visible text on top of the box
	fmt.Fprintf(b, "Dialogue: 1,%s,%s,Hook,,0,0,0,,%s%s\n", startT, endT, fade, clean)
}

// wordsAfter returns the suffix of words whose Start ≥ cutoff. Used to
// suppress body captions during the hook-overlay window.
func wordsAfter(words []voice.Word, cutoff float64) []voice.Word {
	for i, w := range words {
		if w.Start >= cutoff {
			return words[i:]
		}
	}
	return nil
}

func writeKaraoke(b *strings.Builder, words []voice.Word, windowSize int, activeColor string) {
	for i, w := range words {
		text := renderKaraokeText(words, i, windowSize, activeColor)
		fmt.Fprintf(b, "Dialogue: 0,%s,%s,Default,,0,0,0,,%s\n",
			formatASSTime(w.Start), formatASSTime(wordEnd(words, i)), text)
	}
}

func writePhraseBlocks(b *strings.Builder, words []voice.Word, blockSize int) {
	if blockSize < 1 {
		blockSize = 3
	}
	chunks := chunkWords(words, blockSize)
	for i, chunk := range chunks {
		start := chunk[0].Start
		var end float64
		if i+1 < len(chunks) {
			end = chunks[i+1][0].Start
		} else {
			end = chunk[len(chunk)-1].End
			if isSentenceEnd(chunk[len(chunk)-1].Text) {
				end += breathHold
			}
		}
		fmt.Fprintf(b, "Dialogue: 0,%s,%s,Default,,0,0,0,,%s\n",
			formatASSTime(start), formatASSTime(end), renderBlockText(chunk))
	}
}

// chunkWords groups words into chunks of at most maxPerChunk. Breaks
// early at sentence-ending punctuation so a period never appears mid-chunk.
func chunkWords(words []voice.Word, maxPerChunk int) [][]voice.Word {
	var chunks [][]voice.Word
	var cur []voice.Word
	flush := func() {
		if len(cur) > 0 {
			chunks = append(chunks, cur)
			cur = nil
		}
	}
	for _, w := range words {
		cur = append(cur, w)
		if isSentenceEnd(w.Text) || len(cur) >= maxPerChunk {
			flush()
		}
	}
	flush()
	return chunks
}

func renderBlockText(chunk []voice.Word) string {
	parts := make([]string, len(chunk))
	for i, w := range chunk {
		parts[i] = escapeASS(w.Text)
	}
	return strings.Join(parts, " ")
}

// wordEnd computes the on-screen end timestamp for word i in karaoke mode.
func wordEnd(words []voice.Word, i int) float64 {
	w := words[i]
	hasNext := i+1 < len(words)
	if isSentenceEnd(w.Text) {
		end := w.End + breathHold
		if hasNext && end > words[i+1].Start {
			end = words[i+1].Start
		}
		return end
	}
	if hasNext {
		return words[i+1].Start
	}
	return w.End
}

func isSentenceEnd(text string) bool {
	if text == "" {
		return false
	}
	last := text[len(text)-1]
	return last == '.' || last == '?' || last == '!'
}

func renderKaraokeText(words []voice.Word, i, windowSize int, activeColor string) string {
	lo := max(0, i-windowSize)
	hi := min(len(words), i+windowSize+1)
	parts := make([]string, 0, hi-lo)
	for j := lo; j < hi; j++ {
		t := escapeASS(words[j].Text)
		if j == i && activeColor != "" {
			parts = append(parts, fmt.Sprintf("{\\c%s}%s{\\c&H00FFFFFF&}", activeColor, t))
		} else {
			parts = append(parts, t)
		}
	}
	return strings.Join(parts, " ")
}

// formatASSTime → "H:MM:SS.cs" (single-digit hour, zero-padded mm/ss/cs).
func formatASSTime(t float64) string {
	if t < 0 {
		t = 0
	}
	cs := int64(math.Round(t * 100))
	h := cs / 360000
	cs -= h * 360000
	m := cs / 6000
	cs -= m * 6000
	s := cs / 100
	cs -= s * 100
	return fmt.Sprintf("%d:%02d:%02d.%02d", h, m, s, cs)
}

var assEscaper = strings.NewReplacer(
	"\\", "\\\\",
	"{", "\\{",
	"}", "\\}",
)

func escapeASS(s string) string { return assEscaper.Replace(s) }
