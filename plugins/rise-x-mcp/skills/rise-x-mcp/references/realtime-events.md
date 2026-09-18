# Realtime Events (SignalR)

The short answer for anyone writing a federated app today: **there is no
supported realtime subscription.** The server emits events, the host shell
holds a connection, and neither is reachable from an app. Refresh on the user's
own action or on window focus instead.

> **Verification status.** Everything on this page was **read from source, not
> exercised.** No event was observed arriving in a live session. Treat the
> event names and payload shape as what the server is written to send, and
> confirm against the source before depending on them. The one thing that was
> confirmed by absence is the conclusion: no published SDK surface exposes any
> of it.

## What the server emits

A SignalR hub is served at:

```
/events?environment=<env>
```

Four events are sent. **The client event name is the literal enum name.** No
camelCasing, no prefix:

| Event | Meaning |
|---|---|
| `WorkUpdated` | A work item changed |
| `WorkDeleted` | A work item was deleted |
| `DataPatch` | A data patch was applied |
| `DataObjectPatch` | A data-object patch was applied |

### `WorkUpdated` is an invalidation signal, not a state push

Its payload is `{Id, DisplayName}` and nothing else. There is no data, no
status, no step. A subscriber learns only *that* the work changed and must
re-read it:

```
1. on("WorkUpdated", ({Id, DisplayName}) => …)
     # NOTE: the payload is the whole message. No form data, no status,
     #   no activeStepName. Re-read the work to find out what changed.
2. if (Id === theWorkIAmShowing) get_work(Id)   # or the app's own query refetch
```

### Events are addressed to a USER, not to a work item

There is no per-work-item group to join. An event is sent to a **user**, fanned
out to every user in the work's ACL. A subscriber therefore receives every
event for every work item it can see, and **filters by `Id` itself**.

```
# CRITICAL: do not assume a subscription is scoped. A user watching one work
#   item receives events for all of theirs, so an unfiltered handler will
#   refetch on unrelated activity.
```

## Why an app cannot use any of this

The host shell already holds a live connection. It does **not** expose it.

`window.__DIANA_SHELL__`, the shell bridge a federated app reads, offers:

- `getApi`
- `getApiV4`
- `getAi`
- `navigate`
- user and environment subscriptions

and **nothing realtime**. No hub handle, no connection, no event emitter.

No published SDK version adds one. Shell bridge version 4 (the SDK's
`SHELL_BRIDGE_VERSION`) added offline and cache support, not realtime. So an
app cannot subscribe to the shell's connection, and opening a second
connection of its own is not a supported route.

## What to do instead, today

```
1. Refresh on the user's own action.
     # The app made the change, so it knows when to refetch. This covers the
     #   overwhelming majority of what a user notices as staleness.
2. Refresh on window focus.
     # Catches a change made in another tab or by another party while the
     #   app was in the background.
3. Do NOT poll the work on a timer as a substitute for a subscription.
     # It costs a request per interval per viewer for a change that is rare,
     #   and it still is not realtime.
```

Say this plainly to anyone asking for live updates: the platform emits the
events, the shell consumes them, and the app tier has no access. Do not imply
a subscription exists.

## If this changes

The thing to check is whether `window.__DIANA_SHELL__` has gained a realtime
member, and whether the bridge version the app resolved is new enough to carry
it. App-side SDK and shell-bridge detail, `SHELL_BRIDGE_VERSION` included,
lives in the `rise-x-apps` skill in this marketplace, not here. Until the
bridge gains a realtime member, this page stands.
