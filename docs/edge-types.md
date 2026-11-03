# Edge types and legacy getEdgeTypes

`graph_edge_types_list` in graph-read/create uses public `edge_type/list` with the
host workspace and API-user credential. It is hidden in host-bound actor mode,
matching the legacy operation's absence from actorBindings. It creates no business
edge, grants no rights and accepts no delegated Console token or private fallback.

Omit `isSystem` for all types, use true for system types and false for custom types.
The MCP schema rejects null/string values; the public RPC also accepts null as
omission. Filtered rows sort by ascending numeric id before pagination. Default
page size is 50, maximum 100; keep the same filter and advance offset until a short
or empty page. Concurrent deletion can shift offsets; this is not a snapshot.
No total or hidden count request is added. Errors are not successful empty pages.

Optional `filter` is a comma-separated exact field selector, at most 512 characters:
`id,name,accId,userId,isSystem,createdAt,isTree`. Empty/omitted means all seven.
Unknown names are ignored, duplicates have no effect and spaces are not trimmed;
`"id, name"` selects only id, while `"unknown"` returns one `{}` per page row.
Projection never changes page membership. The client reads full typed rows to
validate workspace, identity, ordering and the system flag before locally applying
the selector. The RPC retains its full row response; no selector is forwarded.
Unrecorded public creators are omitted by the RPC; the MCP projection restores
legacy null when userId is selected, and preserves explicit false and zero values.

The backend inserts absent system defaults before its primary read, including
when filters/offsets produce an empty result. Existing IDs/creators/flags are never
replaced or reclassified. The current local caller becomes creator of a missing
default. Authorization precedes repair. Repair failure fails the read; a subsequent
read failure may follow committed repair. No realtime frame is published, and
retries recheck authority without duplicating a default. The MCP client itself
does not retry or cache these reads.

The inspected legacy handler ignores its advertised isSystem parameter and its
SQL omits the isSystem result field. Public true/false filtering and the flag are
intentional improvements, not claimed historical behavior. Legacy default repair,
CSV selection and nullable creators are retained. The fixture under
`internal/publicapi/testdata/parity/getEdgeTypes.json` records these distinctions
and normalizes only the legacy outer data envelope to list for comparable cases.
It is a source-derived oracle, not a live legacy-server comparison.

```json
{"name":"graph_edge_types_list","arguments":{"isSystem":false,"filter":"id,name,isTree","limit":50,"offset":0}}
```

Use actual IDs and tree constraints when planning graph links. Hierarchy is a
special graph relation; select a suitable non-tree connector for return loops.
Counts/read metadata never grant write access. Named parity still requires public
API-user scope/workspace/revoke evidence and installed-client checks; full graph
workflow, issuer-side revoke and model-driven execution are independent gates.
