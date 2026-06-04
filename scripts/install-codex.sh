#!/usr/bin/env bash
#
# Install the review-pr skill as an OpenAI Codex CLI prompt.
#
# After running this script, the `/review-pr` slash command is available
# inside the `codex` CLI. The prompt body is the same SKILL.md the Claude
# Code skill uses — installed via symlink so updates to the repo are
# reflected automatically.
#
# Usage:
#   scripts/install-codex.sh            # install (creates symlink)
#   scripts/install-codex.sh --copy     # install by copy (no symlink)
#   scripts/install-codex.sh --uninstall
#
# Env:
#   CODEX_HOME   override the codex config dir (default: ~/.codex)

set -euo pipefail

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SRC="$REPO_DIR/skills/review-pr/SKILL.md"
CODEX_HOME="${CODEX_HOME:-$HOME/.codex}"
DEST_DIR="$CODEX_HOME/prompts"
DEST="$DEST_DIR/review-pr.md"

mode="symlink"
case "${1:-}" in
  --copy)      mode="copy" ;;
  --uninstall) mode="uninstall" ;;
  "")          ;;
  -h|--help)
    sed -n '2,17p' "$0"
    exit 0
    ;;
  *)
    echo "unknown option: $1" >&2
    exit 2
    ;;
esac

if [ "$mode" = "uninstall" ]; then
  if [ -e "$DEST" ] || [ -L "$DEST" ]; then
    rm -f "$DEST"
    echo "removed: $DEST"
  else
    echo "not installed: $DEST"
  fi
  exit 0
fi

if [ ! -f "$SRC" ]; then
  echo "source not found: $SRC" >&2
  exit 1
fi

mkdir -p "$DEST_DIR"

if [ -e "$DEST" ] || [ -L "$DEST" ]; then
  rm -f "$DEST"
fi

case "$mode" in
  symlink)
    ln -s "$SRC" "$DEST"
    echo "linked: $DEST -> $SRC"
    ;;
  copy)
    cp "$SRC" "$DEST"
    echo "copied: $SRC -> $DEST"
    ;;
esac

cat <<EOF

Installed review-pr as a Codex CLI prompt.

Use it in codex:

  /review-pr <PR_NUMBER>
  /review-pr <PR_NUMBER> --focus security,perf

Prerequisites:
  - gh CLI authenticated (gh auth status)
  - revu binary on \$PATH  (go install ./cmd/revu)
EOF
