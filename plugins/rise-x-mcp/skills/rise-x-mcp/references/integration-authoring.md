# Rise-X MCP Integration Authoring Skill

This reference is the **elicitation + safety harness** for the Rise-X integration tools exposed by the Rise-X MCP server (the `integration_tools.py` module, maintained in the `rise-x-ai` repo — not in this marketplace repo). The domain reference (RiseIntegrationPoco shape, lifecycle, decryption, deletion semantics) lives alongside this file at `integrations.md` — read that for *what* the fields mean. This file is about *how* to gather them from the user without guessing.

## Core principle

> Authoring an integration with a wrong `baseUri`, a wrong `responseValuePath`, or a leaked / mis-typed secret silently corrupts every flow that uses it. **Never invent these values.** If a slot is missing, either (a) derive it from an existing integration in the same ecosystem, or (b) ask the user — in one batched `AskUserQuestion`, not turn-by-turn.

## Mandatory authoring protocol

Follow this sequence for every `update_integration` / `update_integration_endpoint` / `import_integrations` call.

### 1. Introspect first

Before asking the user anything, run:

1. `get_active_ecosystem()` — confirm an ecosystem is set (per the main skill's pre-flight checklist).
2. `list_integrations()` — see what already exists. Reuse names, parameter conventions (`{$.baseUri}/v1`, `Authorization: Bearer {$.token}`), and authentication patterns. **Do not invent a new convention if a sibling integration already uses one.**
3. If the user named a target integration, `get_integration(name=...)` to load its current state — then mutate, never recreate.

### 2. Identify the slots you need

The required slots depend on the operation. The checklists below are the **minimum** — never call the tool with any of these unknown.

#### `update_integration` (whole-integration upsert)

| Slot | Required? | How to fill |
|---|---|---|
| `name` | Always | User-provided, unique within the ecosystem |
| `baseUri` | Always | User-provided. Must be `https://...` for prod. Strip trailing `/`. |
| `description` | Soft-required | Ask if missing — short purpose string |
| `headers` | Optional | Default headers applied to every endpoint (e.g. `Content-Type: application/json`) |
| `authentication` | Always | See **auth checklist** below — fields depend on `type` |
| `parameters` | Always (can be empty list) | Each `{name, value, description, isSecret}` — secrets stay encrypted server-side. **Classify each templated value's binding mode first (§2a)** — constant vs work-data-bound; do not hardcode sample values. |
| `endpoints` | Soft-required | At least one endpoint is normally expected; ask before creating an empty shell |

#### `update_integration_endpoint` (single-endpoint upsert)

The v4 call is `update_integration_endpoint(integration_name: str, endpoint: dict)`. ``integration_name`` is now an explicit argument (the v3 quirk where the endpoint body's `name` field doubled as the parent integration name is gone). ``endpoint.name`` is strictly the endpoint's own display name.

| Slot | Required? | Notes |
|---|---|---|
| `integration_name` (arg) | Always | Parent integration name; looked up case-insensitively in the active ecosystem. Must already exist. |
| `endpoint.name` | Always | The endpoint's own display name. |
| `endpoint.endpointUri` | Always | URI suffix appended to the integration's `baseUri`. Start with `/`. |
| `endpoint.method` | Always | `GET` / `POST` / `PUT` / `PATCH` / `DELETE` — never default to `GET` silently |
| `endpoint.request` | Method-dependent | Required for `POST` / `PUT` / `PATCH`. Optional for `GET` / `DELETE` |
| `endpoint.responseValuePath` | Soft-required | JSONPath into the response (e.g. `$.data.items`). Required if downstream flows need to extract a value. Ask if the user wants a specific field. |
| `endpoint.response` | Optional | Response shape template — only needed for typed extraction |
| `endpoint.parameters` | Optional | Endpoint-level overrides; secrets here too. **Classify binding mode per §2a** — only define a parameter for values that are genuinely constant; leave work-data-bound `{$.field}` tokens undefined. |
| `endpoint.authentication` | Optional | Per-endpoint override; rare — defaults to integration-level |

#### `import_integrations`

| Slot | Required? | Notes |
|---|---|---|
| `content` | Always | Raw text of the collection export |
| `format` | Always | `"postman-v2.1"` (default) or `"rise-native"` — any other value returns HTTP 400. Confirm with the user before passing anything else. |

#### `delete_integration` / `delete_integration_endpoint`

| Slot | Required? | Notes |
|---|---|---|
| `integration_id` | Always | Required for both. A real GUID from `list_integrations` / `get_integration`. Never invent. |
| `endpoint_id` (for endpoint-delete only) | Always | A real GUID from the parent integration's `endpoints[]` list. **v4 requires both ids** — the v3 server-side reverse lookup is gone. |
| User confirmation | Always | Both are PHYSICAL deletes (no soft-delete state in v4) and surface to flows immediately. See **destructive-action protocol** below. |

### 2a. Classify each templated value's binding mode (do NOT default to hardcoded)

This is the most common authoring mistake: filling a parameter with a sample/constant value just to make the call work, then persisting it. A templated value baked in as a constant ships with the integration forever and will **never** pick up real data from a flow.

Any `{$.name}` token in `endpointUri`, `request`, or header values is resolved at call time from a merged token set (`DataSourceConfigurationHelper.BuildTokens`): integration `parameters[]` → endpoint `parameters[]` → caller / **work-item data** overrides → date/context tokens. A token therefore resolves to **a defined parameter's static value if one exists, otherwise from the work item's data** (`TokenExtensions.ReplaceTokens(data, …)`, where `data` is the work JObject).

Every templated value has three possible binding modes. **Classify each one with the user — never silently bake in a constant:**

| Mode | When | How to author it |
|---|---|---|
| **Constant** | The value never changes per call (fixed `Company`, API version, page size) | Define the parameter with the fixed `value` — e.g. `{name: "company", value: "101", isSecret: false}` |
| **Work-data-bound** | The value comes from the Rise-X work item at activity time (a project id the user selected, a location, etc.) | Put `{$.fieldName}` in the `endpointUri`/`request` and **do NOT** define a static parameter for that name — it then resolves from work data. Confirm the exact work-data field name with the user. |
| **Caller / test-only** | Supplied per-invocation for a dry run | Same `{$.fieldName}` token; pass a sample via `test_integration_endpoint(data=…)`. Dry-run only — **never persist test samples as parameter defaults.** |

Robust rule (independent of precedence subtleties): **define the parameter ⇒ constant; leave it undefined and rely on the `{$.field}` token ⇒ work-data-bound.** Any templated slot whose mode you don't yet know is a slot for the batched `AskUserQuestion` (§4): ask *"fixed value (what?) or from work data (which field)?"* per slot. The values you pass to `test_integration_endpoint` are sample data ONLY.

### 3. Auth checklist (by `type`)

`authentication.type` is one of: `None`, `Basic`, `Bearer`, `OAuth2`, `ClientCredentials`, `ApiKey`, `Sha1`, `Sha2`, `WebHook` (enum is case-sensitive on read but matched case-insensitively at runtime — prefer the canonical PascalCase).

| `type` | Required fields | Typical secret slots | Notes |
|---|---|---|---|
| `None` | `type` only | — | Endpoint must be open or auth must be supplied via headers/params |
| `Basic` | `type`, `clientId`, `secret` | `secret` (password) | Server base64-encodes `clientId:secret` at call time |
| `Bearer` | `type`, `tokenUrl`, `clientId`, `secret` | `secret` | Bearer with `ClientCredentials`-style token exchange |
| `OAuth2` | `type`, `tokenUrl`, `clientId`, `secret`, `scope?`, `grantType?` | `secret` | Default `grantType` = `client_credentials` |
| `ClientCredentials` | Same as OAuth2 | `secret` | Equivalent to OAuth2 path server-side |
| `ApiKey` | `type`, `tokenUrl`, `clientId`, `secret`, `parameters?`, `scope?`, `grantType?` | `secret` | Default `grantType` = `api_access_token` |
| `WebHook` | `type` + signing config | depends | Used for inbound callbacks; rare for outbound calls |

If the user picks a `type` and you're missing any field in its row, **stop and ask** — server-side auth call will throw at runtime, surfacing as an opaque 5xx through the activity.

### 4. Batch the questions

When you have one or more missing slots, ask everything you need in a **single** `AskUserQuestion` call with one sub-question per slot. Bad:

> 🚫 "What's the baseUri?"
> *(waits)*
> "What's the auth type?"
> *(waits)*
> "What's the token URL?"

Good:

> ✅ One question with sub-questions: `baseUri`, `authentication.type` (with options), `tokenUrl` (conditional helper text), `clientId`, "secret value (will be stored encrypted)", **and one sub-question per templated parameter: "fixed value (what?) or from work data (which field)?" (§2a)**.

Provide `multiSelect: false` and a short curated `options` array for any enum-ish slot (`method`, `authentication.type`, `error_handling`, etc.). Free-text only for URIs, JSONPaths, names, and descriptions.

### 5. Validate before commit

After assembling the payload, call **`validate_integration`** (or `validate_integration_endpoint`). It is read-only and returns a structured list of `{path, severity, message, hint}` items.

```
errors:    block the write — re-ask the user
warnings:  surface to the user, then proceed if they confirm
info:      log only
```

Do **not** call `update_integration` / `update_integration_endpoint` while any error remains. The validator is fast and side-effect-free — call it as many times as needed during slot-filling.

### 6. Dry-run before final save (when feasible)

If the integration was an *update* to a known endpoint (i.e. the `endpoint_id` already exists server-side), and a sample payload is available, call `test_integration_endpoint(endpoint_id, sample)` and show the user:

1. The `statusCode` and `success` flag.
2. The first 30 lines of `rawResponse`.
3. The current `responseValuePath` and what it would extract from the sample (or an `AskUserQuestion` confirming the path is right).

For a *new* endpoint, `test_integration_endpoint` won't have an id yet — instead persist the integration first, capture the new `endpointId` from the response, then dry-run.

**After the first successful dry-run, align the `response` mapping to the ACTUAL payload.** The keys in your `response` template (and `displayName`/`data.*`) must match the real field names in `rawResponse` — guessing them (e.g. `Name` when the API returns `ProjectName`) yields silently empty mapped fields, not an error. Read `rawResponse`, correct the field names, and re-save. Likewise prefer a `$.`-prefixed JSONPath for `responseValuePath` (e.g. `$.value`); bare paths like `value` still run but trip a validator warning.

### 6b. Post-import auth verification & repair (`import_integrations` only)

`import_integrations` converts a Postman/`rise-native` collection, but **auth does not always survive the conversion intact** — and even a perfect converter cannot invent credentials that the collection only references as variables (`{{clientSecret}}`) resolved from a Postman *environment* that isn't in the export. So treat every import as **"import → verify → repair/elicit"**, never "import → done":

1. **Verify each imported integration with `validate_integration`.** The import envelope lists them under `imported[]` (secret-free summaries). The validator flags an `OAuth2`/`Bearer`/`ClientCredentials`/`ApiKey` integration missing `tokenUrl`/`clientId`/`secret` as **errors** — this is the fast, reliable signal that auth did not come through. Also scan the envelope's `warnings[]` for `postman.auth.*` codes (`tokenUrlMissing`, `credentialMissing`, `apiKeyReview`) — the server lifts the importer's warnings up to the envelope root.
2. **If auth is incomplete, repair before use:**
   - **Missing `tokenUrl`/`scope`/`grantType`** → set them via `update_integration` (derive from a sibling integration in the same ecosystem when possible — §1).
   - **Blank secret params (`clientId`/`secret`)** → the collection used variables or omitted them. **Elicit the plaintext from the user** in one batched `AskUserQuestion` (§4) and re-save with `isSecret: true` (encrypted under the new integration's own key — never copy another integration's ciphertext, anti-pattern #10).
   - **Endpoints showing `authentication.type: None` as an override** when they should inherit → clear the per-endpoint auth (leave it unset) so they fall back to the integration auth.
3. **Confirm with a dry-run on one auth-bearing `GET` endpoint.** `authMode: ClientCredentials` + `statusCode: 200` proves the token exchange works; a `401` with `authMode: None` means the bearer token was never attached — go back to step 2. (Localizing trick from the Troubleshooting section: a `None`-auth endpoint succeeding while auth-bearing ones 401 points at the secret/auth path, not the network.)

Do **not** report an import as successful on endpoint count alone — "53 endpoints imported" with broken auth is a non-functional integration. Success = endpoints present **and** an auth-bearing endpoint returns 200.

### 7. Commit, then summarise

After the write succeeds, summarise: integration name, endpoint count, auth type, list of secret-parameter names (NEVER the values), and the integration GUID.

## MCP-layer auto-behavior (good to know)

When you call `update_integration`, the MCP tool soft-fixes three known footguns before forwarding to the API. You don't need to do these manually, but knowing they happen helps debug surprises:

1. **Plaintext secret promotion.** For every `isSecret: true` parameter where `value` is a plain string and `decryptedValue` is missing, the MCP layer copies `value` into `decryptedValue` so the server's `RiseIntegrationBuilder.Initialise` actually encrypts it. Ciphertext (hex, ≥64 chars) is left alone — re-POSTing a `get_integration` response won't double-encrypt.
2. **Endpoint id auto-assignment.** Endpoints with missing / empty / `Guid.Empty` ids get a fresh UUID. Without this, multiple endpoints with default ids hit Mongo's unique index on `endpoints._id` and the upsert fails with a 400. Specify ids yourself when stability matters (e.g. cross-referenced from flow definitions).
3. **`authentication.target` default.** When absent or `null`, set to `""`. The server's model binder treats the non-nullable property as implicitly required and rejects payloads without it. The field's purpose is to name the placeholder where the auth pipeline stores the bearer token (e.g. `"$.bearerToken"`); empty string means "no extra placeholder needed."

These are belt-and-braces with the server-side fixes (`RiseIntegrationBuilder` defensive promotion, `IntegrationConfigGrainHelper.EnsureEndpointIds`, nullable `AuthenticationConfiguration.Target`). When both layers are in sync, the upsert succeeds for any reasonable payload; when only one layer is up, the MCP side still papers over the friction.

## Secrets protocol (non-negotiable)

1. **Never echo a secret value back to the user.** When the user pastes a secret, your reply may acknowledge "received the value for `<paramName>` — stored as a secret" but must NOT include the literal text.
2. **Always set `isSecret: true`** for tokens, passwords, client secrets, API keys, refresh tokens, signing keys. Default secret name conventions: `apiKey`, `clientSecret`, `bearerToken`, `basicPassword`, `signingKey`.
3. **Secrets are write-only after upsert.** v4 deliberately removed the v3 `GET /{id}/decrypted` endpoint — there is no way to read the plaintext of a stored secret back through the MCP layer. If the user needs to rotate a secret, collect the new value and re-`update_integration` with `isSecret: true`; never offer to "show the current value first".
4. **`get_integration` / `list_integrations` return the ENCRYPTED stored blob in `value` for secret parameters** — re-POSTing that response via `update_integration` would persist the encrypted blob as a new "value" and corrupt the secret. When mutating an existing integration's non-secret fields, **strip secret-parameter `value` fields before re-POSTing** (leave the entry with `isSecret: true` and `value: null` so the server keeps the existing encrypted value). See `integrations.md` pitfall #3.
5. When the user asks "what is the value of X" for a secret, refuse politely — there is no decrypt path in v4. Confirm "value is set / value is not set" via the presence of the parameter entry, not its content.

## Destructive-action protocol

`delete_integration` and `delete_integration_endpoint` are both **physical deletes** in v4 — there is no soft-delete state, and no `undelete` route. The integration document (or endpoint entry) is removed from MongoDB immediately, the key-vault encryption secret is best-effort dropped, and flows that referenced the removed item start failing at runtime. Before calling either:

1. Show the user the integration name, endpoint name(s), and any flow references you can find (no automated lookup exists today — say so).
2. Ask one explicit `AskUserQuestion` with options `["Delete", "Cancel"]`. Do not infer consent from "yeah ok" / "go ahead" earlier in the chat — confirm at the moment of action.
3. If confirmed, call the delete and report the success/404 result.

There is no draft / publish cycle for integrations (see `integrations.md` pitfall #6) and no recovery path once a delete returns success — recreating the integration requires a fresh `update_integration` with all configuration (and re-collecting any secrets). Treat each delete as final.

## Test-tool protocol

`test_integration_endpoint` and `test_integration_endpoint_in_flow` execute **real HTTP calls** against the configured `baseUri`. Treat them as side-effecting from the downstream system's perspective.

- For `GET` endpoints: safe to dry-run freely.
- For `POST` / `PUT` / `PATCH` / `DELETE`: confirm with the user that the test environment / sandbox is OK to call before invoking. Never dry-run a `POST /orders` against a prod baseUri without explicit consent.
- `test_integration_endpoint_in_flow` requires a *real* `work_id`. Invented GUIDs return 404 or empty. Use `create_work(flow_id)` or `list_work(flow_origin_id)` first — and remember `create_work` mutates state (creates a draft work item).

## Pre-flight checklist (always)

1. Ecosystem is set (`get_active_ecosystem` returns a name, not null).
2. Slots in this file's checklist are filled.
3. `validate_integration` has been called and returned zero errors.
4. For new endpoints with a `POST` / `PUT` / `PATCH` / `DELETE` method: user has confirmed the target environment is safe to call.
5. For destructive ops: explicit `AskUserQuestion` confirmation captured.

## Tool routing (when to read this file)

| User intent | Read this file first? | Then which tools |
|---|---|---|
| "What integrations exist?" | No — read-only | `list_integrations` |
| "Show me the X integration" | No — read-only | `get_integration(name="X")` |
| "Read the real value of secret Y" | Yes (to explain the limitation) | None — v4 has no decrypt endpoint. Refuse politely, offer rotate-via-`update_integration` instead. |
| "Create / wire up a new integration to Z API" | Yes | introspect → validate → `update_integration` → test |
| "Add an endpoint to integration X" | Yes | introspect → validate → `update_integration_endpoint(integration_name="X", endpoint=…)` |
| "Import this Postman collection" | Yes | `import_integrations(format="postman-v2.1")` |
| "Test endpoint X" | Yes | test-tool protocol → `test_integration_endpoint` |
| "Delete integration X" | Yes | destructive-action protocol → `delete_integration(id)` (physical, irreversible) |
| "Delete just one endpoint" | Yes | destructive-action protocol → `delete_integration_endpoint(integration_id, endpoint_id)` |

## Anti-patterns (do not do)

1. **Calling `update_integration` with the response of `get_integration` unmodified** — re-POSTs the encrypted secret blob as a new "value" and corrupts the parameter. Strip secret-parameter `value` fields first (leave `isSecret: true`, `value: null` to retain the server-side encrypted value). See pitfall #3.
2. **Defaulting `method` to `GET`** when the user said "send data to X" — write methods change the contract, never silently choose one.
3. **Inventing a `responseValuePath`** like `$.data` because it "looks right" — extract from a real sample via `test_integration_endpoint` first, or ask.
4. **Putting auth tokens into `headers`** as literal strings instead of using the `authentication` config — they leak in logs and bypass the secret-encryption path.
5. **Calling `publish_layout` or expecting a draft step** — integrations have no publish step; every `update_integration` is live.
6. **Offering to "read the current value" of a secret** — v4 has no decrypt endpoint. The only path is rotate-via-`update_integration` with the new value.
7. **Calling `delete_integration_endpoint(endpoint_id)` without the parent id** — v4 requires both ids; the v3 reverse lookup is gone. If the user only gave an endpoint id, fetch the parent via `list_integrations` and scan `endpoints[]`.
8. **Skipping the destructive-action confirmation** — `delete_integration` and `delete_integration_endpoint` are physical, irreversible deletes. Always run the destructive-action protocol (explicit `AskUserQuestion`) at the moment of action, even if the user mentioned deletion earlier in the chat.
9. **Silently hardcoding a templated value** — baking a sample/constant into a parameter `value` just to make the call succeed. A constant ships forever and never reads flow data. Classify each `{$.x}` token's binding mode with the user first (§2a).
10. **Copying a secret's encrypted blob from one integration into another** — secrets are AES-encrypted with a **per-integration** key (`DS-{integrationId}`). An encrypted `value` from integration A is **only decryptable by A**. If you copy A's ciphertext into a new integration B, the MCP layer sees ciphertext and "leaves it alone" (correct for same-integration re-POSTs), so B stores a blob its own `DS-{B}` key cannot decrypt → the auth secret resolves to garbage → runtime **401** at token exchange. When creating or seeding a *new* integration, always supply secrets as **plaintext** so they're encrypted under the new integration's own key. Never lift a `value` blob from another integration's `get_integration` output.

## Troubleshooting

### `test_integration_endpoint` returns 401 at the token exchange
Symptom: `statusCode: 0`, `success: false`, `items: []`, and an `exceptionDetails` stack ending in `AuthenticationHelper.ClientCredentials … Request failed with status code: Unauthorized`. The OAuth token request itself was rejected — the endpoint URL/params were never reached. Candidate causes, in order to check:

1. **Secret blob copied across integrations** (anti-pattern #10) — re-save the secret as plaintext so it's encrypted under this integration's own `DS-{id}` key.
2. **Stale / rotated `clientId` or `secret`** — collect a fresh value and re-`update_integration` (`isSecret: true`).
3. **Wrong `scope`** — e.g. the Azure ARM scope `https://management.azure.com/.default` where the API expects `api://<app-id>/.default`. Compare against a sibling integration that works.
4. **The MCP-targeted deployment can't decrypt the secret** — secret decryption depends on the Key Vault config of *the API stack the MCP points at*, which may differ from where the data was validated in the app. If **every** auth-bearing endpoint 401s identically (across multiple integrations), suspect the deployment's Key Vault wiring, not the credentials.

**Localizing trick:** a `None`-auth endpoint (no secret to decrypt) succeeding while auth-bearing endpoints all 401 points the failure at the **secret-decryption path**, not the network or the test tool itself. Test a no-auth endpoint to confirm before chasing credentials.

## Reference recipes

For worked examples of each auth pattern (`Bearer`, `Basic`, `OAuth2 client_credentials`, `ApiKey`, paginated `GET`, webhook receiver), see `integration-patterns.md`.

For the underlying `RiseIntegrationPoco` / `RiseEndpointPoco` / `RiseEndpointParameter` shape, the lifecycle, secret handling, and the full pitfall list, see `integrations.md`.
