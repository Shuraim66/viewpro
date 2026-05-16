package script

import (
	"fmt"
	"math/rand/v2"
	"strings"
)

// Style identifies a script-structure variant. Rotating these reduces
// the "template" feel of the channel, which matters for YouTube's
// Inauthentic Content policy.
type Style string

const (
	StyleDefault  Style = "default"  // hook → body → twist (the original)
	StyleQuestion Style = "question" // "Why does X?" → answer → reflection
	StyleList     Style = "list"     // "3 things X share" → enumerated → reflection
	StyleStory    Style = "story"    // "A person did X" → explanation → reflection
)

// RandomStyle picks a weighted-random style: default 60%, question 20%,
// list 10%, story 10%.
func RandomStyle() Style {
	r := rand.IntN(100)
	switch {
	case r < 60:
		return StyleDefault
	case r < 80:
		return StyleQuestion
	case r < 90:
		return StyleList
	default:
		return StyleStory
	}
}

// ParseStyle accepts a CLI flag value. Empty / unknown → StyleDefault.
// Returns ok=false on unknown so callers can warn.
func ParseStyle(s string) (Style, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "default", "":
		return StyleDefault, true
	case "question":
		return StyleQuestion, true
	case "list":
		return StyleList, true
	case "story":
		return StyleStory, true
	default:
		return StyleDefault, false
	}
}

// systemPromptFor assembles the full Claude system prompt for a given
// style and target spoken duration in seconds (used for word-count
// guidance, since ElevenLabs Turbo speaks ≈2.5 words/sec).
func systemPromptFor(s Style, targetDurSec float64) string {
	body := stylePromptBody(s)
	targetWords := int(targetDurSec * 2.5)
	// Hook + twist each absorb ~13 words; everything else is body.
	bodyWords := targetWords - 26
	if bodyWords < 30 {
		bodyWords = 30
	}
	pacing := fmt.Sprintf(`
TARGET LENGTH: This script must be approximately %d words total across all
sections combined (hook + body + twist). Speakers average 2.5 words per
second; the final voiceover MUST fit within %.0f seconds.

Distribute words approximately:
- Hook: 12-15 words
- Body: ~%d words
- Twist: 12-15 words

DO NOT exceed the target word count. Be ruthless about cutting unnecessary
phrases, hedges, and filler. A 30-word body is better than a 60-word body
that runs long.
`, targetWords, targetDurSec, bodyWords)
	return body + pacing + sharedEditorialAndSchema
}

// sharedEditorialAndSchema is the tail block appended to every variant.
// It enforces the editorial-voice fingerprint and the JSON output shape.
const sharedEditorialAndSchema = `
EDITORIAL VOICE (REQUIRED): The body MUST contain exactly ONE sentence
written in first-person observation voice. Use one of these patterns
(or a close variant) verbatim:
- "Here's the part most people miss..."
- "What's interesting is..."
- "I noticed this pattern..."
- "The thing that gets me about this..."
- "Watch this: ..."
This single sentence is the editorial fingerprint that distinguishes
real curation from templated output. Do NOT skip it.

CRITICAL — NO FABRICATED PEOPLE OR ANECDOTES: The editorial voice must
be observational about the phenomenon itself, not a story about people
who don't exist. DO NOT invent fictional friends, patients, clients,
acquaintances, or any named/unnamed personal anecdote.

BANNED phrasings (do not use any variant of these):
- "My friend X told me..."
- "A patient I knew..."
- "Someone I know..."
- "I had a client who..."
- "A colleague once..."
- Any sentence asserting first-person acquaintance with a person.

GOOD example: "Here's the part most people miss — you're not choosing to
think about it."
BAD example: "My friend Sarah told me she once spent two hours doing this."

BROLL_KEYWORDS (broll_keywords): Provide 4-6 stock-video search phrases.
Each phrase MUST follow these rules:
- 4-6 words describing a clearly visible, specific scene.
- A filmable action or scene, never an abstract noun. Good: "person
  checking phone in kitchen". Bad: "anxiety", "memory", "feelings".
- NEVER use ambiguous words — they pull off-topic or unsafe stock
  footage: "intimate", "threshold" (returns bedroom shots), "exposed",
  "raw", "vulnerable".
- Describe the visible action, not an abstract emotion: write "person
  looking around confused", not "feeling lost".
- Specify a location when natural: "in office", "in kitchen",
  "outdoor walking", "at desk".

HOOK_OVERLAY_TEXT (4-6 words, all caps): A condensed scroll-stopper for
the first 1.5 seconds. Punchier and shorter than the spoken hook.
Examples:
- "APOLOGIZE TOO MUCH? READ THIS"
- "3 AM BRAIN HACK"
- "WHY YOU CHECK YOUR PHONE"

Return ONLY valid JSON, no markdown fences, no commentary:

{
  "phenomenon_name": "string",
  "hook": "string",
  "hook_overlay_text": "ALL CAPS PUNCHY TEXT",
  "body": "string",
  "twist": "string",
  "broll_keywords": ["string", "string", "string", "string"]
}`

func stylePromptBody(s Style) string {
	switch s {
	case StyleQuestion:
		return questionPromptBody
	case StyleList:
		return listPromptBody
	case StyleStory:
		return storyPromptBody
	default:
		return defaultPromptBody
	}
}

const defaultPromptBody = `You write 45-second psychology YouTube Shorts. Use this structure:

1. HOOK (max 12 words, ~3 seconds spoken): A scroll-stopping opener. Vary patterns across scripts:
   - Specific accusation: "People who [behavior] had [unexpected backstory]."
   - Science reveal: "If you [common experience], your brain is doing something [adjective]."
   - Contrarian claim: "Stop [common advice]. Here's what your brain actually hears."
   - Body-language tell: "Watch what someone does with their [body part] when they [verb]."

2. BODY: Name the psychology concept. Give one concrete real-world example. Write like you're leaning in to tell a friend, not like a textbook.

3. TWIST (max 15 words): A counterintuitive takeaway, a question, or a callout that makes the viewer recognize themselves.`

const questionPromptBody = `You write 45-second psychology YouTube Shorts structured as Q→A. Use this structure:

1. HOOK: An open question that hooks the viewer's specific lived experience. Format: "Why do you [common behavior]?" or "Why does [X] make you [feeling]?" Must be answerable. Max 14 words.

2. BODY: Answer the question. Name the psychology concept. Give a concrete example. The structure should feel like "here's why" — direct, specific, not preachy. Write conversationally, like explaining to a curious friend.

3. TWIST (max 15 words): Turn the answer back on the viewer with a reflective callout — "next time it happens, notice X" or a one-line reframe.`

const listPromptBody = `You write 45-second psychology YouTube Shorts as enumerated lists. Use this structure:

1. HOOK (max 13 words): Announce the count + the topic. Format: "Three things [people with X behavior] all share" or "Four signs your brain is [doing Y]." The number must match what you deliver in the body. Use 2-4 items only (more loses the viewer).

2. BODY: Walk through each numbered item. Each item gets one sentence — a name + one specific real-world tell. Don't repeat the count number ("First...", "Second...") — let the count be implicit in the prose. End with the psychology concept that unifies the items.

3. TWIST (max 15 words): Land on which item is the one most people miss, or a reflective one-liner.`

const storyPromptBody = `You write 45-second psychology YouTube Shorts as mini case studies. Use this structure:

1. HOOK (max 14 words): Introduce a generic, hypothetical person doing one specific behavior. Format: "Some people keep [behavior]." or "Picture someone who [does X every time Y]." Use third-person, present-tense, no names, no claims of personal acquaintance ("I knew...", "my friend..."). Must imply something is going on neurologically.

2. BODY: Explain what the brain is actually doing in that scenario. Name the psychology concept. Connect the described behavior to the general mechanism. Don't moralize; just describe. Stay generic — no fabricated patients, friends, or named individuals.

3. TWIST (max 15 words): Reflective close — what this pattern reveals about the viewer's own behavior.`
