# Managing Integrations

An **integration** is a configured external API that a flow calls from a `JsonEndPoint`
activity. It bundles a `baseUri`, default `headers`, an `authentication` block, shared
`parameters` (often secrets), and a list of `endpoints`. The `validate_integration` and
`validate_integration_endpoint` descriptions give every field; `update_integration` and
`update_integration_endpoint` take the same shapes. Worked payloads per auth pattern live in
`references/integration-patterns.md`.

Read this file before any mutating integration call (`update_integration`,
`update_integration_endpoint`, `import_integrations`, `delete_integration*`,
`test_integration_endpoint*`). Wrong `baseUri`, `responseValuePath`, or secret values
silently break every flow that uses the integration, so never invent them: derive a value from
a sibling integration in the same ecosystem, or ask the user.

## Platform behaviour

- **No draft or publish.** An integration is live the moment `update_integration` returns.
- **Endpoints on update.** Omit `endpoints` from `update_integration` to keep the stored ones;
  pass `[]` to clear them.
- **Placeholder syntax is `{$.name}`.** It works in `endpointUri`, `request`, header values,
  and auth fields. The `{{name}}` form is never substituted.
- **Token resolution order:** integration `parameters[]`, then endpoint `parameters[]`, then
  caller or work-item data, then date and context tokens. So a token resolves to a defined
  parameter's static `value` when one exists, otherwise to the work item's data at activity
  time. **Define a parameter and the value is a constant; leave it undefined and the
  `{$.field}` token reads work data.**
- **Test calls are real HTTP calls** against the configured `baseUri`.

### Secrets

- Secret parameters (`isSecret: true`) are AES-encrypted with a **per-integration** key, the
  Key Vault secret `DS-{integrationId}`. `delete_integration` drops that key.
- Secrets are write-only. There is no decrypt tool and no sample tool. To rotate a secret,
  send the new plaintext `value` with `isSecret: true`. When the user asks for a secret's
  value, say whether it is set; never offer to show it.
- Round-tripping the same integration (`get_integration` then `update_integration`) is safe:
  the stored ciphertext is detected by shape and left alone.
- **Never copy a ciphertext `value` into another integration.** Integration B's key cannot
  decrypt A's blob, the MCP layer leaves it alone because it looks like ciphertext, and the
  token exchange fails at run time with **401**. Seed a new integration with plaintext secrets.
- **A plaintext secret of 64 or more hex characters is not encrypted.** The MCP layer reads
  that shape as ciphertext and skips the promotion, so a hex API key or SHA-256-style token is
  stored as plaintext. Set `decryptedValue` to the plaintext yourself to force encryption.
- Literal credentials typed into `authentication.clientId` / `authentication.secret`, or a
  token written straight into `headers`, are stored in plaintext and come back on every read.
  Put credentials in secret parameters and reference them as `{$.name}`.
- Never echo a secret the user pastes. Acknowledge it by parameter name only.

## Authoring protocol

Follow this sequence for every `update_integration`, `update_integration_endpoint`, and
`import_integrations` call.

### 1. Introspect first

1. `get_active_ecosystem()`.
2. `list_integrations()`: reuse the naming, parameter, and auth conventions sibling
   integrations already use.
3. If the user named an existing integration, `get_integration(name=...)` and mutate it.
   Never recreate it.

### 2. Pick the auth type and fill its slots

| Vendor setup | `authentication.type` | Required fields |
|---|---|---|
| Open API, or a static key or long-lived token sent in a header | `None` | none; put `Authorization: Bearer {$.token}` or `X-API-Key: {$.apiKey}` in `headers` with a secret parameter |
| Username and password | `Basic` | `clientId`, `secret` (the server base64-encodes `clientId:secret`) |
| OAuth2 client credentials | `OAuth2` or `ClientCredentials` (same server path) | `tokenUrl`, `clientId`, `secret`; `grantType` defaults to `client_credentials`; `scope` is vendor-specific |
| Token exchange through `tokenUrl` | `Bearer` | `tokenUrl`, `clientId`, `secret` |
| API-key-style token exchange | `ApiKey` | `tokenUrl`, `clientId`, `secret`; `grantType` defaults to `api_access_token` |
| Inbound signed callbacks | `WebHook` (or `Sha1` / `Sha2`) | `secret` |

"API key" covers two different setups: a static header (`None`) and a token exchange
(`ApiKey`). Confirm which one the vendor uses. A preconfigured bearer token is also `None` plus
a header: `Bearer` models a token exchange and requires `tokenUrl`. When a field for the chosen
type is missing, stop and ask; the runtime otherwise fails with an opaque 5xx from the activity.

Never default `method` to `GET` when the user said "send data to X". Never invent a
`responseValuePath` such as `$.data`; derive it from a dry run or ask.

### 3. Classify each templated value

Filling a parameter with a sample value to make a call work is the most common authoring
mistake: the constant ships with the integration and never reads flow data. Classify every
`{$.name}` token with the user:

| Mode | When | How to author it |
|---|---|---|
| Constant | The value never changes per call (company code, API version, page size) | Define the parameter with the fixed `value` |
| Work-data-bound | The value comes from the work item at activity time | Use `{$.fieldName}` and do **not** define a parameter of that name; confirm the work-data field name |
| Test-only | A sample for a dry run | Pass it in `test_integration_endpoint(data=...)`; never persist it as a parameter |

### 4. Ask everything in one message

Put every missing slot in one message, one sub-question per slot, or use your runtime's
question tool. Offer options for enum-like slots (`method`, `authentication.type`). For each
templated value ask "fixed value (what?) or from work data (which field)?".

### 5. Validate before saving

Loop `validate_integration` (or `validate_integration_endpoint`) and the user until `errors` is
0. Surface warnings and proceed only when the user confirms.

### 6. Dry-run

- An existing endpoint: `test_integration_endpoint(endpoint_id, sample)`.
- A new endpoint has no id yet: save first, take the endpoint id from the result, then test.
- Show the user `statusCode`, `success`, the first 30 lines of `rawResponse`, and what the
  current `responseValuePath` extracts.
- Align the `response` template to the real field names in `rawResponse`. A guessed key (`Name`
  where the API returns `ProjectName`) yields an empty mapped field, not an error. Re-save.
- A `GET` is safe to test freely. For `POST`, `PUT`, `PATCH`, or `DELETE`, confirm with the user
  that the target system is safe to call; a test `POST /orders` on a production `baseUri`
  creates a real order.
- `test_integration_endpoint_in_flow` needs a real `work_id`. Get one from `list_work`, or
  create one with `create_work`, which creates real state.

### 7. Summarise

Report the integration name and GUID, endpoint count, auth type, and the secret parameter
**names**, never their values.

### After `import_integrations`

Auth often does not survive conversion. A collection that references `{{clientSecret}}` from a
Postman environment carries no credential at all. Treat every import as import, verify,
repair:

1. Run `validate_integration` on each integration under `imported[]`. Missing
   `tokenUrl` / `clientId` / `secret` show as errors. Also read the envelope's `warnings[]` for
   `postman.auth.*` codes (`tokenUrlMissing`, `credentialMissing`, `apiKeyReview`).
2. Repair: set a missing `tokenUrl`, `scope`, or `grantType` (copy from a sibling integration
   when one exists); collect blank secrets from the user in one message and save them as
   plaintext with `isSecret: true`; clear an endpoint-level `authentication.type: None`
   override that should inherit the integration's auth.
3. Dry-run one auth-bearing `GET`. `authMode: ClientCredentials` with `statusCode: 200` proves
   the token exchange; a `401` with `authMode: None` means the token was never attached.

Never report an import as successful on endpoint count alone.

### Deleting

`delete_integration` and `delete_integration_endpoint` are physical deletes with no undo, and
flows that call the removed endpoints start failing at once. Before either call:

1. Show the user the integration name, the endpoint names, and any flow references you found.
   No tool finds those references for you; say so.
2. Ask one explicit question with the options `Delete` and `Cancel`, at the moment of action.
   An earlier "go ahead" in the chat is not consent.

`delete_integration_endpoint` needs the parent integration id too. With only an endpoint id,
find the parent in `list_integrations` (each integration lists its endpoint stubs).

## Troubleshooting: 401 at the token exchange

Symptom: `statusCode: 0`, `success: false`, `items: []`, and an `exceptionDetails` stack
ending in `AuthenticationHelper.ClientCredentials … Unauthorized`. The token request failed
before the endpoint was reached. Check, in order:

1. A secret blob copied from another integration: re-save it as plaintext.
2. A stale or rotated `clientId` or `secret`: collect fresh values.
3. A wrong `scope`, for example `https://management.azure.com/.default` where the API expects
   `api://<app-id>/.default`. Compare with a sibling integration that works.
4. The deployment the MCP server targets cannot decrypt secrets. When every auth-bearing
   endpoint of several integrations 401s the same way, suspect that deployment's Key Vault
   wiring, not the credentials.

To localize it, test an endpoint with `None` auth. If it succeeds while the auth-bearing ones
401, the fault is in secret decryption or auth, not the network or the test tool.
