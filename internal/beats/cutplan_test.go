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
	cuts := []float64{0, 2.5, 5.0, 7.5}
	clips := []string{"a.mp4", "b.mp4"}
	segs := AssignClips(cuts, clips)
	if len(segs) != 3 {
		t.Fatalf("want 3 segments, got %d", len(segs))
	}
	if segs[0].ClipPath != "a.mp4" || segs[1].ClipPath != "b.mp4" || segs[2].ClipPath != "a.mp4" {
		t.Errorf("round-robin failed: %+v", segs)
	}
	if math.Abs(segs[0].OutPoint-2.5) > 1e-9 {
		t.Errorf("outpoint wrong: %f", segs[0].OutPoint)
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
