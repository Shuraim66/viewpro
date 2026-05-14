package music

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPick_SingleFile(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "track.mp3")
	if err := os.WriteFile(f, []byte{0}, 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Pick(f)
	if err != nil {
		t.Fatal(err)
	}
	if got != f {
		t.Errorf("single-file path: got %q, want %q", got, f)
	}
}

func TestPick_Directory(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"a.mp3", "b.wav", "c.m4a", "ignore.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte{0}, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// Pick 30 times; verify we hit each audio file (3) at least once and
	// never the non-audio file.
	seen := map[string]bool{}
	for range 30 {
		got, err := Pick(dir)
		if err != nil {
			t.Fatal(err)
		}
		base := filepath.Base(got)
		if base == "ignore.txt" {
			t.Fatalf("picked non-audio file %q", got)
		}
		seen[base] = true
	}
	for _, want := range []string{"a.mp3", "b.wav", "c.m4a"} {
		if !seen[want] {
			t.Errorf("never picked %q across 30 trials", want)
		}
	}
}

func TestPick_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	if _, err := Pick(dir); err == nil {
		t.Error("expected error on empty dir, got nil")
	}
}

func TestList(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"a.mp3", "b.txt", "c.wav"} {
		_ = os.WriteFile(filepath.Join(dir, name), []byte{0}, 0o644)
	}
	got := List(dir)
	if len(got) != 2 {
		t.Errorf("got %d files, want 2: %v", len(got), got)
	}
}
