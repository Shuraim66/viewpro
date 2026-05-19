# NerveLoop — Shipped Log + Next 5 Shorts Commands

Running log of every Short shipped, with parameters and performance. Followed by ready-to-run commands for the next 5.

Posting slot: 2:30 AM PKT daily.

---

## Shipped Shorts — performance log

| # | Concept | Style | Preset | Target dur | Actual dur | First clip type | 24h views | Stay% | Avg view dur | Likes | Subs | Verdict |
|---|---|---|---|---|---|---|---|---|---|---|---|---|
| 1 | Apologizing too much / hypervigilance | default | a | n/a | 31s | Hand on blue (soft) | 96 | 36.3% | 0:19 (61%) | 3 | +1 then -1 | OK baseline, soft open hurt swipe |
| 2 | Rumination / DMN | default | c | 30s | 33s | Bed/awake (decent) | 15 | 47.4% | 0:16 (49%) | 0 | 0 | Algo pulled back (post-Short-1 swipe baseline) |
| 3 | **Doorway effect** | default | c | 28s | 32s | Close-up portrait (great) | 285+ climbing | 16.7%* | 0:24 (76%) | 6 | +1 sustained | **BREAKOUT** — 94.7% Shorts feed |
| 4 | Spotlight effect | default | c | 28s | 30s | Bokeh portrait (blurry) | 172 plateau | 30% | 0:10 (34%) | 0 | 0 | Blurry intro killed retention |
| 5 | **Zeigarnik effect** | default | c | 28s | 26s | Phone in dim room (intriguing) | 500+ | TBD | TBD | TBD | TBD | **BREAKOUT** — 19h to 500 views |
| 6 | Inattentional blindness | default | c | 28s | TBD | (Likely generic phone) | 51 | 23.8% | 0:11 (37%) | 0 | 0 | Hook copy strong but visual didn't earn the stop |

\*Short #3's stay% is misleading — among the 17% who stayed, 76% completed. Niche depth beat reach width.

### Pattern visible in 6 Shorts

The variable that most correlates with view count is **first-frame visual quality**, not concept or hook copy:
- Face / intriguing first frame → 200-500+ views
- Generic / blurry / weak first frame → 50-170 views

Caption preset and script style have shown **no clear correlation** with retention so far — but every Short used `default + c` after Short #1, so this isn't isolated data.

### What's empirically proven

- ✅ `default` script style works (5 of 6 Shorts; range 15-500+)
- ✅ `caption-preset c` works (5 of 6 Shorts; range 15-500+)
- ✅ `28s target duration` works (4 of 6 Shorts; range 51-500+)
- ✅ Concept selection > parameter tuning
- ✅ First-frame visual quality is the single biggest retention lever
- ✅ Hook overlay format `[YOU/YOUR] [PRESENT-TENSE STATE] [INTRIGUE]` works

### What's still unknown (need data on)

- ⚠️ Whether `question`, `list`, or `story` script styles outperform `default`
- ⚠️ Whether `preset a` or `preset b` change retention vs `preset c`
- ⚠️ Whether 30s or 35s target durations affect completion curves
- ⚠️ Whether hook overlays affect retention when the rest is held constant

---

## Strategy shift starting Short #7: variation rotation

The variable-isolation strategy got us to a proven baseline (default/c/28). Continuing to pin these parameters now does two harmful things:

1. **Visual templating risk** — 6 identical-look Shorts is approaching YouTube Inauthentic Content Policy fingerprint
2. **Learning ceiling** — can't determine if other styles/presets perform better without testing them

From Short #7 onward, every Short varies ONE parameter from the baseline. Every 4-5 Shorts, return to baseline (default/c/28) as an anchor for channel-confidence and learning continuity.

---

## Workflow per Short

```bash
# 1. Dry-run first to check the script (~$0.002, no video)
go run ./cmd/shorts generate "<idea>" --script-style X --caption-preset Y --target-dur Z --dry-run

# 2. If script looks good, run full pipeline (~$0.04)
go run ./cmd/shorts generate "<idea>" --script-style X --caption-preset Y --target-dur Z

# 3. If script feels off, regenerate dry-run (temperature 0.8 gives variation)
```

### Mandatory pre-full-pipeline edit

After dry-run, manually edit `script.json` to inject a face-first B-roll keyword as the FIRST entry:

```json
"broll_keywords": [
    "close up portrait face talking to camera",
    "[Claude's original keyword 1]",
    "[Claude's original keyword 2]",
    "[Claude's original keyword 3]"
]
```

Then run full pipeline with `--seed-script` pointing to the edited file:

```bash
go run ./cmd/shorts generate "<concept>" \
  --seed-script /home/alpha/products/viewpro/output/<session>/script.json \
  --caption-preset Y --target-dur Z
```

This forces a face/portrait opening clip and is the single highest-leverage retention fix based on Shorts #3 and #5 data.

---

## Short #7 — Affect labeling (TEST: question style)

```bash
go run ./cmd/shorts generate "saying I'm anxious out loud actually quiets your amygdala — there's a specific neurological reason your therapist makes you do this" \
  --script-style question \
  --caption-preset c \
  --target-dur 28
```

**Variable being tested:** Script style (default → **question**). Caption preset and duration held at proven baseline.

**Why:** Universal recognition (everyone's been told to "name your feelings") + concrete brain mechanism (amygdala calming) + Lieberman's real fMRI research. Pairs with *Permission to Feel* (Brackett) for affiliate later.

**Hook overlay seed:** SAY IT OUT LOUD. WATCH.

**Dry-run check:** Body must contain a specific brain region (amygdala, prefrontal cortex), a concrete action (say it out loud), and a verifiable researcher name (Matt Lieberman or "UCLA research"). If body is abstract ("your brain processes emotions"), regenerate.

---

## Short #8 — Mirror neurons (TEST: caption-preset b)

```bash
go run ./cmd/shorts generate "when you watch someone flinch you flinch a little too — your brain fires as if it happened to you" \
  --script-style default \
  --caption-preset b \
  --target-dur 28
```

**Variable being tested:** Caption preset (c → **b**). Style and duration held at proven baseline.

**Why:** Universal recognition (sports injury videos, accident clips). Caveat: mirror neuron research has replication concerns. On dry-run, verify script says "studies suggest" / "fMRI work shows" rather than "studies prove."

**Hook overlay seed:** YOUR BRAIN COPIES THEM

---

## Short #9 — Self-perception theory (TEST: story style + 35s duration)

```bash
go run ./cmd/shorts generate "you don't smile because you're happy — you're partly happy because you smiled, and the order being backwards explains a lot" \
  --script-style story \
  --caption-preset c \
  --target-dur 35
```

**Variables being tested:** Style (default → **story**) AND duration (28s → **35s**). Caption preset held.

**Why:** Story style suits Bem's pencil-in-teeth experiment which works as a mini-narrative. The extra 7s gives Claude room to walk through the experiment concretely.

**Note:** This is the only multi-variable Short in the next batch. If it performs differently from baseline, isolation is muddier — but the experiment is a natural fit for both variables changing.

**Hook overlay seed:** THE ORDER IS BACKWARDS

---

## Short #10 — Anchoring effect (BASELINE ANCHOR: default/c/28)

```bash
go run ./cmd/shorts generate "the first number you see in any negotiation decides what you'll accept — even when the number is obviously made up" \
  --script-style default \
  --caption-preset c \
  --target-dur 28
```

**Variable being tested:** None — return to proven baseline. Every 4-5 Shorts, anchor back to known-good to maintain channel-level confidence signal.

**Why:** Anchoring is one of the most universal cognitive biases. Concrete (any number example works), filmable (people at desks, price tags, salary negotiations).

**Hook overlay seed:** THE FIRST NUMBER DECIDES

---

## Short #11 — Cocktail party effect (TEST: caption-preset a)

```bash
go run ./cmd/shorts generate "your brain hears your name across a crowded room even when you swore you weren't listening — here's what your brain was actually doing" \
  --script-style default \
  --caption-preset a \
  --target-dur 28
```

**Variable being tested:** Caption preset (c → **a**, the original style from Short #1). Style and duration held.

**Why:** Universal experience, concrete behavior (hearing your name), named mechanism (preconscious filtering), filmable (crowd scenes, conversations).

**Hook overlay seed:** YOUR BRAIN HEARD IT FIRST

---

## Dry-run quality checks

Before approving any script, scan for:

- **Fabricated people** — "my friend Sarah," "a patient I knew." Regenerate if found.
- **Invented statistics** — "research shows 5%" or "67% of people." Regenerate if found. Pipeline regex check should catch but verify manually.
- **Overclaiming science** — "studies prove" → bad. "Studies suggest" / "research finds" → good.
- **Abstract body** — body should reference specific behaviors, brain regions, observable actions. If it reads like a textbook definition, regenerate.
- **Vague broll_keywords** — "feeling anxious" → bad. "Person sitting alone at desk" → good.
- **Hook overlay text length** — 4-6 words, all caps. Regenerate if 8+ words.
- **Duration overshoot** — if `actual_duration_sec` >25% over `target_duration_sec`, consider regenerating with stricter target.

### Mandatory MP4 review (after full pipeline)

- First frame is sharp (no bokeh, no fade-in, no soft focus)
- First frame is a face or visually arresting content
- No content-safety issues (bare skin, personal data on screens, controversial text like "omegle")
- Captions clear of YouTube UI overlay zones
- 12+ unique clips per Short (audit.json's broll_review count)

---

## Posting cadence

One per day at 2:30 AM PKT.

- Don't multi-upload — splits algorithmic test pool
- Don't skip days — kills consistency signal
- Batch-generate 2-3 on a weekend if needed, schedule across week
- Reply to comments on past Shorts (especially Shorts #3 and #5) within an hour when they appear

---

## After Short #11: data analysis checkpoint

By Short #11 you'll have:
- 3 data points on `script-style` (default × 5+, question × 1, story × 1)
- 3 data points on `caption-preset` (c × 5+, b × 1, a × 2 including Short #1)
- 2 duration points (28s × 5+, 35s × 1)

Aggregate the data in a spreadsheet:

| Variable | Variant | Shorts that used it | Avg views | Avg completion |
|---|---|---|---|---|
| Style | default | 1, 2, 3, 4, 5, 6, 8, 10 | calculate | calculate |
| Style | question | 7 | calculate | calculate |
| Style | story | 9 | calculate | calculate |
| Preset | a | 1, 11 | calculate | calculate |
| Preset | b | 8 | calculate | calculate |
| Preset | c | 2-7, 9, 10 | calculate | calculate |

Then bias future Shorts (#12+) toward the highest-retention combinations. Continue rotation at ~30% of uploads to maintain editorial variation.

---

## Idea backlog (Shorts #12-16, pre-staged)

Use after Short #11 with data-informed parameter selection.

| # | Concept | One-line angle | Pairs with affiliate |
|---|---|---|---|
| 12 | Hedonic adaptation | The thing you want most — you'll be neutral about 6 weeks after getting it | The Psychology of Money |
| 13 | Loss aversion | Losing $20 hurts twice as much as finding $20 feels good. That asymmetry runs your decisions. | Thinking Fast and Slow |
| 14 | Co-rumination | Venting to a friend can make you feel worse, not better. Same brain regions, deepened. | The Body Keeps the Score |
| 15 | Endowment effect | The moment something becomes yours, you start valuing it more — that's why returning anything feels weirdly hard | Predictably Irrational |
| 16 | Reciprocity hardwiring | If someone does you an unrequested favor, you're now psychologically in debt — that's why salespeople give you "free" things | Influence (Cialdini) |

---

## Pipeline TODO (deferred, non-blocking)

1. Laplacian sharpness detection on opening frames
2. **Face-first ranking for segment 0** (manual injection is current workaround)
3. Inter-Short clip cache (avoid using same clip across multiple Shorts)
4. Screen-recording deprioritization (Pexels phone clips with personal data)
5. SQLite tracking for cross-Short metrics
6. Inter-Short variant rotation enforcement (warn if same preset used 3+ times consecutively)

---

*File maintained for NerveLoop. Update shipped Shorts log after each upload. Update strategy section after Short #11 data analysis.*