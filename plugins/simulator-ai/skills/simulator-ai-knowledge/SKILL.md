---
name: simulator-ai-knowledge
description: "Answer about an actor: mentions, links, what changed, its card, a value on a date, from the Simulator graph. Triggers: \"where is X mentioned\", \"що відомо про\", \"як пов'язані\", \"что изменилось\"."
---

Use the installed `simulator-ai` catalog (profile `console-full`; a Console run gets these tools
with the `knowledge` group, and `ontology_get` / `entity_context_get` with `core`). Everything
here only reads.

## Start

1. **`ontology_get` once per conversation**, before anything else: the templates (field ids,
   options, the accounts each opens), edge types with their role (`hierarchy`, `dependency` = the
   Gantt link, `transactions` = money edges), system templates and `ledgerWrite`. Keep its
   `version`: a repeat with `ifVersion` answers `notModified`. A large workspace pages
   (`hasMore`, `offset: nextOffset`); `query` or `formIds` narrow it.
2. **Find the actor** the question names: `actors_search {query}` (or `search_all`), or the id
   from the Current context. Several candidates: name them and let the person pick.

## One actor: its card in one call

`entity_context_get {actorId}` answers, in one call: the actor, its `data` with field titles,
`neighbors` by edge type and direction, `accounts` (balances), the latest `reactions`
(comments) and `history` (changes), and its Corezoid `processes`. Long values are cut (`full:
true` keeps them). `transfers` sums the money edges per counterparty: who paid it (`in`) or whom
it paid (`out`), how much, on which account; a `partial` sum is not the total
(`simulator-ai-finance`). `fields` keeps only the
sections you need; `perType` (up to 50) widens each neighbor group; `edgeTypeIds` narrows them.
Prefer it to a chain of `actor_get` + `graph_get` + `accounts_list` + `reactions_list`.

## Where is it mentioned

`actor_backlinks_get {actorId}`: `incoming` edges by type, and `mentions` — actors and comments
whose text names it (`[actor=<id>]`), each with a `snippet`; a comment also names the actor it
stands under (`rootActorId`, `rootTitle`): that is where it was said. Page with
`mentionsOffset: mentionsNextOffset`. Answer with the actor (or the comment's actor) and the
snippet; do not walk `reactions_list` actor by actor to find mentions.

**AI Console dialogs** (Events actors of the console's own conversations) mention everything the
person talked about. The searches, the card, the mentions and the walks leave them out (on a
layer search `aiDialogsHidden` counts them); send `includeAiDialogs: true` only when the person
asks about a past dialog.

## How things are connected

Edge types: `ontology_get {fields: ["edgeTypes", "edgeRoles"]}` is the list (id, name, role and
what each role means); `entity_context_get` names the type and role of each neighbor group, and
the walks and `graph_get` answer `edgeTypes` for the types they hold. Never guess a type from its
number.

- **Gantt dependencies** are the `dependency` role (type `directedInteraction`): source = the
  predecessor, target = the successor. What X depends on: `graph_traverse {actorIds: [X],
  direction: "up", edgeTypeIds: [<dependency id>], depth: 1}` (deeper: `depth` up to 3); what waits
  for X: `direction: "down"`. The same in one call: the card's `dependency` group, `direction: in`
  (predecessors) or `out` (successors).
- One step around an actor, every edge type: `graph_get`.
- Several steps: `graph_traverse {actorIds, direction: down|up|both, edgeTypeIds, depth ≤ 3}` —
  `hierarchy` by default; `down` follows source → target (children, successors, receivers).
  `denied: true` is an actor you may not see: the walk stops there; say so.
  **"How many X"** is `countsByForm` of the answer (reached actors per template, the start ones
  left out): report that number, never one counted by hand; `hasMore` means it counts only what
  was read — raise `limit` (up to 200).
- How two actors are linked: `graph_path_find {fromActorId, toActorId, edgeTypeIds?}` (at most 4
  edges, either direction unless `directed`).
- A whole tree of a tree type (`isTree` in `ontology_get`; `hierarchy` is not one, walk it with
  `graph_traverse`): `graph_tree_get {actorId, edgeTypeId}`.
- A whole layer for a person (structure, key records, balances, money between its actors):
  `layer_digest {layerId}` in one call, before anything else.
- What is on a layer, how many: `layer_stats_get` counts its nodes and edges; `layer_actors_search`
  finds one by title (`simulator-ai-graph`).

## What changed, what it was on a date

`history_list {objType: "actor", objId, from, to, field?}` (Unix MILLISECONDS; `from` exclusive;
the whole history: `from: 0`, `to`: now in ms): changes newest first with `prevValue` →
`nextValue`, who and when. `field` is the field's id or its title as on the card (it is sent as
the stored `data.<id>`, named in `fieldKey`), or a column (`title`, `links`, `account`); without
`field` link and access rows come too, so page with `offset` or narrow by the field. The value on a date is the
`nextValue` of the last change before it (or the `prevValue` of the first change after it).
A balance on a date: `account_by_id_get {id, to}` (the row id from `accounts_list`).
Payments in a month or on a date ("who paid whom in April"): `transfers_filter {from, to, dateBy:
"originalDate"}` — the business date of the payment, not when it was entered; the count with
`transfers_filter_count` and the same filter. sim-api refuses `dateBy: "originalDate"` with
`formFields` or `kind: "formFields"` (-32602: a form-field transfer has no business date).

## Answering

Cite the actors (title and id) and link them (`build_link`). Say what you could not see
(`denied`, a page left unread) instead of guessing.
