// Package script generates the Hook/Body/Twist/Keywords JSON for a Short
// using Anthropic Claude Haiku 4.5 via the official Go SDK.
package script

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// Script is the structured output we expect from Haiku.
type Script struct {
	PhenomenonName  string   `json:"phenomenon_name"`
	Hook            string   `json:"hook"`
	HookOverlayText string   `json:"hook_overlay_text"`
	Body            string   `json:"body"`
	Twist           string   `json:"twist"`
	Keywords        []string `json:"broll_keywords"`
}

// HookOverlay returns the all-caps overlay text for the first 1.5s.
// If the model returned hook_overlay_text it's uppercased and used;
// otherwise we derive it from the first 5 words of Hook.
func (s *Script) HookOverlay() string {
	if t := strings.TrimSpace(s.HookOverlayText); t != "" {
		return strings.ToUpper(t)
	}
	// Fallback: first 5 words of the spoken hook, uppercased
	words := strings.Fields(s.Hook)
	if len(words) == 0 {
		return ""
	}
	if len(words) > 5 {
		words = words[:5]
	}
	return strings.ToUpper(strings.Join(words, " "))
}

// FullText concatenates the spoken portions for TTS.
func (s *Script) FullText() string {
	return strings.TrimSpace(s.Hook + " " + s.Body + " " + s.Twist)
}

type Generator struct {
	client anthropic.Client
}

func New(apiKey string) *Generator {
	return &Generator{client: anthropic.NewClient(option.WithAPIKey(apiKey))}
}

// Result carries the parsed Script plus token usage from the Anthropic
// response (so callers can compute cost).
type Result struct {
	*Script
	InputTokens  int64
	OutputTokens int64
}

// GenerateOpts carries per-call knobs (style variant, target duration).
// Pipeline picks these and passes them in so it can log the choices.
type GenerateOpts struct {
	Style             Style   // structure variant (default/question/list/story)
	TargetDurationSec float64 // for word-count guidance to the model
}

// Generate calls Haiku 4.5 with the chosen style + target duration and
// parses the response into a Script. SDK handles 429/5xx retries
// internally (default 2 retries).
func (g *Generator) Generate(ctx context.Context, idea string, opts GenerateOpts) (*Result, error) {
	if opts.Style == "" {
		opts.Style = StyleDefault
	}
	if opts.TargetDurationSec <= 0 {
		opts.TargetDurationSec = 30
	}
	prompt := systemPromptFor(opts.Style, opts.TargetDurationSec)
	msg, err := g.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:       anthropic.ModelClaudeHaiku4_5_20251001,
		MaxTokens:   700,
		Temperature: anthropic.Float(0.8),
		System: []anthropic.TextBlockParam{
			{Text: prompt},
		},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock("Idea: " + idea)),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("anthropic: %w", err)
	}
	s, err := parseScriptJSON(extractText(msg))
	if err != nil {
		return nil, err
	}
	// Backstop the prompt's PUNCTUATION rule: normalize any Unicode
	// punctuation the model still emitted in the spoken fields before
	// they reach TTS, captions, and script.json.
	s.Hook = sanitizePunctuation(s.Hook)
	s.Body = sanitizePunctuation(s.Body)
	s.Twist = sanitizePunctuation(s.Twist)
	return &Result{
		Script:       s,
		InputTokens:  msg.Usage.InputTokens,
		OutputTokens: msg.Usage.OutputTokens,
	}, nil
}

// punctuationSanitizer maps the Unicode punctuation LLMs tend to emit to
// plain ASCII. strings.NewReplacer is used rather than a map literal so
// there is no duplicate-key hazard between the two single-quote (and two
// double-quote) variants, and it applies every pair in a single pass.
var punctuationSanitizer = strings.NewReplacer(
	"—", " - ", // em-dash
	"–", " - ", // en-dash
	"‘", "'", // left single quote
	"’", "'", // right single quote / apostrophe
	"“", `"`, // left double quote
	"”", `"`, // right double quote
	"…", "...", // ellipsis
)

var whitespaceRE = regexp.MustCompile(`\s+`)

// sanitizePunctuation rewrites curly quotes, em/en dashes, and ellipsis
// characters to ASCII, then collapses whitespace runs. Curly punctuation
// renders unreliably in subtitle fonts and skews TTS pacing; the system
// prompt asks the model to avoid it and this catches anything that slips
// through. Applied to the spoken fields before script.json is written.
func sanitizePunctuation(s string) string {
	s = punctuationSanitizer.Replace(s)
	s = whitespaceRE.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

// editorialVoicePatterns matches the first-person observation sentences
// the system prompt asks for. Case-insensitive substring match. Used by
// pipeline to log whether the editorial fingerprint landed.
var editorialVoicePatterns = []string{
	"here's the part most people miss",
	"what's interesting is",
	"i noticed this pattern",
	"the thing that gets me about this",
	"watch this:",
}

// HasEditorialVoice returns true if body contains any of the editorial
// voice fingerprint phrases (case-insensitive). Used by the audit log
// to record whether the prompt's instruction was followed.
func HasEditorialVoice(body string) bool {
	low := strings.ToLower(body)
	for _, p := range editorialVoicePatterns {
		if strings.Contains(low, p) {
			return true
		}
	}
	return false
}

func extractText(msg *anthropic.Message) string {
	var b strings.Builder
	for _, block := range msg.Content {
		if variant, ok := block.AsAny().(anthropic.TextBlock); ok {
			b.WriteString(variant.Text)
		}
	}
	return b.String()
}

var (
	jsonFenceRE  = regexp.MustCompile("(?s)```(?:json)?\\s*(\\{.*?\\})\\s*```")
	bareObjectRE = regexp.MustCompile(`(?s)\{.*\}`)
)

// parseScriptJSON tries 3 strategies in order:
//  1. direct unmarshal (model obeyed the no-markdown instruction)
//  2. fenced ```json ... ``` block
//  3. greedy outer { ... }
//
// Haiku usually obeys but sometimes adds fences anyway.
func parseScriptJSON(raw string) (*Script, error) {
	raw = strings.TrimSpace(raw)

	var s Script
	if err := json.Unmarshal([]byte(raw), &s); err == nil {
		return &s, validate(&s)
	}
	if m := jsonFenceRE.FindStringSubmatch(raw); len(m) == 2 {
		if err := json.Unmarshal([]byte(m[1]), &s); err == nil {
			return &s, validate(&s)
		}
	}
	if m := bareObjectRE.FindString(raw); m != "" {
		if err := json.Unmarshal([]byte(m), &s); err == nil {
			return &s, validate(&s)
		}
	}
	preview := raw
	if len(preview) > 200 {
		preview = preview[:200] + "…"
	}
	return nil, fmt.Errorf("could not parse script JSON; first 200 chars: %q", preview)
}

func validate(s *Script) error {
	if s.Hook == "" || s.Body == "" || s.Twist == "" {
		return fmt.Errorf("script missing required fields: hook=%q body=%q twist=%q", s.Hook, s.Body, s.Twist)
	}
	if len(s.Keywords) < 3 {
		return fmt.Errorf("need at least 3 broll keywords, got %d", len(s.Keywords))
	}
	return nil
}
