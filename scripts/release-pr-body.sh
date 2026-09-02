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
# The four heading forms are a cross-repo contract, not cosmetic. The
# release-notes skill in rise-x/rise-x-ai-marketplace matches them literally
# and hard-stops on NOT BUMPED; it records the same contract in its
# references/sources.md. Renaming one here silently breaks it:
#   ## <plugin> <old> -> <new>       version bumped this release
#   ## <plugin> <version> (NOT BUMPED)   changed but unbumped; validate blocks
#   ## <plugin> <version> (new plugin)   absent from main
#   ## <plugin> (removed)                gone from the release branch
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

version_at() { # $1=ref-or-empty for worktree, $2=plugin
  local ref="$1" name="$2" path="plugins/$2/.claude-plugin/plugin.json" raw
  if [[ -z "$ref" ]]; then
    [[ -f "${repo_root}/${path}" ]] || { printf ''; return 0; }
    raw="$(cat "${repo_root}/${path}")"
  else
    # Absent at that ref is legitimate: the plugin is new this release.
    raw="$(git -C "$repo_root" show "${ref}:${path}" 2>/dev/null || printf '')"
  fi
  [[ -z "$raw" ]] && { printf ''; return 0; }
  # Present but unparseable is not legitimate. Swallowing it would label a
  # broken manifest "(new plugin)" or "(removed)" and read as deliberate.
  printf '%s' "$raw" | jq -r '.version // empty' \
    || die "cannot read .version from ${path} at ${ref:-the working tree}"
}

# Plugins with any change on this branch relative to main. Three-dot: the
# release's own delta, so a plugin only main touched is not listed.
changed_plugins="$(git -C "$repo_root" diff --name-only "origin/main...HEAD" \
  | sed -n 's|^plugins/\([^/]*\)/.*|\1|p' | sort -u)"

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
pr_rows="$(printf '%s' "$prs" | jq -r '
  .[]
  | . as $pr
  | ([.files[].path | capture("^plugins/(?<p>[^/]+)/").p] | unique) as $plugins
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
      # validate blocks the release PR while this is true; surface it here too.
      printf '## %s %s (NOT BUMPED)\n\n' "$name" "$new"
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
