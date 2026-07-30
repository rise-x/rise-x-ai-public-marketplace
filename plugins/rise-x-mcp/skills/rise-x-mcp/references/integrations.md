# Managing Integrations

> **Authoring an integration?** This reference covers field shape, lifecycle, and pitfalls. For the *protocol* of how to gather missing fields from the user (introspect-first, batched questions, secrets handling, dry-run before commit), load `integration-authoring.md` (sibling reference in this same directory) BEFORE any mutating call (`update_integration`, `update_integration_endpoint`, `import_integrations`, `delete_integration*`, `test_integration_endpoint*`).

> **Backend code references.** File/line pointers in this doc (e.g. `TokenExtensions.cs`, `DataSourceConfigurationHelper.cs`, `IntegrationConfigGrain`, `JsonApiHelper`, `RiseIntegrationBuilder`) refer to the Rise-X backend in the **`rise-x-ai`** repository — **not** this marketplace repo. They are provenance notes for backend maintainers; you don't need them to author integrations.

## What is an Integration?

An **integration** in Rise-X is a configured external API that flows can call from a `JsonEndPoint` activity. Each integration bundles:

- A `baseUri` (e.g. `https://api.example.com`)
- Default `headers` and an `authentication` configuration (Bearer, Basic, OAuth, etc.)
- A list of `parameters` shared across endpoints (often holding secrets like API keys)
- A list of `endpoints` — each with its own URI suffix, HTTP method, request/response template, headers, optional override authentication, and parameters

All integration tools target the **v4** API (`/api/v4/config/integration/...`). The MCP layer is a thin proxy — authorization, the authoritative write-path validation, encryption, and secret scrubbing all happen server-side in the `IntegrationConfigGrain` / `IntegrationGrain` pair. This server-side write-path validation is distinct from the standalone, read-only `validate_*` tools, which run client-side in the MCP server process for pre-commit iteration (see [Validating](#validating)).

## Typical Lifecycle

```
1. list_integrations()                   # see what already exists; reuse names / param conventions
2. update_integration(integration={...}) # create / persist
3. test_integration_endpoint(endpoint_id, data) or
   test_integration_endpoint_in_flow(endpoint_id, flow_id, work_id, ...)
4. Reference the endpoint from a JsonEndPoint activity in a workflow
```

Use `import_integrations(content=..., format="postman-v2.1")` for bulk onboarding from Postman v2.1 or `rise-native` collections.

**v4 retired the v3 ``get_integration_sample`` and ``get_integration_decrypted`` endpoints** — neither has a v4 replacement. Build new integrations from scratch in the validator-driven slot-filling loop (see `integration-authoring.md`), and treat secret values as write-only after configuration.

## Placeholder syntax

Anywhere a string can carry a substitution token — `authentication.clientId`, `authentication.secret`, `endpointUri`, header values, request-body strings — the runtime form is **`{$.name}`**. The matcher lives in `Diana.Core/Src/Services/TokenExtensions.cs:237-242` ("Loops through each character looking for matches in the `{$..}` pair"). Earlier docs / examples that used `{{name}}` are wrong — that form is never substituted.

Resolution order (per `BuildTokens` in `DataSourceConfigurationHelper.cs:37`):

1. Integration-level `parameters[]` (decrypted for secrets via the per-integration KeyVault password)
2. Endpoint-level `parameters[]`
3. Caller overrides (for `test_integration_endpoint_in_flow`)
4. Context tokens (date/time helpers from `DateTokensGenerator`)

## Encryption contract

Secret parameters (`isSecret: true`) are AES-encrypted server-side. The chain:

```
update_integration(payload)
  → IntegrationController.UpsertAsync
    → IntegrationConfigGrain.UpsertAsync
      → IntegrationConfigGrainHelper.UpsertAsync
        → EnsurePassword(integration.Id)            # reads or mints "DS-{id}" secret in Azure Key Vault
        → RiseIntegrationBuilder.Initialise(...).Build()
          → for each isSecret param: Value = DecryptedValue.EncryptString(integrationId, password)
```

**Wire contract.** Send plaintext on the wire — the MCP layer auto-promotes `value` to `decryptedValue` for `isSecret: true` parameters (and skips the promotion when `value` already looks like ciphertext, so re-POSTing a `get_integration` response doesn't double-encrypt). The server's `RiseIntegrationBuilder` also performs the same defensive promotion as a backstop. Either way, ciphertext is what lands in Mongo.

**Ciphertext format.** `EncryptString` outputs `IV (16 bytes hex = 32 chars) + AES-PKCS7-encrypted blocks (32 hex chars per 16-byte block)`. Minimum ciphertext length is therefore 64 hex chars. The IV is derived from the integration id; the AES key is derived from the Key Vault password.

**Reads.** `get_integration` / `list_integrations` return parameters with `value` set to the ciphertext blob and `decryptedValue` always `null`. v4 deliberately has no decrypt endpoint — there's no way to read the plaintext through the API or MCP layer. To rotate a secret, call `update_integration` with the new plaintext `value`.

**Pitfall** — see #3 below for the get-then-re-POST round-trip. The MCP layer protects against accidental double-encryption by shape-detecting ciphertext, but you should still strip secret-parameter `value` fields when mutating an existing integration's non-secret fields.

## Discovery

### `list_integrations(name: str | None = None)`
List integrations configured in the active ecosystem. Deletes are physical in v4, so removed integrations simply no longer appear (there is no `IsDeleted` soft-delete state). Optional `name` filters by case-insensitive exact match.

Returns a YAML array of `RiseIntegrationPoco_v4`. Each item includes: `id`, `name`, `description`, `baseUri`, `headers`, `authentication`, `endpoints[]`, `parameters[]`. **Secret parameter `decryptedValue` is always `null` — v4 deliberately has no decrypt endpoint.**

### `get_integration(integration_id: str | None = None, name: str | None = None)`
Look up a single integration. Provide **exactly one** of `integration_id` or `name`.

- `integration_id` path: direct `GET /api/v4/config/integration/{id}`. Returns 404-as-string if no match.
- `name` path: dispatches to `list_integrations(name=...)` and returns the single match — v4 has no dedicated name-lookup route. The tool errors clearly if more than one integration matches (defensive — names are unique within an ecosystem).

## Authoring

### `update_integration(integration: dict)`
Create or update an integration. Existing integrations are matched by `id`; omit or blank the id to create.

The body is an `UpsertIntegrationRequest` (compatible field names with the v3 `RiseIntegrationPoco`):

| Field | Type | Notes |
|---|---|---|
| `id` | str (GUID) | omit / empty for create |
| `name` | str | unique within the ecosystem |
| `description` | str | free text |
| `baseUri` | str | e.g. `https://api.example.com` |
| `headers` | dict[str, str \| None] | default headers applied to every endpoint |
| `authentication` | dict | `AuthenticationConfiguration` — mode + credentials |
| `endpoints` | list[dict] | each item is a `RiseEndpointPoco` (see below). **Omit to retain existing endpoints**; pass `[]` to clear them. |
| `parameters` | list[dict] | each item is a `RiseEndpointParameter` (see below) |

`RiseEndpointPoco`: `id`, `name`, `endpointUri`, `method` (`"GET"` / `"POST"` / `"PUT"` / `"DELETE"` / `"PATCH"`), `request` (JSON template), `responseValuePath` (JSONPath), `response` (JSON template), `headers`, `authentication`, `parameters[]`.

`RiseEndpointParameter`: `name`, `value` (str | null), `description` (str | null), `isSecret` (bool). Secret parameters are encrypted server-side; the v4 wire shape never returns `decryptedValue`.

`AuthenticationConfiguration`: `type` (`None`/`Basic`/`Bearer`/`OAuth2`/`ClientCredentials`/`ApiKey`/`Sha1`/`Sha2`/`WebHook`), `tokenUrl`, `clientId`, `secret`, `scope`, `grantType`, `scheme`, `target`. `scheme` is the **`Authorization` header prefix** the resolved token is sent with (e.g. `Bearer`) — it's a real, applied field (`JsonApiHelper` uses `endpoint.Authentication?.Scheme ?? integration.Authentication?.Scheme`), and the validator **requires it for `Basic`/`ApiKey`**. `target` is the placeholder the auth pipeline writes the resolved token into (`""` when none).

Returns the persisted `RiseIntegrationPoco_v4`.

### `update_integration_endpoint(integration_name: str, endpoint: dict)`
Add or replace a **single endpoint** inside an existing integration without sending the whole integration body.

**v4 cleanup:** unlike v3, the parent integration is referenced by an explicit `integration_name` argument — the v3 quirk where the endpoint body's `name` field doubled as the integration name is gone. `endpoint.name` is now strictly the endpoint's own display name.

```yaml
# Example call:
integration_name: "Acme API"
endpoint:
  id: "00000000-0000-0000-0000-000000000000"
  name: "Create order"             # ← real endpoint name, not parent
  endpointUri: "/v1/orders"
  method: "POST"
  request: {...}
  parameters:
    - {name: "orderId", value: null, isSecret: false}
```

Endpoints are matched by `endpoint.id` — matching id ⇒ replace in place; missing / new id ⇒ append. Returns the updated parent `RiseIntegrationPoco_v4`.

### `import_integrations(content: str, format: str = "postman-v2.1")`
Bulk-import from an external collection format. `content` is the raw text of the file (e.g. the JSON body of a Postman v2.1 export). v4 supports `postman-v2.1` (default) and `rise-native`.

Returns the standard mutation envelope: `{ok, action, entity, imported: [<secret-free summaries>], counts: {imported: N}, warnings: [{code, message, path?}, ...]}`. The server unwraps the API's `IntegrationImportResponseV4` — projecting each integration to a secret-free summary under `imported[]` and lifting the importer's own warnings (e.g. `postman.auth.tokenUrlMissing`) into `warnings[]`. Failed/partial entries surface as warnings; successful entries still apply. Always verify each `imported[]` integration with `validate_integration` before use — auth often doesn't survive conversion (see `integration-authoring.md`'s post-import protocol).

## Validating

The validators are **read-only, side-effect-free, and do not contact the API** — they run in the MCP server process. Use them freely during slot-filling to iterate on a partially-built integration before committing it.

### `validate_integration(integration: dict)`
Validate a full `UpsertIntegrationRequest` payload (the same shape `update_integration` accepts). Returns a structured report:

```yaml
ok: bool          # true iff zero error-severity items
errors: int       # count of severity=='error' (block the write)
warnings: int     # count of severity=='warning' (surface to user, then proceed if confirmed)
info: int         # count of severity=='info' (log only)
issues:           # each item:
  - path: str             # e.g. 'authentication.tokenUrl' or 'endpoints[2].method'
    severity: error|warning|info
    message: str
    hint: str             # optional, present when actionable
```

Checks performed:

- Top-level shape (`name`, `baseUri`, `authentication`, `parameters`, `endpoints`).
- `baseUri` scheme (must start with `http://` or `https://`), trailing-slash hygiene, and a hard error if it's empty when endpoints are defined.
- `authentication.type` is one of the known `DataSourceAuthType` enum values (`None`, `Basic`, `Bearer`, `OAuth2`, `ClientCredentials`, `ApiKey`, `Sha1`, `Sha2`, `WebHook` — matched case-insensitively).
- Per-type required fields (e.g. `Bearer` / `OAuth2` / `ClientCredentials` / `ApiKey` require `tokenUrl`, `clientId`, `secret`; `Basic` requires `clientId`, `secret`; `WebHook` / `Sha1` / `Sha2` require `secret`).
- `{$.paramName}` token references in auth fields resolve to a declared `parameters[]` entry.
- Each parameter has a unique `name`, an explicit `isSecret: bool`, and no obvious placeholder secret value (`changeme`, `your-secret-here`, etc.).
- Each endpoint: required `name`, `endpointUri` (leading-slash check), `method` is a valid HTTP verb, `request` body present for write methods (`POST`/`PUT`/`PATCH`), `responseValuePath` is a JSONPath shape (starts with `$`).
- Endpoint-level `authentication` overrides are validated against the union of integration-level + endpoint-level parameters.
- Duplicate endpoint names within the integration.

**When to use:** every time the user changes a field while you're authoring. Loop validate → ask → validate → … → `update_integration`. Do **not** call `update_integration` while `errors > 0`.

### `validate_integration_endpoint(endpoint: dict, integration_parameters?: list[dict])`
Validate a single `RiseEndpointPoco` payload, with an optional list of the parent integration's parameters so `{$.paramName}` references in endpoint-level auth can be resolved.

In v4 the `name` field on an endpoint body is unambiguously the endpoint's own name (the v3 overload is gone), so the validator's `endpoint.name` error means what it says.

Returns the same `{ok, errors, warnings, info, issues}` shape as `validate_integration`.

## Deleting

### `delete_integration(integration_id: str)`
**Physical delete.** Removes the integration document from the `riseIntegrations` Mongo collection and best-effort drops the integration's key-vault encryption secret (`DS-{id}`). Flows referencing the integration's endpoints start failing at runtime; there is no `undelete` route.

Authorization: environment editor. Returns 404-as-string when no integration matches the id; returns `true` on success.

### `delete_integration_endpoint(integration_id: str, endpoint_id: str)`
Remove a single endpoint from its parent integration. **v4 requires both the parent integration id AND the endpoint id in the route** — the v3 server-side reverse lookup is gone.

The integration itself is NOT touched (apart from its endpoint list) — use `delete_integration` for that.

Returns the updated parent `RiseIntegrationPoco_v4` (with the endpoint removed), or a 404-as-string when no endpoint with that id exists within the integration.

Use this to retire a single operation without touching the rest of the integration's configuration (auth, base headers, sibling endpoints, parameters).

## Testing

### `test_integration_endpoint(endpoint_id: str, data: dict)`
The simple test path. Sends `data` as the work-data JObject and returns the full HTTP trace.

v4 wraps the body as a `TestEndpointRequest` carrying only `data`. The server resolves the parent integration via the endpoint id (no work id required). The v3 `integration_name`/`endpoint_name` parameters have been removed — they were silently ignored on the wire and no longer appear in the tool surface.

Does **NOT** execute inside a flow / work context — tokens that resolve against flow data (`{$.someField}`) will not interpolate. Use this for endpoints that don't depend on workflow state.

### `test_integration_endpoint_in_flow(endpoint_id, flow_id, work_id, parameters?, request?, response_template?, json_path?, section_name?, name?, error_handling="Throw", is_array=False)`
Execute the endpoint as a flow activity in real flow + work context. Tokens like `{$.field}` resolve against the work item's data; secret parameters are pulled and decrypted server-side.

v4 wraps the activity configuration inside `RunActivityRequest.configuration`. The MCP tool keeps the flat v3-style keyword args for convenience and bundles them into the `configuration` JObject before sending.

Returns `List<IntegrationResult>` — the integration result entries the activity would have recorded; **no transaction is committed**, so work data is not mutated.

**Required:** `endpoint_id`, `flow_id`, `work_id` (must be a real work item the caller can access — obtain via `create_work` or `list_work`).

Each `IntegrationResult` entry contains:

| Field | Meaning |
|---|---|
| `message` | server-side summary |
| `code` / `statusCode` | HTTP status code from the downstream call |
| `uri` / `requestUri` | the URI the server actually called (after token substitution) |
| `requestHeaders` | headers sent (including authentication) |
| `requestBody` | request body sent |
| `response` | parsed response (when JSON) |
| `rawResponse` | raw response body |
| `authMode` | which authentication mode was applied |
| `exceptionDetails` | populated on failure |
| `executionTimeMs` | wall-clock duration |
| `success` | `true` when `code < 400` |

## Common Pitfalls

1. **Integration tools target v4** — routes live at `/api/v4/config/integration/...` (was `/api/v3/integration/...` in the deprecated path). The change matters when correlating with API logs.
2. **Secrets are write-only after upsert** — v4 deliberately removed the v3 `GET /{id}/decrypted` endpoint. There is no way to read the plaintext of a stored secret back through the MCP layer. If the user needs to rotate a secret, capture the new value and re-`update_integration` with `isSecret: true`; never try to "read then write".
3. **`get_integration` / `list_integrations` return parameters with `value` as the encrypted blob** for secrets — re-POSTing that response via `update_integration` would persist the encrypted blob as a new "value" and corrupt the secret. When mutating an existing integration's non-secret fields, **strip secret-parameter `value` fields before re-POSTing** (leave the parameter entry with `isSecret: true` and `value: null` so the server keeps the existing encrypted value). The v4 helper preserves the existing encrypted value when `value` is null and `isSecret` is true.
4. **`test_integration_endpoint_in_flow` requires a real `work_id`** — invented GUIDs return 404 or empty results. Use `create_work(flow_id)` (and capture the returned `workId`) or `list_work(flow_origin_id)` first.
5. **Import format limited to `postman-v2.1` and `rise-native`** — other formats raise HTTP 400. Convert before importing.
6. **No `publish` step** — integrations are saved live the moment `update_integration` returns. There is no draft / publish cycle, unlike flows and layouts.
7. **`error_handling="Ignore"`** still returns the failure detail in `exceptionDetails`; `success: false` and `statusCode` reflect the downstream call. The `Ignore` flag only changes what happens to the surrounding activity step in production — for the test endpoint, the response shape is identical. **Do not use `Ignore` to suppress auth or network failures during authoring** — a `401`/`5xx` still means broken configuration that must be fixed, not a passing test; never mark an integration "working" because `Ignore` let the step continue.
8. **`delete_integration` is irreversible** — physically removes the document from MongoDB and drops the key-vault secret. There is no `undelete` route. Audit usages before deleting.
9. **`delete_integration_endpoint` requires the parent integration id** — v3 took only the endpoint id and reverse-looked-up the parent server-side; v4 takes both. If you only have an endpoint id, fetch the parent via `list_integrations` and scan `endpoints[]` for the matching id.
10. **No `get_integration_sample` in v4** — start from a blank dict (or use `import_integrations` with a Postman collection) and validate iteratively.
