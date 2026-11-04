---
name: simulator-ai-forms
description: Design and change Simulator templates (forms, Account Templates, «Шаблон рахунків») — sections and fields, dynamic selects, the template tree and default accounts — and find system templates, through the public Simulator AI tools. Use when the user wants to create or modify a template, add fields, define an actor schema, set a template's status, give every actor of a template an account, or asks what system templates exist ("створи шаблон", "додай поле", "шаблон рахунків", "создай форму", "добавь поле").
---

Use the installed `simulator-ai` catalog. `console` has `form_get` and `forms_list`; writes
(`form_create`, `form_update`, `form_status_update`, `form_delete`), `forms_count`, the tree
reads and the account definitions need `console-full`. The records of a template are
`simulator-ai-actors`.

A template is the schema of its actors: `sections[]`, each `{title, content[]}`, and each
content item a field. The field `id` is the key of that field in every actor's `data`.

## Read

- `forms_list {query?, type?}`: custom templates by default; `type: "system"` for Events,
  Layers, Graphs, Scripts and the other platform templates (their ids differ per workspace),
  `"all"` for every kind. Rows have no body.
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
3. To change: `form_get {formId, with: ["editor"]}`, edit the returned `sections` (append new
   fields with fresh ids), and send the whole array to `form_update {formId,
   expectedEditToken, sections, ...}`. A stale token is refused instead of overwriting another
   edit: read again and redo. Renaming a field's title is safe; changing its id orphans the
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
and later; removing it removes it from all of them.

- `form_accounts_list {formId}`, `form_account_create {formId, nameId, currencyId,
  accountType?}` (`fact` default, `plan`, `min`, `max`, `avg`), `form_account_delete {formId,
  id}` (confirm first).
- The name and currency must exist: `account_pair_create` finds or creates them by name
  (`simulator-ai-finance`). An account on one actor only is `account_create`.

After creating a template you may offer 3–6 accounts worth tracking (an account name with a
currency, or a counter: Mileage in km, Budget in USD, Hours Spent). Reuse existing names and
currencies, show the plan, and create only after a yes.

## Smart Forms

A Smart Form is an app, not a template: `simulator-ai-smart-forms`. The data template a Smart
Form saves into is created with `smart_form_data_form_create`.
