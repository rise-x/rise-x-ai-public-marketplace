#!/usr/bin/env bash
# Vendors the Rise-X design system into kit/ so the app ships without Node.
# Usage:
#   kit/scripts/sync-ui.sh [version]   refresh the vendored copy (default: $APPS_SDK_VERSION)
#   kit/scripts/sync-ui.sh --verify    re-download the pinned tarball and check its sha256 still matches
set -euo pipefail

# Pinned so an unreviewed @rise-x/apps-sdk publish can't silently change what
# ships in the app -- bump this deliberately, in its own commit.
APPS_SDK_VERSION="0.11.1"

VERIFY=0
if [[ "${1:-}" == "--verify" ]]; then
  VERIFY=1
  shift
fi

VERSION="${1:-$APPS_SDK_VERSION}"
SPEC="@rise-x/apps-sdk@${VERSION}"

# The @rise-x scope is often redirected to a private feed in a developer's
# ~/.npmrc; the design-system tarball only ships to public npm.
REGISTRY="https://registry.npmjs.org"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
KIT_DIR="$(dirname "$SCRIPT_DIR")"
CSS_OUT="$KIT_DIR/internal/web/rise-x-ui.css"
DEMO_OUT="$KIT_DIR/design/reference/demo.html"

WORK_DIR="$(mktemp -d)"
trap 'rm -rf "$WORK_DIR"' EXIT

if [[ "$VERIFY" -eq 1 ]]; then
  recorded_version="$(sed -n '1s/.*apps-sdk@\([^ ]*\).*/\1/p' "$CSS_OUT")"
  recorded_sha="$(sed -n '1s/.*sha256:\([0-9a-f]*\).*/\1/p' "$CSS_OUT")"
  if [[ -z "$recorded_version" || -z "$recorded_sha" ]]; then
    echo "error: $CSS_OUT has no recorded version/sha256 to verify against" >&2
    exit 1
  fi

  echo "Verifying @rise-x/apps-sdk@${recorded_version} against $REGISTRY ..."
  tarball_url="$(npm view "@rise-x/apps-sdk@${recorded_version}" dist.tarball \
    --@rise-x:registry="$REGISTRY" --loglevel=error)"
  actual_sha="$(curl -fsSL "$tarball_url" | shasum -a 256 | cut -d' ' -f1)"

  if [[ "$actual_sha" != "$recorded_sha" ]]; then
    echo "error: @rise-x/apps-sdk@${recorded_version} tarball sha256 changed" >&2
    echo "  recorded: $recorded_sha" >&2
    echo "  actual:   $actual_sha" >&2
    exit 1
  fi
  echo "OK: sha256 matches ${CSS_OUT#"$KIT_DIR/"}"
  exit 0
fi

echo "Resolving $SPEC from $REGISTRY ..."
TARBALL_URL="$(npm view "$SPEC" dist.tarball --@rise-x:registry="$REGISTRY" --loglevel=error)"
TARBALL="$WORK_DIR/package.tgz"
curl -fsSL -o "$TARBALL" "$TARBALL_URL"
SHA256="$(shasum -a 256 "$TARBALL" | cut -d' ' -f1)"
tar -xzf "$TARBALL" -C "$WORK_DIR"

SRC="$WORK_DIR/package/build/ui"
RESOLVED="$(node -p "require('$WORK_DIR/package/package.json').version")"

for f in styles.css demo.html; do
  test -f "$SRC/$f" || { echo "error: $f missing from $SPEC (resolved $RESOLVED)" >&2; exit 1; }
done

mkdir -p "$(dirname "$CSS_OUT")" "$(dirname "$DEMO_OUT")"

{
  echo "/* vendored from @rise-x/apps-sdk@${RESOLVED} (sha256:${SHA256}) build/ui/styles.css — refresh with kit/scripts/sync-ui.sh */"
  cat "$SRC/styles.css"
} > "$CSS_OUT"

cp "$SRC/demo.html" "$DEMO_OUT"

echo "apps-sdk        ${RESOLVED}  (sha256:${SHA256})"
echo "rise-x-ui.css   $(wc -c < "$CSS_OUT" | tr -d ' ') bytes  -> ${CSS_OUT#"$KIT_DIR/"}"
echo "demo.html       $(wc -c < "$DEMO_OUT" | tr -d ' ') bytes  -> ${DEMO_OUT#"$KIT_DIR/"}"
