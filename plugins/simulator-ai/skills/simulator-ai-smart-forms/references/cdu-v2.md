# Verified CDU v2 starter

A page config is `schemaVersion`, `id`, `tree`, optional `bindings`, `routes` and `meta`.
The following uses the `Stack` and `Label` components; check their props with
`smart_form_components {types: ["Stack", "Label"]}` and do not translate it to the legacy
grid/forms format.

`pages/main/config` (type `application/json`):

```json
{
  "schemaVersion": "2.0",
  "id": "main",
  "tree": {
    "id": "root",
    "type": "Stack",
    "children": [
      {"id":"title","type":"Label","props":{"text":"{{ $locale.title }}","size":"lg","weight":"bold"}},
      {"id":"summary","type":"Label","props":{"text":"{{ vm.summary }}"}}
    ]
  }
}
```

`pages/main/locale` `{"en":{"title":"Overview"},"uk":{"title":"Огляд"}}` (language first),
`pages/main/viewModel` `{"summary":"Ready"}`, and the root `config` `{"startPage":"main"}`.
All are `application/json`. Reuse existing seed folders and files; a second file with the same
name does not replace the first.

The snapshot's root node id is the root folder id. `pages` is a child folder, `main` its child,
and `config`, `locale`, `viewModel` are files inside `main`. The server validates on save and on
release.

This is a display-only starter. A form that saves records is `smart_form_data_form_create` in
`console`; Corezoid-backed data and submits are `simulator-ai-smart-forms-logic`. Never put API
keys or Corezoid credentials in view models or sources. These tools do not run `page/render`
or `page/submit`.
