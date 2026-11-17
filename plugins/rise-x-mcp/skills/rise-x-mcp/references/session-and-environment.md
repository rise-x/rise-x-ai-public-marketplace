# Session, Ecosystem & Deployment

## Two distinct concepts

1. **Deployment** (`test`, `staging`, `prod`, …)
   - Fixed at MCP server startup (`MCP_DEFAULT_ENV`); it decides which Rise API the server calls.
   - You cannot switch it at runtime. `get_active_ecosystem()` and `whoami()` report it.
   - `compare` works only between ecosystems of this one deployment.

2. **Ecosystem** (workspace or tenant name)
   - Chosen with `set_active_ecosystem`. Every tool except the 5 session tools
     (`list_ecosystems`, `set_active_ecosystem`, `get_active_ecosystem`, `whoami`,
     `resolve_ecosystem`) needs one.
   - With OAuth sign-in the server stores the selection **per user**, so it carries across
     sessions and server restarts. A server running on a single API key keeps it in memory
     only, and a restart loses it. Call `get_active_ecosystem()` at the start either way.
   - Switching takes effect for every later tool call at once.
   - A client can pin the ecosystem for a single request with the `Ecosystem` HTTP header. The
     pin overrides the stored selection for that request only.
   - The backend receives it as the `Environment` HTTP header; only the MCP-facing name is
     "ecosystem".

`get_active_ecosystem()` returns:

```json
{
  "ecosystem": "workspace-name",
  "deployment": {"name": "test", "displayName": "Test", "apiUrl": "https://api.test.auxos.io"}
}
```

`ecosystem` is `null`, with a `message`, when none is selected.

## Session initialization

```
1. get_active_ecosystem()
2. If ecosystem is null:
   a. list_ecosystems()
   b. set_active_ecosystem("name")
3. Proceed
```

## Rise-X tools are deferred (client-side)

In clients that lazy-load MCP tools, only the Rise-X tool names are known up front, and a tool
cannot be called until the client's tool-search tool fetches its schema. Select the tool by
name first. Clients that load every MCP tool up front skip this step. If no Rise-X tools
appear at all, the connector is disabled: enable it for the session, then the tools attach.

- A fresh session may need a tool-search pass, or a connector toggle, before the first
  `set_active_ecosystem` or `whoami` call works.
- The tool-search tool's argument shape depends on the client. Some builds take a single
  `query` string, others a `keywords` **list**; a list-expecting build answers a bare string
  with `keywords: Input should be a valid list`. If one form is rejected, try the other.
