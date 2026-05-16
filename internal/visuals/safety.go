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
