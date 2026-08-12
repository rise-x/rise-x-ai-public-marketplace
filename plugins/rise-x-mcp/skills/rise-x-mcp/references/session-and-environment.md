# Session, Ecosystem & Deployment

## Two Distinct Concepts

1. **Deployment Environment** (dev/test/prod)
   - Set at MCP server startup via `MCP_DEFAULT_ENV`
   - Determines which Rise API URL is targeted
   - **Not changeable at runtime** — fixed for the server's lifetime
   - Check with `get_active_ecosystem()` which also returns deployment info

2. **Ecosystem** (workspace / tenant name)
   - Set via `set_active_ecosystem`; the server remembers the selection
     **per user** (best-effort, in memory), so it can carry across sessions
     or be lost on a server restart — check `get_active_ecosystem()` rather
     than assuming either way
   - All API calls include it as the `Environment` HTTP header (the backend
     header name is unchanged — only the MCP-facing terminology is "ecosystem")
   - Determines which workspace's data is accessed
   - Switchable at any time — affects ALL subsequent tool calls immediately

## Tools

### `list_ecosystems(count=20, skip=0)`
List all ecosystems (workspaces) available to the current user. Paginated.

Returns: `[{"id": "guid", "name": "workspace-name", "displayName": "Display Name", "active": true}]`

### `set_active_ecosystem(ecosystem_name: str)`
Select the active ecosystem (workspace). Validates the name against the server before setting.
All subsequent tool calls will target this ecosystem.

Returns: confirmation message with ecosystem name, deployment, and API URL.

### `get_active_ecosystem()`
Get the currently active ecosystem (workspace) and deployment info.

Returns:
```json
{
  "ecosystem": "workspace-name",    // null if not set
  "deployment": {
    "name": "test",
    "displayName": "Test",
    "apiUrl": "https://api-test.rise-x.io"
  }
}
```

## Rise-X tools are deferred (client-side)

In clients that lazy-load MCP tools (e.g. Claude Code), the Rise-X tools are **deferred**: only their names are known up front, and a tool cannot be invoked until its schema is fetched via **`tool_search`** (select the tool by name first). If no Rise-X tools appear at all, the **connector is disabled** — enable/re-enable the Rise-X MCP connector for the session, then the tools attach. Two consequences worth knowing:

- A fresh session may need a `tool_search` pass (or a connector toggle) before the first `set_active_ecosystem` / `whoami` call resolves.
- `tool_search`'s argument shape is **client-specific** — some builds take a single `query` string, others a `keywords` **list** (a list-expecting build returns `keywords: Input should be a valid list` for a bare string, so wrap single terms in a list there). If one form is rejected, try the other.

## Session Initialization Recipe

Always run this before any other operations:

```
1. get_active_ecosystem()              # check existing state
2. If ecosystem is null:
   a. list_ecosystems()                # discover available workspaces
   b. set_active_ecosystem("name")     # select one
3. Proceed with work
```

## Key Behaviors

- **Switching ecosystem mid-session** is safe but changes the target for ALL tools immediately
- **Deployment environment** is fixed — you cannot switch between dev/test/prod at runtime
- **Every tool except the 3 session tools** requires an active ecosystem — calls will fail with an error message
- `list_ecosystems` uses a "bare" API wrapper (no ecosystem header) since it lists across workspaces
