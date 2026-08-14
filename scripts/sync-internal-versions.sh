#!/usr/bin/env bash
# Sync the internal marketplace's externalized (git-subdir) entry versions to the
# plugin.json versions in this repo, and patch-bump its metadata.version when
# anything changed. Edits the internal checkout's marketplace.json in place.
#
# Usage:
#   ./scripts/sync-internal-versions.sh <internal-repo-checkout>
#
# Output: one line per updated entry ("<name>: <old> -> <new>") on stdout.
# No output means no drift. Used by .github/workflows/sync-internal-marketplace.yml.
#
# Exit codes:
#   0 - success (with or without changes)
#   2 - usage / environment error

set -euo pipefail

die() {
  printf 'sync-internal-versions.sh: %s\n' "$*" >&2
  exit 2
}

[[ $# -eq 1 ]] || die "usage: $0 <internal-repo-checkout>"
internal_root="$1"
public_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
marketplace_file="${internal_root}/.claude-plugin/marketplace.json"

command -v jq >/dev/null 2>&1 || die "jq is required"
[[ -f "$marketplace_file" ]] || die "marketplace.json not found at $marketplace_file"
jq -e . "$marketplace_file" >/dev/null 2>&1 || die "marketplace.json is not valid JSON at $marketplace_file"

is_semver() {
  [[ "$1" =~ ^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ ]]
}

write_json() {
  # Usage: write_json <file> [jq args...] <filter>
  local file="$1"
  shift
  local tmp
  tmp="$(mktemp "${file}.XXXXXX")"
  jq --indent 2 "$@" "$file" >"$tmp"
  mv "$tmp" "$file"
}

changed=0
matched=0

# Entries whose git-subdir source points at this repo, as "name<TAB>path<TAB>version".
# The url test covers both https and SSH forms.
while IFS=$'\t' read -r name path entry_version; do
  [[ -z "$name" ]] && continue
  matched=$((matched + 1))
  plugin_file="${public_root}/${path}/.claude-plugin/plugin.json"
  if [[ ! -f "$plugin_file" ]]; then
    # Stale entry (plugin gone from this repo): skip it so the others still
    # sync; stdout stays clean for the caller's drift summary.
    printf 'sync-internal-versions.sh: warning: entry '\''%s'\'' references '\''%s'\'' but %s does not exist — skipping\n' \
      "$name" "$path" "$plugin_file" >&2
    continue
  fi
  public_version="$(jq -r '.version // empty' "$plugin_file")"
  is_semver "$public_version" || die "entry '$name': public version '$public_version' is not valid semver"
  [[ "$public_version" == "$entry_version" ]] && continue
  write_json "$marketplace_file" --arg n "$name" --arg v "$public_version" \
    '(.plugins[] | select(.name == $n) | .version) = $v'
  changed=1
  printf '%s: %s -> %s\n' "$name" "$entry_version" "$public_version"
done < <(jq -r '
  .plugins[]
  | select((.source | type) == "object"
      and .source.source == "git-subdir"
      and (.source.url // "" | test("github\\.com[:/]rise-x/rise-x-ai-public-marketplace")))
  | [.name, .source.path, .version // ""] | @tsv
' "$marketplace_file")

# Zero matches means the internal marketplace no longer references this repo (or
# the url form changed) — fail loudly rather than report "no drift" forever.
(( matched > 0 )) || die "no externalized entries referencing this repo found in $marketplace_file"

# The internal repo's check-versions.sh requires a strictly-greater metadata.version
# whenever an entry changes.
if (( changed )); then
  meta="$(jq -r '.metadata.version // empty' "$marketplace_file")"
  is_semver "$meta" || die "metadata.version '$meta' is not valid semver"
  [[ "$meta" =~ ^([0-9]+)\.([0-9]+)\.([0-9]+)$ ]]
  new_meta="${BASH_REMATCH[1]}.${BASH_REMATCH[2]}.$((BASH_REMATCH[3] + 1))"
  write_json "$marketplace_file" --arg v "$new_meta" '.metadata.version = $v'
fi
