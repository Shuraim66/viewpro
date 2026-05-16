# NerveLoop — Next 5 Shorts Commands

Ready-to-run pipeline commands for Shorts #4–8. Variants are pinned where there's a reason (style fit, caption rotation, duration tuning); everything else rolls random per run.

Run order: one per day, 2:30 AM PKT schedule slot.

---

## Workflow per Short

```bash
# 1. Dry-run first to check the script (~$0.002, no video)
go run ./cmd/shorts generate "<idea>" --script-style X --caption-preset Y --target-dur Z --dry-run

# 2. If script looks good, run full pipeline (~$0.04)
go run ./cmd/shorts generate "<idea>" --script-style X --caption-preset Y --target-dur Z

# 3. If script feels off, regenerate dry-run (temperature 0.8 gives variation)
go run ./cmd/shorts generate "<idea>" --script-style X --caption-preset Y --target-dur Z --dry-run
```

---

## Short #4 — Doorway effect

```bash
go run ./cmd/shorts generate "you walked into a room and forgot why you came in — your brain literally just deleted the goal" \
  --script-style default \
  --caption-preset c \
  --target-dur 28
```

**Why these settings:** Universal "everyone has done this" recognition trigger. Default hook→body→twist fits because the phenomenon needs one named explanation (event boundaries) and a clean twist. Preset C is your current best caption look (Short #3 winner). Shorter target (28s) — concept doesn't need padding.

**Concept name:** Event boundary memory purge / "doorway effect"

**Hook overlay seed:** WALKED IN, FORGOT WHY?

---

## Short #5 — Spotlight effect

```bash
go run ./cmd/shorts generate "you think people noticed that embarrassing thing you did — they didn't, and the research is brutal" \
  --script-style question \
  --caption-preset b \
  --target-dur 32
```

**Why these settings:** First use of `question` style — opens with "Why do you assume…?" which is a different scroll-stop pattern than previous Shorts. Preset B introduces variation to caption library (used A on Short #1, C on Shorts #2–3). Target 32s gives room to cite Gilovich (15% noticed vs 50% predicted).

**Concept name:** Spotlight effect

**Hook overlay seed:** NOBODY NOTICED. SERIOUSLY.

---

## Short #6 — Affect labeling

```bash
go run ./cmd/shorts generate "saying I'm anxious out loud literally calms your brain — there's a specific neurological reason your therapist makes you do this" \
  --script-style default \
  --caption-preset a \
  --target-dur 30
```

**Why these settings:** High-recognition concept (everyone's been told "name your feelings") with a satisfying neurological payoff (Lieberman's fMRI work on amygdala reduction). Preset A — your original style, viewers who've watched all Shorts get visual variety. Default duration. Pairs with *Permission to Feel* (Brackett) when affiliate is live.

**Concept name:** Affect labeling

**Hook overlay seed:** SAY IT OUT LOUD. WATCH.

---

## Short #7 — Self-perception theory (smile)

```bash
go run ./cmd/shorts generate "you don't smile because you're happy — you're partly happy because you smiled, and the order being backwards explains a lot" \
  --script-style story \
  --caption-preset c \
  --target-dur 35
```

**Why these settings:** First use of `story` style — gives Claude room to walk through Bem's pencil-in-teeth experiment as a mini-narrative. Concrete experiment lands harder than abstract explanation, so 35s gives room. Preset C performed cleanest on Short #3.

**Concept name:** Self-perception theory

**Hook overlay seed:** THE ORDER IS BACKWARDS

---

## Short #8 — Mirror neurons / emotional contagion

```bash
go run ./cmd/shorts generate "when you watch someone flinch you flinch a little too — your brain is firing as if it happened to you, and it has consequences" \
  --script-style default \
  --caption-preset b \
  --target-dur 30
```

**Why these settings:** Universal recognition trigger (sports injury videos, accident clips). Default structure handles "phenomenon → mechanism → implication" cleanly. Preset B for caption rotation.

**Caveat:** Mirror neuron research has replication concerns. On dry-run, check that the script says "studies suggest" / "fMRI work shows" rather than "studies prove." Regenerate if Claude overclaims.

**Concept name:** Mirror neuron / emotional contagion

**Hook overlay seed:** YOUR BRAIN COPIES THEM

---

## Dry-run quality checks

Before approving any script, scan for:

- **Fabricated people** — "my friend Sarah," "a patient I knew." If you see any made-up person, regenerate. The Short #2 fix should prevent this but stay vigilant.
- **Overclaiming science** — "studies prove" → bad. "Studies suggest" / "research finds" → good. Matters for credibility on replication-shaky concepts.
- **Vague broll_keywords** — "feeling anxious" → bad, generic Pexels matches. "Person sitting alone at table" → good, filmable.
- **Hook overlay text length** — should be 4–6 words, all caps. If Claude returns 8+ words, regenerate.
- **Duration target overshoot** — audit.json shows `target_duration_sec` vs `actual_duration_sec`. >25% over = consider using `--strict-duration` flag to force regenerate.

---

## Posting cadence

One per day at 2:30 AM PKT. Don't burn through these in 3 days — the daily slot is what trains the algorithm. Batch-generate 2–3 on a weekend, schedule them across the week.

---

## After Short #8: review what's working

By Short #8 you'll have ~7 days of data. In Studio → Analytics → Reach for each Short, find:

- Which `--script-style` got highest retention?
- Which `--caption-preset` got highest channel-page click-through (subscribe signal)?
- Which `--target-dur` had cleanest swipe-away curve?

Then bias future Shorts toward what's working. Aim for ~70% on the winning combination, ~30% on experiments. Variation isn't permanent — it's how you find the formula.

---

## Idea backlog (Shorts #9–13, pre-staged)

Don't burn these yet. Use after Short #8 once you have retention data.

| # | Concept | One-line angle |
|---|---|---|
| 9 | Zeigarnik effect | The text you didn't reply to is using more memory than the ones you did |
| 10 | Hedonic adaptation | The thing you want most — you'll be neutral about 6 weeks after getting it |
| 11 | Loss aversion | Losing $20 hurts twice as much as finding $20 feels good. That asymmetry runs your decisions. |
| 12 | Cocktail party effect | Your brain hears your name across a noisy room — even though you weren't listening |
| 13 | Co-rumination | Venting to a friend can make you feel worse, not better. Same brain regions, deepened. |

---

*File maintained for NerveLoop. Update with retention data after Short #8.*
