#!/usr/bin/env bash
# Sync the internal marketplace's externalized (git-subdir) entry versions to the
# plugin.json versions in this repo, and patch-bump its metadata.version when
# anything changed. Edits the internal checkout's marketplace.json in place.
#
# Mirroring is deliberately unconditional — the entry is set to whatever this
# repo publishes, downgrades included. The mirror's contract is equality with
# the source, not monotonicity; this repo's own CI already gates regressions.
#
# Usage:
#   ./scripts/sync-internal-versions.sh <internal-repo-checkout>
#
# Output: one line per updated entry ("<name>: <old> -> <new>") on stdout.
# No output means no drift. Warnings go to stderr so stdout stays clean for
# the caller's drift summary. Used by
# .github/workflows/sync-internal-marketplace.yml.
#
# Exit codes:
#   0 - success (with or without changes)
#   2 - usage / environment / data error

set -euo pipefail

die() {
  printf 'sync-internal-versions.sh: %s\n' "$*" >&2
  exit 2
}

warn() {
  printf 'sync-internal-versions.sh: warning: %s\n' "$*" >&2
}

[[ $# -eq 1 ]] || die "usage: $0 <internal-repo-checkout>"
internal_root="$1"
public_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
marketplace_file="${internal_root}/.claude-plugin/marketplace.json"
public_marketplace_file="${public_root}/.claude-plugin/marketplace.json"

command -v jq >/dev/null 2>&1 || die "jq is required"
[[ -f "$marketplace_file" ]] || die "marketplace.json not found at $marketplace_file"
jq -e . "$marketplace_file" >/dev/null 2>&1 || die "marketplace.json is not valid JSON at $marketplace_file"

is_semver() {
  [[ "$1" =~ ^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ ]]
}

# Validated before any entry is written: the internal repo's check-versions.sh
# rejects entry updates without a metadata bump, so failing here after mutations
# would leave the checkout in exactly the state it rejects.
meta="$(jq -r '.metadata.version // empty' "$marketplace_file")"
is_semver "$meta" || die "metadata.version '$meta' is not valid semver"

write_json() {
  # Usage: write_json <file> [jq args...] <filter>
  local file="$1"
  shift
  local tmp
  tmp="$(mktemp "${file}.XXXXXX")"
  if ! jq --indent 2 "$@" "$file" >"$tmp"; then
    rm -f "$tmp"
    die "jq failed while updating $file"
  fi
  mv "$tmp" "$file"
}

# Entries whose git-subdir source points at this repo, as "name<TAB>path<TAB>version".
# Materialized (not streamed through process substitution) so a jq failure is a
# loud die instead of a silently truncated entry list. The url test is anchored
# and case-insensitive, covering https and SSH forms. Fields fall back to
# placeholders so tab-run collapsing in `read` can never shift them.
entries="$(jq -r '
  .plugins[]
  | select((.source | type) == "object"
      and .source.source == "git-subdir"
      and (.source.url // "" | test("github\\.com[:/]rise-x/rise-x-ai-public-marketplace(\\.git)?/?$"; "i")))
  | [(.name // "(unnamed)"), (.source.path // "(missing)"), (.version // "(none)")] | @tsv
' "$marketplace_file")" || die "cannot read plugin entries from $marketplace_file"

changed=0
matched=0
synced=0
matched_names=""

while IFS=$'\t' read -r name path entry_version; do
  [[ -z "$name" ]] && continue
  matched=$((matched + 1))
  matched_names+="${name}"$'\n'
  plugin_file="${public_root}/${path}/.claude-plugin/plugin.json"
  if [[ ! -f "$plugin_file" ]]; then
    # Stale entry (plugin gone from this repo): skip it so the others still sync.
    warn "entry '$name' references '$path' but ${plugin_file} does not exist — skipping"
    continue
  fi
  synced=$((synced + 1))
  public_version="$(jq -r '.version // empty' "$plugin_file")" || die "cannot read $plugin_file"
  is_semver "$public_version" || die "entry '$name': public version '$public_version' is not valid semver"
  [[ "$public_version" == "$entry_version" ]] && continue
  # The write carries the read's full predicate (plus path, the real identity):
  # selecting on name alone would also overwrite an unrelated same-named entry.
  write_json "$marketplace_file" --arg n "$name" --arg p "$path" --arg v "$public_version" \
    '(.plugins[]
      | select(.name == $n
          and (.source | type) == "object"
          and .source.source == "git-subdir"
          and .source.path == $p)
      | .version) = $v'
  changed=1
  printf '%s: %s -> %s\n' "$name" "$entry_version" "$public_version"
done <<< "$entries"

# Zero matches means the internal marketplace no longer references this repo (or
# the url form changed) — and matched-but-all-stale means nothing can sync at
# all. Fail loudly rather than report "no drift" forever.
(( matched > 0 )) || die "no externalized entries referencing this repo found in $marketplace_file"
(( synced > 0 )) || die "every matching entry in $marketplace_file is stale — nothing can sync"

# A plugin hosted here with no internal entry never propagates to the org — the
# exact gap this script exists to close. Warn rather than die: a public-only
# plugin may be intentional.
local_plugins="$(jq -r '.plugins[] | select((.source | type) == "string") | .name' \
  "$public_marketplace_file")" || die "cannot read $public_marketplace_file"
while IFS= read -r local_name; do
  [[ -z "$local_name" ]] && continue
  grep -qxF "$local_name" <<< "$matched_names" \
    || warn "plugin '$local_name' has no externalized entry in $marketplace_file — its releases will not propagate to the org"
done <<< "$local_plugins"

# The internal repo's check-versions.sh requires a strictly-greater metadata.version
# whenever an entry changes.
if (( changed )); then
  [[ "$meta" =~ ^([0-9]+)\.([0-9]+)\.([0-9]+)$ ]]
  new_meta="${BASH_REMATCH[1]}.${BASH_REMATCH[2]}.$((10#${BASH_REMATCH[3]} + 1))"
  write_json "$marketplace_file" --arg v "$new_meta" '.metadata.version = $v'
fi
