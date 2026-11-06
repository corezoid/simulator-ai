# Live backend authorization checks

A mock or generated schema cannot establish live authorization behavior. Validate the
actual packaged MCP binary against the intended Simulator API using disposable API-user
identities and known test objects. Record plugin/backend revisions and the environment in
private operational evidence; do not commit keys or workspace-specific data here.

## Read and permission checks

1. Start a persistent MCP process with the intended origin, workspace and `read` profile.
   Initialize it, inspect `tools/list`, then call an allowed actor and form read.
2. Confirm string JSON-RPC request IDs round-trip correctly through the public API.
   Transport success must contain the expected ID and a valid result, not an error envelope.
3. Try an object that is not shared with the test identity and an object in another workspace.
   Both must be denied without leaking data or retrying under another identity.
4. Grant read access to a disposable object, read it, revoke that object's access, then read
   again through the same MCP process. Verify current denial, including any documented cache
   window. Check that a read-only credential cannot perform writes.
5. Verify public-audience enforcement in the backend's authorization tests. Do not add private
   methods to this plugin just to probe them.

## Credential revocation is a separate check

Object ACL revocation does not revoke an API-user key. Block the disposable API user through
its issuer's supported API, read back the issuer's blocked state, and prove the old key is
rejected after the documented cache window. A successful HTTP status from a block request
is insufficient if the issuer still reports the identity active.

Issuer-level revocation has not yet been established for the current integration. Treat it
as an unresolved rollout gate; do not describe ACL tests, short-lived token expiry, local
cache deletion or credential-file replacement as proof of permanent key revocation.

## Writes, cleanup and rollback

For write profiles, use disposable objects and test response loss, retained receipt lookup,
stale CAS rejection and current permissions. Verify the intended effect through an authorized
read, and distinguish historical receipts from current state. Follow each profile's guide;
Console retries and strict graph-create/Smart Forms recovery have different contracts.

Delete only fixtures created by the verification run. Confirm issuer-side cleanup separately
from object cleanup. Keep the previous compatible binary/backend pair available for rollback,
and verify all replicas use the intended candidate before enabling strict write profiles.

The [client smoke](client-acceptance.md) covers installation and fixture protocol calls.
Backend-only functional tests do not establish MCP installation, model execution or a deployed
fleet's behavior. No model call is necessary for these direct protocol checks.
