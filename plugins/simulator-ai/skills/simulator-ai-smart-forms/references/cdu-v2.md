# Verified CDU v2 starter

The public `app_file/create` reference defines a page config as `schemaVersion`, `id`,
`tree`, optional `bindings` and `routes`. The following uses its demonstrated `Stack`
and `Label` props; do not translate it to the legacy grid/forms format.

Create `pages/main/config` with type `application/json` and JSON text source:

```json
{
  "schemaVersion": "2.0",
  "id": "main",
  "tree": {
    "id": "root",
    "type": "Stack",
    "children": [
      {"id":"title","type":"Label","props":{"text":"[[title]]","size":"lg","weight":"bold"}},
      {"id":"summary","type":"Label","props":{"text":"{{summary}}"}}
    ]
  }
}
```

Create `pages/main/locale` with `{"title":{"en":"Overview","uk":"Огляд"}}`,
`pages/main/viewModel` with `{"summary":"Ready"}`, and root `config` with
`{"startPage":"main"}`. All use `application/json`. Reuse existing seed folders/files;
creating a second file with the same name does not replace the original.

The snapshot's root node id is the root folder id. `pages` is a child folder, `main`
is its child, and `config`, `locale`, `viewModel` are files inside `main`.
The server validates on save and release. Matching schema/catalog support is required.

This is a display-only starter, not a backend transaction example. Dynamic/business
behavior requires the current catalog's binding/action contracts plus separately configured
Corezoid logic. Do not put API keys or Corezoid credentials in view models or source files.
The public Smart Forms tools here intentionally do not execute `page/render` or
`page/submit`: initialization itself can invoke a business process, and submit needs its
own viewer/session/idempotency contract.
