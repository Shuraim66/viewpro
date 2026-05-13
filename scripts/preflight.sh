#!/usr/bin/env bash
# One-time setup: system packages, Python venv, font cache.
# Run with `bash scripts/preflight.sh` from the project root.
# Requires sudo for apt-get.

set -euo pipefail

cd "$(dirname "$0")/.."

echo ">>> System packages (apt)…"
# apt-get update may fail on unrelated 3rd-party repos (Modular, Cloudflare, etc.)
# We only need Ubuntu archive packages, which work even if other repos error.
sudo apt-get update || echo "WARNING: apt-get update had errors (likely 3rd-party repos); continuing"
sudo apt-get install -y ffmpeg fonts-montserrat python3-pip python3-venv

echo ">>> Python venv (.venv)…"
if [ ! -d .venv ]; then
  python3 -m venv .venv
fi
.venv/bin/pip install --upgrade pip
# numpy<2.0 pinned: librosa's numba dep has an ABI conflict with numpy 2.x
.venv/bin/pip install "numpy<2.0" librosa==0.10.2 soundfile==0.12.1

echo ">>> Font cache (fc-cache)…"
fc-cache -f

echo ">>> Pre-warm librosa (avoids 30s numba JIT on first real run)…"
.venv/bin/python3 -c "import numpy, librosa; librosa.onset.onset_detect(y=numpy.zeros(22050), sr=22050)" >/dev/null 2>&1 || true

echo ">>> Verify…"
ffmpeg -version | head -1
.venv/bin/python3 -c "import librosa; print('librosa', librosa.__version__)"
fc-list | grep -i montserrat | head -3 || echo "WARNING: no Montserrat fonts found"

# Copy Montserrat ExtraBold into assets/fonts/ as a libass fallback
MONTSERRAT_TTF=$(fc-list | grep -i "montserrat.*extrabold\|montserrat-extrabold" | head -1 | cut -d: -f1)
if [ -n "${MONTSERRAT_TTF:-}" ] && [ -f "$MONTSERRAT_TTF" ]; then
  cp "$MONTSERRAT_TTF" assets/fonts/Montserrat-ExtraBold.ttf
  echo "Copied $MONTSERRAT_TTF → assets/fonts/"
else
  echo "WARNING: ExtraBold weight not found in fonts-montserrat. Download manually from Google Fonts → assets/fonts/Montserrat-ExtraBold.ttf"
fi

echo ""
echo "Pre-flight complete. Next: copy .env.example → .env and fill in API keys."
