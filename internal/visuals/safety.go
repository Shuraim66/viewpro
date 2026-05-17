package visuals

import "strings"

// Content-safety screening for Pexels results. Pexels matches loose
// keywords ("person pausing at threshold" → bedroom thresholds → bare-
// legs/bedroom imagery), and even brief inappropriate visuals hurt a
// channel's advertiser-friendly score. We screen every returned clip's
// metadata before it can be selected, and rank survivors by how well
// they fit the psychology niche.

// blockedTerms disqualify a clip outright. Match is case-insensitive
// substring against the clip's metadata (URL slug + tags). Tune this
// list over time using the broll_review log in audit.json.
var blockedTerms = []string{
	"bikini", "lingerie", "underwear", "swimsuit", "topless",
	"nude", "intimate", "seductive", "boudoir", "sensual",
	"bare legs", "barefoot in bed", "thighs", "cleavage",
	"shirtless", "wet", "shower", "bath", "spa",
	"bedroom shot", "in bed", "lying in bed",
}

// preferredTerms signal a clip is a good fit for the psychology niche
// (people, faces, ordinary settings). They don't gate selection — they
// rank the safe survivors so the best-fitting clip wins.
var preferredTerms = []string{
	"person", "face", "thinking", "portrait", "looking",
	"walking", "standing", "talking", "working", "studying",
	"close up", "head", "eyes", "hands gesturing",
	"outdoor", "office", "kitchen", "living room", "café",
}

// clipMetadata flattens a video's searchable text — the page URL (whose
// slug describes the clip) plus any tags — into one lowercased string.
// Hyphens and underscores become spaces so multi-word terms ("bare
// legs") match Pexels' hyphen-delimited URL slugs ("/bare-legs-in-bed-1/").
// Note: the Pexels video object exposes no free-text description field,
// so URL slug + tags are all the metadata available.
func clipMetadata(rawURL string, tags []string) string {
	var b strings.Builder
	b.WriteString(rawURL)
	for _, t := range tags {
		b.WriteByte(' ')
		b.WriteString(t)
	}
	s := strings.ToLower(b.String())
	s = strings.ReplaceAll(s, "-", " ")
	s = strings.ReplaceAll(s, "_", " ")
	return s
}

// blockedTermIn returns the first blocked term found in meta, or "" if
// the clip is clean. meta must already be clipMetadata-normalized.
func blockedTermIn(meta string) string {
	for _, term := range blockedTerms {
		if strings.Contains(meta, term) {
			return term
		}
	}
	return ""
}

// preferredScore counts how many distinct preferred terms appear in
// meta. Higher is a better niche fit. meta must be clipMetadata-normalized.
func preferredScore(meta string) int {
	n := 0
	for _, term := range preferredTerms {
		if strings.Contains(meta, term) {
			n++
		}
	}
	return n
}

// softIntroTerms flag clips likely to open on a bokeh / soft-focus /
// fade-in frame that a fixed InPoint skip can't reliably clear. Used
// only for the opening segment (segment 0): a matching clip is
// down-ranked, never rejected — so a sharp alternative wins when one
// exists, but a soft-intro clip is still accepted if it's all there is.
var softIntroTerms = []string{
	"bokeh", "soft focus", "blurred background",
	"out of focus", "depth of field", "shallow depth",
	"cinematic portrait", "moody", "ambient",
	"fade in", "slow motion", "abstract",
}

// softIntroPenalty is subtracted from a clip's rank score when it
// matches a softIntroTerm. It exceeds the maximum possible
// preferredScore, so any soft-intro clip sorts below every clean one
// while ties among soft-intro clips still respect preferredScore.
const softIntroPenalty = 1000

// softIntroTermIn returns the first soft-intro term found in meta, or
// "". meta must be clipMetadata-normalized.
func softIntroTermIn(meta string) string {
	for _, term := range softIntroTerms {
		if strings.Contains(meta, term) {
			return term
		}
	}
	return ""
}

// screenRecordingTerms flag clips that are actually screen recordings
// of a real device. Many Pexels "phone" results expose personal data
// (contacts, messages, notifications, search history) and are unsafe
// for editorial content. Like softIntroTerms these down-rank a clip
// rather than rejecting it — but they apply to EVERY segment, since a
// personal-data leak is just as bad mid-Short as in the opener.
var screenRecordingTerms = []string{
	"screen recording", "phone screen", "smartphone screen",
	"app screen", "search bar", "notification panel",
	"messaging app", "social media app",
}

// screenRecordingPenalty is subtracted from a clip's rank score when it
// matches a screenRecordingTerm. Like softIntroPenalty it exceeds the
// maximum preferredScore, so flagged clips sort below every clean one.
const screenRecordingPenalty = 1000

// screenRecordingTermIn returns the first screen-recording term found
// in meta, or "". meta must be clipMetadata-normalized.
func screenRecordingTermIn(meta string) string {
	for _, term := range screenRecordingTerms {
		if strings.Contains(meta, term) {
			return term
		}
	}
	return ""
}
