#!/usr/bin/env sh
set -eu

REPO="ystsbry/revu"
BIN_NAME="revu"
INSTALL_DIR="${REVU_INSTALL_DIR:-${HOME}/.local/bin}"
VERSION="${VERSION:-latest}"

err() { echo "error: $*" >&2; exit 1; }
warn() { echo "warning: $*" >&2; }

# --- Detect OS / arch ---------------------------------------------------------
os="$(uname -s)"
case "$os" in
  Linux)  os="linux" ;;
  Darwin) os="darwin" ;;
  *) err "unsupported OS: $os" ;;
esac

arch="$(uname -m)"
case "$arch" in
  x86_64|amd64)   arch="amd64" ;;
  arm64|aarch64)  arch="arm64" ;;
  *) err "unsupported arch: $arch" ;;
esac

# --- Resolve version ----------------------------------------------------------
if [ "$VERSION" = "latest" ]; then
  VERSION="$(
    curl -sSfL "https://api.github.com/repos/${REPO}/releases/latest" \
    | grep '"tag_name"' \
    | head -n1 \
    | sed -E 's/.*"tag_name"[[:space:]]*:[[:space:]]*"([^"]+)".*/\1/'
  )"
  [ -n "$VERSION" ] || err "could not determine latest version"
fi

ver_num="${VERSION#v}"
archive="revu_${ver_num}_${os}_${arch}.tar.gz"
base="https://github.com/${REPO}/releases/download/${VERSION}"

# --- Download -----------------------------------------------------------------
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

echo "Downloading ${base}/${archive}"
curl -sSfL "${base}/${archive}"     -o "${tmp}/${archive}" \
  || err "download failed: ${base}/${archive}"

# --- Verify checksum ----------------------------------------------------------
echo "Verifying checksum"
if curl -sSfL "${base}/checksums.txt" -o "${tmp}/checksums.txt"; then
  expected="$(grep "  ${archive}\$" "${tmp}/checksums.txt" | awk '{print $1}')"
  [ -n "$expected" ] || err "checksum entry not found for ${archive}"

  if command -v sha256sum >/dev/null 2>&1; then
    actual="$(sha256sum "${tmp}/${archive}" | awk '{print $1}')"
  elif command -v shasum >/dev/null 2>&1; then
    actual="$(shasum -a 256 "${tmp}/${archive}" | awk '{print $1}')"
  else
    warn "no sha256 tool available; skipping verification"
    actual="$expected"
  fi

  [ "$expected" = "$actual" ] || err "checksum mismatch (expected $expected, got $actual)"
else
  warn "checksums.txt not found; skipping verification"
fi

# --- Install ------------------------------------------------------------------
tar -xzf "${tmp}/${archive}" -C "$tmp"
[ -f "${tmp}/${BIN_NAME}" ] || err "binary not found in archive"

mkdir -p "$INSTALL_DIR"
mv "${tmp}/${BIN_NAME}" "${INSTALL_DIR}/${BIN_NAME}"
chmod +x "${INSTALL_DIR}/${BIN_NAME}"

echo "Installed ${BIN_NAME} ${VERSION} to ${INSTALL_DIR}/${BIN_NAME}"

case ":${PATH}:" in
  *":${INSTALL_DIR}:"*) ;;
  *) warn "${INSTALL_DIR} is not in your PATH; add it to use '${BIN_NAME}'" ;;
esac
