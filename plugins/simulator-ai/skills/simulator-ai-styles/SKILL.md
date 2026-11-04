---
name: simulator-ai-styles
description: Style a CDU v2 Smart Form — its theme file (built-in or own themes, brand colours, fonts, radius and density presets, tokens, a theme the viewer can switch) and the visual props of its components — with the public Simulator AI tools. Use when the user wants to restyle or brand a Smart Form, give it a theme or dark mode, change its colours, fonts or spacing, or make it look like a reference ("style the smart form", "theme the form", "застилізувати смартформу", "тема для форми", "застилизовать смартформу", "тема формы").
---

Use the installed `simulator-ai` catalog (`smart_form_files_get`, `smart_form_files_create`,
`smart_form_files_update`, `smart_form_release`, `smart_form_components`). Page layout is
`simulator-ai-smart-forms`; this skill owns the look.

A CDU v2 page has **no CSS or Less**: components draw themselves from theme tokens, so the look
comes from two places only.

1. **The app-root `theme` file** (one per environment, shipped by a release). It decides the
   palette, fonts and shape for every page.
2. **Component props** such as size, variant, tone or surface. Take them only from
   `smart_form_components {types: [...]}`: an unknown prop is refused on save. Never invent a
   `style`, `className` or colour prop.

A `styles/index` file in the tree is the legacy stylesheet; v2 pages do not use it.

## The theme file

Root folder `/`, title `theme`, type `application/json`. Two formats:

- **Flat:** `{"<token id>": "<value>"}`: overrides on top of the viewer's own theme.
- **Structured** (prefer it):

```json
{
  "default": "brand",
  "switchable": ["brand", "dark"],
  "themes": {
    "brand": {
      "label": "Brand",
      "extends": "light",
      "scales": {"primary": "#0B5FFF", "gray": "#5B6472"},
      "fonts": {"sans": {"family": "Inter", "source": "google"}},
      "presets": {"radius": "soft", "density": "comfortable"},
      "tokens": {"background.page": "#F4F6FA"}
    }
  }
}
```

- `default`: `light`, `dark`, `darcula`, an own theme id, or `viewer` (follow the viewer;
  the default when absent).
- An own theme `extends` one built-in theme (no chains); id `^[A-Za-z][A-Za-z0-9_-]{0,63}$`, up
  to 16 themes; `label` (1–64 characters) is what a theme picker shows.
- `scales`: one base hex per scale (`primary`, `secondary`, `gray`, `success`, `warning`,
  `error`); the client derives the shades and component roles from it. This is the main lever
  for a brand.
- `fonts` per role `sans` (all UI text), `heading`, `serif`, `mono`: `{family, source}` with
  `source` `google`, `local`, or `files` (`files: [{src, weight?, style?}]`, up to 12, where
  `src` is `attachments/<title>` of a file attached to the Smart Form actor or an `https://`
  link; `weight` `400` or a range `100 900`, `style` `normal` or `italic`).
- `presets`: `radius` (`sharp`, `soft`, `default`, `round`), `density` (`compact`,
  `default`, `comfortable`), `borders` (`thin`, `bold`, `heavy`), `fills` (`kit`, `exact`).
- `tokens`: dotted token ids overriding what was generated, e.g. `colors.primary.500`,
  `colors.semantic.surfaceRaised`, `background.page`, `borderRadius.sm`, `spacing.md`.
  `colors.*` take no gradient; a gradient page background is `background.page`.
- `switchable` lists the themes a viewer may switch between; a page switches with the client
  action `theme.set`.

Save validation checks the shape, not the token ids: no `; { } < >`, `url(`, `expression(`,
`@import`, values of 1–256 characters, at most 512 tokens. A refusal names the problem.

## Steps

1. Agree on the intent: brand colours or a logo, a reference, light or dark, phone or desktop.
   No preference: state the defaults you will use.
2. `smart_form_files_get {appId, withSource: true}`: is there a root `theme`? Keep what the user
   did not ask to change.
3. Write it: `smart_form_files_update` with the file's id, current `title` and `type` (and
   `ifMatchHash` from its `sourceHash`), or `smart_form_files_create` in the root folder when
   it is missing. Prefer `scales` and `presets` over many single tokens.
4. Adjust component props on the pages only through the catalog's props.
5. `smart_form_release` publishes it. A form already live elsewhere: name it and what changes,
   and get an explicit yes first (`smart_form_release_rollback` in `console-full` undoes a
   release). `smart_form_page_render` shows a page still builds, but only a person sees the
   result: tell the user what to look at; a human check of contrast and layout is part of done,
   at phone width too.
