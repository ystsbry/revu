#!/usr/bin/env bash
#
# Install the review-pr skill for the OpenAI Codex CLI.
#
# Codex CLI loads Agent Skills from $HOME/.agents/skills (user-scope).
# See: https://developers.openai.com/codex/skills
#
# The skill is installed by symlinking the whole skill DIRECTORY, not the
# SKILL.md file inside it. Codex's skill loader follows directory symlinks but
# silently drops symlinked SKILL.md files (see openai/codex#17344 / #15756),
# so a file-level symlink would never be discovered.
#
# Restart Codex after install to pick up the new skill.
#
# Usage:
#   scripts/install-codex.sh            # install (creates directory symlink)
#   scripts/install-codex.sh --copy     # install by copy (no symlink)
#   scripts/install-codex.sh --uninstall
#
# Env:
#   AGENTS_HOME  override the agents config dir (default: ~/.agents)

set -euo pipefail

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SRC_DIR="$REPO_DIR/skills/review-pr"
AGENTS_HOME="${AGENTS_HOME:-$HOME/.agents}"
DEST_PARENT="$AGENTS_HOME/skills"
DEST="$DEST_PARENT/review-pr"

mode="symlink"
case "${1:-}" in
  --copy)      mode="copy" ;;
  --uninstall) mode="uninstall" ;;
  "")          ;;
  -h|--help)
    sed -n '2,21p' "$0"
    exit 0
    ;;
  *)
    echo "unknown option: $1" >&2
    exit 2
    ;;
esac

if [ "$mode" = "uninstall" ]; then
  if [ -e "$DEST" ] || [ -L "$DEST" ]; then
    rm -rf "$DEST"
    echo "removed: $DEST"
  else
    echo "not installed: $DEST"
  fi
  exit 0
fi

if [ ! -f "$SRC_DIR/SKILL.md" ]; then
  echo "source SKILL.md not found: $SRC_DIR/SKILL.md" >&2
  exit 1
fi

mkdir -p "$DEST_PARENT"

if [ -e "$DEST" ] || [ -L "$DEST" ]; then
  rm -rf "$DEST"
fi

case "$mode" in
  symlink)
    ln -s "$SRC_DIR" "$DEST"
    echo "linked: $DEST -> $SRC_DIR"
    ;;
  copy)
    cp -r "$SRC_DIR" "$DEST"
    echo "copied: $SRC_DIR -> $DEST"
    ;;
esac

cat <<EOF

Installed review-pr as a Codex CLI skill.
Restart Codex to pick up the new skill.

Use it in codex:

  \$review-pr <PR_NUMBER>
  \$review-pr <PR_NUMBER> --focus security,perf

Prerequisites:
  - gh CLI authenticated (gh auth status)
  - revu binary on \$PATH  (go install ./cmd/revu)
EOF
