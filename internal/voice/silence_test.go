package voice

import (
	"math"
	"reflect"
	"testing"
)

func TestParseSilenceOutput(t *testing.T) {
	const sample = `
[silencedetect @ 0x55a] silence_start: 0
[silencedetect @ 0x55a] silence_end: 0.117 | silence_duration: 0.117
[silencedetect @ 0x55a] silence_start: 28.95
[silencedetect @ 0x55a] silence_end: 29.15 | silence_duration: 0.2
`
	b := parseSilenceOutput(sample, 30.0)
	if b.LeadEnd != 0.117 {
		t.Errorf("LeadEnd: got %f, want 0.117", b.LeadEnd)
	}
	// silence_end (29.15) is not at EOF (30) → TailStart stays = duration
	if b.TailStart != 30.0 {
		t.Errorf("TailStart: got %f, want 30.0", b.TailStart)
	}
}

func TestParseSilenceOutput_TrailingToEOF(t *testing.T) {
	// silence runs to end of file → ffmpeg emits silence_start but no matching end
	const sample = `
[silencedetect] silence_start: 0
[silencedetect] silence_end: 0.117 | silence_duration: 0.117
[silencedetect] silence_start: 28.95
`
	b := parseSilenceOutput(sample, 29.2)
	if b.LeadEnd != 0.117 {
		t.Errorf("LeadEnd: got %f, want 0.117", b.LeadEnd)
	}
	if b.TailStart != 28.95 {
		t.Errorf("TailStart: got %f, want 28.95", b.TailStart)
	}
}

func TestParseSilenceOutput_NoSilence(t *testing.T) {
	b := parseSilenceOutput("", 10.0)
	if b.LeadEnd != 0 || b.TailStart != 10.0 {
		t.Errorf("expected no-silence defaults, got Lead=%f Tail=%f", b.LeadEnd, b.TailStart)
	}
}

func TestShiftWords(t *testing.T) {
	tests := []struct {
		name         string
		words        []Word
		leadingShift float64
		newDur       float64
		want         []Word
	}{
		{
			name: "simple shift",
			words: []Word{
				{Text: "Hi", Start: 0.20, End: 0.40},
				{Text: "you", Start: 0.50, End: 0.80},
			},
			leadingShift: 0.20,
			newDur:       0.60,
			want: []Word{
				{Text: "Hi", Start: 0, End: 0.20},
				{Text: "you", Start: 0.30, End: 0.60},
			},
		},
		{
			name: "cap last word at newDuration",
			words: []Word{
				{Text: "last", Start: 5.0, End: 5.8},
			},
			leadingShift: 0,
			newDur:       5.5,
			want: []Word{
				{Text: "last", Start: 5.0, End: 5.5},
			},
		},
		{
			name: "drop word entirely before 0",
			words: []Word{
				{Text: "ghost", Start: 0.05, End: 0.10},
				{Text: "real", Start: 0.30, End: 0.50},
			},
			leadingShift: 0.20,
			newDur:       0.40,
			want: []Word{
				{Text: "real", Start: 0.10, End: 0.30},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ShiftWords(tt.words, tt.leadingShift, tt.newDur)
			if !reflect.DeepEqual(round(got), round(tt.want)) {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func round(ws []Word) []Word {
	out := make([]Word, len(ws))
	for i, w := range ws {
		w.Start = math.Round(w.Start*1000) / 1000
		w.End = math.Round(w.End*1000) / 1000
		out[i] = w
	}
	return out
}
