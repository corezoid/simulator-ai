package mcpserver

// actorTools are the legacy actor tools (createActor, getActor, getActorByRef, updateActor,
// deleteActor, setActorStatus, searchActors, filterActors, getSystemActor) on public methods.
var actorTools = []parityTool{
	{Name: "actor_get", Method: "actor/get", Description: "Read one actor: its columns and template field values (data). fields narrows the answer (data.<key> keeps one field); with adds related data such as owner, privs, access, reactionsStats or forms. Needs view access."},
	{Name: "actor_by_ref_get", Method: "actor/by-ref/get", Description: "Read an actor by its template and your own ref (unique within the template), instead of its id. fields/with as in actor_get."},
	{Name: "actor_system_get", Method: "actor/by-system_obj/get", Description: "Read the actor that stands for a platform object, e.g. objType user with the user id: the user's twin actor. fields/with as in actor_get."},
	{Name: "actor_create", Method: "actor/create", Defaults: map[string]any{"data": map[string]any{}, "fields": metaFields["fields"]},
		Description: "Create ONE actor; for two or more use actors_create (one call). Create an actor from an account template (formId). data holds template field values keyed by field id (see form_get); ref is your own key, unique within the template; color, picture, appId/appSettings, geoPosition as on the actor. For flowchart blocks use actors_create with formId and data from flowchart_layout. Returns the new actor (compact fields unless you pass fields). Does not place it on a layer."},
	{Name: "actor_update", Method: "actor/update", Defaults: metaFields,
		Description: "Change an actor: only the sent keys change; data is merged key by key (replaceEmpty replaces the whole object); an empty appId clears it. Needs modify access. Returns the actor (compact fields unless you pass fields)."},
	{Name: "actor_status_update", Method: "actor/status/update", Description: "Set an actor's approval status (e.g. verified, pending, rejected). Needs modify access."},
	{Name: "actor_delete", Method: "actor/delete", Description: "Delete an actor (to the trash) with its placements and links. Needs remove access. Destructive: confirm with the user first."},
	{Name: "actors_search", Method: "actor/search/list", Description: "Search actors of the workspace by title or fragment (query), optionally within one template (formId). One page per call (limit/offset); fields/with shape each row."},
	{Name: "actors_filter", Method: "actor/filter/list", Description: "List actors of a template (formId) filtered by queryData (field conditions), linked actor, owners, dates, starred or reactions; one page per call (limit/offset), fields/with shape each row. The total is a separate actors_filter_count call with the same filters."},
	{Name: "actors_filter_count", Method: "actor/filter/count", Description: "Count the actors actors_filter would return for the same filters; cap bounds the count."},
}
