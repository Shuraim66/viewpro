package beats

import (
	"math"
	"testing"
)

func TestPlanCuts(t *testing.T) {
	t.Run("no onsets → evenly-spaced fallback", func(t *testing.T) {
		cuts := PlanCuts(nil, 10.0, 1.8, 3.5)
		// Expect 0, midpoints to cover 10s with max gap 3.5, then 10
		assertMonotonic(t, cuts)
		assertBounds(t, cuts, 0, 10)
		assertMaxGap(t, cuts, 3.5)
	})

	t.Run("dense onsets get filtered to minI", func(t *testing.T) {
		// Onsets every 0.5s; minI=1.8, maxI=3.5
		onsets := []float64{0.5, 1.0, 1.5, 2.0, 2.5, 3.0, 3.5, 4.0, 4.5, 5.0, 5.5, 6.0}
		cuts := PlanCuts(onsets, 8.0, 1.8, 3.5)
		assertMonotonic(t, cuts)
		assertBounds(t, cuts, 0, 8)
		assertMinGap(t, cuts, 1.8)
		assertMaxGap(t, cuts, 3.5)
	})

	t.Run("sparse onsets get midpoints inserted", func(t *testing.T) {
		onsets := []float64{5.0} // one onset at t=5 in a 10s clip
		cuts := PlanCuts(onsets, 10.0, 1.8, 3.5)
		assertMonotonic(t, cuts)
		assertBounds(t, cuts, 0, 10)
		assertMaxGap(t, cuts, 3.5)
	})

	t.Run("onsets outside [0,duration] are skipped", func(t *testing.T) {
		onsets := []float64{-1.0, 3.0, 100.0}
		cuts := PlanCuts(onsets, 5.0, 1.8, 3.5)
		assertMonotonic(t, cuts)
		assertBounds(t, cuts, 0, 5)
	})

	t.Run("very short duration", func(t *testing.T) {
		cuts := PlanCuts(nil, 1.0, 1.8, 3.5)
		// Should give us at least [0, 1.0]
		if len(cuts) != 2 {
			t.Errorf("want 2 cuts for short duration, got %v", cuts)
		}
	})
}

func TestAssignClips(t *testing.T) {
	cuts := []float64{0, 2.5, 5.0, 7.5, 10.0, 12.5} // 5 segments
	clips := []string{"opener.mp4", "b.mp4", "c.mp4", "d.mp4", "e.mp4"}
	segs := AssignClips(cuts, clips)
	if len(segs) != 5 {
		t.Fatalf("want 5 segments, got %d", len(segs))
	}
	// Segment 0 is always pinned to the opener (clips[0]).
	if segs[0].ClipPath != "opener.mp4" {
		t.Errorf("segment 0: want opener.mp4, got %q", segs[0].ClipPath)
	}
	if math.Abs(segs[0].OutPoint-2.5) > 1e-9 {
		t.Errorf("outpoint wrong: %f", segs[0].OutPoint)
	}
	// First segment skips the clip intro; the rest start at clip t=0.
	if math.Abs(segs[0].InPoint-firstSegmentIntroSkip) > 1e-9 {
		t.Errorf("segment 0 InPoint: want %f, got %f", firstSegmentIntroSkip, segs[0].InPoint)
	}
	for i := 1; i < len(segs); i++ {
		if segs[i].InPoint != 0 {
			t.Errorf("segment %d InPoint: want 0, got %f", i, segs[i].InPoint)
		}
	}
	// Every clip path is from the input set.
	known := map[string]bool{}
	for _, c := range clips {
		known[c] = true
	}
	for i, s := range segs {
		if !known[s.ClipPath] {
			t.Errorf("segment %d: unknown clip %q", i, s.ClipPath)
		}
	}
	// 5 clips, 5 segments → pool not exhausted → no clip repeats.
	seen := map[string]bool{}
	for _, s := range segs {
		if seen[s.ClipPath] {
			t.Errorf("clip %q repeated before pool exhausted: %+v", s.ClipPath, segs)
		}
		seen[s.ClipPath] = true
	}
}

func TestClipPool(t *testing.T) {
	clips := []string{"a", "b", "c", "d", "e", "f"}
	pool := newClipPool(clips)
	const draws = 18
	got := make([]string, 0, draws)
	for i := 0; i < draws; i++ {
		got = append(got, pool.next(i))
	}
	// No-repeat-until-exhausted: each block of len(clips) draws is a
	// permutation of the pool — no clip appears twice within a block.
	for block := 0; block*len(clips) < len(got); block++ {
		seen := map[string]bool{}
		for j := block * len(clips); j < (block+1)*len(clips) && j < len(got); j++ {
			if seen[got[j]] {
				t.Errorf("block %d: %q repeated before pool exhausted: %v", block, got[j], got)
			}
			seen[got[j]] = true
		}
	}
	// Cooldown: with 6 clips and cooldown 3, no clip reappears within
	// clipCooldown segments of its last placement.
	last := map[string]int{}
	for i, c := range got {
		if prev, ok := last[c]; ok && i-prev <= clipCooldown {
			t.Errorf("clip %q reused at seg %d, only %d after seg %d (cooldown %d)", c, i, i-prev, prev, clipCooldown)
		}
		last[c] = i
	}
}

func TestSummarizeClipUsage(t *testing.T) {
	segs := []Segment{
		{ClipPath: "a"}, {ClipPath: "b"}, {ClipPath: "c"},
		{ClipPath: "a"}, {ClipPath: "d"},
	}
	u := SummarizeClipUsage(segs)
	if u.Segments != 5 {
		t.Errorf("Segments = %d, want 5", u.Segments)
	}
	if u.UniqueClips != 4 {
		t.Errorf("UniqueClips = %d, want 4", u.UniqueClips)
	}
	if u.MaxRepeats != 2 {
		t.Errorf("MaxRepeats = %d, want 2", u.MaxRepeats)
	}
	if u.MinGap != 3 { // "a" at index 0 and 3
		t.Errorf("MinGap = %d, want 3", u.MinGap)
	}

	// No repeats → MinGap is -1.
	if g := SummarizeClipUsage([]Segment{{ClipPath: "x"}, {ClipPath: "y"}}).MinGap; g != -1 {
		t.Errorf("MinGap with no repeats = %d, want -1", g)
	}
}

// helpers

func assertMonotonic(t *testing.T, cuts []float64) {
	t.Helper()
	for i := 1; i < len(cuts); i++ {
		if cuts[i] <= cuts[i-1] {
			t.Fatalf("not monotonic: %v", cuts)
		}
	}
}

func assertBounds(t *testing.T, cuts []float64, lo, hi float64) {
	t.Helper()
	if cuts[0] != lo {
		t.Errorf("first cut: want %f, got %f", lo, cuts[0])
	}
	if math.Abs(cuts[len(cuts)-1]-hi) > 1e-9 {
		t.Errorf("last cut: want %f, got %f", hi, cuts[len(cuts)-1])
	}
}

func assertMinGap(t *testing.T, cuts []float64, minGap float64) {
	t.Helper()
	for i := 1; i < len(cuts); i++ {
		// Allow the final stub interval if we couldn't reach minGap from the last onset
		if i == len(cuts)-1 {
			continue
		}
		if cuts[i]-cuts[i-1] < minGap-1e-9 {
			t.Errorf("gap[%d→%d] = %f < minGap %f; cuts=%v", i-1, i, cuts[i]-cuts[i-1], minGap, cuts)
		}
	}
}

func assertMaxGap(t *testing.T, cuts []float64, maxGap float64) {
	t.Helper()
	for i := 1; i < len(cuts); i++ {
		if cuts[i]-cuts[i-1] > maxGap+1e-9 {
			t.Errorf("gap[%d→%d] = %f > maxGap %f; cuts=%v", i-1, i, cuts[i]-cuts[i-1], maxGap, cuts)
		}
	}
}
