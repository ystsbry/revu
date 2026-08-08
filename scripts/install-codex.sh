#!/usr/bin/env bash
#
# Install the revu plugin for the OpenAI Codex CLI.
#
# Registers this repository as a local plugin marketplace
# (.agents/plugins/marketplace.json -> ./plugin) and installs the `revu`
# plugin from it. Codex copies the plugin into ~/.codex/plugins/cache/,
# so re-run this script after updating the repo to pick up changes.
#
# Restart Codex after install. Skills are invoked as:
#
#   $revu:pr <PR_NUMBER>
#   $revu:edit <dir or instructions>
#
# Usage:
#   scripts/install-codex.sh              # marketplace add + plugin add
#   scripts/install-codex.sh --uninstall  # plugin remove + marketplace remove

set -euo pipefail

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

mode="install"
case "${1:-}" in
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

if ! command -v codex >/dev/null 2>&1; then
  echo "codex CLI not found on PATH: https://developers.openai.com/codex/cli" >&2
  exit 1
fi

if [ "$mode" = "uninstall" ]; then
  codex plugin remove revu || true
  codex plugin marketplace remove revu || true
  echo "Removed the revu plugin and marketplace. Restart codex."
  exit 0
fi

if [ ! -f "$REPO_DIR/.agents/plugins/marketplace.json" ]; then
  echo "marketplace definition not found: $REPO_DIR/.agents/plugins/marketplace.json" >&2
  exit 1
fi

codex plugin marketplace add "$REPO_DIR"
codex plugin add revu@revu

cat <<EOF

Installed the revu plugin for Codex CLI.
Restart Codex to pick it up.

Use it in codex:

  \$revu:pr <PR_NUMBER>
  \$revu:pr <PR_NUMBER> --focus security,perf
  \$revu:edit <dir or instructions>

Prerequisites:
  - gh CLI authenticated (gh auth status)
  - revu binary on \$PATH  (go install ./cmd/revu)
EOF
