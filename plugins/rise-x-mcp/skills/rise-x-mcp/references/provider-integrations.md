# Provider integration skills

Use a provider skill for account setup, executable scripts and vendor-specific
contracts; keep the common Rise-X integration authoring protocol in
[integration-authoring.md](integration-authoring.md). Before any MCP call, load
the main `rise-x-mcp` skill as usual. A provider recipe does not bypass its secrets
or mutation protocol.

| Provider/service | Skill | Scope |
| --- | --- | --- |
| Microsoft 365 Outlook calendar | [integrate-microsoft-outlook-calendar](../../integrate-microsoft-outlook-calendar/SKILL.md) | Shared mailbox setup, dedicated Entra identity, Exchange mailbox-scoped access, six Rise-X endpoints, HTML invites and controlled lifecycle testing; pilot readiness, Teams deferred |

For future providers use `integrate-<provider>-<service>`. Each skill should bundle
or reference installable public resources, define supported account/cloud models,
admin versus runtime privileges, secure credential acquisition, fixed versus
runtime endpoint values, paging/retry/notification behavior, resumability and
cleanup, app bindings, and separate offline/live acceptance evidence. Keep tenant
configuration outside the installed skill. Add routing here and in the app skill
when applicable. Do not publish empty provider placeholders or copy the common
authoring protocol into each recipe.
