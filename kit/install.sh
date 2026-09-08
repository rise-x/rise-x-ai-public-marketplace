#!/usr/bin/env bash
# Installs rise-x-kit on macOS: downloads the latest kit-v* release, verifies
# its checksum, and puts the binary on PATH.
#
# Env overrides:
#   RISE_X_KIT_VERSION   install this version instead of the latest (e.g. v0.1.0)
#   RISE_X_KIT_RUN=1     run rise-x-kit once installed
set -euo pipefail

if [ "$(uname -s)" != "Darwin" ]; then
  echo "error: install.sh only supports macOS. See kit/install.ps1 for Windows." >&2
  exit 1
fi

REPO="rise-x/rise-x-ai-public-marketplace"
API="https://api.github.com/repos/${REPO}"

api_get() {
  curl -fsSL -H "Accept: application/vnd.github+json" "$1"
}

resolve_tag() {
  local tag

  if [ -n "${RISE_X_KIT_VERSION:-}" ]; then
    printf 'kit-%s\n' "$RISE_X_KIT_VERSION"
    return
  fi

  tag="$(api_get "${API}/releases/latest" 2>/dev/null \
    | grep '"tag_name"' | head -n1 | sed -E 's/.*"tag_name": *"([^"]+)".*/\1/' || true)"

  case "$tag" in
    kit-v*)
      printf '%s\n' "$tag"
      return
      ;;
  esac

  # /releases/latest didn't point at a kit release -- the repo may host other
  # release kinds too. GitHub lists releases newest-first, so the first
  # kit-v* tag we see is the newest one.
  tag="$(api_get "${API}/releases?per_page=100" \
    | grep '"tag_name"' | sed -E 's/.*"tag_name": *"([^"]+)".*/\1/' \
    | grep '^kit-v' | head -n1 || true)"

  if [ -z "$tag" ]; then
    echo "error: no kit-v* release found in ${REPO}" >&2
    exit 1
  fi
  printf '%s\n' "$tag"
}

TAG="$(resolve_tag)"
VERSION="${TAG#kit-}"
ASSET="rise-x-kit_${VERSION}_darwin_universal.tar.gz"
DOWNLOAD_BASE="https://github.com/${REPO}/releases/download/${TAG}"

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

curl -fsSL -o "$tmp/$ASSET" "$DOWNLOAD_BASE/$ASSET"
curl -fsSL -o "$tmp/checksums.txt" "$DOWNLOAD_BASE/checksums.txt"

(
  cd "$tmp"
  awk -v f="$ASSET" '$2 == f' checksums.txt > "$ASSET.sha256"
  if [ ! -s "$ASSET.sha256" ]; then
    echo "error: $ASSET is not listed in checksums.txt" >&2
    exit 1
  fi
  shasum -a 256 -c "$ASSET.sha256"
)

tar -xzf "$tmp/$ASSET" -C "$tmp"

LOCAL_BIN="$HOME/.local/bin"
if mkdir -p "$LOCAL_BIN" 2>/dev/null && [ -w "$LOCAL_BIN" ]; then
  dest_dir="$LOCAL_BIN"
elif [ -w /usr/local/bin ]; then
  dest_dir="/usr/local/bin"
else
  echo "error: neither $LOCAL_BIN nor /usr/local/bin is writable" >&2
  exit 1
fi

dest="$dest_dir/rise-x-kit"
cp "$tmp/rise-x-kit" "$dest"
chmod +x "$dest"
xattr -d com.apple.quarantine "$dest" 2>/dev/null || true

case ":$PATH:" in
  *":$dest_dir:"*) ;;
  *)
    echo
    echo "$dest_dir is not on your PATH. Add this to ~/.zshrc:"
    echo
    echo "  export PATH=\"$dest_dir:\$PATH\""
    echo
    ;;
esac

echo "Installed rise-x-kit ${VERSION}. Run: rise-x-kit"

if [ "${RISE_X_KIT_RUN:-}" = "1" ]; then
  exec "$dest"
fi
