---
name: simulator-ai-actors
description: Create, read, update, search and filter Simulator actors — the records of a template — with the data value protocol right, through the public Simulator AI tools. Use when the user wants to create an actor or record, fill in a template, update actor data, find actors where a field has a value, rank them by a balance, set an actor's status, make a placeholder (hole), or asks which Corezoid processes an actor can call ("створи актор", "заповни шаблон", "додай запис", "процеси актора", "создай актор", "заполни шаблон", "добавь запись").
---

Use the installed `simulator-ai` catalog. `console` has `actor_get`, `actor_create`,
`actors_create`, `actor_update`, `actors_search`, `actor_by_ref_get` and `actor_attach`; `actors_filter`,
`actor_status_update`, `actor_delete`, `actor_system_get` and `actor_corezoid_processes_list`
need `console-full` (the reads are also in `read`). Template design is `simulator-ai-forms`;
links and layers are `simulator-ai-graph`; accounts are `simulator-ai-finance`.

## `data` is keyed by field id

1. Read the template: `form_get {formId}`. Each field in `sections[].content[]` has an `id`;
   that id, not the title, is the key in actor `data`. Read the template's and the fields'
   `description` for what they mean; never guess a field from its title.
2. Map each value you want to set to its field id and `class`, and build `data`:

| Field | Value in `data` |
| --- | --- |
| `edit` text, email, phone, password | string |
| `edit` int / float | number |
| `check` | boolean |
| `radio` | the chosen option's `value` |
| `select` with static `options` | array of chosen options: `[{"title":"good","value":"c2"}]` |
| `select` from actors (`layer`, `actorFilter`, `actorsBag`, `actors`, `formFilter`) | `[{"type":"actor","title":…,"value":"<actor id>"}]` |
| `select` from `forms` / `currencies` | `[{"type":"form" or "currency","title":…,"value":<id number>}]` |
| `select` from `accountNames` | `[{"type":"accountName","title":…,"value":"<name id>"}]` |
| `select` from `workspaceMembers` | `[{"type":"workspaceMembers","title":…,"value":<userId>}]` |
| `multiSelect` | array of `{title, value}` |
| `calendar` | `{"startDate":…,"endDate":…}` in Unix seconds |
| `upload` | array of file references, `[]` when empty |
| `label`, `button`, `image` | no entry: display only |

A dynamic `select` value must be a real id: resolve it first (`actors_search`, `users_search`,
`currencies_list`, `account_names_list`, `forms_list`). Never invent field ids.

**Templates in a tree.** `data` of the template you create under uses plain field ids; a
field of another template of the same tree is `__form__<thatFormId>:<fieldId>`. When the user
names a child template (`form_get` shows a `parentId`), find the root with `forms_tree_get`,
create the actor under the root, and put the child's fields under the prefix. Do not change a
template's status to work around a refusal.

## Create

Search first (`actors_search {query, formId}` or `actor_by_ref_get {formId, ref}`) so you do
not duplicate. Then `actor_create {formId, title, data, description?, ref?, color?}`; for two
or more actors use one `actors_create {actors: [...]}` (one failed item fails the call and
creates nothing). `ref` is your key, unique within the template. Without `color` an editor
colour is picked. `geoPosition {lat, lon}` and `geoName` set a location. `appId` (with
`appSettings`) embeds a Smart Form in the actor's card. `actor_create` does not place the actor
on a layer (`simulator-ai-graph`).

A **hole** (`hole: true`) is a placeholder node; its data fields are the conditions that close
it. `actor_update {hole: false}` turns it into a normal actor.

## Read and update

- `actor_get {actorId, fields: ["id","title","data"]}`; `with` adds `access`, `privs`, `owner`,
  `reactionsStats` and the like only when needed. By ref: `actor_by_ref_get {formId, ref}`.
- `actor_update {actorId, ...}` changes only the keys you send; `data` merges key by key
  (`replaceEmpty` replaces the whole object). Send `[]` to clear a multi-value field. `ref` can
  be set or changed later.
- `actor_status_update {actorId, status}` sets verified, pending, rejected, ...
- `actor_delete {actorId}` moves the actor to the trash with its placements and links. Confirm
  with the user first.

## Search and filter

- `actors_search {query, formId?, searchType?}`: by title or fragment; `searchType: "semantic"`
  for meaning. `search_all` searches users and actors in one call; `layer_actors_search` one layer.
- `actors_filter {formId, queryData, ...}`: a template's actors by field values, MongoDB-like:
  `{"status":"paid","amount":{"$gte":1000}}`. Also `linkedToActorId` (connected by a hierarchy
  edge, either direction), `ownerIds`, `from`/`to` (Unix seconds), `starred`, `search`
  (full text), `orderBy`/`orderByKey`/`orderValue`. Sort by a current balance with
  `orderBy: "balance"`, `accountNameId` and `currencyId`. Balances are real values: do not
  divide by the precision. The total is `actors_filter_count` with the same filters.
- Turnover over a period is not a filter: read each actor's accounts (`simulator-ai-finance`).

## Cards on an actor

"Add an actor / a form / a Smart Form to this actor" means a card in its view, never a comment
(reaction) or a file: `actor_attach {actorId, kind, ...}`.

- `kind: "actor"`, `targetId`: the actor card it shows (one; replaces the current one).
- `kind: "smartForm"`, `targetId`: the appId of the Smart Form it opens (one; replaces it).
- `kind: "form"`: a form card, pinned to it by a hierarchy edge, as many as needed: a new
  record `{formId, title, data}` of an account template, or an existing actor (`targetId`).
  The record keeps its own access: share it with `access_rules_save` when others should open it.

Detach a card or Smart Form with `actor_update {actorId, cardActorId: ""}` / `{appId: ""}`.

## Related

- A user's twin actor: `actor_system_get {objType: "user", objId: <userId>}`; transfers between
  people go between their twins' accounts.
- "Which processes or functions can this actor call": `actor_corezoid_processes_list
  {actorId}` lists the Corezoid processes of the API keys shared with it; details appear only
  for processes you can see in Corezoid yourself.
