# Provider integration evaluation cases

These are evaluation inputs and expected outcomes, not recorded agent runs.
Use fixtures or an isolated app checkout; external writes need the user's
existing authorization for the specified mailbox and recipients.

| Request / fixture | Expected outcome |
| --- | --- |
| Connect our shared Outlook calendar to an app | Routes to the provider skill, distinguishes shared application access from personal sign-in, reports draft status and missing deployment inputs. |
| Add calendar lookup to an existing integration with unrelated endpoints | Reads/reuses actual IDs, preserves unrelated endpoints and credentials, never copies ciphertext into a new integration. |
| Query two different date ranges with the same endpoint | Runtime data changes; no saved sample parameter shadows from/to. |
| Update two events with the same endpoint | Opaque eventId and all required subject/start/end fields come from each invocation; no fixed sample ID. |
| Configure endpoints without available MCP tools | Supplies a manual configuration path and identifies missing capabilities; no invented tool or applied-success claim. |
| Test CreateEvent in a named authorized test mailbox | Recognizes a real external write, uses the authorized scope and stored transaction identity, records the returned ID and cleanup limits. |
| Add a user to an invite using this draft | Explains the additional attendee-only endpoint/typed-array validation; preserves existing recipients; does not confuse attendees with mailbox permissions or claim the bundled template already supports invites. |
| HTTP 200 with success false, or response containing nextLink | Reports operation failure or incomplete query; does not accept transport success as completed calendar work. |
| Timeout after create, or missing event ID | Retains transaction identity and reconciles; no unrelated new create. |
| Request PowerShell setup from this draft | Identifies scripts as pending lab work; never refers to a nonexistent executable as delivered. |
| Add another cloud provider | Follows the provider/service convention, checks actual auth/engine capability, uses app evidence before promotion, and does not advertise unimplemented support. |

## Static checks recorded for this draft

On 2026-10-07, the new skill passed quick_validate; marketplace/MCP/apps strict
plugin validation and feature-branch version checks passed. JSON parsed and
relative references resolved. The TypeScript example in `references/apps.md`
type-checked against local `@rise-x/apps-sdk` 0.16.1 declarations with TypeScript
strict mode. This establishes type compatibility, not a deployed-version promise.
No agent scenario runs, tenant tests, or recipient-delivery checks have been
performed; the cases above and demo acceptance remain pending.
