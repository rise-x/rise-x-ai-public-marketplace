#!/usr/bin/env bash
# Vendors the Rise-X design system into kit/ so the app ships without Node.
# Usage: kit/scripts/sync-ui.sh [version]   (default: latest)
set -euo pipefail

VERSION="${1:-latest}"
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

echo "Packing $SPEC from $REGISTRY ..."
TARBALL="$(cd "$WORK_DIR" && npm pack "$SPEC" \
  --@rise-x:registry="$REGISTRY" \
  --prefer-online \
  --loglevel=error)"
tar -xzf "$WORK_DIR/$TARBALL" -C "$WORK_DIR"

SRC="$WORK_DIR/package/build/ui"
RESOLVED="$(node -p "require('$WORK_DIR/package/package.json').version")"

for f in styles.css demo.html; do
  test -f "$SRC/$f" || { echo "error: $f missing from $SPEC (resolved $RESOLVED)" >&2; exit 1; }
done

mkdir -p "$(dirname "$CSS_OUT")" "$(dirname "$DEMO_OUT")"

{
  echo "/* vendored from @rise-x/apps-sdk@${RESOLVED} build/ui/styles.css — refresh with kit/scripts/sync-ui.sh */"
  cat "$SRC/styles.css"
} > "$CSS_OUT"

cp "$SRC/demo.html" "$DEMO_OUT"

echo "apps-sdk        ${RESOLVED}"
echo "rise-x-ui.css   $(wc -c < "$CSS_OUT" | tr -d ' ') bytes  -> ${CSS_OUT#"$KIT_DIR/"}"
echo "demo.html       $(wc -c < "$DEMO_OUT" | tr -d ' ') bytes  -> ${DEMO_OUT#"$KIT_DIR/"}"
