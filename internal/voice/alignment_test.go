package voice

import (
	"reflect"
	"testing"
)

func TestWordsFromChars(t *testing.T) {
	tests := []struct {
		name   string
		chars  []string
		starts []float64
		ends   []float64
		want   []Word
	}{
		{
			name:   "simple two words",
			chars:  []string{"H", "i", " ", "y", "o", "u"},
			starts: []float64{0.00, 0.05, 0.10, 0.15, 0.20, 0.25},
			ends:   []float64{0.05, 0.10, 0.15, 0.20, 0.25, 0.30},
			want: []Word{
				{Text: "Hi", Start: 0.00, End: 0.10},
				{Text: "you", Start: 0.15, End: 0.30},
			},
		},
		{
			name:   "trailing punctuation attaches",
			chars:  []string{"O", "k", "!", " ", "g", "o"},
			starts: []float64{0.0, 0.05, 0.10, 0.15, 0.20, 0.25},
			ends:   []float64{0.05, 0.10, 0.15, 0.20, 0.25, 0.30},
			want: []Word{
				{Text: "Ok!", Start: 0.0, End: 0.15},
				{Text: "go", Start: 0.20, End: 0.30},
			},
		},
		{
			name:   "empty breath tokens treated as whitespace",
			chars:  []string{"a", "", " ", "b"},
			starts: []float64{0.0, 0.05, 0.10, 0.15},
			ends:   []float64{0.05, 0.10, 0.15, 0.20},
			want: []Word{
				{Text: "a", Start: 0.0, End: 0.05},
				{Text: "b", Start: 0.15, End: 0.20},
			},
		},
		{
			name:   "mismatched lengths returns nil",
			chars:  []string{"a", "b"},
			starts: []float64{0.0},
			ends:   []float64{0.05},
			want:   nil,
		},
		{
			name:   "all whitespace returns nil",
			chars:  []string{" ", " ", " "},
			starts: []float64{0.0, 0.05, 0.10},
			ends:   []float64{0.05, 0.10, 0.15},
			want:   nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := WordsFromChars(tt.chars, tt.starts, tt.ends)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}
