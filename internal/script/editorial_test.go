package script

import "testing"

func TestHasEditorialVoice(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
		want bool
	}{
		{"exact match", "Here's the part most people miss: ...", true},
		{"different casing", "WATCH THIS: a small detail", true},
		{"partial pattern: 'what's interesting is'", "Now what's interesting is that the brain...", true},
		{"i noticed this pattern", "I noticed this pattern across hundreds of cases.", true},
		{"no editorial voice", "Confirmation bias makes people defend beliefs.", false},
		{"empty", "", false},
		{"the thing that gets me about this", "The thing that gets me about this is the timing.", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := HasEditorialVoice(tc.body); got != tc.want {
				t.Errorf("HasEditorialVoice(%q) = %v, want %v", tc.body, got, tc.want)
			}
		})
	}
}
