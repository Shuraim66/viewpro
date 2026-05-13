package assembly

import (
	"math"
	"testing"
)

func TestMusicVolumeForVoice(t *testing.T) {
	tests := []struct {
		voiceMean float64
		want      float64
	}{
		{-18.0, 0.15}, // neutral reference point
		{-10.0, 0.18}, // loud voice → clamped at ceiling
		{-28.0, 0.08}, // quiet voice → clamped at floor
		{-20.0, 0.13}, // slightly quiet
		{-15.0, 0.18}, // 0.15 + 0.03 = 0.18 (edge)
		{0.0, 0.18},   // unrealistic-loud, clamped
	}
	for _, tt := range tests {
		got := MusicVolumeForVoice(tt.voiceMean)
		if math.Abs(got-tt.want) > 1e-9 {
			t.Errorf("MusicVolumeForVoice(%f) = %f, want %f", tt.voiceMean, got, tt.want)
		}
	}
}
