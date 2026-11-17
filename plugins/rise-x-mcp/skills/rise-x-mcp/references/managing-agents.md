# Managing Agent Configurations

An **agent** on the Rise-X platform is a stored configuration, not a running process: a name,
description, system prompt, model tier, the MCP servers it may call, and hosted OpenAI tools
(`file_search`, `web_search`, …). The platform agent runtime loads the configuration on every
run. The `id` that `create_agent` returns **is** the `agent_id` used to run the agent
(`POST /api/v1/agent/run`) and to wire it into federated apps, so treat it as stable from
creation.

The five agent tools manage the stored configuration only; none of them runs an agent. Their
descriptions give every argument, the field shapes, the PATCH rules, and the secret redaction.
This file covers what a saved configuration does at run time.

## Choosing a model

An agent names a **model tier**, not an OpenAI model; the runtime maps each tier to a model and
reasoning effort on every run. Pick `balanced` when unsure.

Agents saved before tiers existed can still carry `gpt-5.6-luna`, `gpt-5.6-terra`, or
`gpt-5.6-sol`. They keep running: `gpt-5.6-luna` runs as `fast`, the other two as `balanced`.
`update_agent` rejects those names, so never copy one from `get_agent` into an update; pass a
tier.

## Writing the system prompt

`systemPrompt` is the agent's standing instructions on every run.

- Open with the role and the job in one or two sentences.
- State the scope: what is in bounds, what is out, and what to refuse or hand to a human.
- Name each tool and when to use it. The runtime wires in `mcpServers` and
  `defaultOpenAiTools`; the prompt is what makes the agent pick the right one.
- Specify the output contract: format, tone, and language.
- Make behaviours concrete ("confirm with the user before creating or deleting records"), not
  vague ("be careful").
- Keep it as short as clarity allows. The runtime rejects a run when `systemPrompt` exceeds
  65,536 characters; saving a longer one succeeds.

## Editing MCP servers with stored secrets

Stored `apiKey` values and secret-looking headers read back as `"***"`. To change an agent
that has them:

1. `get_agent(agent_id)`.
2. Edit only what changes, and leave unchanged secrets as `"***"`.
3. Send the **complete** `mcp_servers` list to `update_agent`. A non-empty list replaces the
   whole list, so a server you leave out is dropped.

`"***"` resolves to the stored secret only for a server with the same `name` **and** the same
`url` it was read from. Renaming a server or pointing it at a new URL needs the real secret
again.

## What saves but fails at run time

`create_agent` and `update_agent` validate shape only. A configuration can save cleanly and
still fail every run. A runtime check failure rejects the **whole** `POST /api/v1/agent/run`
call up front; it never drops only the offending server or tool.

| Setting | Saves | Runs |
|---|---|---|
| `authType` | `None`, `Bearer`, `ApiKey`, `CallerToken` | only `None` and `CallerToken`. A saved `Bearer` or `ApiKey` server fails the entire run, because the runtime cannot read the redacted secret |
| `CallerToken` reach | not checked | forwards the signed-in user's own token, only to hosts on a server-configured allowlist; private, loopback, and link-local URLs are blocked |
| `model` | the three tiers | the three tiers plus the legacy `gpt-5.6-*` names on old agents |
| `systemPrompt` length | any | up to 65,536 characters |
| `defaultOpenAiTools[].name` | any non-blank string | `file_search`, `web_search`, `code_interpreter`, `image_generation` only |
| `file_search` `config` | any dict | needs a non-empty `config.vector_store_ids` (snake_case, passed through verbatim) |
| `mcpServers` count | up to 50 | a server-configured cap, 5 by default |
| `defaultOpenAiTools` count | up to 50 | a server-configured cap, 8 by default |

## Where a vector store id goes

Put a store id in the agent's `file_search` `config.vector_store_ids` only for a corpus every
user of the agent should search. A corpus for one conversation or one work item goes on
`vector_store_ids` in the run call instead. Create stores with `create_vector_store`; see
`references/managing-vector-stores.md`.
