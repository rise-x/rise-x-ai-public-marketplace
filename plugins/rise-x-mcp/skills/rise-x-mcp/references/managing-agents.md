# Managing Agent Configurations

An **agent** on the Rise-X platform is a stored configuration, not a running process. It bundles a
display name, description, system prompt, model, a list of MCP servers it's allowed to call as
tools, and a list of hosted OpenAI tools (`file_search`, `web_search`, …). The **platform agent
runtime** loads this configuration and executes it on every run. The `id` returned by
`create_agent` **is** the `agent_id` — the same handle used to run the agent (`POST
/api/v1/agent/run`) and to wire it into federated apps that embed it. Treat it as stable from the
moment it's created.

> **Scope:** these 5 tools manage the agent's **stored configuration** only — they never run an
> agent. What saves through this API is not always what the runtime will accept at run time; see
> §Config API vs. Runtime Capability Matrix below before assuming a saved config actually works.

## Tool Inventory

| Tool | What it does |
|---|---|
| `list_agents(page=1, page_size=20, search?)` | Paginated listing in the active ecosystem; `page_size` caps at 100 server-side; `search` is a case-insensitive substring match on `name` |
| `get_agent(agent_id)` | Full config (`RiseAgentPoco`) by id — 404 for an unknown id, one from another environment, or a soft-deleted agent |
| `create_agent(name, model, description?, system_prompt?, mcp_servers?, default_open_ai_tools?)` | Creates a new agent; the server assigns the `id` |
| `update_agent(agent_id, name?, description?, system_prompt?, model?, mcp_servers?, default_open_ai_tools?)` | PATCH — only the parameters you pass are sent; see §PATCH Semantics |
| `delete_agent(agent_id)` | Soft-delete; the id 404s on further `get_agent`/`update_agent` calls |

`agent_id` is UUID-validated client-side on every tool that takes one — a malformed id fails fast
with a `validation` error and no network call is made.

## Wire Schema (`RiseAgentPoco`)

Fields are camelCase on the wire:

| Field | Type | Notes |
|---|---|---|
| `id` | GUID | server-assigned; also the `agent_id` for `POST /api/v1/agent/run` |
| `name` | string | required, non-blank |
| `description` | string \| null | free text |
| `systemPrompt` | string \| null | free text; runtime caps it at 65,536 characters — see §Writing the System Prompt |
| `model` | string | required, non-blank; **not** validated against the runtime's model allowlist at save time — see §Choosing a Model |
| `mcpServers` | list of `McpServerConfig` | max 50 entries |
| `defaultOpenAiTools` | list of `OpenAiToolConfig` | max 50 entries |
| `lastModified` / `lastModifiedBy` | — | server-managed, read-only |

`McpServerConfig` (one entry of `mcpServers`):

| Field | Type | Notes |
|---|---|---|
| `name` | string | required, unique per agent |
| `url` | string | required, absolute `http(s)://` |
| `transport` | `"Http"` \| `"Sse"` \| `"Stdio"` | default `"Http"`. **`"Stdio"` is rejected client-side** — see Pitfalls |
| `authType` | `"None"` \| `"Bearer"` \| `"ApiKey"` \| `"CallerToken"` | default `"None"`. `Bearer`/`ApiKey` require a non-blank `apiKey` |
| `apiKey` | string \| null | required when `authType` is `Bearer`/`ApiKey`; always redacted to `"***"` on read |
| `headers` | dict[str, str] \| null | any header whose **name** matches `auth\|token\|key\|secret\|password\|cookie\|credential\|signature` (case-insensitive) is redacted to `"***"` on read |

`OpenAiToolConfig` (one entry of `defaultOpenAiTools`):

| Field | Type | Notes |
|---|---|---|
| `name` | string | required; the tool name as understood by the OpenAI Responses API (e.g. `file_search`) |
| `config` | dict \| null | tool-specific config, passed through **verbatim** — keys are NOT camelCased (e.g. `vector_store_ids` stays snake_case) |

## Choosing a Model

The platform supports exactly three models — any other model name is **rejected at validation**
when the agent runs (see §Config API vs. Runtime Capability Matrix):

| model | tier | good for |
|---|---|---|
| `gpt-5.6-luna` | economical default | simple, high-volume agents |
| `gpt-5.6-terra` | mid | everyday agents needing more capability |
| `gpt-5.6-sol` | premium | demanding agents where quality matters most |

All three share the same 1,050,000-token context window and 128,000-token max output, so the
choice is about cost and capability, not window size. All three also carry a pricing cliff: prompts
above 272,000 input tokens are billed at 2x input / 1.5x output for the **entire** request, not
just the tokens over the line — keep prompts under that threshold where practical.

## Writing the System Prompt

`systemPrompt` is the agent's standing instructions, applied on every run. Follow prompt
best practices when authoring one:

- **Open with role and job** in one or two sentences ("You are X. You help users do Y.").
- **Scope it explicitly** — what's in bounds, what's out, and what to refuse or hand off to a
  human.
- **Name the agent's tools and when to reach for each** — the runtime wires in the `mcpServers`
  and `defaultOpenAiTools`, but it's the prompt that makes the agent pick the right one at the
  right moment.
- **Specify the output contract** — format, tone, and language the consuming app or user expects.
- **Make behaviors concrete** ("always confirm with the user before creating or deleting
  records"), not vague ("be careful").
- **Keep it as short as clarity allows** — quality beats length, and the runtime rejects a run
  outright when `systemPrompt` exceeds 65,536 characters.

## Lifecycle: Create → Verify → Update → Delete

```yaml
# 1. Create
create_agent(
  name: "Support Bot"
  model: "gpt-5.6-terra"
  system_prompt: "You help customers track their shipments."
  mcp_servers:
    - {name: "kb", url: "https://mcp.example.com/kb", transport: "Http",
       authType: "CallerToken"}
  default_open_ai_tools:
    - {name: "web_search"}
)
# → ok: true, id: "11111111-1111-1111-1111-111111111111", changed: [...]

# 2. Verify
get_agent(agent_id: "11111111-1111-1111-1111-111111111111")
# → full RiseAgentPoco — confirm name/model/systemPrompt/mcpServers landed as sent

# 3. Update (partial)
update_agent(
  agent_id: "11111111-1111-1111-1111-111111111111"
  description: "Now covers returns too"
)
# → ok: true, changed: ["description"]

# 4. Delete
delete_agent(agent_id: "11111111-1111-1111-1111-111111111111")
# → ok: true, id: "11111111-1111-1111-1111-111111111111"
# get_agent on this id now 404s (error.code: http_404)
```

`list_agents(search="Support Bot")` finds it by name at any point in the lifecycle. Its response
uses the shared list envelope plus the API's full match count:
`{items, returned, skip, limit, hasMore, nextSkip?, total}`. `nextSkip` is present only when
`hasMore` is true. The *input* parameters are `page`/`page_size`, not `skip`/`limit`; to page
forward, increment `page` rather than feeding `nextSkip` back in as `page`.

**Always check `warnings[]`** after `create_agent`/`update_agent` — same rule as every other
mutation tool in this skill (see main SKILL.md § Response Envelopes & Verification).

## PATCH Semantics (`update_agent`)

| Value passed | Effect |
|---|---|
| omitted / not passed | field left unchanged (FastMCP can't distinguish "omitted" from "explicit null", and the API treats both the same) |
| `""` on `description` / `system_prompt` | clears the stored value |
| `[]` on `mcp_servers` / `default_open_ai_tools` | clears the whole list |
| non-empty list on `mcp_servers` / `default_open_ai_tools` | **replaces the whole list** — no per-item merge or append; include every server/tool you want to keep |
| non-blank `name` / `model` | overwrites; a blank value is rejected client-side |

## Secret Redaction & Round-Trips

- `apiKey` and any header **value** whose header **name** matches
  `auth|token|key|secret|password|cookie|credential|signature` (case-insensitive) come back as the
  literal string `"***"` from `get_agent`, `list_agents`, `create_agent`, and `update_agent` — there
  is no way to read a stored secret back through these tools.
- `create_agent` **rejects** a literal `"***"` for `apiKey` or a redacted header value outright —
  there's nothing to round-trip on create.
- `update_agent` **accepts** `"***"` — the API resolves it back to the currently-stored secret, but
  only for an MCP server with the **same `name`** it was read from. Renaming a server, or sending
  `"***"` for a brand-new server, is rejected.
- Practical pattern: `get_agent` → edit only the fields that change → send the full `mcpServers`
  list back with unchanged secrets left as `"***"` → `update_agent`.

## Config API vs. Runtime Capability Matrix

`create_agent`/`update_agent` validate shape — required fields, enums, URL format, list caps — but
do **not** enforce everything the agent runtime needs to actually execute a run. A config can save
cleanly and still fail when it's run.

| Capability | Config API (`create_agent`/`update_agent`) | Agent runtime (`POST /api/v1/agent/run`) |
|---|---|---|
| `transport` | `"Http"` / `"Sse"` accepted; **`"Stdio"` rejected client-side** by the MCP tools (the runtime can't run Stdio servers yet) | same restriction |
| `authType` | `"None"` / `"Bearer"` / `"ApiKey"` / `"CallerToken"` all accepted and saved | **only `"None"` and `"CallerToken"` are honored.** A saved `"Bearer"`/`"ApiKey"` MCP server fails the **entire run** — stored secrets are always redacted on read, so the runtime has no way to resolve them yet |
| `CallerToken` reach | not checked | forwards the signed-in user's own token to the MCP server's host; the runtime only allows this to a server-configured allowlist of vetted hosts, and blocks private/loopback/link-local URLs outright |
| `model` | any non-blank string accepted and saved | must be one of the three supported models — `gpt-5.6-sol`, `gpt-5.6-terra`, `gpt-5.6-luna`. Any other model name (including older `gpt-5`/`gpt-5.1`/`gpt-4*` names) is rejected at validation and fails the run |
| `systemPrompt` length | any length accepted and saved | rejected at run time above 65,536 characters |
| `defaultOpenAiTools[].name` | any non-blank string | must be one of `file_search`, `web_search`, `code_interpreter`, `image_generation` — anything else fails the run |
| `file_search` config | `config` dict accepted as-is, no shape check | requires a non-empty `config.vector_store_ids` list (snake_case key, passed through verbatim) — missing it fails the run |
| `mcpServers` count | up to 50 | server-configured cap (default 5) |
| `defaultOpenAiTools` count | up to 50 | server-configured cap (default 8) |

A runtime validation failure rejects the **whole** `POST /api/v1/agent/run` call up front — it is
not a partial or best-effort degradation of just the offending server or tool.

## Pitfalls

1. **`"Stdio"` transport is always rejected** — client-side, before any network call, on both
   `create_agent` and `update_agent`. The agent runtime can't run Stdio MCP servers yet; use
   `"Http"` or `"Sse"`.
2. **`"***"` on `create_agent` is rejected** — the redacted-secret sentinel only round-trips
   through `update_agent`, and only for an MCP server with the same `name` it was read from.
3. **A non-empty `mcp_servers` / `default_open_ai_tools` list on `update_agent` REPLACES the whole
   list** — there is no merge. `get_agent` first, edit in memory, and send the complete list back
   (with unchanged secrets left as `"***"`), or you'll silently drop servers/tools you meant to
   keep.
4. **`Bearer`/`ApiKey` MCP servers save fine but fail the agent at run time** — the config API
   happily persists them; `POST /api/v1/agent/run` rejects the whole run the moment it loads a
   server with either auth type, because the runtime can never see the redacted secret. Use
   `"CallerToken"` (forwards the caller's own token) or `"None"` for a server the agent must
   actually reach today.
5. **`file_search` needs `config.vector_store_ids`** — a `defaultOpenAiTools` entry named
   `file_search` with no `vector_store_ids` (or an empty list) in its `config` saves fine and fails
   at run time. The key is snake_case inside `config` — the dict passes through verbatim, not
   camelCased like the rest of the wire format.
6. **`model` isn't checked against the runtime allowlist at save time** — `create_agent`/
   `update_agent` only reject a blank string. A typo'd or unsupported model name saves without
   complaint and only surfaces as a run-time failure.
7. **Always check `warnings[]`** — same rule as every other mutation tool in this skill. A
   `dropped_property`/`dropped_item` warning means part of your `mcpServers`/`defaultOpenAiTools`
   request did not persist.
8. **`list_agents` takes `page`/`page_size`, not `skip`/`limit`** — its response is
   `{items, returned, skip, limit, hasMore, nextSkip?, total}`, with `nextSkip` only when another
   page exists. Increment `page` to move forward; don't recompute `skip` yourself.
9. **`agent_id` must be a real UUID** — `get_agent`/`update_agent`/`delete_agent` validate the
   format client-side and fail fast (`validation` error, no network call) on anything else, so a
   copy-paste mistake is caught immediately instead of surfacing as a confusing 404.
