---
name: simulator-ai-app-generator
description: Generate a multi-page CDU v2 Smart Form from a set of existing Corezoid processes and a product description — pull each process, derive its real input/output contract, design pages that use them all, build the form and the Corezoid middleware that bridges them, test and release — with the public Simulator AI tools and the Corezoid plugin. Use when the user has a working Corezoid backend (process ids or a folder) and wants an app, site or portal on top of it ("build an app from these processes", "turn this API into a smart form", "згенеруй застосунок з цих процесів", "сделай сайт на базе процессов корезоид").
---

You orchestrate. You own contract extraction, the app design and verification; you delegate
page files to the `smart_form_*` tools (`simulator-ai-smart-forms`), the backend contract to
`simulator-ai-smart-forms-logic`, process authoring to `corezoid-create` / `corezoid-edit`,
and the look to `simulator-ai-styles`. Answer in the user's language.

## 0. Preflight and input

- The Corezoid plugin with `pull-process`, `lint-process`, `push-process`, `run-task` and the
  `corezoid-create` / `corezoid-edit` skills; otherwise stop and ask the user to install
  `github.com/corezoid/corezoid-ai-plugin`. Never hand-write a `.conv.json`.
- Input: process ids (or a folder), a product description (who uses the app, what they do,
  whether it needs a login) and the design intent (brand, reference, phone or desktop). Ask
  once for what is missing; state the defaults you will use.

## 1. Contract per process

`pull-process` each id; never rename the `<ID>_<Title>.conv.json` files. For each process:

- **Outputs come from `api_rpc_reply` nodes**, not the declared `params` (they drift). Group
  replies: success (its payload keys are the output), alternate (`result` such as
  registration or recovery: a navigation to its own page), error (a notification). Types
  from `res_data_type`, majority vote. An array's element shape: the reply node's
  description, else the upstream Code node, else a probe (below), else "unknown".
- **Inputs** = declared inputs plus `{{placeholders}}` consumed before produced; read
  `extra` (`api_rpc`, `set_param`), `data` (`api_copy`) and `raw_body` (raw `api`). Drop a
  declared input no node reads, and say why.
- **Side effects**: `likely`, `unlikely` or `unknown`, from the nodes reachable from Start
  (`api_copy`, mutating verbs in a callee or URL path such as create, send, register, pay),
  not the HTTP method. Report the reachable/total node ratio when lopsided.

Show the contract table before designing; state your evidence for any claim of a broken
backend. **Probe** (`run-task` with test data) only processes the user approves: a call can
send a real message or allocate a real number.

## 2. Design (approval gate)

- Map contracts to pages: credentials + token → entry page; a shared key chain → a wizard;
  array output → a list or table page; input-heavy → a form page; each alternate outcome → its
  own page.
- **A `likely` or `unknown` process never runs on a page's `/get`** (it fires on every open,
  refresh and shared link): reach it from a submit, e.g. a start button.
- **Coverage table**: every process → page → trigger (`/get` of a page, or a Form submit). A
  process that does not fit: say so and ask; never drop one silently.
- Per page: id, Forms and their fields, buttons and what they do, view-model keys (seeded in
  `viewModel`), navigation. Decide the language once: Corezoid literals are not localised.
- A design brief: per-page archetype, brand scales and font, density, every state (loading,
  empty, error, no session); a Corezoid call can take 30 s.

Show the map, coverage and brief; build only after a yes.

## 3. Build the form

`smart_form_create {title, ref}` → `smart_form_components {brief: true}`, then `{types}` for
the types you use; take types and props only from it. Write `pages/<page>/config`
(`schemaVersion: "2.0"`), `locale` and `viewModel` with `smart_form_files_create` /
`smart_form_files_update`. Pages whose data comes from Corezoid set `meta.initViaCorezoid:
true`; their submits are `form.submit` with `via: "corezoid"` naming the Form. Seed every
view-model key a page reads. Each new page needs a `pages/<page>` folder:
`smart_form_folders_create {actorId: <appId>, items: [{folderId: <id of pages>, title: "<page>"}]}`,
then its files in that folder.

A session (token) must not travel in the URL `query` or the view model unless the backend
leaves no alternative and the user agreed: both reach the browser. Never carry a password.

## 4. Middleware

One bound process routes `/get` (per `body.page`) and `/send` (per `body.formId`) to the
domain processes via `api_rpc`, and answers with `viewModel` or `data.actions` — the contract
and node rules of `simulator-ai-smart-forms-logic` §1–§2. Write the brief there, including
every Code node body, the language of its literals, and namespacing replies when a branch
calls two processes; hand it to `corezoid-create`. Then bind it to both environments with
`corezoid_project_bind` (`console-full`; the steps are `simulator-ai-smart-forms-logic` §3), after
the user's yes.

## 5. Test, repair, release

1. Static: `lint-process` clean; page writes accepted (a refusal names the component or prop).
2. Backend: `run-task` one `/get` per page and one `/send` per Form with representative data;
   the final callback fails without a real `__callback_url`: check the task reached it with
   the right `viewModel` / `actions`.
3. Fix in the owning layer (page file, or `corezoid-edit` for the process), re-run, at most
   three passes per defect; then report what still fails and why.
4. Theme (`simulator-ai-styles`), then `smart_form_release` after the user confirms.
5. End-to-end (`console-full`): `smart_form_page_render {ref, envTitle: "develop", page}` for
   every page; it runs each page's `/get`. A submit runs the bound `/send` process and whatever
   it writes, so show the user the test values per Form and get a yes before
   `smart_form_page_submit` with them. Then the user walks every page and outcome, phone width
   too. Say which pages nobody checked.

Report: the Smart Form `appId`/`ref`, the middleware process id, the coverage table, results
per test layer, and known gaps (unknown shapes, unprobed processes, dropped ones).
