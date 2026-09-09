# Rulesets

GitHub does not read this directory. These files are the checked-in copy of
two rulesets that live in repository settings, so the intent is reviewable and
the settings can be rebuilt from the repo.

To apply one: **Settings** → **Rules** → **Rulesets** → **New ruleset** →
**Import a ruleset**, then upload the JSON file. Review the imported ruleset
before saving; GitHub resolves the status-check names and bypass actors at
import time, and it reports anything it could not match.

| File | Ruleset | Guards |
|------|---------|--------|
| `protect-release-kit.json` | `protect-release-kit` | `release-kit/*` branches |
| `kit-tags.json` | `kit-tags` | `kit-v*` tags |

The plugin track's `protect-main` and `protect-release` rulesets are not here
yet. They were created in the UI before this directory existed.

## protect-release-kit

Gates `release-kit/*` the way `protect-release` gates `release/*`: a pull
request, one approval that satisfies `CODEOWNERS`, resolved review threads, a
passing `kit-ci`, and no force-push. It deliberately carries no `deletion`
rule, because every kit release deletes its branch afterwards.

The required checks are `kit-ci`'s job names:

- `test`
- `build (darwin, arm64)`
- `build (darwin, amd64)`
- `build (windows, amd64)`

Two things to know about them:

- The names come from the job ids and the build matrix in
  `.github/workflows/kit-ci.yml`. Renaming a job, or adding a matrix entry,
  changes or adds a check name and the ruleset has to be re-imported.
- The JSON pins no app, so any check reporting those names satisfies the rule.
  This repo runs only GitHub Actions, so that is fine; to tighten it, set the
  check's source to **GitHub Actions** in the UI after import.

`kit-ci` runs only for pull requests that touch `kit/**` or
`.github/workflows/kit-*.yml`. A pull request into `release-kit/*` touching
neither never starts it, and a required check that never reports leaves the
pull request blocked as pending. Keep kit release branches to kit changes; if
one genuinely needs an unrelated change, a maintainer has to merge past the
rule.

## kit-tags

`kit-v*` tags are the kit's release history: `install.sh` and `install.ps1`
resolve the newest `kit-v*` release and download its assets by name, so a tag
that moves or disappears rewrites what partners install. This ruleset
restricts creating, updating, and deleting those tags.

`kit-release.yml` creates the tag through the releases API as the GitHub
Actions app. The JSON ships with an empty bypass list because the import
rejects the app's actor id in this enterprise ("invalid actor"). After the
import, open the ruleset and add the bypass in the UI: **Bypass list** →
**Add bypass** → pick **GitHub Actions** if it is offered; if it is not,
add the maintainers team instead and keep the release fallback below in mind.
Nobody pushes a `kit-v*` tag by hand.

Until the first real release has run, set **Enforcement** to **Evaluate**:
the rule then only records what it would have blocked under **Rule
insights**, so a wrong bypass list cannot stop the release. Switch to
**Active** once the workflow has created a tag successfully. Unverified and
worth checking on that first release: whether the enterprise policy that
forbids Actions from creating pull requests also stops it from creating tags
or releases. The manual fallback is in `kit-release.yml`'s header comment.

## protect-main and the kit

`protect-main` requires `validate`, which every pull request into `main` runs.
It does not require `kit-ci`.

Adding `kit-ci` as a required check on `protect-main` would guard the
`release-kit/* -> main` pull request, where the `kit/VERSION` bump check
runs, but it would also block every plugin release pull request that touches
no kit path, because `kit-ci`'s path filter keeps it from ever reporting
there. Requiring it is therefore only safe alongside a second `protect-main`
condition or ruleset scoped to kit-only pull requests, which rulesets cannot
express by changed path today. Until then the bump check is enforced by
`kit-ci` running on the pull request, not by a required check.
