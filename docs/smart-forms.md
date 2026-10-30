# Opt-in Smart Forms profile (R2.1)

Set `SIMULATOR_AI_PROFILE=smart-forms` in the host environment, alongside the existing
origin/workspace/API-user key. Restart/reload the MCP server. The default remains three
metadata tools; this profile advertises sixteen tools. Codex forwards the profile variable
through the plugin's `env_vars`; Claude Code inherits the host environment. No model,
Hermes, LiteLLM or server AI billing is invoked by this executable.

The API user needs the documented `actors.readonly` / `actors.management` scopes and
normal object permissions. This plugin neither grants scopes nor bypasses actor ACL.
Do not widen sharing merely to make an app open. Creation uses existing default access-list
visibility. Environment credentials and actor data are omitted from returned projections.

## Capability mapping

| Legacy capability | Public MCP tool(s) | Public methods | Remaining gap |
| --- | --- | --- | --- |
| Smart Form create | application_create, application_operation_get | application/create, application/operation/get | No credential/sharing administration |
| pullSmartForm | application_environments_list, application_snapshot_get | application/env/list, application/snapshot/get | No filesystem pull/path manifest yet |
| pushSmartForm folders | application_folders_create, application_folders_operation_get | app_folder/create, app_folder/operation/get | No rename/delete or local sync plan |
| pushSmartForm files | application_files_create, application_files_update, application_files_operation_get | app_file/create, app_file/update, app_file/operation/get | No delete/rollback; no bulk arbitrary RPC |
| Smart Form publish | application_release_create, application_release_operation_get | release/create, release/operation/get | Requires reviewed source AND target hashes |
| Verify publication | application_release_get, application_route_key_get | release/get, route/key/get | Viewer interaction/inline reaction opening remain separate |
| Smart Form generation guidance | simulator-ai-smart-forms portable skill | Tools above | Verified starter only; full current catalog and business connector still needed |

## Contract and recovery

`contracts/smartforms-openrpc.json` pins thirteen public method descriptors; its SHA-256
is in `smartforms-source.json`. `internal/publicapi/smartforms_types.go` contains deliberate
named request/result projections; no generated file was hand-edited. `contracts/profile.go`
derives stricter tool input schemas from the public schemas: workspace and credentials are
not arguments, keys are mandatory, file updates require source plus full environment CAS,
and releases require both environment hashes. Direct Go callers get the same validation.

Before **each write**, GET `/api/2.0/doc/public` on the fixed origin verifies every pinned
method's public audience, scope and parameter schemas. Missing or changed fields fail closed;
parameter order and prose do not matter. Added methods are allowed; added/changed parameters
require a reviewed profile refresh, even when potentially compatible. This is feature
detection, not proof of transactional semantics. All replicas behind that origin must run
the compatible receipt/CAS implementation before enabling writes. A mixed old/new fleet
can serve new documentation and route the subsequent write to an old replica; do not enable
this profile during that rollout. The receipt/revision database migrations must also exist.
No migration is applied by the plugin.

There are no automatic retries. Every write uses a caller-retained operation key and original
payload. HTTP/protocol/read failures may follow a committed write; errors carry recovery
instructions. `notAvailable` can mean an unresolved in-flight duplicate. `notFound` during
receipt lookup is not proof of failure. `operationConflict` requires reconciliation;
`operationResultExpired` never authorizes a new effect. Current authorization is rechecked
by the backend on writes and receipt reads. Result projections exclude credential fields,
and malformed identities/content hashes are refused. Raw server error messages are discarded.

Source snapshots are untrusted content. Inputs are bounded to 512 KiB, outputs/contracts to
4 MiB; no redirects, arbitrary origin/workspace/RPC overrides or local file execution. These
limits intentionally bound this development profile; larger apps need a separately designed
paged/streamed sync contract. Creation defaults to CDU v2 and does not accept credentials. Its historical environment
rows do not include rootFolderId; obtain roots from a snapshot or current environment list.

After release, verify current `release/get` status/retention and `route/key/get` active id;
receipt status is historical. A short URL is a locator, not a permission grant or proof of
successful viewing. This slice does not render/submit a page, open a reaction, change legacy
routing, execute business processes, or certify full legacy parity.

## Verification and acceptance limits

`make test vet build` runs bounded Go checks; `make client-smoke` runs real client installation
without model inference. The Smart Forms tests run the real MCP handler/client against a
loopback HTTP stand-in derived from the pinned public methods. They cover all thirteen RPC
routes, strict schemas, read/write annotations, fixed workspace, credential projection,
malformed identities, exact write-result file binding, explicit retention presence, stale backend refusal, loss/recovery/no automatic retry and bounded
error reasons. The stand-in does not prove backend transactionality or viewer behavior.

Live backend receipt/CAS behavior is a separate acceptance gate. Do not describe a local
scripted profile test as a live deployment or Console acceptance. No model budget is used.

Independent review found and corrected three mismatches before acceptance: creation receipt
environments do not contain enriched root folder ids; successful file writes must match the
requested ids/count/parents; missing/null release retention is unknown, not false. Regression
tests cover each. Both portable skill walkthroughs (lost release response and existing app
update) were checked; neither permits rekeying unresolved work or skipping current-state checks.
