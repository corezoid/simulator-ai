---
name: simulator-ai-smart-forms-logic
description: Plan and brief the Corezoid process behind a CDU v2 Smart Form — a page view model loaded from Corezoid (/get) and form submits handled by it (/send) — and hand the authoring to the Corezoid plugin's corezoid-create and corezoid-edit skills. Use when a Smart Form must load dynamic data, react to a submit, navigate, notify or write back to actors, or its existing backend must change ("smart form backend", "wire smart form to corezoid", "/get /send process", "submit handler", "логіка смарт форми", "бекенд смарт форми", "процес для смарт форми", "логика смарт формы").
---

Layout, texts, the stored view model and the theme live in the Smart Form's files
(`simulator-ai-smart-forms`, `simulator-ai-styles`). Dynamic data and submits run in **one
Corezoid process bound to each environment** (develop, production). This skill does not write
process JSON: it prepares a brief and hands it to the Corezoid plugin.

## 0. Preflight

The Corezoid plugin must be installed and authenticated: its `create-process`,
`push-process`, `lint-process` and `run-task` tools and its `corezoid-create` /
`corezoid-edit` skills (plugin-qualified `corezoid:corezoid-create` on newer hosts). If they
are missing, stop and tell the user to install `github.com/corezoid/corezoid-ai-plugin` and run
`/corezoid-init`. Never hand-write a `.conv.json`.

The workspace is the Corezoid company. Put the process in a **project stage** of that company:
only processes of a project can be bound (step 4).

## 1. The v2 contract

The platform calls the bound process through the Sync API. The task carries `path`, `body`,
`sessionData` (the viewer) and `__callback_url`; the process answers by POSTing to
`{{__callback_url}}`.

| `path` | When | `body` | Answer |
| --- | --- | --- | --- |
| `/get` | a page opens whose config has `meta.initViaCorezoid: true` | `page`, `query`, `route`, `context` | `{code: 200, viewModel: {…}, refreshOn?: {actorIds?, formIds?}}` |
| `/send` | a `form.submit` with `via: "corezoid"` runs | `page`, `formId` (the Form node id), `data` (its values), `query`, `route`, `context` | `{code, data: {actions: [...]}}` |

- Without `initViaCorezoid` the page never calls `/get` and starts from its `viewModel` file.
  A non-200 or slow (15 s) `/get` is swallowed: the page silently shows the stored view model,
  so seed every key there. `meta.vmKeys` keeps only the listed top-level keys.
- `refreshOn` limits which realtime changes re-request the page; without it any entity change
  the viewer receives does.
- `/send` actions, applied in order before the submit's `onSuccess`: `navigate {to, query?}`,
  `vm.merge {patch}`, `assign {path, value}`, `patch {target: "tree"|"vm", ops}` (JSON Patch),
  `notify` (a toast), `redirect {url}`. They are applied as sent, with no expressions. With no
  actions, `code: 302` navigates to `data.nextPage` and `205` to `data.pageId`. The page's
  `onSuccess` also receives the raw reply as `$event`, a non-2xx `code` included.
- One Form node is one submit source: dispatch `/send` on `body.formId`. Field changes never
  reach Corezoid unless the page wires a submit for them; cascades belong in client actions.
- A page submit names its Form: `{"type": "form.submit", "formId": "<Form id>", "via":
  "corezoid", "onSuccess": [...], "onError": [...]}`. It does not validate: run
  `form.validate` and an `if` first.

## 2. Process shape (default: one bound process)

Start → condition on `path` → `/get`: condition on `{{body.page}}` → build `viewModel` (Code
node) → callback; `/send`: condition on `{{body.formId}}` (then `{{body.page}}` when ids
repeat) → domain call → build `data.actions` → callback. Split into aliased sub-processes
(`api_copy` / `api_rpc`) only for large forms; the bound process needs no alias.

Rules the brief must state:

- `path` is top level, so `"param": "path"`; a nested key is a template:
  `"param": "{{body.formId}}"`. An unbracketed nested key never matches and silently falls
  through to the default branch.
- The callback is an `api` node: `POST {{__callback_url}}`, `rfc_format: true`, `extra` with
  `code` (`extra_type` `number`) and `viewModel` or `data` (`extra_type` `object`),
  `customize_response: false` (the ack body is empty), and the full canonical field set
  `format`, `send_sys`, `debug_info`, `cert_pem`, `max_threads` (lint rejects it otherwise).
- Every fallible node has its own error target; an error answers with a `notify` action (or a
  4xx/5xx `code`) instead of leaving the page hanging. Every `api_rpc` has a time semaphore
  (30 s minimum).
- An `api_rpc` reply merges into the task at top level: when one task calls two processes,
  copy each reply into its own keys before the next call.
- Use `sessionData.userInfo` for the viewer; never put secrets in `viewModel`, `query` or
  actions: they reach the browser and the URL.

## 3. Create

1. Agree with the user on pages, Forms, what `/get` loads and what each submit does; set
   `meta.initViaCorezoid` on the pages that need `/get` and `via: "corezoid"` on their submits
   (`smart_form_files_update`).
2. Write one brief per process for `corezoid-create`: purpose; inputs (`path`, the `body`
   keys used, `sessionData` if read); expected answer per path from §1; project/stage and
   process name; node list in order with every Code node body written out; constants for
   `_ENV_VARS_`.
3. After the push, keep the numeric process id.
4. **Bind it** to both environments. The binding (sim-api `corezoid/project/bind`) sets up the
   API key and shares the process itself, with no secret in the conversation, but this catalog
   has no tool for it: ask the user to bind it in the Smart Form editor's Corezoid settings
   ("bind existing processes", develop and production). Releases ship files, never bindings.
5. Test the process with `run-task` (via the Corezoid skills) on synthetic `/get` and `/send`
   bodies; the final callback fails without a real `__callback_url`, so check that the task
   reached it with the right `viewModel` / `data`. Then release (`smart_form_release`) and let
   the user open the form.

## 4. Edit

Find the bound process (the Smart Form editor shows it), classify the change — a new
view-model key the page reads needs the `/get` builder, a new Form or button needs a `/send` branch, a new page
a `body.page` branch — and hand `corezoid-edit` a brief: process, goal, nodes to add or change
with their payloads, everything else kept. Keep page files and process in step: change the
page config in the same task. A new process id needs a new binding.
