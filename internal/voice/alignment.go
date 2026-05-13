package voice

import (
	"strings"
)

// Word is one spoken word with its [start, end] in seconds.
type Word struct {
	Text  string  `json:"text"`
	Start float64 `json:"start"`
	End   float64 `json:"end"`
}

// WordsFromChars walks the ElevenLabs per-character alignment and groups
// runs of non-whitespace into words.
//
// Edge cases handled:
//   - ElevenLabs occasionally emits "" as a silent breath token. TrimSpace
//     catches that.
//   - Punctuation attaches to the preceding word (good for karaoke).
//   - chars is []string not []byte because some "characters" are multi-rune.
func WordsFromChars(chars []string, starts, ends []float64) []Word {
	if len(chars) == 0 || len(chars) != len(starts) || len(chars) != len(ends) {
		return nil
	}

	var words []Word
	var buf strings.Builder
	var curStart float64
	inWord := false

	for i, ch := range chars {
		if strings.TrimSpace(ch) == "" {
			if inWord {
				words = append(words, Word{
					Text:  strings.TrimSpace(buf.String()),
					Start: curStart,
					End:   ends[i-1],
				})
				buf.Reset()
				inWord = false
			}
			continue
		}
		if !inWord {
			curStart = starts[i]
			inWord = true
		}
		buf.WriteString(ch)
	}
	if inWord {
		words = append(words, Word{
			Text:  strings.TrimSpace(buf.String()),
			Start: curStart,
			End:   ends[len(chars)-1],
		})
	}
	return words
}
