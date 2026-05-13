// Package voice calls ElevenLabs TTS with timestamps and converts the
// per-character alignment into per-word boundaries.
package voice

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"viewpro/internal/httpx"
)

const endpointTmpl = "https://api.elevenlabs.io/v1/text-to-speech/%s/with-timestamps"

type Client struct {
	apiKey  string
	voiceID string
	http    *http.Client
}

func New(apiKey, voiceID string) *Client {
	return &Client{
		apiKey:  apiKey,
		voiceID: voiceID,
		http:    &http.Client{Timeout: 120 * time.Second},
	}
}

// Result is what Synthesize returns.
type Result struct {
	MP3Path  string  // canonical (silence-trimmed) voice.mp3
	Words    []Word  // word timestamps, shifted to match trimmed audio
	Duration float64 // seconds = trimmed MP3 duration
	Chars    int     // input character count, for cost accounting
}

type ttsRequest struct {
	Text          string         `json:"text"`
	ModelID       string         `json:"model_id"`
	VoiceSettings *voiceSettings `json:"voice_settings,omitempty"`
}

type voiceSettings struct {
	Stability       float64 `json:"stability"`
	SimilarityBoost float64 `json:"similarity_boost"`
	Style           float64 `json:"style"`
	SpeakerBoost    bool    `json:"use_speaker_boost"`
}

type ttsResponse struct {
	AudioBase64 string    `json:"audio_base64"`
	Alignment   alignment `json:"alignment"`
}

type alignment struct {
	Characters []string  `json:"characters"`
	Starts     []float64 `json:"character_start_times_seconds"`
	Ends       []float64 `json:"character_end_times_seconds"`
}

// Result is what Synthesize returns: the path to the (trimmed) MP3, the
// per-word alignment after silence-trim shift, and total duration.
//
// Chars is the input character count, useful for cost accounting.
//
// Synthesize POSTs the text and writes voice.mp3 + alignment.json to outDir.
// outDir must already exist.
func (c *Client) Synthesize(ctx context.Context, text, outDir string) (*Result, error) {
	body, err := json.Marshal(ttsRequest{
		Text:    text,
		ModelID: "eleven_turbo_v2_5",
		VoiceSettings: &voiceSettings{
			Stability:       0.45,
			SimilarityBoost: 0.75,
			Style:           0.30,
			SpeakerBoost:    true,
		},
	})
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf(endpointTmpl, c.voiceID)

	resp, err := httpx.Do(ctx, 3, time.Second, func() (*http.Response, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		req.Header.Set("xi-api-key", c.apiKey)
		req.Header.Set("content-type", "application/json")
		req.Header.Set("accept", "application/json")
		return c.http.Do(req)
	})
	if err != nil {
		return nil, fmt.Errorf("elevenlabs request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		preview, _ := io.ReadAll(io.LimitReader(resp.Body, 500))
		return nil, fmt.Errorf("elevenlabs: http %d: %s", resp.StatusCode, string(preview))
	}

	var out ttsResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("elevenlabs: decode response: %w", err)
	}

	// Decode audio and write the raw version (debugging artifact)
	audio, err := base64.StdEncoding.DecodeString(out.AudioBase64)
	if err != nil {
		return nil, fmt.Errorf("decode audio_base64: %w", err)
	}
	rawPath := filepath.Join(outDir, "voice_raw.mp3")
	if err := os.WriteFile(rawPath, audio, 0o644); err != nil {
		return nil, fmt.Errorf("write voice_raw.mp3: %w", err)
	}

	// Build word alignment from the raw response
	words := WordsFromChars(out.Alignment.Characters, out.Alignment.Starts, out.Alignment.Ends)
	if len(words) == 0 {
		return nil, fmt.Errorf("alignment returned 0 words (chars=%d)", len(out.Alignment.Characters))
	}

	// Trim leading + trailing silence; shift word timestamps to match.
	// voice.mp3 (canonical) is the trimmed file; voice_raw.mp3 is kept for debugging.
	canonicalPath := filepath.Join(outDir, "voice.mp3")
	newDur, leadingShift, err := TrimSilence(ctx, rawPath, canonicalPath)
	if err != nil {
		return nil, fmt.Errorf("silence trim: %w", err)
	}
	words = ShiftWords(words, leadingShift, newDur)
	if len(words) == 0 {
		return nil, fmt.Errorf("all words dropped by silence shift (leading=%.3f newDur=%.3f)", leadingShift, newDur)
	}

	// Persist alignment.json with the SHIFTED timestamps (the canonical truth)
	alignPath := filepath.Join(outDir, "alignment.json")
	alignJSON, _ := json.MarshalIndent(map[string]interface{}{
		"words":         words,
		"trimmed_dur":   newDur,
		"leading_shift": leadingShift,
	}, "", "  ")
	_ = os.WriteFile(alignPath, alignJSON, 0o644)

	return &Result{
		MP3Path:  canonicalPath,
		Words:    words,
		Duration: newDur,
		Chars:    len(text),
	}, nil
}
