// Package httpx provides a shared retry wrapper for HTTP calls.
// Used by visuals (Pexels) and voice (ElevenLabs) — the Anthropic SDK
// has its own retry logic, so it doesn't use this.
package httpx

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

// Do calls fn up to maxAttempts times. Retries on network errors,
// 429, and 5xx. Honors Retry-After header on 429 when present.
// Returns the last response (you own the Body) or the last error.
func Do(ctx context.Context, maxAttempts int, base time.Duration, fn func() (*http.Response, error)) (*http.Response, error) {
	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		resp, err := fn()
		if err == nil && resp.StatusCode < 500 && resp.StatusCode != 429 {
			return resp, nil
		}

		// Compute wait
		wait := base * (1 << attempt)
		if resp != nil && resp.StatusCode == 429 {
			if ra := resp.Header.Get("Retry-After"); ra != "" {
				if s, err := strconv.Atoi(ra); err == nil {
					wait = time.Duration(s) * time.Second
				}
			}
		}

		// Drain and close before retrying
		if resp != nil {
			resp.Body.Close()
		}
		lastErr = err
		if err == nil && resp != nil {
			lastErr = fmt.Errorf("http %d", resp.StatusCode)
		}

		// Last attempt: don't sleep, just return
		if attempt == maxAttempts-1 {
			break
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(wait):
		}
	}
	return nil, fmt.Errorf("after %d attempts: %w", maxAttempts, lastErr)
}
