#!/usr/bin/env bash
# Generate the body of a kit release PR (release-kit/<name> -> main): the
# kit's version change plus the PRs merged into the branch, and a
# hand-editable block that survives regeneration.
#
# Usage:
#   ./scripts/release-kit-pr-body.sh <release-kit-branch> [<existing-body-file>]
#
# <existing-body-file> is the current PR body, if any. Whatever sits between
# the <!-- notes --> markers in it is carried across verbatim; everything else
# is regenerated. Pass nothing on the first run.
#
# Body shape, matching scripts/release-pr-body.sh's marker convention so the
# same tooling can read either:
#   <!-- notes -->      hand-written summary, preserved
#   <!-- changelog -->  one section for the kit, listing "- <PR title> (#<n>)"
#
# The kit ships as one binary, so there is one section rather than one per
# plugin, and its heading is the kit's own:
#   ## rise-x-kit <old> -> <new>            version bumped this release
#   ## rise-x-kit <version> (NOT BUMPED)    kit changed but unbumped; kit-ci blocks
#   ## rise-x-kit <version> (no bump needed) nothing under kit/ or the kit
#                                            workflows changed
#   ## rise-x-kit <version> (new)           kit/VERSION absent on main
#
# These headings are this track's own. The five plugin headings in
# release-pr-body.sh are a cross-repo contract that the release-notes skill in
# rise-x/rise-x-ai-marketplace matches literally, so they are left alone.
#
# The version comes from git: kit/VERSION at the merge base with origin/main,
# against the working tree. That is where the branch left main, or main's tip
# once main has been merged back in; either way it is the commit the file list
# is taken from. Run it with the release-kit branch checked out and origin/main
# fetched. PR titles come from gh.
#
# Output: the body on stdout.
#
# Exit codes:
#   0 - success
#   2 - usage / environment error

set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
version_path='kit/VERSION'

die() {
  printf 'release-kit-pr-body.sh: %s\n' "$*" >&2
  exit 2
}

warn() {
  printf 'release-kit-pr-body.sh: warning: %s\n' "$*" >&2
}

[[ $# -ge 1 && $# -le 2 ]] || die "usage: $0 <release-kit-branch> [<existing-body-file>]"
branch="$1"
existing_body="${2:-}"

command -v jq >/dev/null 2>&1 || die "jq is required"
command -v gh >/dev/null 2>&1 || die "gh is required"
git -C "$repo_root" rev-parse origin/main >/dev/null 2>&1 || die "origin/main does not resolve; fetch it first"

# The git half of this script reads the checked-out tree; the PR half reads
# $branch. Disagreeing produces a changelog with one branch's version and
# another's PR list, which looks entirely plausible, so refuse instead. A
# detached HEAD is one of the shapes a checkout leaves, so it is allowed, but
# only at the tip of the branch being described.
current_branch="$(git -C "$repo_root" symbolic-ref --quiet --short HEAD || printf '')"
if [[ -n "$current_branch" ]]; then
  [[ "$current_branch" == "$branch" ]] || die \
    "checked out '${current_branch}' but asked about '${branch}'; check out '${branch}' first, or the version and the PR list would describe different branches"
else
  branch_sha="$(git -C "$repo_root" rev-parse --verify --quiet "refs/heads/${branch}" \
    || git -C "$repo_root" rev-parse --verify --quiet "refs/remotes/origin/${branch}" \
    || printf '')"
  [[ -n "$branch_sha" ]] || die \
    "HEAD is detached and '${branch}' resolves to nothing here, so the tree cannot be checked against it; check out '${branch}'"
  [[ "$branch_sha" == "$(git -C "$repo_root" rev-parse HEAD)" ]] || die \
    "HEAD is detached at a commit that is not the tip of '${branch}'; the version would come from this tree and the PR list from the branch"
fi

placeholder='_Replace this line with a short kit release summary. It is preserved when the changelog regenerates._'

# Carry the hand-written block across. Everything outside it is regenerated,
# so only this is read back from the existing body.
#
# The notes are defined as whatever precedes the generated changelog, with the
# notes markers themselves dropped — the same rule as release-pr-body.sh, and
# for the same reason: it handles a regenerated body, one whose closing marker
# someone deleted while editing, and the hand-written body the PR starts life
# as, because a person opens it.
notes="$placeholder"
if [[ -n "$existing_body" && -f "$existing_body" ]]; then
  carried="$(awk '
    /^[[:space:]]*<!-- changelog -->[[:space:]]*$/ { exit }
    /^[[:space:]]*<!-- \/?notes -->[[:space:]]*$/  { next }
                                                   { print }
  ' "$existing_body")"
  if [[ -n "${carried//[$' \t\n']/}" ]]; then
    notes="$carried"
  fi
fi

# All whitespace is stripped, as in check-kit-version.sh: a stray space would
# otherwise reach the heading and read as part of the version.
version_at() { # $1=ref, empty for the working tree
  local ref="$1" raw where="${1:-the working tree}"
  if [[ -z "$ref" ]]; then
    # Absent is legitimate on neither side today, but a kit deleted on the
    # branch should say so rather than crash.
    [[ -f "${repo_root}/${version_path}" ]] || { printf ''; return 0; }
    raw="$(cat "${repo_root}/${version_path}")"
  else
    # Absent at that ref means the first kit release.
    raw="$(git -C "$repo_root" show "${ref}:${version_path}" 2>/dev/null)" \
      || { printf ''; return 0; }
  fi
  [[ -n "${raw//[$' \t\r\n']/}" ]] || die "${version_path} is empty at ${where}"
  printf '%s' "$raw" | tr -d '[:space:]'
}

# Files changed on this branch relative to main. Three-dot: the release's own
# delta, so a change only main made is not listed.
if ! changed_files="$(git -C "$repo_root" diff --name-only "origin/main...HEAD")"; then
  die "cannot diff 'origin/main...HEAD'; no merge base? Fetch enough history that origin/main and HEAD share an ancestor"
fi

# The file list above is a three-dot diff, so its left side is the merge base,
# and the version has to be read from that same commit. Reading it from main's
# tip instead would describe the release against a point its file list never
# used, once main moved.
if ! merge_base="$(git -C "$repo_root" merge-base origin/main HEAD)"; then
  die "cannot find the merge base of origin/main and HEAD"
fi
if [[ "$merge_base" != "$(git -C "$repo_root" rev-parse origin/main)" ]]; then
  warn "origin/main has moved on since this branch left it, so these notes describe the release against the cut point. Merge main into the release-kit branch before releasing."
fi

# Kept identical to check-kit-version.sh: those are the paths whose change
# requires a bump, so an unchanged version outside them is correct rather than
# a mistake.
kit_regex='(^kit/|^\.github/workflows/kit-[^/]*\.yml$)'
kit_changed="$(printf '%s\n' "$changed_files" | grep -E "$kit_regex" || true)"

old="$(version_at "$merge_base")"
new="$(version_at '')"

# One call: number, title and file list for every PR merged into the branch.
pr_limit=500
prs="$(gh pr list --repo "${GITHUB_REPOSITORY:-$(gh repo view --json nameWithOwner --jq .nameWithOwner)}" \
  --state merged --base "$branch" --limit "$pr_limit" \
  --json number,title,files)" || die "cannot list merged PRs for base '$branch'"
# Hitting the limit would drop the oldest PRs from the notes with no sign of
# it, so stop instead of publishing a changelog that is quietly incomplete.
pr_total="$(printf '%s' "$prs" | jq length)" || die "cannot count the PR list"
(( pr_total < pr_limit )) \
  || die "'$branch' has at least ${pr_limit} merged PRs; raise pr_limit"

# Every PR is cut down to its files that appear in the release delta, and one
# left with none drops out: merging main back in is itself a merged PR whose
# files are already on main, and listing it would credit this release with
# work it does not ship.
#
# Unlike the plugin track there is nothing to group by — one binary ships —
# so a PR that touched only the kit workflows is listed alongside the code.
pr_rows="$(printf '%s' "$prs" | jq -r --arg delta "$changed_files" '
  ($delta | split("\n") | map(select(length > 0)) | INDEX(.)) as $shipped
  | .[]
  | select([.files[].path | select($shipped[.])] | length > 0)
  | "\(.number)\t\(.title)"
')" || die "cannot parse the PR list"

{
  printf '<!-- notes -->\n%s\n<!-- /notes -->\n\n' "$notes"
  printf '<!-- changelog -->\n'
  if [[ -z "$new" ]]; then
    printf '## rise-x-kit (removed)\n\n'
  elif [[ -z "$old" ]]; then
    printf '## rise-x-kit %s (new)\n\n' "$new"
  elif [[ "$old" == "$new" ]]; then
    if [[ -n "$kit_changed" ]]; then
      # kit-ci blocks the release PR while this is true; surface it here.
      printf '## rise-x-kit %s (NOT BUMPED)\n\n' "$new"
    else
      printf '## rise-x-kit %s (no bump needed)\n\n' "$new"
    fi
  else
    printf '## rise-x-kit %s -> %s\n\n' "$old" "$new"
  fi

  if [[ -z "${pr_rows//[$' \t\n']/}" ]]; then
    printf -- '- _no merged PRs recorded_\n'
  else
    # read with IFS cleared takes whole lines; the number and title are then
    # split with parameter expansion, so a tab in a title cannot shift them.
    while IFS= read -r row; do
      [[ -z "$row" ]] && continue
      printf -- '- %s (#%s)\n' "${row#*$'\t'}" "${row%%$'\t'*}"
    done <<< "$pr_rows"
  fi
  printf '\n<!-- /changelog -->\n'
}
