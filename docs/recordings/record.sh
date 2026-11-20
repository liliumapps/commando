#!/usr/bin/env bash
# Regenerate every docs GIF from scratch, reproducibly.
#
# What it does:
#   1. builds a fresh commando binary from the current source
#   2. builds an isolated sandbox (see build-sandbox.sh) — real config/history
#      are never touched; all sample data is synthetic
#   3. renders each .tape into docs/assets/gifs/*.gif with VHS
#
# Requirements: go, vhs (brew install vhs), ffmpeg.
# Usage:  bash docs/recordings/record.sh [tape-name ...]
#         (no args = render all tapes)
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$REPO_ROOT"

command -v vhs >/dev/null || { echo "vhs not found — brew install vhs"; exit 1; }
command -v go  >/dev/null || { echo "go not found"; exit 1; }

echo "==> Building commando"
go build -o commando .

# Render the requested tapes, or all of them.
if [ "$#" -gt 0 ]; then
  tapes=()
  for name in "$@"; do tapes+=("docs/recordings/tapes/${name%.tape}.tape"); done
else
  tapes=(docs/recordings/tapes/*.tape)
fi

# Rebuild the sandbox before EACH tape so ranking (usage counts) and memory
# start from a known-clean state — recordings stay deterministic run to run.
for tape in "${tapes[@]}"; do
  echo "==> Building sandbox"
  bash docs/recordings/build-sandbox.sh >/dev/null
  out="$(awk '$1 == "Output" { print $2; exit }' "$tape")"
  if [ -n "$out" ]; then
    rm -f "$out"
  fi
  echo "==> Recording $tape"
  vhs "$tape"
done

echo "==> Done. GIFs in docs/assets/gifs/"
ls -lh docs/assets/gifs/
