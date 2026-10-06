#!/usr/bin/env bash
#
# Sets up the local toolchain scribin depends on: clones and builds
# whisper.cpp, downloads the ggml "small" model, checks ffmpeg, and
# runs a test transcription to confirm the whole chain works end to end.
#
# Safe to re-run: every step is idempotent (skips work that is already done).

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

WHISPER_DIR="$ROOT_DIR/third_party/whisper.cpp"
WHISPER_REPO="https://github.com/ggerganov/whisper.cpp"
MODELS_DIR="$ROOT_DIR/models"
MODEL_NAME="small"
MODEL_FILE="$MODELS_DIR/ggml-${MODEL_NAME}.bin"
WHISPER_BIN="$WHISPER_DIR/build/bin/whisper-cli"
SAMPLE_WAV="$WHISPER_DIR/samples/jfk.wav"

log()  { printf '[setup] %s\n' "$1"; }
fail() { printf '[setup] ERROR: %s\n' "$1" >&2; exit 1; }

# 1. Clone or update whisper.cpp.
if [ -d "$WHISPER_DIR/.git" ]; then
    log "whisper.cpp already present at $WHISPER_DIR, updating..."
    git -C "$WHISPER_DIR" pull --ff-only || fail "could not update whisper.cpp (check for local changes in $WHISPER_DIR)"
else
    log "cloning whisper.cpp into $WHISPER_DIR..."
    mkdir -p "$(dirname "$WHISPER_DIR")"
    git clone "$WHISPER_REPO" "$WHISPER_DIR" || fail "could not clone $WHISPER_REPO"
fi

# 2. Build whisper.cpp with cmake (skip if the binary is already built).
if [ -x "$WHISPER_BIN" ] || [ -x "${WHISPER_BIN}.exe" ]; then
    log "whisper-cli already built at $WHISPER_BIN, skipping build."
else
    command -v cmake >/dev/null 2>&1 || fail "cmake not found in PATH. Install cmake and a C/C++ compiler (gcc or clang) before running this script."
    log "building whisper.cpp (this can take a few minutes)..."
    cmake -B "$WHISPER_DIR/build" -S "$WHISPER_DIR" || fail "cmake configure step failed"
    cmake --build "$WHISPER_DIR/build" -j --config Release || fail "cmake build step failed"
    if [ ! -x "$WHISPER_BIN" ] && [ ! -x "${WHISPER_BIN}.exe" ]; then
        fail "build finished but whisper-cli was not found at $WHISPER_BIN"
    fi
fi

# Resolve the actual binary path (plain or .exe, depending on platform).
if [ -x "$WHISPER_BIN" ]; then
    WHISPER_BIN_RESOLVED="$WHISPER_BIN"
else
    WHISPER_BIN_RESOLVED="${WHISPER_BIN}.exe"
fi

# 3. Download the ggml "small" model (skip if already present).
mkdir -p "$MODELS_DIR"
if [ -f "$MODEL_FILE" ]; then
    log "model $MODEL_NAME already present at $MODEL_FILE, skipping download."
else
    DOWNLOAD_SCRIPT="$WHISPER_DIR/models/download-ggml-model.sh"
    [ -f "$DOWNLOAD_SCRIPT" ] || fail "download-ggml-model.sh not found in $WHISPER_DIR/models"
    # Normalize line endings: a git checkout on Windows (core.autocrlf=true) can
    # leave this shell script with CRLF endings, which breaks bash under WSL/Linux.
    sed -i 's/\r$//' "$DOWNLOAD_SCRIPT"
    log "downloading ggml model '$MODEL_NAME'..."
    (cd "$WHISPER_DIR/models" && bash download-ggml-model.sh "$MODEL_NAME") || fail "failed to download model '$MODEL_NAME'"
    DOWNLOADED_FILE="$WHISPER_DIR/models/ggml-${MODEL_NAME}.bin"
    [ -f "$DOWNLOADED_FILE" ] || fail "download script ran but $DOWNLOADED_FILE was not created"
    mv "$DOWNLOADED_FILE" "$MODEL_FILE"
fi

# 4. Verify ffmpeg is installed.
if ! command -v ffmpeg >/dev/null 2>&1; then
    cat >&2 <<'EOF'
[setup] ERROR: ffmpeg not found in PATH.

Install it with:
  Linux (Debian/Ubuntu): sudo apt-get update && sudo apt-get install -y ffmpeg
  macOS (Homebrew):      brew install ffmpeg

Then re-run this script.
EOF
    exit 1
fi
log "ffmpeg found: $(ffmpeg -version | head -n 1)"

# 5. Run a test transcription to confirm the whole chain works.
[ -f "$SAMPLE_WAV" ] || fail "sample audio not found at $SAMPLE_WAV"
log "running test transcription on $(basename "$SAMPLE_WAV")..."
"$WHISPER_BIN_RESOLVED" -m "$MODEL_FILE" -f "$SAMPLE_WAV" || fail "test transcription failed"

log "setup complete: ffmpeg + whisper.cpp chain is working."
