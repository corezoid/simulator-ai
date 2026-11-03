# Reaction statistics

`SIMULATOR_AI_PROFILE=reactions-read` exposes the three default metadata tools plus
`reaction_stats_get`. It accepts only the configured API-user key; no delegated
Console token, hidden runtime, write tool, billing or private/legacy fallback.
The portable `simulator-ai-reactions` skill describes this bounded workflow.

Set optional host-only `SIMULATOR_AI_ACTOR` with `reactions-read` to bind the process
to one actor. Its catalog then contains only `reaction_stats_get`, with empty `{}`
arguments; supplying any actor/workspace argument is rejected. The client injects
the configured actor and backend ACL still applies on every call. Other profiles
reject this variable rather than silently ignoring it. This is the legacy actor
binding for this capability, not a delegated credential or Console integration.

| Legacy capability | Public tool / RPC | Behavior |
| --- | --- | --- |
| `getReactionsStats(actorId)` | `reaction_stats_get(actorId)` → `reaction/stats/get(rootActorId)` | Dynamic type→integer map, same actor and host-injected workspace; outer legacy `data` wrapper removed |

The backend requires `actors.readonly`, workspace authorization and current root
View. Execute permits hidden reactions but does not widen immutable recipient
access. API users cannot read human-recipient reactions. Shared legacy counters
are not authorization evidence; every request is a fresh current-authority read.
The tool preserves absent keys, `{}` and explicit zero values, rejects null,
fractional, negative or malformed counters and never converts an API error to zero.
No ordering/pagination or per-author metadata is implied by this aggregate map.

```json
{"name":"reaction_stats_get","arguments":{"actorId":"ACTUAL_ACTOR_ID"}}
```

A successful result such as `{"comment":2,"view":0}` leaves all other keys absent.
In the host-bound catalog use `{"name":"reaction_stats_get","arguments":{}}`.
A denied/missing actor returns the public API error, without counters. Tools cannot
supply `accId`, another endpoint, `with`, scopes or credentials. The HTTP client
never automatically retries. Scope/revoke failures must be surfaced to the user.

The source contract is pinned in `contracts/reactions-openrpc.json`. Refresh with
`cmd/gen-graph-contract -profile reactions-read` from a reviewed local public
OpenRPC and record its backend revision/digest. Contract pin/input/output tests,
malformed-result tests and real stdio catalog tests accompany this tool. Backend
functional acceptance must exercise the actual binary and current ACL/recipient/
Execute/cache/revoke behavior before the named row is accepted. Schema presence
alone is not legacy parity, and this skill does not close the full reactions skill.
