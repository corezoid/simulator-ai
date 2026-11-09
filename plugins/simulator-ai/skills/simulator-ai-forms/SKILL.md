---
name: simulator-ai-forms
description: Design and change Simulator templates (forms, Account Templates, «Шаблон рахунків») — sections and fields, dynamic selects, the template tree and default accounts — and find system templates, through the public Simulator AI tools. Use when the user wants to create or modify a template, add fields, define an actor schema, set a template's status, give every actor of a template an account, or asks what system templates exist ("створи шаблон", "додай поле", "шаблон рахунків", "создай форму", "добавь поле").
---

Use the installed `simulator-ai` catalog. `console` has `form_get`, `forms_list`, `forms_count`, `form_create`,
`form_fields_patch`, `form_update` and `form_account_create`; `form_status_update`, `form_delete`, the
tree reads and the rest of the account definitions need `console-full`. The records of a template are
`simulator-ai-actors`.

A template is the schema of its actors: `sections[]`, each `{title, content[]}`, and each
content item a field. The field `id` is the key of that field in every actor's `data`.

Settings of system-form actors (a layer's Accounts on the graph and default form, an event's
meeting and recurrence, a dashboard's source, a stream's order) are keys of their `data`:
[references/system-form-settings.md](references/system-form-settings.md).

## Read

- `forms_list {query?, type?}`: custom templates by default; `type: "system"` for Events,
  Layers, Graphs, Scripts and the other platform templates (their ids differ per workspace),
  `"all"` for every kind. Rows are compact (id, title, type, ref, parentId when set; `full` for
  owner, flags and dates) and have no body. Ask only for as many rows as you will show.
- `forms_count {query?, type?}`: the exact number for the same filters; answer "how many" with
  it, never by counting pages.
- `form_get {formId}` returns the full body. Its `description` and each field's `description`
  are the authoritative meaning of the template and its fields: read them before explaining a
  field, and read again rather than answering from memory. `with: ["editor"]` adds the
  `editToken` an update needs; `with: ["parents"]` the ancestor chain.

## Fields

| `class` | Field | Notes |
| --- | --- | --- |
| `edit` | text or number input | `type`: text (default), password, email, phone, int, float; `regexp`, `errorMsg` |
| `check` | checkbox | |
| `radio` | one choice | `options: [{title, value, color?}]` |
| `select` | dropdown | static `options`, or `extra.optionsSource` (below) |
| `multiSelect` | several choices | static `options` |
| `calendar` | date / time | `extra` `time`, `minDate`, `maxDate`, `dateRange` (Unix seconds) |
| `upload` | files | |
| `label`, `button`, `image` | display only | no actor data |

Common item keys: `id`, `class`, `title`, `value` (default), `required`, `description`
(helper text), `visibility` (`visible`, `disabled`, `hidden`).

`extra.optionsSource.type` of a dynamic `select`: `manual` (default, the static options),
`layer` `{id: <layer id>}`, `actorFilter` `{id}`, `actorsBag`, `actors` `{ids: [...]}`,
`formFilter` `{id: <template id>}` (that template's actors), `forms`, `currencies`,
`accountNames`, `workspaceMembers`, `api`, `corezoidSyncApi`. Picking an actor in an actor
source links the two by a hierarchy edge from this actor to the picked one;
`extra.reverseEdge: true` reverses it.

## Create and change

1. Search first: `forms_list {query}`.
2. `form_create {title, sections, description?, color?, ref?, parentId?}`. Give every field
   a unique, stable `id` without `.`, `[` or `]`. `ref` is unique in the workspace.
   `isTemplate: false` makes a private draft that lists never return: keep its id.
3. To change a template, change only what the user asked, field by field:
   `form_fields_patch {formId, expectedEditToken, ops, form?}`. Read the template once
   (`form_get {formId, with: ["editor"]}`) for the field ids and `editToken`; every patch answers
   the next `editToken` and only what changed, so chain patches without reading again. Ops, in
   order, all or none:
   - `add {field: {class, title, id?, type?, options?, required?, description?, extra?}, section?, before?|after?}`
     (`section` is an index or a title; without one the field goes to the end of the last section;
     without `id` one is generated and returned);
   - `update {id, set: {property: value}, unset?: [property]}` (`set.id` is refused);
   - `remove {id}`, `move {id, section?|before?|after?}`;
   - `section_add {title, at?}`, `section_update {section, title}`, `section_remove {section}` (empty only).
   `form` patches the template's own `title`, `description`, `color`, `ref`, `tags`, `parentId`
   and `settings` key by key (a key set to null is dropped; the other keys stay). A stale `expectedEditToken` is refused (someone changed the template): read again
   and redo the ops. A refused value names the property and the values it takes: fix that one op.
   `form_update` with whole `sections` is for rebuilding the structure from scratch only: never
   resend every field to change one. Renaming a field's title is safe; changing its id orphans the
   values in existing actors (they stay in their `data`).
4. `form_status_update {formId, status}`: verified, rejected or pending. `form_delete` removes
   the template: confirm with the user first.

System templates are used as they are, not edited.

## Template tree

`parentId` nests a template under another. `forms_tree_get {formId}` returns the whole tree
the template belongs to; `form_linked_get {formId, direction}` every template under it
(`children`) or its ancestors (`parents`). An
actor can carry fields of several templates of its tree (`simulator-ai-actors`).

## Default accounts

An account definition on a template opens that account on every actor of the template, now
and later (the Account Template's accounts).

- `form_account_create {formId, accountName, currencyName, accountType?}` by NAME (or `nameId`,
  `currencyId`; a missing name or currency is created): binds the pair (`fact` default or
  `plan`), so actors created from the template open it, and opens it on the template's existing
  actors too, 500 per call (`nextOffset`: repeat with `offset`); `skipExisting: true` leaves them.
  A pair already bound is kept (`wasBound`).
- `form_accounts_list {formId}`, `form_account_delete {formId, id}` (confirm first) are
  `console-full`. An account on one actor only is `account_create` (`simulator-ai-finance`).

After creating a template you may offer 3–6 accounts worth tracking (an account name with a
currency, or a counter: Mileage in km, Budget in USD, Hours Spent). Reuse existing names and
currencies, show the plan, and create only after a yes.

## Smart Forms

A Smart Form is an app, not a template: `simulator-ai-smart-forms`. The data template a Smart
Form saves into is created with `smart_form_data_form_create`.
