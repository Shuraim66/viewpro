package subtitles

import (
	"strings"
	"testing"

	"viewpro/internal/voice"
)

func TestFormatASSTime(t *testing.T) {
	tests := []struct {
		in   float64
		want string
	}{
		{0.0, "0:00:00.00"},
		{0.005, "0:00:00.01"}, // rounds up
		{0.004, "0:00:00.00"}, // rounds down
		{1.23, "0:00:01.23"},
		{59.99, "0:00:59.99"},
		{60.0, "0:01:00.00"},
		{3600.0, "1:00:00.00"},
		{3661.42, "1:01:01.42"},
		{-1.0, "0:00:00.00"}, // negative clamped to 0
	}
	for _, tt := range tests {
		got := formatASSTime(tt.in)
		if got != tt.want {
			t.Errorf("formatASSTime(%f) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestEscapeASS(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"hello", "hello"},
		{"don't", "don't"},
		{"line1\nline2", "line1\nline2"}, // newline left alone
		{"a{b}c", "a\\{b\\}c"},
		{`back\slash`, `back\\slash`},
	}
	for _, tt := range tests {
		got := escapeASS(tt.in)
		if got != tt.want {
			t.Errorf("escapeASS(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestRenderKaraokeText_ActiveWordYellow(t *testing.T) {
	words := []voice.Word{
		{Text: "one"}, {Text: "two"}, {Text: "three"}, {Text: "four"}, {Text: "five"},
	}
	got := renderKaraokeText(words, 2, 2, "&H0000FFFF&") // active = "three", window = 2
	want := "one two {\\c&H0000FFFF&}three{\\c&H00FFFFFF&} four five"
	if got != want {
		t.Errorf("got %q\nwant %q", got, want)
	}
}

func TestRenderKaraokeText_GreenColor(t *testing.T) {
	words := []voice.Word{{Text: "alpha"}, {Text: "bravo"}}
	got := renderKaraokeText(words, 0, 1, "&H0000FF00&")
	if !strings.Contains(got, "{\\c&H0000FF00&}alpha{\\c&H00FFFFFF&}") {
		t.Errorf("expected green color tag, got %q", got)
	}
}

func TestRenderKaraokeText_NoColor(t *testing.T) {
	words := []voice.Word{{Text: "one"}, {Text: "two"}}
	got := renderKaraokeText(words, 0, 1, "")
	if strings.Contains(got, "\\c") {
		t.Errorf("expected no color tag with activeColor=\"\", got %q", got)
	}
	if got != "one two" {
		t.Errorf("expected 'one two', got %q", got)
	}
}

func TestRenderKaraokeText_ClampedAtEdges(t *testing.T) {
	words := []voice.Word{{Text: "first"}, {Text: "second"}}
	got := renderKaraokeText(words, 0, 2, "&H0000FFFF&")
	if !strings.HasPrefix(got, "{\\c&H0000FFFF&}first") {
		t.Errorf("expected active word at start, got %q", got)
	}
	if !strings.Contains(got, "second") {
		t.Errorf("expected 'second' in window, got %q", got)
	}
}

func TestRenderKaraokeText_Window1(t *testing.T) {
	words := []voice.Word{
		{Text: "one"}, {Text: "two"}, {Text: "three"}, {Text: "four"}, {Text: "five"},
	}
	t.Run("middle word", func(t *testing.T) {
		got := renderKaraokeText(words, 2, 1, "&H0000FFFF&")
		want := "two {\\c&H0000FFFF&}three{\\c&H00FFFFFF&} four"
		if got != want {
			t.Errorf("got %q\nwant %q", got, want)
		}
	})
	t.Run("first word: clamped to 2 words", func(t *testing.T) {
		got := renderKaraokeText(words, 0, 1, "&H0000FFFF&")
		want := "{\\c&H0000FFFF&}one{\\c&H00FFFFFF&} two"
		if got != want {
			t.Errorf("got %q\nwant %q", got, want)
		}
	})
	t.Run("last word: clamped to 2 words", func(t *testing.T) {
		got := renderKaraokeText(words, 4, 1, "&H0000FFFF&")
		want := "four {\\c&H0000FFFF&}five{\\c&H00FFFFFF&}"
		if got != want {
			t.Errorf("got %q\nwant %q", got, want)
		}
	})
}

func TestChunkWords(t *testing.T) {
	t.Run("3-word chunks, no sentence breaks", func(t *testing.T) {
		words := []voice.Word{
			{Text: "a"}, {Text: "b"}, {Text: "c"}, {Text: "d"}, {Text: "e"}, {Text: "f"}, {Text: "g"},
		}
		chunks := chunkWords(words, 3)
		if len(chunks) != 3 {
			t.Fatalf("got %d chunks, want 3 (3+3+1): %v", len(chunks), chunks)
		}
		if len(chunks[0]) != 3 || len(chunks[1]) != 3 || len(chunks[2]) != 1 {
			t.Errorf("chunk sizes wrong: %v", chunks)
		}
	})
	t.Run("breaks early at sentence end", func(t *testing.T) {
		words := []voice.Word{
			{Text: "Stop"}, {Text: "doing"}, {Text: "this."}, {Text: "You"}, {Text: "should"},
		}
		chunks := chunkWords(words, 3)
		// Chunk 1 ends at "this." (sentence end). Chunk 2 is "You should".
		if len(chunks) != 2 {
			t.Fatalf("got %d chunks, want 2: %v", len(chunks), chunks)
		}
		if chunks[0][len(chunks[0])-1].Text != "this." {
			t.Errorf("chunk 1 should end at 'this.', got %v", chunks[0])
		}
	})
	t.Run("mid-chunk sentence end also breaks", func(t *testing.T) {
		words := []voice.Word{
			{Text: "Hi."}, {Text: "Two"}, {Text: "three"}, {Text: "four"},
		}
		chunks := chunkWords(words, 3)
		// Chunk 1 = ["Hi."], Chunk 2 = ["Two", "three", "four"]
		if len(chunks) != 2 || len(chunks[0]) != 1 {
			t.Errorf("expected ['Hi.'] and 3-word chunk, got %v", chunks)
		}
	})
}

func TestRandomPreset_Distribution(t *testing.T) {
	// Smoke test: across 1000 picks, every preset appears at least once
	// and PresetA dominates (heuristic check, not exact distribution).
	counts := map[Preset]int{}
	for range 1000 {
		counts[RandomPreset()]++
	}
	for _, p := range []Preset{PresetA, PresetB, PresetC} {
		if counts[p] == 0 {
			t.Errorf("Preset %s never picked in 1000 trials", p)
		}
	}
	// PresetA expected ~500, B ~300, C ~200. Verify A > B > C roughly.
	if counts[PresetA] < counts[PresetB] {
		t.Errorf("PresetA (%d) should beat PresetB (%d) on weighted random", counts[PresetA], counts[PresetB])
	}
}

func TestParsePreset(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want Preset
		ok   bool
	}{
		{"a", PresetA, true},
		{"A", PresetA, true},
		{"b", PresetB, true},
		{"C", PresetC, true},
		{"", PresetA, true},
		{"unknown", PresetA, false},
	} {
		got, ok := ParsePreset(tc.in)
		if got != tc.want || ok != tc.ok {
			t.Errorf("ParsePreset(%q) = (%v, %v), want (%v, %v)", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}

func TestIsSentenceEnd(t *testing.T) {
	for _, tt := range []struct {
		in   string
		want bool
	}{
		{"phone.", true},
		{"really?", true},
		{"Stop!", true},
		{"hello", false},
		{"don't", false},
		{"", false},
	} {
		if got := isSentenceEnd(tt.in); got != tt.want {
			t.Errorf("isSentenceEnd(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestWordEnd(t *testing.T) {
	t.Run("non-sentence-end uses gapless karaoke", func(t *testing.T) {
		words := []voice.Word{
			{Text: "hello", Start: 0.0, End: 0.4},
			{Text: "world", Start: 0.5, End: 0.9},
		}
		got := wordEnd(words, 0)
		if got != 0.5 {
			t.Errorf("got %f, want 0.5 (next word's start)", got)
		}
	})

	t.Run("sentence-end holds 150ms", func(t *testing.T) {
		words := []voice.Word{
			{Text: "stop.", Start: 0.0, End: 0.5},
			{Text: "Next", Start: 1.0, End: 1.4}, // 500ms natural pause
		}
		got := wordEnd(words, 0)
		// 0.5 + 0.15 = 0.65, less than next.Start=1.0, so we get 0.65
		if got != 0.65 {
			t.Errorf("got %f, want 0.65 (hold +150ms)", got)
		}
	})

	t.Run("sentence-end cap at next word's start", func(t *testing.T) {
		words := []voice.Word{
			{Text: "stop.", Start: 0.0, End: 0.5},
			{Text: "Next", Start: 0.55, End: 0.9}, // 50ms pause < 150ms hold
		}
		got := wordEnd(words, 0)
		if got != 0.55 {
			t.Errorf("got %f, want 0.55 (capped at next start)", got)
		}
	})

	t.Run("last word sentence-end extends without cap", func(t *testing.T) {
		words := []voice.Word{
			{Text: "Done.", Start: 0.0, End: 1.0},
		}
		got := wordEnd(words, 0)
		if got != 1.15 {
			t.Errorf("got %f, want 1.15", got)
		}
	})

	t.Run("last word non-sentence stays at natural end", func(t *testing.T) {
		words := []voice.Word{
			{Text: "trailing", Start: 0.0, End: 1.0},
		}
		got := wordEnd(words, 0)
		if got != 1.0 {
			t.Errorf("got %f, want 1.0", got)
		}
	})
}
