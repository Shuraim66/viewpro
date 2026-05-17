package visuals

import "testing"

func TestRejectedClipsRoundTrip(t *testing.T) {
	dir := t.TempDir()

	// Missing file → empty set, no error.
	if got := LoadRejectedClips(dir); len(got) != 0 {
		t.Fatalf("empty cache: got %d entries, want 0", len(got))
	}

	// First rejection is newly added.
	added, err := RejectClip(dir, 6586070, "personal data visible", "output/20260517-x")
	if err != nil || !added {
		t.Fatalf("RejectClip: added=%v err=%v, want true,nil", added, err)
	}

	// Re-rejecting the same ID is idempotent.
	added, err = RejectClip(dir, 6586070, "dup reason", "")
	if err != nil || added {
		t.Fatalf("RejectClip duplicate: added=%v err=%v, want false,nil", added, err)
	}

	// A second, different ID is added.
	if added, err := RejectClip(dir, 111, "", ""); err != nil || !added {
		t.Fatalf("RejectClip second: added=%v err=%v, want true,nil", added, err)
	}

	// Load reflects both IDs.
	set := LoadRejectedClips(dir)
	if !set[6586070] || !set[111] {
		t.Errorf("loaded set missing entries: %v", set)
	}
	if len(set) != 2 {
		t.Errorf("loaded set size = %d, want 2", len(set))
	}
}
