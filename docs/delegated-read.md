# Console delegated read profile

`SIMULATOR_AI_PROFILE=delegated-read` is an explicit runtime-host profile. It
exposes only `actor_get` and `form_get`, mapped to public `actor/get` and `form/get`
at the fixed `/api/2.0/rpc` endpoint. No layer reads, Smart Forms writes, arbitrary
RPC, private API or runtime/model launch are available. The desktop `read` default
and `smart-forms` opt-in retain API-user authorization and their own agent.

The trusted runtime manager supplies these process environment variables:

- `SIMULATOR_AI_ORIGIN`: explicit HTTPS origin; only literal loopback HTTP is allowed for tests.
- `SIMULATOR_AI_WORKSPACE`: the admitted workspace.
- `SIMULATOR_AI_PROFILE`: `delegated-read`.
- `SIMULATOR_AI_DELEGATED_TOKEN`: the opaque credential delivered through the manager's
  secret channel. Its value must never appear in a committed file, prompt, tool call or log.

`SIMULATOR_AI_API_KEY` must be absent/empty for this profile. Conversely, any delegated
token in a desktop profile is a configuration error. Ambiguous configurations fail
before network access; no auth fallback or implicit token refresh exists. The token
must be a canonical unpadded base64url encoding of 32 bytes. That syntax check does
not issue a grant or establish authority. Config JSON/fmt/slog projections omit or
redact both credential values.

Every request uses `Authorization: Delegated <token>`. Redirects and automatic
retries remain disabled. Only the two admitted methods can reach the transport,
even when a caller invokes a hidden client method directly. The configured workspace
cannot be replaced through tool arguments. Public API codes/reasons survive error
mapping; untrusted message text, credentials and unknown response fields do not.
An `accessDenied` response does not trigger an ApiKey retry.

The backend must explicitly enable the matching server-owned delegated read profile
and issue a short-lived task/attempt-bound grant. A catalog entry is not authorization:
the backend independently enforces grant methods/object bounds, current task state,
expiry/revocation, fresh parent/membership and current object ACL. A grant allowing
only one actor does not authorize all actors or the advertised form tool. No plugin
setting can widen that authority.

This profile does not provision Hermes, a vault, a process/container sandbox or
Console routing. Only an isolated runtime may receive it; inheriting a general user
environment is not a substitute for isolation. Token replacement requires the
manager's explicit reviewed attempt/recovery workflow. Desktop tools never acquire
server credentials or silently start Hermes.

Verification includes real stdio initialization/catalog/tool calls against a local
HTTP simulator, exact scheme/workspace binding, invalid argument rejection,
unavailable layer/write methods with zero HTTP calls, revoked-style API denial
without retry/fallback, secret redaction and unchanged desktop profiles. Separate
sim-api acceptance uses a real issued grant and the actual public MCP binary; neither
scripted test is a claim of deployed Hermes/Console readiness or paid-model evaluation.
