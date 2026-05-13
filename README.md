# shorts

CLI that generates 1080×1920 YouTube Shorts (~30–45s) from a one-line idea.

```
./shorts generate "people who apologize too much"
# → output/20260514-143022/short_20260514-143022.mp4
```

Pipeline: Anthropic Haiku 4.5 (script) → ElevenLabs (voice + timestamps) → Pexels (b-roll) → librosa (beats) → ASS subtitles → FFmpeg.

## Setup

```bash
bash scripts/preflight.sh    # installs ffmpeg, fonts-montserrat, python deps
cp .env.example .env         # then fill in API keys
go build -o shorts ./cmd/shorts
```

Drop one or more royalty-free instrumental MP3s into `assets/music/`. Default expected at `assets/music/bg.mp3`.

## Usage

```
./shorts generate "<idea>"                        # full pipeline
./shorts generate "<idea>" --seed-script path     # skip script gen, useful for iterating on later steps
```

## Manual upload

`output/<ts>/short_<ts>.mp4` is the deliverable. Watch it locally, then upload via YouTube Studio. No automated upload — intentional, keep this simple.

## Cost

~$0.05 per Short: Anthropic ~$0.002, ElevenLabs ~$0.03, Pexels free.

## Layout

See the plan at `/home/alpha/.claude/plans/ai-shorts-pipeline-effervescent-whisper.md`.
