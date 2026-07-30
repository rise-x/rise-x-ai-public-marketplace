# Integration Authoring Recipes

Worked `RiseIntegrationPoco` examples for the most common shapes. Use these as starting points — never copy auth values verbatim; always re-collect from the user.

The `id` fields in the examples below use `00000000-0000-0000-0000-000000000000` to indicate a new record; the MCP layer assigns a real UUID before sending, and the server assigns parameter-level GUIDs as needed.

**Placeholder syntax.** The runtime token form is `{$.name}` (per `TokenExtensions.cs:237-242` in the Rise-X backend / `rise-x-ai` repo — not this marketplace repo). Earlier versions of this guide used `{{name}}` — that form is **not** recognised by the runtime and never substitutes. Every example below uses `{$.name}`.

**Encryption.** Secret parameters (`isSecret: true`) are AES-encrypted server-side with a per-integration key stored in Azure Key Vault as `DS-{integrationId}`. **You send plaintext on the wire** — the MCP tool auto-promotes plaintext `value` into `decryptedValue` so the server's `RiseIntegrationBuilder.Initialise` encrypts it. After `update_integration` returns, `value` holds the ciphertext (hex IV + AES blocks). See `integrations.md` for the full encryption contract.

> ⚠️ **Per-integration key — never copy a ciphertext `value` between integrations.** Because the encryption key is `DS-{integrationId}`, an encrypted blob is only decryptable by the integration that owns it. The MCP layer leaves already-ciphertext `value`s alone (correct for re-POSTing the *same* integration), so lifting integration A's secret blob into a new integration B silently stores something B's own key cannot decrypt → runtime **401** at token exchange. When creating or seeding a new integration, always supply secrets as **plaintext**.

**Parameter binding modes.** A `{$.name}` token resolves to a defined parameter's static `value` if one exists, otherwise from the work item's data at activity time. So: **define a parameter ⇒ the value is a hardcoded constant; omit it and use the `{$.field}` token ⇒ the value is pulled from work data.** Recipe #5 (`page`/`pageSize`) shows constants; recipe #6 (`{$.fromLocation}`) shows work-data binding. Classify every templated value with the user before authoring (`integration-authoring.md` §2a) — don't persist a sample value as a constant.

For the field reference and lifecycle, see `integrations.md`. This file is recipes only.

---

## 1. Bearer token (preconfigured)

Use when the vendor issued you a long-lived bearer token and there's no token-exchange endpoint. **Do NOT use `type: "Bearer"` here** — the validator requires a valid `tokenUrl` for `Bearer`/`OAuth2` (they model a token *exchange*). A preconfigured token is injected directly via an `Authorization` header with `type: "None"` (same shape as the static-API-key recipe #4).

```yaml
id: "00000000-0000-0000-0000-000000000000"
name: "Acme Bearer"
description: "Acme orders API — long-lived bearer token"
baseUri: "https://api.acme.example.com"
headers:
  Content-Type: "application/json"
  Accept: "application/json"
  Authorization: "Bearer {$.bearerToken}"   # static token injected via header
authentication:
  type: "None"           # preconfigured token → no token exchange; Bearer/OAuth2 would require a tokenUrl
  target: ""
parameters:
  - name: "bearerToken"
    value: "<paste real token>"   # plaintext on the wire; MCP promotes → DecryptedValue → server encrypts
    description: "Acme-issued long-lived bearer token"
    isSecret: true
endpoints:
  - id: "00000000-0000-0000-0000-000000000000"
    name: "List orders"
    endpointUri: "/v1/orders"
    method: "GET"
    request: null
    responseValuePath: "$.data"
    response: null
    headers: {}
    authentication: null
    parameters: []
```

**Required user input:** `bearerToken` secret value.

**Validate then dry-run** `GET /v1/orders` via `test_integration_endpoint` to confirm `responseValuePath` extracts the expected shape before persisting.

---

## 2. Basic auth (username + password)

```yaml
name: "Legacy SOAP-ish API"
baseUri: "https://legacy.example.com"
authentication:
  type: "Basic"
  clientId: "{$.basicUser}"
  secret: "{$.basicPassword}"
  target: ""
parameters:
  - {name: "basicUser",     value: "<svc-account-username>", description: "Service-account username", isSecret: true}
  - {name: "basicPassword", value: "<svc-account-password>", description: "Service-account password", isSecret: true}
```

Server base64-encodes `clientId:secret` per request — both fields are treated as secrets (the username is often sensitive too).

---

## 3. OAuth2 client credentials with encrypted params (IfsApim model)

The recommended shape for any OAuth2 / `ClientCredentials` integration. Secrets live as **parameters with `isSecret: true`** and are referenced from the `authentication` block via `{$.name}` placeholders. The MCP layer promotes plaintext into `DecryptedValue`; the server encrypts with the per-integration Key Vault password (`DS-{id}`).

```yaml
id: "00000000-0000-0000-0000-000000000000"
name: "IFS10-APIM"
description: "IFS10 via Azure APIM (production gateway)"
baseUri: "https://apim.example.com/int/ifsapplications/projection/v1"
headers:
  Accept: "application/json"
authentication:
  type: "ClientCredentials"                                           # or "OAuth2" — same server path
  tokenUrl: "https://login.microsoftonline.com/<tenant>/oauth2/v2.0/token"
  clientId: "{$.clientId}"                                            # ← references parameter below
  secret: "{$.secret}"                                                # ← references parameter below
  target: ""                                                          # required field; non-null
  grantType: "client_credentials"
  scope: "https://management.azure.com/.default"
  scheme: "Bearer"
parameters:
  - name: "clientId"
    value: "<plaintext-client-id>"                                    # MCP auto-promotes → DecryptedValue
    description: "Azure AD app registration client id"
    isSecret: true
  - name: "secret"
    value: "<plaintext-client-secret>"
    description: "Azure AD app registration client secret"
    isSecret: true
endpoints:
  - id: "00000000-0000-0000-0000-000000000001"                        # explicit GUID per endpoint
    name: "GetExpenseRule"
    endpointUri: "/BExpenseClaimService.svc/GetExpenseRule()"
    method: "GET"
    responseValuePath: "$.value"
    headers: {}
    parameters: []
  # Add additional endpoints with their own non-empty `id` values.
```

**Required user input:** `tokenUrl`, `clientId` (plaintext), `secret` (plaintext), `scope` (vendor-specific).

**Why the indirection (parameters + placeholders) instead of inline auth values?**

- Encryption only flows through the parameter pipeline — values placed directly into `authentication.clientId` / `authentication.secret` as literals are stored in plaintext.
- Parameters can be overridden per-call (caller passes a different `{$.clientId}` token value at runtime), inline auth values can't.
- `get_integration` / `list_integrations` return parameter `value`s as encrypted blobs; inline auth values would leak the plaintext in any read.

**Common mistakes**

- Putting the token URL into `baseUri`. `baseUri` is for the resource API; `tokenUrl` is for the OAuth2 token endpoint.
- Pasting the same plaintext into `authentication.clientId` *and* a `clientId` parameter — pick one (the parameter form).
- Omitting `authentication.target`. The server's model binder treats it as required; the MCP layer defaults it to `""` if absent. Don't rely on that — set it explicitly when authoring.
- Forgetting endpoint `id`s. Multiple endpoints with `00000000-0000-0000-0000-000000000000` will hit Mongo's unique index. The MCP layer auto-assigns UUIDs, but specifying them keeps your reference IDs stable.

---

## 4. API key with token-exchange (`ApiKey`)

This is *not* "API key in a header" — for that, use `type: None` + `headers`. `ApiKey` here means the vendor's flow uses an API-key style token exchange via `tokenUrl`.

```yaml
name: "Vendor Y ApiKey"
baseUri: "https://api.vendor-y.example.com"
authentication:
  type: "ApiKey"
  tokenUrl: "https://api.vendor-y.example.com/token"
  clientId: "{$.apiClientId}"
  secret: "{$.apiSecret}"
  target: ""
  grantType: "api_access_token"                 # default for ApiKey
  parameters:
    audience: "https://api.vendor-y.example.com"
parameters:
  - {name: "apiClientId", value: "<plaintext>", description: "Client id",     isSecret: true}
  - {name: "apiSecret",   value: "<plaintext>", description: "Client secret", isSecret: true}
```

If the vendor wants a static `X-API-Key: <value>` header instead, use:

```yaml
authentication:
  type: "None"
  target: ""
headers:
  X-API-Key: "{$.apiKey}"
parameters:
  - {name: "apiKey", value: "<plaintext>", description: "Vendor Y static API key", isSecret: true}
```

**Always confirm with the user which model the vendor actually uses** — the words "API key" cover both patterns and they are not interchangeable.

---

## 5. Paginated GET with extraction

Most list endpoints paginate. The `responseValuePath` decides what gets handed to the downstream flow.

```yaml
endpoints:
  - id: "00000000-0000-0000-0000-000000000010"
    name: "List shipments"
    endpointUri: "/v2/shipments?page={$.page}&pageSize={$.pageSize}"
    method: "GET"
    request: null
    parameters:
      - {name: "page",     value: "1",   description: "Page number",    isSecret: false}
      - {name: "pageSize", value: "100", description: "Items per page", isSecret: false}
    responseValuePath: "$.data.items"           # array of shipments
    response: null
```

Tokens like `{$.page}` and `{$.pageSize}` resolve from `parameters` (endpoint-level) or `data` (flow context) at activity time. Confirm the exact JSONPath against a real sample — vendor docs and actual responses disagree more often than you'd expect.

---

## 6. Write endpoint (POST) with flow-data interpolation

```yaml
endpoints:
  - id: "00000000-0000-0000-0000-000000000020"
    name: "Create shipment"
    endpointUri: "/v2/shipments"
    method: "POST"
    request:
      origin:      "{$.fromLocation}"           # flow-data token
      destination: "{$.toLocation}"
      weightKg:    "{$.weight}"
      reference:   "{$.workId}"
    responseValuePath: "$.id"                   # newly-created shipment id
    response:
      id: ""
      status: ""
      trackingUrl: ""
    parameters: []
```

**Always confirm with the user before dry-running a write endpoint against a prod `baseUri`.** The test tool actually performs the HTTP call — a `POST /shipments` on prod creates a real shipment.

`{$.foo}` resolves from work-item data at activity time (and during `test_integration_endpoint_in_flow`). It does NOT resolve in the simple `test_integration_endpoint` path — pass the values via the `data` argument there.

---

## 7. Webhook receiver (inbound)

```yaml
name: "Vendor Z webhook in"
baseUri: ""                                     # webhooks are inbound — no baseUri
authentication:
  type: "WebHook"
  secret: "{$.signingKey}"                      # used to verify incoming signatures
  target: ""
parameters:
  - {name: "signingKey", value: "<plaintext>", description: "HMAC signing key for webhook verification", isSecret: true}
endpoints: []
```

Webhook integrations are inbound — flows don't *call* them; they *receive* from them. Use this shape when the vendor will POST to a Rise-X-hosted URL with a signed payload. The `baseUri` is intentionally empty.

---

## 8. Endpoint-level auth override

Rare — used when one endpoint of an integration needs different auth than its siblings (e.g. an admin endpoint behind a different OAuth2 client).

```yaml
name: "Vendor W mixed-auth"
baseUri: "https://api.vendor-w.example.com"
authentication:
  type: "Bearer"
  tokenUrl: "https://auth.vendor-w.example.com/oauth/token"
  clientId: "{$.readerClientId}"
  secret: "{$.readerSecret}"
  target: ""
endpoints:
  - id: "00000000-0000-0000-0000-000000000030"
    name: "List items"                          # uses integration-level auth
    endpointUri: "/v1/items"
    method: "GET"
    authentication: null
  - id: "00000000-0000-0000-0000-000000000031"
    name: "Delete item (admin)"
    endpointUri: "/v1/items/{$.itemId}"
    method: "DELETE"
    authentication:                             # endpoint-level override
      type: "Bearer"
      tokenUrl: "https://auth.vendor-w.example.com/oauth/token"
      clientId: "{$.adminClientId}"
      secret: "{$.adminSecret}"
      target: ""
parameters:
  - {name: "readerClientId", value: "<plaintext>", description: "Read-scope client id",     isSecret: true}
  - {name: "readerSecret",   value: "<plaintext>", description: "Read-scope client secret", isSecret: true}
  - {name: "adminClientId",  value: "<plaintext>", description: "Admin-scope client id",    isSecret: true}
  - {name: "adminSecret",    value: "<plaintext>", description: "Admin-scope client secret", isSecret: true}
```

---

## Question templates

Snippets for the batched `AskUserQuestion` call during slot-filling. Adapt — don't paste verbatim.

### New REST integration with token auth

```
header: "Setting up integration: <vendor>"
questions:
  - question: "Base URI of the API?"
    placeholder: "https://api.vendor.example.com"
  - question: "Authentication type?"
    options: ["Bearer", "OAuth2 / client credentials", "API key in header", "Basic", "None"]
  - question: "Short description (1 line) — what is this integration for?"
  - question: "Should I test a sample endpoint after creating the integration?"
    options: ["Yes — GET only", "Yes — including a write endpoint (need confirmation)", "No, save and exit"]
```

### Filling auth values (OAuth2 example)

```
header: "OAuth2 credentials for <vendor>"
questions:
  - question: "Token URL (OAuth2 token endpoint)?"
    placeholder: "https://auth.vendor.example.com/oauth/token"
  - question: "Client ID?"
  - question: "Client secret? (stored encrypted, not echoed back)"
  - question: "Scope string? (vendor-specific — leave blank if not required)"
```

### Confirming responseValuePath after a dry-run

```
header: "Endpoint test result"
context: "GET <baseUri><endpointUri> → 200, response sample:\n<first 30 lines of rawResponse>"
questions:
  - question: "Is `<currentResponseValuePath>` the right JSONPath to extract the value flows will consume?"
    options: ["Yes, keep it", "No, I'll specify a different JSONPath"]
```

### Confirming a destructive op

```
header: "Confirm deletion"
context: "Delete integration '<name>' (id <guid>) and its <N> endpoints? This cannot be undone and any flow calling these endpoints will start failing immediately."
questions:
  - question: "Proceed?"
    options: ["Delete", "Cancel"]
```
