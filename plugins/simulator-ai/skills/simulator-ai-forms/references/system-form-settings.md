# Settings of system-form actors

What a "setting" of a layer, an event, a dashboard or another system object is, and where it is
stored. Every key below has a writer and a reader in the platform code: sim-api
(`internal/entity`, `internal/service`), pong-server (system forms are seeded only there,
`migrations/init/up.sql` and later `migrations/<version>/up.sql`), simulator-ui-kit (the new
editors) and pong-front-end (legacy). Where they disagree it says so. Do not invent other keys.

## How a setting is written

- Most settings are keys of the actor's own `data`. `actor_update {actorId, data: {<key>: …}}`
  merges `data` one level deep: a sent key replaces the stored one, keys not sent stay. A nested
  list or object (`accounts`, `accountsPeriod`, `source`) is replaced whole, so send all of it.
  `replaceEmpty: true` replaces the whole `data`: never for a setting. `[]`, `{}` and `null`
  are written as sent (one ui-kit comment claims an empty `{}` is skipped; the server's merge
  has no such filter).
- Some are columns of the actor, top-level `actor_update` params: `layerPicture` (a layer's
  background image; also `graph/layer/picture/update`), `appId` / `appSettings` (a Smart Form
  embedded in the actor), `picture`, `pictureObject`, `color`.
- A placement's look on one layer is not the actor's: node `layerSettings` and edge layer settings
  belong to the placement (see "Placements" below).
- Read: `actor_get {actorId, fields: ["id", "title", "formId", "data"]}`.

## Layers (a layer actor's `data`)

`layer_settings_update` writes all of these; the Graph Editor shows them in the layer's settings.

| key | shape | what the user sees |
| --- | --- | --- |
| `defForm` | `[{"title": "Clients", "value": 123, "type": "form"}]`, `[]` clears | Default actors form: the template new objects on the layer get (pencil drawings, quick-created actors) |
| `edgeCurveStyle` | `[{"value": "curved"}]`: `curved` (default), `rounded`, `roundedDownward`, `straight` | Edges type: the default shape of the layer's links; an edge's own `curveStyle` wins |
| `anisotropy` | `true` (default) | Anisotropy: an opened node pushes its neighbours aside |
| `outerEdges` | `false` (default) | Outer edges: stubs for links whose other end is on another layer |
| `initX`, `initY` | numbers, graph units | Initial coordinates: the point the layer opens centred on |
| `accounts` | `[{"value": "<nameId>_<currencyId>", "title": "<name>, <currency>"}]`, at most 32 drawn | Accounts on the graph: the balances shown on every actor of the layer; `[]` hides them |
| `accountsPeriod` | `{"startDate": <s>, "endDate": <s>, "timeZoneOffset": <minutes>}`, Unix seconds, either bound optional | The window those balances are read over; `{timeZoneOffset}` alone = all time |
| `accountName`, `currency` | `[{"title", "value"}]` lists | The OLD accounts format (every name × every currency); both editors clear them to `null` when they write `accounts` |
| `inheritForms`, `inheritAccounts` | booleans | Layers form fields: actors under the layer inherit its form fields / accounts (sim-api actor context) |
| `type` | `[{"title": "Graph", "value": "graph"}]`, `graph` or `tree` | Layers form field "Type" |

Disagreements: the Layers form offers only curved, rounded and straight edges while both editors
also write `roundedDownward`; legacy stores `edgeCurveStyle` entries with a `title`, ui-kit
without; ui-kit adds pair fields (`nameId`, `accountName`, `currencyId`, `currencyName`, currency
metadata) beside `value`/`title` in `accounts`, which legacy and sim-api ignore; `defForm`'s form
default is `""`, the editors clear it with `[]`. `actorSizeHierarchy` (boolean) is a legacy layer
key (size by link hierarchy) that no form field and no new editor has: change it only when asked.
Not stored anywhere: the background's opacity, a live presentation (server state).

## Placements (one actor or edge on one layer)

Node `layerSettings`, written by `graph/layer/actor_settings/save` (`layer_placements_manage` at
creation): `width`, `height` (graph units, at least 1), `zIndex`, `opacity` (0..1), `pin`,
`sizeGrade` (`S1` default, `S2`, `S3`), `expand` + `expandType` (`cardView`, `chart`, `layer`,
`profile`, `script`, `actor`, `text`), `fullScreen`, `isCardView`, `profile`, `flaskView`; text
notes `isTextNode`, `textWidth`, `textHeight`, `textNodeScale`, `textSizing` (`auto`,
`fixedWidth`, `fixed`), `labelStyle {fontSize, color, bold, italic, align}`; pictures
`image {naturalWidth, naturalHeight, fit contain|cover|fill, crop {x, y, w, h} as fractions}` and
`rotation` (the file is the actor's `picture`). Never send `offset`: the API types it `{x, y}`,
the editors store `{top, right, bottom, left}`. Edge placements (`layer_edge_settings_save`):
`name`, `linkedActorId`, `curveStyle`, `lineStyle` (`solid`, `dashed`, `dotted`), `width`,
`color`, `routingPoints [{w, d}]`.

## Events (events, tasks, chats and meetings)

All are actors of the Events form; roles (executor, approver, signer) are access rules, not data.

| key | shape | meaning |
| --- | --- | --- |
| `startDate`, `endDate` | Unix seconds | Start and end (also `actor_create` params) |
| `chatType` | `"p2p"` or `"group"`; absent for an event or task | Makes the actor a chat |
| `scheduleMeeting` | `true` | A call at the start; sim-api reminds 300 s before (not a setting) |
| `isPersistentMeeting` | `true` | A permanent meeting room |
| `isActiveMeeting` | boolean | A call is live: server state, never write it |
| `moderator` | `[{"value": <SuperAdmin user id>, "title", "type": "user"}]` | Meeting hosts (SA ids, not workspace user ids) |
| `recurrence` | `{"freq": "daily"|"weekly"|"monthly"|"yearly", "interval", "byDay", "byMonthDay", "bySetPos", "byDayOfWeek", "until", "count", "timezone"}` | Repeat rule; sim-api expands it and fills `timezone` |
| `agenda` | `{"list": [{"id", "title", "description", "users": [...]}], "planTime"}` | Meeting agenda (`meet/agenda/*`) |
| `channel`, `smartTags`, `disableCallbackSoundUsers`, `autoCollapseOnEntry` | form fields | Channel, smart tags, users whose call sound is muted, collapse on entry |

Disagreements: the form types `recurrence` and `agenda` as text, every writer stores objects; a
ui-kit comment calls `recurrence` inert while sim-api expands it. `allDay` is not stored.

## Dashboards (chart actors)

The whole chart is `data.source` (ui-kit writes an object, legacy a JSON string; sim-api reads
both); a save replaces all of it. Use `chart_create` / `chart_update` (skill
`simulator-ai-charts`) rather than writing `source` by hand. Its keys: `chartType` (`bar`
default, `stackedBar`, `line`, `pie`, `doughnut`, `funnel`, `table`, `polarArea`, `radar`,
`kpiCard`), `counterType` (`amount`, `count`), `chartViewMode`, `sourceType` (`manual`,
`actorFilter`), `range` (`allTime` default, `today`, `lastWeek`, … `custom` with `rangeDates`),
`orderValue`, `showTotal`, `displayChartDataLabels`, `legend {...}`, `accounts[]` (≤100: actor,
`nameId`, `currencyId`, `incomeType`, `accountType`, `color`, `customName`), `dynamicSource`.

## Other system forms

- Streams (custom feeds): `type` (`events`, `chats`), `orderBy` (`created_at`, `updated_at`
  default, `account`), `orderValue` (`DESC`, `ASC`), for account order `nameId`, `currencyId`,
  `accountType`, `incomeType`; each stored as `[{"title", "value"}]`.
- Graphs (folders) and State: `inheritForms`, `inheritAccounts`; State also `polygon`.
- Scripts (Smart Forms): `sharedWith` and `cduVersion`, written by the Smart Form tools
  (`application/update` replaces the whole `data`).
- Widgets: `autocloseValue`, `autoclosePeriod`, `joinConversation`, `showHistory`, `startScript`,
  `endScript`, `customBarScripts`, `status`. AccountTriggers: the trigger tools.
- No settings at all: Skills (the playbook is the `description`), Phase, Tags, System.
