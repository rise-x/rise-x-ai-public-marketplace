# Provider integration recipes

Use service-specific skills for provider permissions and operation behaviour;
continue to use the [integration-authoring protocol](integration-authoring.md) for
Rise-X discovery, secret handling, configuration writes, and tests.

| Provider/service | Skill | Status |
| --- | --- | --- |
| Microsoft 365 shared Outlook calendar | [integrate-microsoft-outlook-calendar](../../integrate-microsoft-outlook-calendar/SKILL.md) | Draft query/create/update recipe; app-local and live acceptance pending |

## Adding another service

Name a skill `integrate-<provider>-<service>`. Keep the entrypoint focused on the
account model, essential constraints, and routing. Put provider setup, endpoint
contracts, app binding examples, and evaluation guidance in progressive
references; include templates/scripts only when actually implemented.

Start beside a representative test app. Record supported authentication and
scopes, fixed connection targets versus runtime data, request/response mapping,
secret inputs, side effects, pagination, retry identity, errors, and tested
versions. Verify the existing Rise-X engine can express the provider's auth and
payload contracts; OAuth support does not imply support for every cloud's request
signing scheme. Report a gap instead of inventing a tool or credential mechanism.

Promote after the app's acceptance, including live provider evidence, then test
the reusable package outside that app. Reuse the generic authoring/app skills
and add narrow routing links instead of duplicating their protocols. Catalog only
implemented recipes with their actual validation status; future providers do not
need empty placeholder skills.
