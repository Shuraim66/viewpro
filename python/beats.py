#!/usr/bin/env python3
"""
Single-purpose: read an audio file, print onset times + duration as JSON.

Called by internal/beats/beats.go via:
    .venv/bin/python3 python/beats.py /abs/path/voice.mp3
"""
import json
import sys

import librosa


def main() -> None:
    if len(sys.argv) != 2:
        print("usage: beats.py <audio-file>", file=sys.stderr)
        sys.exit(2)

    y, sr = librosa.load(sys.argv[1], sr=22050, mono=True)
    onsets = librosa.onset.onset_detect(
        y=y,
        sr=sr,
        units="time",
        delta=0.07,        # threshold; lower => more onsets
        backtrack=False,
    )
    print(json.dumps({
        "duration": float(librosa.get_duration(y=y, sr=sr)),
        "onsets":   [float(t) for t in onsets],
    }))


if __name__ == "__main__":
    main()
