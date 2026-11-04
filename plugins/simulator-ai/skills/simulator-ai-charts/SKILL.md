---
name: simulator-ai-charts
description: Create and change Simulator charts (dashboards of account values on a graph layer) with the public Simulator AI console tools. Use when the user wants to create, add or reconfigure a chart or dashboard, plot actor balances or transaction counts, change the chart type, range, sorting, series or size ("create chart", "add dashboard", "switch to bar chart", "show last week", "visualise account data", "створи графік", "додай дашборд", "покажи баланси на графіку", "создай график", "дашборд").
---

Use the installed `simulator-ai` catalog with profile `console-full` (`chart_create` and
`chart_update` are not in the `console` core). A **chart** is an actor of the `Dashboards` system
template placed on a layer and opened as a chart; its `data.source` holds the configuration.

## Two kinds of chart

| Kind | Pass | Shows |
| --- | --- | --- |
| **manual** | `accounts: [{actorId, nameId, currencyId, color?, incomeType?}]` | one series per account |
| **actorFilter** | `sourceFormId`, `accountNameId`, `currencyId` (and `top`, default 20), or `filterActorId` | the top actors of a template ranked by one account |

An actorFilter chart reads its actors through an actor of the `ActorFilters` template:
`chart_create` makes one (titled `filterTitle`, else the chart's title), or reuses `filterActorId`
so several charts share one filter.

## Find the ids first

- `layerId`: the layer the user named, else the current layer of the request's Current context;
  `layer_create {title}` only with neither or when asked for a new one; its graph's layers via `graph_list`.
- `nameId` / `accountNameId`: `account_names_list`; `currencyId`: `currencies_list`. Both come from
  accounts the actors really have (`accounts_list {actorId}`); `account_pair_create` registers a
  missing pair.
- `actorId` of each series: `actors_search`, `actors_filter`; ids, never titles.
- `sourceFormId`: the template of the actors to rank, from `forms_list` / `form_get`.

## Create

```
chart_create {layerId, title, chartType: "bar", range: "lastMonth", orderValue: "desc",
  position: {x: 300, y: 200},
  accounts: [{actorId: "<id>", nameId: "<name id>", currencyId: 7}, {actorId: "<id2>", nameId: "<name id>", currencyId: 7}]}
chart_create {layerId, title: "Top customers", sourceFormId: 19, accountNameId: "<name id>", currencyId: 7, top: 10}
```

Returns `{chartId, filterActorId, laId, warnings}`; the chart is drawn 650 x 450 at `position`.
Give the user `build_link {entity: "chart", id: chartId}` or the layer's link.

## What it shows

| `counterType` | series `incomeType` | Each value is |
| --- | --- | --- |
| `amount` (default) | `total` (default) | credit minus debit, the net amount |
| `amount` | `credit` / `debit` | incoming / outgoing amount |
| `count` | `credit` | the number of incoming transactions |

- `chartType`: `line` (default), `bar`, `stackedBar`, `pie`, `doughnut`, `funnel`, `table`,
  `polarArea`, `radar`. `kpiCard` is built in the web app.
- `range`: `lastHour` (default), `allTime`, `lastMinute`, `last10Minutes`, `today`, `yesterday`,
  `lastWeek`, `last14Days`, `lastMonth`, `previousMonth`, `lastYear` (and the `…ExcludeToday`
  variants), `realTime`, `lineRealTime`. `custom` is set in the web app.
- `orderValue`: `default`, `desc`, `asc`.
- `line` and `stackedBar` refuse `allTime`, store `realTime` as `lineRealTime` and ignore sorting;
  other types store `lineRealTime` as `realTime`. `warnings` names every such change: tell the
  user.
- Two series that differ only by `incomeType` get it in the legend; an exact repeat is refused
  (the chart would draw it once). There is no `turnover`, `lastDay` or `area`.

## Change a chart

Always `chart_update {chartId, …}` with only what changes; never recreate the chart and never write
`data.source` with `actor_update`. Everything left out, other keys of the stored source included,
stays.

```
chart_update {chartId, chartType: "bar", orderValue: "desc"}
chart_update {chartId, range: "lastWeek"}
chart_update {chartId, counterType: "count"}
chart_update {chartId, incomeType: "credit"}          // every series of a manual chart
chart_update {chartId, top: 5}                        // an actorFilter chart
chart_update {chartId, currencyId: 9}                 // new filter; the other ids from the current one
chart_update {chartId, filterActorId: "<filter id>"}  // point it at an existing filter
chart_update {chartId, accounts: [...]}               // replace the series, make it manual
chart_update {chartId, layerId, laId, width: 900, height: 600}   // redraw the placement (expand: false collapses it)
```

- A changed `sourceFormId`, `accountNameId` or `currencyId` makes a **new** `ActorFilters` actor (a
  filter may serve other charts, so it is never edited). When the chart stops using a filter, a
  warning names it: delete it (`actor_delete`) only if no other chart uses it, and ask first.
- A chart grouped by fields in the web app keeps its grouping while the template stays the same,
  and takes no `top`.
- `incomeType` belongs to manual charts; `top` and `filterTitle` to actorFilter charts.
- The `laId` of a chart: from `chart_create`, or `layer_placements_list {actorId: layerId}`.
  Width and height are at least 650 x 450.

Account inheritance from the layer's graph is not set up by these tools: a chart reads the
accounts its source names.
