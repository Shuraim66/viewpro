package pipeline

import "math/rand/v2"

// RandomTargetDuration picks a target spoken duration (seconds) using
// the weighted distribution from the variation spec:
//
//	60% → 28–32 s   (standard)
//	25% → 32–40 s   (longer body)
//	10% → 22–28 s   (snappier)
//	 5% → 40–48 s   (deep-dive)
//
// Caller passes this to script.GenerateOpts.TargetDurationSec so the
// system prompt can target a word count.
func RandomTargetDuration() float64 {
	r := rand.IntN(100)
	var lo, hi float64
	switch {
	case r < 60:
		lo, hi = 28, 32
	case r < 85:
		lo, hi = 32, 40
	case r < 95:
		lo, hi = 22, 28
	default:
		lo, hi = 40, 48
	}
	return lo + rand.Float64()*(hi-lo)
}
