---
name: simulator-ai-reactions
description: Read reaction counts under an actor with the public Simulator AI reactions-read profile. Use when the user asks for comment, view, approval or other reaction totals.
---

Use the installed `simulator-ai` catalog. This version requires `reactions-read`
and `reaction_stats_get`; it is a desktop read workflow, with the host's own agent.
It does not add comments, mark reactions read, grant access or start a runtime.

1. Inspect the tool's declared schema. In host-bound mode it accepts only `{}` and
   targets the host actor; workspace metadata tools are absent. Do not infer or
   change that target. Otherwise resolve the intended actor from the user's context
   or `actor_get`, keeping the configured workspace and exact ID.
2. Call `reaction_stats_get` with `{actorId}`, or `{}` in host-bound mode. Workspace and API-user credentials
   belong to the host, never tool arguments. The API rechecks root View and scope.
3. Report the returned per-type counts. Preserve an explicitly returned zero;
   an absent type has no eligible counted rows. `{}` means no eligible reactions,
   not proof of no private/hidden reactions. Hidden counts require Execute and
   recipient restrictions remain independent. Do not infer another user's counts.
4. On a transport, scope, permission or malformed-result error, report the failed
   read. Never render it as zero or reuse a prior count as currently authorized.
   A deliberate retry repeats the same actor ID under the current configuration;
   the tool performs no automatic retry or authorization caching.
5. For reaction text, authors, filtered counts or writes, use only a separately
   available accepted capability. Missing tools stop that part of the workflow;
   do not use private RPC, legacy `/papi`, SQL or another identity as a fallback.

Parity fixture: `parity/getReactionsStats`. The legacy response's outer `data`
wrapper is removed; per-type fields and absent/zero distinctions are retained.
Current backend recipient and cache protections are required even if a historical
legacy installation had a broader counter cache. The `reactions-read` profile provides counts only. Console profiles separately expose
reaction list/create/update/delete/mark-read tools; use them only when present and authorized.
A hidden reaction is an executors' note: a reply to one (`parentId`) is always hidden, whatever
`hidden` says, and only a user who reads the note may answer it — anyone else gets "Parent actor
not found". Do not try to show such a reply with `reaction_update {hidden: false}` (it fails with
`notAllowed`). A hidden reaction never reaches a live meeting transcript, and hiding a transcribed
one takes its line out.
Pinned reactions are unsupported.
