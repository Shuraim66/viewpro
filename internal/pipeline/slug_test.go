package pipeline

import "testing"

func TestSlugify(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"Chronic Apologizing", "chronic-apologizing"},
		{"  Punctuated! Words?!  ", "punctuated-words"},
		{"", "unknown"},
		{"!!!", "unknown"},
		{"already-slugged", "already-slugged"},
		{"MixedCase123", "mixedcase123"},
		{"a   b   c", "a-b-c"},
		{"this is a really really really really really long phenomenon name that should be cut", "this-is-a-really-really-really-really-re"},
	}
	for _, tt := range tests {
		got := Slugify(tt.in)
		if got != tt.want {
			t.Errorf("Slugify(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
