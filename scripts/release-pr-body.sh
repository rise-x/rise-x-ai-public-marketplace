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
# Versions come from git (origin/main vs the working tree), so run it with the
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

[[ $# -ge 1 && $# -le 2 ]] || die "usage: $0 <release-branch> [<existing-body-file>]"
branch="$1"
existing_body="${2:-}"

command -v jq >/dev/null 2>&1 || die "jq is required"
command -v gh >/dev/null 2>&1 || die "gh is required"
git -C "$repo_root" rev-parse origin/main >/dev/null 2>&1 || die "origin/main does not resolve; fetch it first"

placeholder='_Replace this line with a short release summary. It is preserved when the changelog regenerates._'

# Carry the hand-written block across. Everything outside the markers is
# regenerated, so only this is read back from the existing body. awk rather
# than sed: the block is multi-line and may itself contain markdown.
notes="$placeholder"
if [[ -n "$existing_body" && -f "$existing_body" ]]; then
  carried="$(awk '
    /<!-- \/notes -->/ { inblock = 0 }
    inblock            { print }
    /<!-- notes -->/   { inblock = 1 }
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
if ! changed_files="$(git -C "$repo_root" diff --name-only "origin/main...HEAD")"; then
  die "cannot diff 'origin/main...HEAD'; no merge base? Fetch enough history that origin/main and HEAD share an ancestor"
fi
changed_plugins="$(printf '%s\n' "$changed_files" \
  | sed -n 's|^plugins/\([^/]*\)/.*|\1|p' | sort -u)"

# Kept identical to check-version.sh: a plugin whose whole release delta is
# docs or tests needs no bump, so an unchanged version there is correct, not
# a mistake. Emitting NOT BUMPED for it would be a false alarm, and the
# release-notes tooling hard-stops on that marker.
exempt_regex='(^plugins/[^/]+/README\.md$|^plugins/[^/]+/tests/|^plugins/[^/]+/test/)'

needs_bump() { # $1=plugin; true when its delta includes a non-exempt file
  local own
  own="$(printf '%s\n' "$changed_files" | grep -E "^plugins/$1/" || true)"
  [[ -n "$own" ]] || return 1
  printf '%s\n' "$own" | grep -qEv "$exempt_regex"
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
# The select() is redundant: capture() yields nothing for a path that does not
# match, so non-plugin paths drop out either way and such a PR falls to the
# Other section. It is spelled out because relying on that is easy to misread
# as a crash waiting to happen, and it states the intent at the line itself.
pr_rows="$(printf '%s' "$prs" | jq -r '
  .[]
  | . as $pr
  | ([.files[].path
      | select(startswith("plugins/"))
      | capture("^plugins/(?<p>[^/]+)/").p] | unique) as $plugins
  | if ($plugins | length) == 0
    then "\t\($pr.number)\t\($pr.title)"
    else $plugins[] | "\(.)\t\($pr.number)\t\($pr.title)"
    end
')" || die "cannot parse the PR list"

emit_prs() { # $1=plugin name, empty for the Other section
  local key="$1" found=0 row rest num title
  # Split with parameter expansion, not `read`: a tab in IFS is whitespace, so
  # `read` would strip the empty leading field that marks an Other row.
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
    old="$(version_at origin/main "$name")"
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

  # A leading tab marks a PR that touched nothing under plugins/.
  if printf '%s' "$pr_rows" | grep -q '^	'; then
    printf '## Other\n\n'
    emit_prs ''
    printf '\n'
  fi
  printf '<!-- /changelog -->\n'
}
