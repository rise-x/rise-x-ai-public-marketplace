#!/usr/bin/env bash
# Verify that a change to the kit carries a version bump in kit/VERSION.
#
# kit/VERSION is the kit's only version source: merging a release-kit/*
# branch into main is what publishes a kit release, and kit-release.yml reads
# this file to decide which tag to cut. A release-kit/* -> main PR that
# changes the kit without raising it would therefore ship nothing.
#
# Only PRs into main need the bump. A PR into an open release-kit/* branch is
# collecting work for a release that has not happened yet, so kit-ci runs this
# check on the main-bound PR only (see .github/workflows/kit-ci.yml).
#
# Usage:
#   ./scripts/check-kit-version.sh [<base-ref>]
#
# If <base-ref> is omitted, defaults to origin/main.
#
# Exit codes:
#   0 - pass
#   1 - one or more violations
#   2 - usage / environment error

set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
version_path='kit/VERSION'

die() {
  printf 'check-kit-version.sh: %s\n' "$*" >&2
  exit 2
}

fail() {
  printf 'check-kit-version.sh FAILED (base=%s):\n  - %s\n' "$base" "$*" >&2
  exit 1
}

base="${1:-origin/main}"

git -C "$repo_root" rev-parse "$base" >/dev/null 2>&1 || die "cannot resolve base ref '$base'. Try: git fetch origin"

# Same set of paths kit-ci gates on: the kit itself and the workflows that
# build and ship it. A change to either alters what the next kit release
# contains, so either requires a bump.
kit_regex='(^kit/|^\.github/workflows/kit-[^/]*\.yml$)'

if ! changed_files="$(git -C "$repo_root" diff --name-only "${base}...HEAD")"; then
  die "cannot diff '${base}...HEAD' — no merge base? Try: git fetch origin (or fetch enough history that $base and HEAD share an ancestor)"
fi

kit_changed="$(printf '%s\n' "$changed_files" | grep -E "$kit_regex" || true)"
if [[ -z "$kit_changed" ]]; then
  printf 'check-kit-version.sh OK (base=%s): no kit changes\n' "$base"
  exit 0
fi

# Validate X.Y.Z semver format — no leading zeros (each component is "0" or
# starts with 1-9), per the semver spec. Kept identical to check-version.sh so
# the two tracks accept the same version strings.
is_semver() {
  [[ "$1" =~ ^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ ]]
}

looks_like_semver_shape() {
  [[ "$1" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]
}

# Pure-bash X.Y.Z comparator. Returns 0 if $1 > $2. Assumes both inputs are
# already validated via is_semver. `10#` forces base-10 interpretation as
# belt-and-braces against bash's octal parsing of zero-prefixed literals.
semver_gt() {
  [[ "$1" =~ ^([0-9]+)\.([0-9]+)\.([0-9]+)$ ]]
  local a1="${BASH_REMATCH[1]}" a2="${BASH_REMATCH[2]}" a3="${BASH_REMATCH[3]}"
  [[ "$2" =~ ^([0-9]+)\.([0-9]+)\.([0-9]+)$ ]]
  local b1="${BASH_REMATCH[1]}" b2="${BASH_REMATCH[2]}" b3="${BASH_REMATCH[3]}"
  (( 10#$a1 != 10#$b1 )) && { (( 10#$a1 > 10#$b1 )); return; }
  (( 10#$a2 != 10#$b2 )) && { (( 10#$a2 > 10#$b2 )); return; }
  (( 10#$a3 > 10#$b3 ))
}

# All whitespace is stripped, not just the trailing newline: "0.1.0 " and a
# CRLF line ending are typos rather than versions, and letting them through
# here would only move the failure to the tag name.
read_version() { # $1=ref, empty for the working tree
  local ref="$1" raw
  if [[ -z "$ref" ]]; then
    [[ -f "${repo_root}/${version_path}" ]] || return 1
    raw="$(cat "${repo_root}/${version_path}")"
  else
    raw="$(git -C "$repo_root" show "${ref}:${version_path}" 2>/dev/null)" || return 1
  fi
  printf '%s' "$raw" | tr -d '[:space:]'
}

head_version="$(read_version '' || true)"
[[ -n "$head_version" ]] || fail "${version_path} is missing or empty — the kit's version lives there and nowhere else"

if ! is_semver "$head_version"; then
  if looks_like_semver_shape "$head_version"; then
    fail "${version_path} '${head_version}' is not valid semver (leading zeros not allowed)"
  fi
  fail "${version_path} '${head_version}' is not valid semver (expected X.Y.Z, no leading v)"
fi

if ! base_version="$(read_version "$base")" || [[ -z "$base_version" ]]; then
  printf 'check-kit-version.sh: %s not present at %s (first kit release) — pass\n' "$version_path" "$base"
  exit 0
fi

is_semver "$base_version" || fail "${version_path} '${base_version}' at ${base} is not valid semver — pre-existing issue on the base branch"

if [[ "$base_version" == "$head_version" ]]; then
  fail "the kit changed but ${version_path} was not bumped (still ${head_version}). Merging this is what publishes kit-v${head_version}, so raise it."
fi
if ! semver_gt "$head_version" "$base_version"; then
  fail "${version_path} ${head_version} is not strictly greater than base ${base_version}"
fi

printf 'check-kit-version.sh OK (base=%s): %s -> %s\n' "$base" "$base_version" "$head_version"
