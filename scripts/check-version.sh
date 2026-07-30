#!/usr/bin/env bash
# Verify that any plugin changed under plugins/<name>/ carries a version bump
# in that plugin's own plugin.json (the only place a plugin's version lives —
# marketplace.json intentionally carries no version field), and that
# plugins/*/ stays in sync with marketplace.json's local-source entries.
#
# Usage:
#   ./scripts/check-version.sh [<base-ref>]
#
# If <base-ref> is omitted, defaults to origin/main.
#
# Exit codes:
#   0 - pass
#   1 - one or more violations
#   2 - usage / environment error

set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
marketplace_file="${repo_root}/.claude-plugin/marketplace.json"

die() {
  printf 'check-version.sh: %s\n' "$*" >&2
  exit 2
}

base="${1:-origin/main}"

command -v jq >/dev/null 2>&1 || die "jq is required"
[[ -f "$marketplace_file" ]] || die "marketplace.json not found at $marketplace_file"
git -C "$repo_root" rev-parse "$base" >/dev/null 2>&1 || die "cannot resolve base ref '$base'. Try: git fetch origin"

# Files exempt from triggering a version-bump requirement.
# A change to ONLY these files within a plugin does NOT require a bump.
exempt_regex='(^plugins/[^/]+/README\.md$|^plugins/[^/]+/tests/|^plugins/[^/]+/test/)'

if ! changed_files="$(git -C "$repo_root" diff --name-only "${base}...HEAD")"; then
  die "cannot diff '${base}...HEAD' — no merge base? Try: git fetch origin (or fetch enough history that $base and HEAD share an ancestor)"
fi

# Rename pairs between base and HEAD, as newline-separated "old<TAB>new" rows
# (from `git diff --name-status -M`, which emits "R<score>\told\tnew"). Used
# below so a renamed plugin dir isn't miscategorized as either "brand new"
# (skipping the strictly-greater check) or "removed" (skipping entirely).
rename_pairs="$(git -C "$repo_root" diff --name-status -M "${base}...HEAD" 2>/dev/null | awk -F'\t' '$1 ~ /^R/ { print $2 "\t" $3 }' || true)"

# A renamed plugin's OLD path may not otherwise appear in changed_files (git
# diff --name-only reports only the new path for a detected rename) — add it
# back so the old plugin name's per-plugin checks still see the change.
if [[ -n "$rename_pairs" ]]; then
  changed_files="$(printf '%s\n%s\n' "$changed_files" "$(printf '%s\n' "$rename_pairs" | cut -f1)")"
fi

# Given the NEW path of a possibly-renamed file, prints the OLD path if
# rename_pairs recorded one for it, otherwise prints nothing. Bash-3.2-safe
# (no associative arrays) — a linear scan of the newline-separated pairs.
find_rename_old_path() {
  local new_path="$1"
  [[ -z "$rename_pairs" ]] && return 0
  printf '%s\n' "$rename_pairs" | awk -F'\t' -v new="$new_path" '$2 == new { print $1; exit }'
}

# Validate X.Y.Z semver format — no leading zeros (each component is "0" or
# starts with 1-9), per the semver spec. Returns 0 if valid.
is_semver() {
  [[ "$1" =~ ^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ ]]
}

# Loose X.Y.Z shape check (digits only, leading zeros allowed) — used only to
# distinguish "wrong format entirely" from "right shape, leading zeros" in
# violation messages.
looks_like_semver_shape() {
  [[ "$1" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]
}

# Pure-bash X.Y.Z comparator. Returns 0 if $1 > $2. Assumes both inputs are
# already validated via is_semver (no leading zeros). `10#` forces base-10
# interpretation as belt-and-braces against bash's octal parsing of
# zero-prefixed numeric literals in arithmetic context.
semver_gt() {
  [[ "$1" =~ ^([0-9]+)\.([0-9]+)\.([0-9]+)$ ]]
  local a1="${BASH_REMATCH[1]}" a2="${BASH_REMATCH[2]}" a3="${BASH_REMATCH[3]}"
  [[ "$2" =~ ^([0-9]+)\.([0-9]+)\.([0-9]+)$ ]]
  local b1="${BASH_REMATCH[1]}" b2="${BASH_REMATCH[2]}" b3="${BASH_REMATCH[3]}"
  (( 10#$a1 != 10#$b1 )) && { (( 10#$a1 > 10#$b1 )); return; }
  (( 10#$a2 != 10#$b2 )) && { (( 10#$a2 > 10#$b2 )); return; }
  (( 10#$a3 > 10#$b3 ))
}

# Validates that `version` (belonging to `plugin`) is well-formed semver,
# recording a violation (with `suffix` appended for context) if not. Returns
# 1 on violation so callers can `continue`. Shared by the head-version and
# base-version checks below — they differ only in what happens when the
# version is *empty*, which each caller still handles itself before calling
# this (see finding 2/9: the base-version-empty path also consults the
# rename table before deciding "new plugin").
validate_semver_or_violate() {
  local plugin="$1" version="$2" suffix="$3"
  if ! is_semver "$version"; then
    if looks_like_semver_shape "$version"; then
      add_violation "plugin '${plugin}' version '${version}' is not valid semver (leading zeros not allowed)${suffix}"
    else
      add_violation "plugin '${plugin}' version '${version}' is not valid semver (expected X.Y.Z)${suffix}"
    fi
    return 1
  fi
  return 0
}

violations=()
add_violation() { violations+=("$1"); }

# Union of plugin names from: directories at HEAD, directories at base, and
# local-source entries in marketplace.json at HEAD. Covers adds, deletes, and
# an entry that never had (or no longer has) a matching directory.
# Bash-3.2-safe de-dup (no associative arrays).
list_plugin_dirs_head() {
  (cd "$repo_root" && for d in plugins/*/; do [[ -d "$d" ]] && basename "$d"; done)
}
list_plugin_dirs_base() {
  git -C "$repo_root" ls-tree -d --name-only "${base}:plugins" 2>/dev/null || true
}
list_marketplace_local_names() {
  jq -r '.plugins[] | select((.source | type) == "string") | .name' "$marketplace_file" 2>/dev/null || true
}

plugin_names=()
add_unique() {
  local candidate="$1" existing
  [[ -z "$candidate" ]] && return
  for existing in "${plugin_names[@]:-}"; do
    [[ "$existing" == "$candidate" ]] && return
  done
  plugin_names+=("$candidate")
}
while IFS= read -r line; do add_unique "$line"; done < <(list_plugin_dirs_head)
while IFS= read -r line; do add_unique "$line"; done < <(list_plugin_dirs_base)
while IFS= read -r line; do add_unique "$line"; done < <(list_marketplace_local_names)

# --- Per-plugin: any non-exempt change requires a strictly-greater version ---
for name in "${plugin_names[@]:-}"; do
  [[ -z "$name" ]] && continue
  plugin_prefix="plugins/${name}/"
  plugin_file="plugins/${name}/.claude-plugin/plugin.json"

  plugin_changed="$(printf '%s\n' "$changed_files" | grep -E "^${plugin_prefix}" || true)"
  [[ -z "$plugin_changed" ]] && continue

  if [[ ! -f "${repo_root}/${plugin_file}" ]]; then
    mp_has_entry="$(jq -r --arg n "$name" '.plugins | map(select(.name == $n)) | length' "$marketplace_file" 2>/dev/null)" ||
      die "cannot read marketplace.json entry for '$name' (invalid JSON in ${marketplace_file}?)"
    if [[ "$mp_has_entry" != "0" ]]; then
      if [[ -d "${repo_root}/plugins/${name}" ]]; then
        # Directory exists but has no plugin.json — a scaffolded/unfinished
        # plugin, not a removal. Keep this distinct from the true
        # removed-directory case below.
        add_violation "plugins/${name}/ exists but has no .claude-plugin/plugin.json — unfinished plugin?"
      else
        # Directory genuinely gone at HEAD. Fine, as long as marketplace.json agrees.
        add_violation "plugin '$name' directory was removed but marketplace.json still lists it — remove the marketplace entry too"
      fi
    fi
    continue
  fi

  non_exempt="$(printf '%s\n' "$plugin_changed" | grep -Ev "$exempt_regex" || true)"
  [[ -z "$non_exempt" ]] && continue

  head_version="$(jq -r .version "${repo_root}/${plugin_file}" 2>/dev/null || echo "")"
  if [[ -z "$head_version" || "$head_version" == "null" ]]; then
    add_violation "plugin '$name' has no .version in ${plugin_file}"
    continue
  fi
  validate_semver_or_violate "$name" "$head_version" " in ${plugin_file}" || continue

  base_version="$(git -C "$repo_root" show "${base}:${plugin_file}" 2>/dev/null | jq -r .version 2>/dev/null || echo "")"
  rename_note=""
  if [[ -z "$base_version" || "$base_version" == "null" ]]; then
    old_plugin_file="$(find_rename_old_path "$plugin_file")"
    if [[ -n "$old_plugin_file" ]]; then
      base_version="$(git -C "$repo_root" show "${base}:${old_plugin_file}" 2>/dev/null | jq -r .version 2>/dev/null || echo "")"
      rename_note=" (renamed from ${old_plugin_file})"
    fi
  fi
  if [[ -z "$base_version" || "$base_version" == "null" ]]; then
    printf 'check-version.sh: %s not present at %s (new plugin) — pass\n' "$plugin_file" "$base"
    continue
  fi
  validate_semver_or_violate "$name" "$base_version" " — base version${rename_note}, pre-existing issue in base branch" || continue

  if [[ "$base_version" == "$head_version" ]]; then
    add_violation "plugin '$name' changed but version not bumped (still $head_version)${rename_note}. Run: ./scripts/bump.sh $name patch"
    continue
  fi
  if ! semver_gt "$head_version" "$base_version"; then
    add_violation "plugin '$name' version $head_version is not strictly greater than base $base_version${rename_note}"
  fi
done

# --- Marketplace <-> plugins/ consistency (always checked, regardless of diff) ---
for name in "${plugin_names[@]:-}"; do
  [[ -z "$name" ]] && continue
  has_dir=0
  [[ -d "${repo_root}/plugins/${name}" ]] && has_dir=1
  entry_source_type="$(jq -r --arg n "$name" '.plugins[] | select(.name == $n) | (.source | type)' "$marketplace_file" 2>/dev/null || echo "")"

  if [[ "$entry_source_type" == "string" && "$has_dir" == "0" ]]; then
    add_violation "marketplace.json lists local-source plugin '$name' but plugins/${name}/ does not exist"
  fi
  if [[ -z "$entry_source_type" && "$has_dir" == "1" ]]; then
    add_violation "plugins/${name}/ exists but marketplace.json has no entry for it"
  fi
done

if (( ${#violations[@]} > 0 )); then
  printf 'check-version.sh FAILED (base=%s):\n' "$base" >&2
  for v in "${violations[@]}"; do
    printf '  - %s\n' "$v" >&2
  done
  exit 1
fi

printf 'check-version.sh OK (base=%s)\n' "$base"
