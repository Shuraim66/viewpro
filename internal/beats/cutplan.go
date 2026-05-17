package beats

import (
	"math"
	"math/rand/v2"
)

// Segment is one b-roll slot on the timeline, played from ClipPath.
// The source window is [InPoint, InPoint+OutPoint]; the on-screen
// duration is OutPoint and the timeline span is [Start, End].
type Segment struct {
	ClipPath string
	Start    float64 // timeline start
	End      float64 // timeline end
	OutPoint float64 // = End - Start, duration to play from the clip
	InPoint  float64 // offset into the source clip where playback begins (0 = clip start)
}

// firstSegmentIntroSkip trims this many seconds off the start of the
// opening clip's source window. Pexels clips frequently open on a
// bokeh/soft-focus frame, a fade-from-black, or a logo sting, and some
// soft-focus intros run a full second — so the opener starts ~1.0s in
// to land on the sharp subject. The on-screen segment duration is
// unchanged; the source window just shifts later. Assembly's clamp
// reduces the skip if the clip is too short to fit it.
const firstSegmentIntroSkip = 1.0

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

// clipCooldown is the minimum number of segments between two
// placements of the same clip: a clip used at segment N is barred from
// segments N+1..N+clipCooldown. The pool allows a repeat inside the
// window only as a last resort, when every clip is still cooling down
// (happens only with very small pools).
const clipCooldown = 3

// clipPool hands out clip paths so that every clip is placed once
// before any repeats, each repeat cycle is reshuffled (not cyclic), and
// a clip does not reappear within clipCooldown segments of its last use.
type clipPool struct {
	available []string       // not yet drawn this cycle (shuffled)
	used      []string       // drawn this cycle
	lastSeg   map[string]int // clip → segment index of its last placement
}

func newClipPool(clips []string) *clipPool {
	avail := append([]string(nil), clips...)
	rand.Shuffle(len(avail), func(i, j int) { avail[i], avail[j] = avail[j], avail[i] })
	return &clipPool{available: avail, lastSeg: make(map[string]int)}
}

// consume removes a specific clip from the current cycle and records it
// as placed at segIdx. Used for the pinned opener clip at segment 0.
func (p *clipPool) consume(clip string, segIdx int) {
	for i, c := range p.available {
		if c == clip {
			p.available = append(p.available[:i], p.available[i+1:]...)
			break
		}
	}
	p.used = append(p.used, clip)
	p.lastSeg[clip] = segIdx
}

// next returns a clip for segment segIdx, refilling and reshuffling the
// cycle when the pool empties.
func (p *clipPool) next(segIdx int) string {
	if len(p.available) == 0 {
		p.available, p.used = p.used, nil
		rand.Shuffle(len(p.available), func(i, j int) {
			p.available[i], p.available[j] = p.available[j], p.available[i]
		})
	}
	// Prefer a clip outside its cooldown window; fall back to the first
	// available if every candidate is still cooling down (tiny pool).
	pick := 0
	for i, clip := range p.available {
		if last, seen := p.lastSeg[clip]; !seen || segIdx-last > clipCooldown {
			pick = i
			break
		}
	}
	clip := p.available[pick]
	p.available = append(p.available[:pick], p.available[pick+1:]...)
	p.used = append(p.used, clip)
	p.lastSeg[clip] = segIdx
	return clip
}

// AssignClips converts cuts into Segments. Segment 0 is pinned to the
// opener clip (clips[0] — the strict close-up FetchForKeywords returns
// first) and gets the intro skip. Later segments are drawn from a
// no-repeat-until-exhausted shuffled pool, so b-roll variety is
// maximized and the cyclic "templated" signature is broken. Requires
// len(clips) >= 1; returns nil otherwise.
func AssignClips(cuts []float64, clips []string) []Segment {
	if len(cuts) < 2 || len(clips) == 0 {
		return nil
	}
	pool := newClipPool(clips)
	segs := make([]Segment, 0, len(cuts)-1)
	for i := 0; i < len(cuts)-1; i++ {
		start, end := cuts[i], cuts[i+1]
		var clip string
		if i == 0 {
			clip = clips[0] // opener — pinned, never shuffled
			pool.consume(clip, 0)
		} else {
			clip = pool.next(i)
		}
		seg := Segment{
			ClipPath: clip,
			Start:    start,
			End:      end,
			OutPoint: end - start,
		}
		// The opening clip skips its intro frames (bokeh/fade/logo).
		// Assembly pulls this back if the clip is too short to fit.
		if i == 0 {
			seg.InPoint = firstSegmentIntroSkip
		}
		segs = append(segs, seg)
	}
	return segs
}

// ClipUsage summarizes how b-roll clips are distributed across the
// assembled segments — for the end-of-cut-plan log line.
type ClipUsage struct {
	Segments    int
	UniqueClips int
	MaxRepeats  int // most placements of any single clip
	MinGap      int // smallest segment gap between two uses of one clip; -1 if no clip repeats
}

// SummarizeClipUsage computes ClipUsage stats for a segment list.
func SummarizeClipUsage(segs []Segment) ClipUsage {
	counts := make(map[string]int)
	lastIdx := make(map[string]int)
	minGap := -1
	for i, s := range segs {
		counts[s.ClipPath]++
		if prev, ok := lastIdx[s.ClipPath]; ok {
			if gap := i - prev; minGap < 0 || gap < minGap {
				minGap = gap
			}
		}
		lastIdx[s.ClipPath] = i
	}
	maxRep := 0
	for _, n := range counts {
		if n > maxRep {
			maxRep = n
		}
	}
	return ClipUsage{
		Segments:    len(segs),
		UniqueClips: len(counts),
		MaxRepeats:  maxRep,
		MinGap:      minGap,
	}
}
