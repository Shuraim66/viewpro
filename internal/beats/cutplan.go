package beats

import "math"

// Segment is one b-roll slot: [In, Out) on the timeline, played from
// ClipPath starting at 0 inside the clip.
type Segment struct {
	ClipPath string
	Start    float64 // timeline start
	End      float64 // timeline end
	OutPoint float64 // = End - Start, duration to play from the clip
}

// PlanCuts filters speech onsets so consecutive intervals stay within
// [minI, maxI] seconds. Always starts at 0 and ends at duration.
// Length of the returned slice = (number of clips needed) + 1.
func PlanCuts(onsets []float64, duration, minI, maxI float64) []float64 {
	if duration <= 0 {
		return []float64{0}
	}

	cuts := []float64{0.0}
	for _, t := range onsets {
		if t <= 0 || t >= duration {
			continue
		}
		gap := t - cuts[len(cuts)-1]
		if gap < minI {
			continue // too close, skip
		}
		if gap > maxI {
			// gap is too big; inject midpoints before appending t
			mids := fillGap(cuts[len(cuts)-1], t, maxI)
			cuts = append(cuts, mids...)
		}
		cuts = append(cuts, t)
	}

	// Tail handling — make sure the last interval lands at `duration`
	tail := duration - cuts[len(cuts)-1]
	if tail < minI && len(cuts) > 1 {
		// Drop the last cut so the previous interval absorbs the stub
		cuts = cuts[:len(cuts)-1]
	}
	tail = duration - cuts[len(cuts)-1]
	if tail > maxI {
		cuts = append(cuts, fillGap(cuts[len(cuts)-1], duration, maxI)...)
	}
	cuts = append(cuts, duration)
	return cuts
}

// fillGap inserts evenly-spaced midpoints to make every sub-gap ≤ maxI.
// Does NOT include `start` or `end`.
func fillGap(start, end, maxI float64) []float64 {
	gap := end - start
	if gap <= maxI {
		return nil
	}
	// Number of midpoints needed
	n := int(math.Ceil(gap/maxI)) - 1
	if n <= 0 {
		return nil
	}
	step := gap / float64(n+1)
	out := make([]float64, n)
	for i := 0; i < n; i++ {
		out[i] = start + step*float64(i+1)
	}
	return out
}

// AssignClips converts cuts into Segments by round-robin assigning
// clip paths. Requires len(clips) >= 1; returns nil if not.
func AssignClips(cuts []float64, clips []string) []Segment {
	if len(cuts) < 2 || len(clips) == 0 {
		return nil
	}
	segs := make([]Segment, 0, len(cuts)-1)
	for i := 0; i < len(cuts)-1; i++ {
		start, end := cuts[i], cuts[i+1]
		segs = append(segs, Segment{
			ClipPath: clips[i%len(clips)],
			Start:    start,
			End:      end,
			OutPoint: end - start,
		})
	}
	return segs
}
