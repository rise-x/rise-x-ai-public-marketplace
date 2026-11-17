# Integration Authoring Recipes

Worked integration payloads for the common shapes. Use them as starting points and collect
every auth value from the user; never copy one verbatim. Placeholder syntax, token binding,
secrets, and the authoring protocol are in `references/integrations.md`.

An `id` of `00000000-0000-0000-0000-000000000000` marks a new record. Secret values go on the
wire as plaintext; the server encrypts them.

---

## 1. Static token or API key in a header

Use when the vendor issued a long-lived bearer token or a static key and there is no token
exchange. Authentication stays `None`; the header carries the secret parameter.

```yaml
id: "00000000-0000-0000-0000-000000000000"
name: "Acme Orders"
description: "Acme orders API, long-lived bearer token"
baseUri: "https://api.acme.example.com"
headers:
  Content-Type: "application/json"
  Authorization: "Bearer {$.bearerToken}"   # or  X-API-Key: "{$.apiKey}"
authentication:
  type: "None"
  target: ""
parameters:
  - {name: "bearerToken", value: "<paste real token>", description: "Acme-issued token", isSecret: true}
endpoints:
  - id: "00000000-0000-0000-0000-000000000000"
    name: "List orders"
    endpointUri: "/v1/orders"
    method: "GET"
    responseValuePath: "$.data"
```

---

## 2. Basic auth (username and password)

```yaml
name: "Legacy API"
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

Treat the username as a secret too; it is often sensitive.

---

## 3. OAuth2 client credentials with secret parameters

The recommended shape for any `OAuth2` / `ClientCredentials` integration: the credentials are
secret parameters referenced from `authentication` through `{$.name}`.

```yaml
id: "00000000-0000-0000-0000-000000000000"
name: "IFS10-APIM"
description: "IFS10 through Azure APIM"
baseUri: "https://apim.example.com/int/ifsapplications/projection/v1"
headers:
  Accept: "application/json"
authentication:
  type: "ClientCredentials"            # or "OAuth2": same server path
  tokenUrl: "https://login.microsoftonline.com/<tenant>/oauth2/v2.0/token"
  clientId: "{$.clientId}"
  secret: "{$.secret}"
  target: ""
  grantType: "client_credentials"
  scope: "https://management.azure.com/.default"
  scheme: "Bearer"
parameters:
  - {name: "clientId", value: "<plaintext-client-id>",     description: "App registration client id",     isSecret: true}
  - {name: "secret",   value: "<plaintext-client-secret>", description: "App registration client secret", isSecret: true}
endpoints:
  - id: "00000000-0000-0000-0000-000000000001"
    name: "GetExpenseRule"
    endpointUri: "/BExpenseClaimService.svc/GetExpenseRule()"
    method: "GET"
    responseValuePath: "$.value"
```

**Required user input:** `tokenUrl`, `clientId`, `secret`, `scope`.

Common mistakes:

- Putting the token URL in `baseUri`. `baseUri` is the resource API; `tokenUrl` is the token
  endpoint.
- Setting the same value both as a literal in `authentication.clientId` and as a `clientId`
  parameter. Use the parameter form only.
- Giving explicit endpoint ids is optional, but it keeps them stable when flows reference them.

---

## 4. API-key-style token exchange (`ApiKey`)

`ApiKey` means the vendor swaps API-key credentials for a token at `tokenUrl`. A static
`X-API-Key` header is recipe 1 instead.

```yaml
name: "Vendor Y ApiKey"
baseUri: "https://api.vendor-y.example.com"
authentication:
  type: "ApiKey"
  tokenUrl: "https://api.vendor-y.example.com/token"
  clientId: "{$.apiClientId}"
  secret: "{$.apiSecret}"
  target: ""
  grantType: "api_access_token"
  parameters:
    audience: "https://api.vendor-y.example.com"
parameters:
  - {name: "apiClientId", value: "<plaintext>", description: "Client id",     isSecret: true}
  - {name: "apiSecret",   value: "<plaintext>", description: "Client secret", isSecret: true}
```

---

## 5. Paginated GET with constants

`page` and `pageSize` are defined parameters, so they are constants on every call.

```yaml
endpoints:
  - id: "00000000-0000-0000-0000-000000000010"
    name: "List shipments"
    endpointUri: "/v2/shipments?page={$.page}&pageSize={$.pageSize}"
    method: "GET"
    parameters:
      - {name: "page",     value: "1",   description: "Page number",    isSecret: false}
      - {name: "pageSize", value: "100", description: "Items per page", isSecret: false}
    responseValuePath: "$.data.items"
```

Confirm the JSONPath against a real response; vendor docs and real payloads often disagree.

---

## 6. POST with work-data binding

No parameter is defined for these tokens, so each one reads the work item's data at activity
time.

```yaml
endpoints:
  - id: "00000000-0000-0000-0000-000000000020"
    name: "Create shipment"
    endpointUri: "/v2/shipments"
    method: "POST"
    request:
      origin:      "{$.fromLocation}"
      destination: "{$.toLocation}"
      weightKg:    "{$.weight}"
      reference:   "{$.workId}"
    responseValuePath: "$.id"
    response:
      id: ""
      status: ""
      trackingUrl: ""
```

Confirm with the user before you dry-run a write endpoint against a production `baseUri`.

---

## 7. Webhook receiver (inbound)

```yaml
name: "Vendor Z webhook in"
baseUri: ""                          # inbound: flows receive, they do not call
authentication:
  type: "WebHook"
  secret: "{$.signingKey}"           # verifies incoming signatures
  target: ""
parameters:
  - {name: "signingKey", value: "<plaintext>", description: "HMAC signing key", isSecret: true}
endpoints: []
```

Use this shape when the vendor posts signed payloads to a Rise-X-hosted URL.

---

## 8. Endpoint-level auth override

Rare: one endpoint needs different credentials from its siblings. `authentication: null` on an
endpoint inherits the integration's auth.

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
    name: "List items"
    endpointUri: "/v1/items"
    method: "GET"
    authentication: null
  - id: "00000000-0000-0000-0000-000000000031"
    name: "Delete item (admin)"
    endpointUri: "/v1/items/{$.itemId}"
    method: "DELETE"
    authentication:
      type: "Bearer"
      tokenUrl: "https://auth.vendor-w.example.com/oauth/token"
      clientId: "{$.adminClientId}"
      secret: "{$.adminSecret}"
      target: ""
parameters:
  - {name: "readerClientId", value: "<plaintext>", description: "Read-scope client id",      isSecret: true}
  - {name: "readerSecret",   value: "<plaintext>", description: "Read-scope client secret",  isSecret: true}
  - {name: "adminClientId",  value: "<plaintext>", description: "Admin-scope client id",     isSecret: true}
  - {name: "adminSecret",    value: "<plaintext>", description: "Admin-scope client secret", isSecret: true}
```

---

## Question templates

Adapt these for the single batched question; do not paste them verbatim.

```
header: "Setting up integration: <vendor>"
questions:
  - question: "Base URI of the API?"
    placeholder: "https://api.vendor.example.com"
  - question: "Authentication type?"
    options: ["OAuth2 / client credentials", "Static key or token in a header", "Basic", "Token exchange (Bearer / ApiKey)", "None"]
  - question: "Short description: what is this integration for?"
  - question: "For {$.projectId}: fixed value (what?) or from work data (which field)?"
  - question: "Test a sample endpoint after saving?"
    options: ["Yes, GET only", "Yes, including a write endpoint", "No"]
```

```
header: "Endpoint test result"
context: "GET <baseUri><endpointUri> → 200, response sample:\n<first 30 lines of rawResponse>"
questions:
  - question: "Is `<currentResponseValuePath>` the right JSONPath for the value flows will use?"
    options: ["Yes, keep it", "No, I'll give a different JSONPath"]
```
