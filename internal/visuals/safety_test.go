package visuals

import "testing"

func TestBlockedTermIn(t *testing.T) {
	cases := []struct {
		name string
		url  string
		tags []string
		want string
	}{
		{"clean", "https://www.pexels.com/video/person-walking-in-park-123/", []string{"person", "walking"}, ""},
		{"blocked in url slug", "https://www.pexels.com/video/woman-in-lingerie-456/", nil, "lingerie"},
		{"blocked in tag", "https://www.pexels.com/video/clip-789/", []string{"morning", "bikini"}, "bikini"},
		{"multi-word term across hyphens", "https://www.pexels.com/video/bare-legs-in-bed-999/", nil, "bare legs"},
		{"case-insensitive", "https://www.pexels.com/video/SHIRTLESS-man-1/", nil, "shirtless"},
		{"threshold returns bedroom", "https://www.pexels.com/video/woman-lying-in-bed-2/", nil, "in bed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := blockedTermIn(clipMetadata(tc.url, tc.tags)); got != tc.want {
				t.Errorf("blockedTermIn(%q, %v) = %q, want %q", tc.url, tc.tags, got, tc.want)
			}
		})
	}
}

func TestPreferredScore(t *testing.T) {
	// url contributes "person" + "office"; tags add "thinking" + "face".
	meta := clipMetadata("https://www.pexels.com/video/person-in-office-1/", []string{"thinking", "face"})
	if got := preferredScore(meta); got != 4 {
		t.Errorf("preferredScore = %d, want 4", got)
	}
	if got := preferredScore(clipMetadata("https://www.pexels.com/video/city-night-2/", nil)); got != 0 {
		t.Errorf("preferredScore (no match) = %d, want 0", got)
	}
}
