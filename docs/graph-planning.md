# Deterministic native flowchart planning

`graph-create` adds `graph_flowchart_plan` (14 tools total, including exact readback). This is a read-only
planning tool: the only network operation is the already pinned public
`form/flowchart-block/search` with exact blockIds. It never creates actors, assigns
operation IDs, retains credentials, launches Hermes or retries a write. Other
profiles are unchanged. The portable `simulator-ai-graph` skill orchestrates the
existing create/recovery tools in the desktop host.

Input is `{nodes:[{key,blockId,title}],edges:[{key,source,target,name?}]}`. Plan-local
keys use `[A-Za-z0-9][A-Za-z0-9_-]{0,63}`. Bounds are 1–100 nodes, 0–100 edges,
32 KiB encoded input and 1 MiB output. Empty/missing/duplicate identities, unknown
endpoints, self-loops and duplicate directed endpoint pairs are refused before
catalog access. Same-direction parallel links would collapse under public edge
natural-key reuse, so this planner refuses them rather than promising two links.

Exact catalog results must supply every requested kind. Definitions follow UI Kit
`server/forms/flowchartBlocks.ts` and `geometry/flowchartShape.ts`: positive root and
child IDs; matching blockId item; view stored as JSON text with shape, size and
textFrame. Rect/roundedRect/line/path primitives require their named finite fields;
missing coordinates, empty paths/primitive sets, unsupported kinds, duplicate form
item IDs and negative extents fail the entire plan. Dimensions must be 1–100000;
coordinates/frame values are bounded to ±100000, primitives to 256 and path strings
to 64 KiB. This is intentionally narrower than arbitrary frontend-authored JSON.
No validation failure silently substitutes a generic shape.

Output preserves directed semantic edges and branch names. Each planned node
contains actual root formId, childFormId, catalogUpdatedAt, prefixed data with the
view parsed as an object, native layerSettings and center position. The algorithm
`native-flowchart-dag-v1` visits nodes/edges in input order, excludes DFS back edges
only from ranking, assigns longest-path DAG ranks and packs each rank using actual
native widths/heights plus 150 graph units between boxes/rows. Return edge keys are
reported, never removed. Cycles, joins and disconnected components are supported;
input order is the stable tie-breaker. Same ordered input and catalog content yield
the same plan. It does not route edges, avoid all connector crossings, autosize
labels or prove browser edit/reload behavior. Backend snapping may alter positions:
verify persisted values with [exact readback](graph-elements.md) rather than treating planned coordinates as receipts.

Save the plan and exact keyed write payloads before execution. The planner's
catalog revision is provenance, not a CAS token; a regenerated plan must not replace
an in-flight operation's payload after failure. Current authorization and per-write
contract preflight remain in the existing create tools. See [creation/recovery](graph-create.md)
and the [portable workflow](../plugins/simulator-ai/skills/simulator-ai-graph/SKILL.md).

Tests cover deterministic six-block approval geometry and return links, joins,
non-overlap with different native dimensions, empty edges/disconnected cycles,
invalid definitions/input, profile fences and the read-only public network path.
Functional API acceptance remains separately recorded in sim-api; planner tests
alone do not prove Console execution or a visible editable graph.
