package visuals

import (
	"reflect"
	"testing"
)

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
