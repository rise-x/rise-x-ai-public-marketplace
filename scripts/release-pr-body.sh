#!/usr/bin/env bash
# Generate the body of a release PR (release/<name> -> main): a per-plugin
# changelog of the PRs merged into the release branch, plus a hand-editable
# block that survives regeneration.
#
# Usage:
#   ./scripts/release-pr-body.sh <release-branch> [<existing-body-file>]
#
# <existing-body-file> is the current PR body, if any. Whatever sits between
# the <!-- notes --> markers in it is carried across verbatim; everything else
# is regenerated. Pass nothing on the first run.
#
# Body shape (stable; release-notes tooling parses these markers):
#   <!-- notes -->      hand-written summary, preserved
#   <!-- changelog -->  one section per changed plugin, each listing
#                       "- <PR title> (#<n>)", then "## Other" for PRs
#                       touching nothing under plugins/
#
# The five heading forms are a cross-repo contract, not cosmetic. The
# release-notes skill in rise-x/rise-x-ai-marketplace matches them literally
# and hard-stops on NOT BUMPED; it records the same contract in its
# references/sources.md. Renaming one here silently breaks it:
#   ## <plugin> <old> -> <new>            version bumped this release
#   ## <plugin> <version> (NOT BUMPED)     changed but unbumped; validate blocks
#   ## <plugin> <version> (no bump needed) docs or tests only, which
#                                          check-version.sh exempts
#   ## <plugin> <version> (new plugin)     absent from main
#   ## <plugin> (removed)                  gone from the release branch
#
# Versions come from git: the merge base with origin/main, which is the point
# this release branched from, against the working tree. Run it with the
# release branch checked out and origin/main fetched. PR titles come from gh.
#
# Output: the body on stdout.
#
# Exit codes:
#   0 - success
#   2 - usage / environment error

set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

die() {
  printf 'release-pr-body.sh: %s\n' "$*" >&2
  exit 2
}

warn() {
  printf 'release-pr-body.sh: warning: %s\n' "$*" >&2
}

[[ $# -ge 1 && $# -le 2 ]] || die "usage: $0 <release-branch> [<existing-body-file>]"
branch="$1"
existing_body="${2:-}"

command -v jq >/dev/null 2>&1 || die "jq is required"
command -v gh >/dev/null 2>&1 || die "gh is required"
git -C "$repo_root" rev-parse origin/main >/dev/null 2>&1 || die "origin/main does not resolve; fetch it first"

# The git half of this script reads the checked-out tree; the PR half reads
# $branch. Disagreeing produces a changelog with one branch's plugin sections
# and another's PR list, which looks entirely plausible, so refuse instead.
# A detached HEAD reports no branch and is not a mismatch: it is one of the
# shapes a CI checkout leaves behind.
current_branch="$(git -C "$repo_root" symbolic-ref --quiet --short HEAD || printf '')"
if [[ -n "$current_branch" && "$current_branch" != "$branch" ]]; then
  die "checked out '${current_branch}' but asked about '${branch}'; check out '${branch}' first, or the plugin sections and the PR list would describe different branches"
fi

placeholder='_Replace this line with a short release summary. It is preserved when the changelog regenerates._'

# Carry the hand-written block across. Everything outside it is regenerated,
# so only this is read back from the existing body.
#
# The notes are defined as whatever precedes the generated changelog, with the
# notes markers themselves dropped. Keying on the changelog marker rather than
# on a matching pair of notes markers is what makes the three real bodies all
# work: a regenerated one, one whose closing marker someone deleted while
# editing (pairing would swallow the whole changelog into the notes, and stay
# that way), and a hand-written one with no markers at all, which the release
# PR starts life as because a person opens it (pairing would find no block and
# silently replace their summary with the placeholder).
#
# Markers are anchored to a whole line, which is how this script writes them,
# so a note that merely mentions one is text rather than a delimiter.
notes="$placeholder"
if [[ -n "$existing_body" && -f "$existing_body" ]]; then
  carried="$(awk '
    /^[[:space:]]*<!-- changelog -->[[:space:]]*$/ { exit }
    /^[[:space:]]*<!-- \/?notes -->[[:space:]]*$/  { next }
                                                   { print }
  ' "$existing_body")"
  # A block holding only whitespace falls back to the placeholder, so an
  # accidentally emptied block does not silently strip the prompt.
  if [[ -n "${carried//[$' \t\n']/}" ]]; then
    notes="$carried"
  fi
fi

version_at() { # $1=ref, empty for the working tree; $2=plugin
  local ref="$1" path="plugins/$2/.claude-plugin/plugin.json" raw version
  local where="${ref:-the working tree}"
  if [[ -z "$ref" ]]; then
    # Absent is legitimate: the plugin was removed on this branch.
    [[ -f "${repo_root}/${path}" ]] || { printf ''; return 0; }
    raw="$(cat "${repo_root}/${path}")"
  else
    # Absent at that ref is legitimate too: the plugin is new this release.
    # Test the exit status, not the output: git show prints nothing both for
    # a missing path and for a file that exists and is empty.
    raw="$(git -C "$repo_root" show "${ref}:${path}" 2>/dev/null)" \
      || { printf ''; return 0; }
  fi
  # Present is a different matter. Empty, unparseable, or carrying no .version
  # all mean a broken manifest, and reporting one as "(new plugin)" or
  # "(removed)" would dress a defect up as a deliberate change.
  [[ -n "$raw" ]] || die "${path} is empty at ${where}"
  version="$(printf '%s' "$raw" | jq -r '.version // empty')" \
    || die "cannot parse ${path} at ${where}"
  [[ -n "$version" ]] || die "${path} has no .version at ${where}"
  printf '%s' "$version"
}

# Files changed on this branch relative to main. Three-dot: the release's own
# delta, so a plugin only main touched is not listed.
#
# --no-renames because the question here is which plugin DIRECTORIES the
# release touched. With rename detection on, moving plugins/foo to plugins/bar
# reports only the new path, so foo would drop out of the notes entirely and
# the release would not mention that it is gone. Turning it off reports the
# rename as a delete plus an add, which lands foo under (removed) and bar
# under (new plugin) - two true statements about the release.
if ! changed_files="$(git -C "$repo_root" diff --name-only --no-renames "origin/main...HEAD")"; then
  die "cannot diff 'origin/main...HEAD'; no merge base? Fetch enough history that origin/main and HEAD share an ancestor"
fi

# The file list above is a three-dot diff, so its left side is the merge base.
# Versions have to be read from that same commit. Reading them from main's tip
# instead described a release against a point its file list never used: once a
# hotfix moved main, a plugin the release did not touch rendered as a version
# change, backwards ones included, and a plugin main had deleted rendered as
# new. Both are headings the release-notes tooling reads literally.
if ! merge_base="$(git -C "$repo_root" merge-base origin/main HEAD)"; then
  die "cannot find the merge base of origin/main and HEAD"
fi
if [[ "$merge_base" != "$(git -C "$repo_root" rev-parse origin/main)" ]]; then
  warn "origin/main has moved on since this branch left it, so these notes describe the release against the cut point. Merge main into the release branch before releasing."
fi
changed_plugins="$(printf '%s\n' "$changed_files" \
  | sed -n 's|^plugins/\([^/]*\)/.*|\1|p' | sort -u)"

# Kept identical to check-version.sh: a plugin whose whole release delta is
# docs or tests needs no bump, so an unchanged version there is correct, not
# a mistake. Emitting NOT BUMPED for it would be a false alarm, and the
# release-notes tooling hard-stops on that marker.
exempt_regex='(^plugins/[^/]+/README\.md$|^plugins/[^/]+/tests/|^plugins/[^/]+/test/)'

needs_bump() { # $1=plugin; true when its release delta has a non-exempt file
  # Prefix-matched with ==, not grep: the plugin name is a directory name, not
  # a pattern, and a "." or "+" in one would otherwise match a sibling
  # directory and pin the wrong bump verdict on this plugin.
  local prefix="plugins/$1/" f
  while IFS= read -r f; do
    [[ "$f" == "$prefix"* ]] || continue
    [[ "$f" =~ $exempt_regex ]] || return 0
  done <<< "$changed_files"
  return 1
}

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

# "<plugin>\t<n>\t<title>" for every (plugin, PR) pair, with an empty first
# field for PRs touching nothing under plugins/. A PR spanning two plugins is
# listed under both, which is what the diff actually says.
#
# Each PR is first cut down to the files that survive into the release delta.
# protect-release requires a PR to touch the branch, so merging main back in
# is itself a merged PR whose files are already on main and are therefore
# absent from origin/main...HEAD. Listing it would credit this release with
# work it does not ship. A PR left with nothing in the delta drops out, which
# also covers one whose changes were later reverted on the branch.
#
# The select() on plugins/ is redundant: capture() yields nothing for a path
# that does not match. It is spelled out because relying on that is easy to
# misread as a crash waiting to happen.
pr_rows="$(printf '%s' "$prs" | jq -r --arg delta "$changed_files" '
  ($delta | split("\n") | map(select(length > 0)) | INDEX(.)) as $shipped
  | .[]
  | . as $pr
  | [.files[].path | select($shipped[.])] as $paths
  | if ($paths | length) == 0
    then empty
    else ([$paths[]
            | select(startswith("plugins/"))
            | capture("^plugins/(?<p>[^/]+)/").p] | unique) as $plugins
      | if ($plugins | length) == 0
        then "\t\($pr.number)\t\($pr.title)"
        else $plugins[] | "\(.)\t\($pr.number)\t\($pr.title)"
        end
    end
')" || die "cannot parse the PR list"

emit_prs() { # $1=plugin name, empty for the Other section
  local key="$1" found=0 row rest num title
  # read with IFS cleared takes whole lines; the fields are then split with
  # parameter expansion. Letting read split them on a tab would not work: a
  # tab in IFS counts as whitespace, so it would strip the empty leading
  # field that marks an Other row.
  while IFS= read -r row; do
    [[ -z "$row" ]] && continue
    [[ "${row%%$'\t'*}" == "$key" ]] || continue
    found=1
    rest="${row#*$'\t'}"
    num="${rest%%$'\t'*}"
    title="${rest#*$'\t'}"
    printf -- '- %s (#%s)\n' "$title" "$num"
  done <<< "$pr_rows"
  (( found )) || printf -- '- _no merged PRs recorded_\n'
}

{
  printf '<!-- notes -->\n%s\n<!-- /notes -->\n\n' "$notes"
  printf '<!-- changelog -->\n'
  if [[ -z "$changed_plugins" ]]; then
    printf 'No plugin changes on this release branch.\n\n'
  fi
  while IFS= read -r name; do
    [[ -z "$name" ]] && continue
    old="$(version_at "$merge_base" "$name")"
    new="$(version_at '' "$name")"
    if [[ -z "$new" ]]; then
      printf '## %s (removed)\n\n' "$name"
    elif [[ -z "$old" ]]; then
      printf '## %s %s (new plugin)\n\n' "$name" "$new"
    elif [[ "$old" == "$new" ]]; then
      if needs_bump "$name"; then
        # validate blocks the release PR while this is true; surface it here.
        printf '## %s %s (NOT BUMPED)\n\n' "$name" "$new"
      else
        printf '## %s %s (no bump needed)\n\n' "$name" "$new"
      fi
    else
      printf '## %s %s -> %s\n\n' "$name" "$old" "$new"
    fi
    emit_prs "$name"
    printf '\n'
  done <<< "$changed_plugins"

  # A leading tab marks a PR that touched nothing under plugins/. Written as
  # $'\t' rather than a literal tab, which is invisible here and one careless
  # editor away from becoming spaces.
  if printf '%s' "$pr_rows" | grep -q $'^\t'; then
    printf '## Other\n\n'
    emit_prs ''
    printf '\n'
  fi
  printf '<!-- /changelog -->\n'
}
