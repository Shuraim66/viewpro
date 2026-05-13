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
	PhenomenonName string   `json:"phenomenon_name"`
	Hook           string   `json:"hook"`
	Body           string   `json:"body"`
	Twist          string   `json:"twist"`
	Keywords       []string `json:"broll_keywords"`
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

// Generate calls Haiku 4.5 and parses the response into a Script.
// SDK handles 429/5xx retries internally (default 2).
func (g *Generator) Generate(ctx context.Context, idea string) (*Result, error) {
	msg, err := g.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:       anthropic.ModelClaudeHaiku4_5_20251001,
		MaxTokens:   600,
		Temperature: anthropic.Float(0.8),
		System: []anthropic.TextBlockParam{
			{Text: systemPrompt},
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
	return &Result{
		Script:       s,
		InputTokens:  msg.Usage.InputTokens,
		OutputTokens: msg.Usage.OutputTokens,
	}, nil
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
