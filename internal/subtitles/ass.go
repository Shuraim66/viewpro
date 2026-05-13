// Package subtitles emits an ASS file with word-by-word karaoke highlighting.
//
// One Dialogue line per word; active word in yellow, surrounding context in
// white. Time format: H:MM:SS.cs (centiseconds, NOT milliseconds).
//
// ASS color format: &HAABBGGRR& (alpha-blue-green-red), opaque alpha is 00.
//   white  = &H00FFFFFF&
//   yellow = &H0000FFFF& (BB=00, GG=FF, RR=FF)
package subtitles

import (
	"fmt"
	"math"
	"os"
	"strings"

	"viewpro/internal/voice"
)

// Options control the look of the captions. Default is reasonable.
type Options struct {
	FontName   string // exact family name as fontconfig sees it
	FontSize   int    // pt
	WindowSize int    // words on each side of the active word
	MarginV    int    // pixels from bottom edge (Alignment 2)
}

func Defaults() Options {
	return Options{
		FontName:   "Montserrat ExtraBold",
		FontSize:   70,
		WindowSize: 2,
		// 480 px ≈ 25% of 1920 vertical, clears YouTube's mobile Shorts
		// UI chrome (title, handle, action buttons) which overlays the
		// bottom quarter of the player. Alignment stays at 2 (bottom-
		// centered) — we're just lifting the bottom anchor.
		MarginV: 480,
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
Style: Default,%s,%d,&H00FFFFFF,&H000000FF,&H00000000,&H80000000,0,0,0,0,100,100,0,0,1,4,0,2,40,40,%d,1

[Events]
Format: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text
`

// breathHold is extra time the screen holds a sentence-ending word past
// its natural end, capped at the next word's start. Gives the viewer
// time to register the period/question mark/exclamation before the next
// word appears.
const breathHold = 0.15

// Write emits the ASS file for the given words to path.
func Write(path string, words []voice.Word, opts Options) error {
	if len(words) == 0 {
		return fmt.Errorf("no words to render")
	}

	var b strings.Builder
	fmt.Fprintf(&b, header, opts.FontName, opts.FontSize, opts.MarginV)

	for i, w := range words {
		start := w.Start
		end := wordEnd(words, i)
		text := renderText(words, i, opts.WindowSize)
		fmt.Fprintf(&b, "Dialogue: 0,%s,%s,Default,,0,0,0,,%s\n",
			formatASSTime(start), formatASSTime(end), text)
	}

	return os.WriteFile(path, []byte(b.String()), 0o644)
}

// wordEnd computes the on-screen end timestamp for word i.
//   - Sentence-ending words (., ?, !): w.End + breathHold, capped at next word's start.
//     If a natural pause already exceeds breathHold, the screen briefly blanks
//     before the next word, giving "breathing space".
//   - Other words: gapless karaoke (end = next word's start).
//   - Last word: w.End (+ breathHold if sentence-ending).
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

func renderText(words []voice.Word, i, windowSize int) string {
	lo := i - windowSize
	if lo < 0 {
		lo = 0
	}
	hi := i + windowSize + 1
	if hi > len(words) {
		hi = len(words)
	}
	parts := make([]string, 0, hi-lo)
	for j := lo; j < hi; j++ {
		t := escapeASS(words[j].Text)
		if j == i {
			parts = append(parts, "{\\c&H0000FFFF&}"+t+"{\\c&H00FFFFFF&}")
		} else {
			parts = append(parts, t)
		}
	}
	return strings.Join(parts, " ")
}

// formatASSTime → "H:MM:SS.cs" (single-digit hour, zero-padded mm/ss/cs).
// Uses math.Round to avoid centisecond off-by-one.
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

// escapeASS handles the three reserved chars inside Dialogue text.
// Commas are NOT reserved — only the first 9 commas in a Dialogue line
// separate fields; the Text field is everything after the 9th.
var assEscaper = strings.NewReplacer(
	"\\", "\\\\",
	"{", "\\{",
	"}", "\\}",
)

func escapeASS(s string) string { return assEscaper.Replace(s) }
