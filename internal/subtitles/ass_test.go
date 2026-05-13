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

func TestRenderText_ActiveWordYellow(t *testing.T) {
	words := []voice.Word{
		{Text: "one"}, {Text: "two"}, {Text: "three"}, {Text: "four"}, {Text: "five"},
	}
	got := renderText(words, 2, 2) // active = "three", window = 2
	want := "one two {\\c&H0000FFFF&}three{\\c&H00FFFFFF&} four five"
	if got != want {
		t.Errorf("got %q\nwant %q", got, want)
	}
}

func TestRenderText_ClampedAtEdges(t *testing.T) {
	words := []voice.Word{{Text: "first"}, {Text: "second"}}
	got := renderText(words, 0, 2)
	if !strings.HasPrefix(got, "{\\c&H0000FFFF&}first") {
		t.Errorf("expected active word at start, got %q", got)
	}
	if !strings.Contains(got, "second") {
		t.Errorf("expected 'second' in window, got %q", got)
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
