# A native request form with recoverable submission

Use `native-request.json` as the page config for a **new, single-use application** when
one viewer should create one actor in an existing authorized account template. It uses
only catalog `Form`, `Stack`, `TextInput`, `Button`, `Label` and existing action contracts.
The form's business result is a new actor titled by the user's input. This is not a full
leave-approval process. Match the actual requested business data/template before choosing it.

## Authoring through the Smart Forms tools

1. Create a new CDU v2 app using the skill's keyed creation/recovery procedure. Keep
   default access-list visibility. Never add recipients, public sharing or credentials.
2. Read the develop snapshot. Update the existing `pages/index/config` with the page
   JSON, using its `sourceHash` and the environment hash. Reuse, never duplicate, seeded
   paths. Add or update `pages/index/viewModel` with:

   ```json
   {
     "workspaceId": "the configured workspace id",
     "targetFormId": 123,
     "operationId": "generated-request-APP_UUID.submit",
     "request": {"title": ""},
     "busy": false,
     "status": ""
   }
   ```

   Obtain the real template ID through an authorized public read; do not infer a system
   form ID or grant additional access. The operation key must be unique to this newly
   created app and stay unchanged across reloads and retries. Backend scopes it to the
   authenticated viewer as well as workspace/method. It is not a credential.
3. Localize labels/status copy for the user's language, keeping their meaning. Extend
   fields only with current catalog types and authoritative template field shapes.
   Use explicit `form.validate` followed by a `vm.valid.request` condition; `form.submit`
   itself does not validate. Set no `meta.initViaCorezoid` or write-on-mount binding.
4. Validate source using matching CDU schema/catalog and test interaction in a supported
   runtime. Save validation proves schema only. The source fixture's exact tests are
   listed in the evidence section; changes to it need their own interaction test.
5. Read fresh develop/production snapshots and use `application_release_create` with
   both hashes and the retained key. Verify the current release and active route as in
   the parent skill. A failed validation/save/release is a failed stage, never ready.

## Runtime behavior and boundaries

The viewer enters a title and explicitly submits. The page freezes that first input in
`pending`, calls `form.submit` via native `actor/create` with `operationId`, and disables
another submission after success. It never uses the agent's credential. The runtime
uses its current viewer session; template/write ACL and scope are checked by sim-api.
The key prevents a second actor after response loss, double delivery or reopening.

On failure the input stays frozen. Retry uses the same key and original business input.
**Recover request** calls public `actor/operation/get` without another write. Missing or
expired receipts are not success and do not permit generating a replacement key. After
reload the local input is not persisted: use recovery first; if unresolved, restore the
original input before retrying. Changed input with the old key must conflict rather than
create another actor. A second intended request requires a new app/explicit workflow.

The `smart-forms` MCP profile authors files/releases; it does not itself expose actor
creation or execute the page. Native `actor/create` and `actor/operation/get` are viewer
runtime bindings. Before using this pattern, verify the backend supports the documented
keyed actor contract; a deployment that silently ignores operationId is unsafe. Mixed
old/new fleets are not supported. Do not substitute unkeyed writes.

This workflow does not call `page/submit` or Corezoid. Those bindings require an independent
business receipt/recovery contract before retry-safe writes can be claimed. `onSubmitted`
is only UI lifecycle feedback and must not trigger an automatic agent loop.

For Console delivery the server must independently verify the release/artifact and set
existing `reaction.appId` plus `appSettings.autorun=true`. Model text is not that proof.
The current inline runtime follows production rather than an immutable revision: do not
republish an app while a viewer has an active draft. Concurrent human release/session
coherence and the Console artifact projector remain separate acceptance gates.

## Evidence

The same fixture is consumed by sim-api `TestSmartFormWorkflow` (public application/file/
release/reaction/render and native write/receipt recovery) and UI runtime
`ReactionSmartForm.workflow.test.tsx` (real reaction wrapper, autorun, input, validation,
native submit, response loss and recovery). These are deterministic domain/UI tests;
they do not prove model generation, Console orchestration, actual user 5199, or full R4.
