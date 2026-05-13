package pipeline

import "strings"

// Slugify converts a free-form string into a filesystem-friendly slug.
// Lowercases, replaces runs of non-alphanumerics with single hyphens,
// trims hyphens from both ends, caps at 40 chars.
//
//	"Chronic Apologizing" → "chronic-apologizing"
//	"  Punctuated! Words?!  " → "punctuated-words"
//	"" → "unknown"
func Slugify(s string) string {
	var b strings.Builder
	lastDash := false
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastDash = false
		} else if !lastDash && b.Len() > 0 {
			b.WriteRune('-')
			lastDash = true
		}
	}
	slug := strings.Trim(b.String(), "-")
	if slug == "" {
		return "unknown"
	}
	if len(slug) > 40 {
		slug = strings.Trim(slug[:40], "-")
	}
	return slug
}
