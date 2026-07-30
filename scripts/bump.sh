#!/usr/bin/env bash
# Bump a plugin's version.
#
# Usage:
#   ./scripts/bump.sh <plugin-name> <patch|minor|major>
#
# Effects:
#   plugins/<plugin-name>/.claude-plugin/plugin.json .version -> bumped at
#   the requested level. This is the ONLY place a plugin's version lives —
#   marketplace.json intentionally carries no version field.
#
# Exit codes:
#   0 - success
#   2 - usage / environment error (bad args, missing jq, unknown plugin, bad version)

set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
plugins_dir="${repo_root}/plugins"

die() {
  printf 'bump.sh: %s\n' "$*" >&2
  exit 2
}

list_plugins() {
  for d in "$plugins_dir"/*/; do
    [[ -f "${d}.claude-plugin/plugin.json" ]] && basename "$d"
  done
  return 0
}

usage() {
  {
    printf 'Usage: %s <plugin-name> <patch|minor|major>\n\n' "$0"
    printf 'Known plugins:\n'
    list_plugins | sed 's/^/  - /'
  } >&2
  exit 2
}

[[ $# -eq 2 ]] || usage
name="$1"
level="$2"
[[ "$level" =~ ^(patch|minor|major)$ ]] || usage

command -v jq >/dev/null 2>&1 || die "jq is required. Install with 'brew install jq' or 'apt-get install jq'."

plugin_file="${plugins_dir}/${name}/.claude-plugin/plugin.json"
if [[ ! -f "$plugin_file" ]]; then
  {
    printf 'bump.sh: no plugin named "%s" (missing plugins/%s/.claude-plugin/plugin.json). Known plugins:\n' "$name" "$name"
    list_plugins | sed 's/^/  - /'
  } >&2
  exit 2
fi

current="$(jq -r .version "$plugin_file" 2>/dev/null)" || die "cannot read .version from $plugin_file (invalid JSON?)"
# No leading zeros — each component is "0" or starts with 1-9, per semver.
[[ "$current" =~ ^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ ]] || die "not a valid X.Y.Z version in $plugin_file: '$current' (semver requires no leading zeros)"
major="${BASH_REMATCH[1]}" minor="${BASH_REMATCH[2]}" patch="${BASH_REMATCH[3]}"

# `10#` forces base-10 interpretation, belt-and-braces against bash's octal
# parsing of zero-prefixed numeric literals in arithmetic context.
case "$level" in
  major) next="$((10#$major + 1)).0.0" ;;
  minor) next="${major}.$((10#$minor + 1)).0" ;;
  patch) next="${major}.${minor}.$((10#$patch + 1))" ;;
esac

tmp="$(mktemp "${plugin_file}.XXXXXX")"
if ! jq --indent 2 --arg next "$next" '.version = $next' "$plugin_file" >"$tmp" 2>/dev/null; then
  rm -f "$tmp"
  die "failed to write updated version to $plugin_file (invalid JSON?)"
fi
mv "$tmp" "$plugin_file"

printf '%s: %s -> %s\n' "$name" "$current" "$next"
