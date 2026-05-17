package visuals

import (
	"reflect"
	"testing"
)

func TestSlugTitle(t *testing.T) {
	tests := []struct{ in, want string }{
		{"https://www.pexels.com/video/man-looking-at-camera-12345/", "man looking at camera"},
		{"https://www.pexels.com/video/expressive-male-portrait-with-bokeh-background-67890/", "expressive male portrait with bokeh background"},
		{"https://www.pexels.com/video/single-99/", "single"},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			if got := slugTitle(tt.in); got != tt.want {
				t.Errorf("slugTitle(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestSimplifyKeyword(t *testing.T) {
	tests := []struct {
		in   string
		want []string
	}{
		{"nervous hand gestures", []string{"hand gestures", "gestures"}},
		{"person checking phone", []string{"checking phone", "phone"}},
		{"phone", nil},
		{"two words", []string{"words"}},
		{"", nil},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got := simplifyKeyword(tt.in)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}
