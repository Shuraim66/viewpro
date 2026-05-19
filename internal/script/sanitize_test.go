package script

import "testing"

func TestSanitizePunctuation(t *testing.T) {
	for _, tc := range []struct {
		name, in, want string
	}{
		{"em-dash becomes spaced hyphen", "your brain—watch this", "your brain - watch this"},
		{"en-dash becomes spaced hyphen", "pages 3–5", "pages 3 - 5"},
		{"curly apostrophe", "you’re not", "you're not"},
		{"curly double quotes", "say “I am anxious”", `say "I am anxious"`},
		{"ellipsis becomes three dots", "wait… there's more", "wait... there's more"},
		{"collapses whitespace runs", "too    many   spaces", "too many spaces"},
		{"trims ends", "  hello  ", "hello"},
		{"clean text untouched", "already clean text.", "already clean text."},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := sanitizePunctuation(tc.in); got != tc.want {
				t.Errorf("sanitizePunctuation(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
