package pipeline

// Audit captures every per-run variant decision so we can prove
// non-templated output if YouTube flags the channel. Persisted to
// <session_dir>/audit.json and surfaced on Result for the CLI to print.
type Audit struct {
	Idea string `json:"idea"`
	Slug string `json:"slug"`

	// Variant choices — these are what YouTube's "Inauthentic Content"
	// detector cares about. Rotating these per Short keeps each upload
	// visually + structurally distinct.
	CaptionPreset string  `json:"caption_preset"`
	MusicFile     string  `json:"music_file"`
	ScriptStyle   string  `json:"script_style"`
	TargetDurSec  float64 `json:"target_duration_sec"`
	ActualDurSec  float64 `json:"actual_duration_sec"`

	// Content fingerprint
	HookOverlayText     string `json:"hook_overlay_text"`
	EditorialVoiceFound bool   `json:"editorial_voice_found"`

	// Visual content review — one entry per selected b-roll clip, so
	// off-topic or advertiser-unfriendly Pexels matches can be traced
	// in post-review and the blocked-terms list tuned over time.
	BrollReview []BrollReviewItem `json:"broll_review"`

	// Cost rollup
	ClaudeUSD     float64 `json:"claude_usd"`
	ElevenLabsUSD float64 `json:"elevenlabs_usd"`
	TotalUSD      float64 `json:"total_usd"`

	// Outputs
	OutputMP4 string `json:"output_mp4"`
}

// BrollReviewItem traces one selected Pexels clip back to the keyword
// that requested it, so a human reviewer can spot keyword→clip
// mismatches (e.g. "threshold" → bedroom footage) after the fact.
type BrollReviewItem struct {
	KeywordRequested string   `json:"keyword_requested"`
	PexelsURL        string   `json:"pexels_url"`
	SelectedPath     string   `json:"selected_path"`
	TagsReturned     []string `json:"tags_returned"`
}
