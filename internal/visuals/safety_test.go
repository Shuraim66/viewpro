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

func TestSoftIntroTermIn(t *testing.T) {
	cases := []struct {
		name string
		url  string
		tags []string
		want string
	}{
		{"clean portrait", "https://www.pexels.com/video/man-looking-at-camera-1/", nil, ""},
		{"bokeh in slug", "https://www.pexels.com/video/expressive-portrait-with-bokeh-background-2/", nil, "bokeh"},
		{"soft focus across hyphens", "https://www.pexels.com/video/woman-in-soft-focus-3/", nil, "soft focus"},
		{"term in tag", "https://www.pexels.com/video/clip-4/", []string{"moody", "evening"}, "moody"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := softIntroTermIn(clipMetadata(tc.url, tc.tags)); got != tc.want {
				t.Errorf("softIntroTermIn(%q, %v) = %q, want %q", tc.url, tc.tags, got, tc.want)
			}
		})
	}
}

func TestContainsTerm(t *testing.T) {
	cases := []struct {
		meta, term string
		want       bool
	}{
		{"organized workspace with a modern touch", "spa", false}, // the bug: "spa" inside "workspace"
		{"a quiet day at the spa", "spa", true},
		{"man looking at camera outdoors", "looking at camera", true},
		{"headshot of a person", "head", false}, // "head" is not a whole word here
		{"turn your head slowly", "head", true},
		{"close up of a notebook", "close up", true},
	}
	for _, tc := range cases {
		if got := containsTerm(tc.meta, tc.term); got != tc.want {
			t.Errorf("containsTerm(%q, %q) = %v, want %v", tc.meta, tc.term, got, tc.want)
		}
	}
}

func TestFaceTermIn(t *testing.T) {
	cases := []struct {
		name string
		url  string
		tags []string
		want string
	}{
		{"generic object", "https://www.pexels.com/video/coffee-cup-on-table-1/", nil, ""},
		{"object close-up is not a face", "https://www.pexels.com/video/close-up-of-handwriting-in-a-notebook-1/", nil, ""},
		{"portrait in slug", "https://www.pexels.com/video/expressive-portrait-of-a-woman-2/", nil, "portrait"},
		{"looking at camera across hyphens", "https://www.pexels.com/video/man-looking-at-camera-3/", nil, "looking at camera"},
		{"term in tag", "https://www.pexels.com/video/clip-4/", []string{"head shot", "studio"}, "head shot"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := faceTermIn(clipMetadata(tc.url, tc.tags)); got != tc.want {
				t.Errorf("faceTermIn(%q, %v) = %q, want %q", tc.url, tc.tags, got, tc.want)
			}
		})
	}
}

func TestFaceScore(t *testing.T) {
	cases := []struct {
		name string
		url  string
		want int
	}{
		{"face only", "https://www.pexels.com/video/close-up-portrait-1/", 10},
		{"generic, no terms", "https://www.pexels.com/video/city-street-at-night-2/", 0},
		{"face minus soft-intro", "https://www.pexels.com/video/portrait-with-bokeh-background-3/", 5},   // +10 −5
		{"face minus screen content", "https://www.pexels.com/video/face-near-a-phone-screen-4/", 0},     // +10 −10
		{"screen content only", "https://www.pexels.com/video/a-search-bar-and-results-5/", -10},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := faceScore(clipMetadata(tc.url, nil)); got != tc.want {
				t.Errorf("faceScore(%q) = %d, want %d", tc.url, got, tc.want)
			}
		})
	}
}

func TestScreenRecordingTermIn(t *testing.T) {
	cases := []struct {
		name string
		url  string
		tags []string
		want string
	}{
		{"clean", "https://www.pexels.com/video/person-holding-phone-1/", nil, ""},
		{"phone screen in slug", "https://www.pexels.com/video/close-up-of-phone-screen-2/", nil, "phone screen"},
		{"screen recording across hyphens", "https://www.pexels.com/video/screen-recording-of-app-3/", nil, "screen recording"},
		{"term in tag", "https://www.pexels.com/video/clip-4/", []string{"messaging app", "chat"}, "messaging app"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := screenRecordingTermIn(clipMetadata(tc.url, tc.tags)); got != tc.want {
				t.Errorf("screenRecordingTermIn(%q, %v) = %q, want %q", tc.url, tc.tags, got, tc.want)
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
