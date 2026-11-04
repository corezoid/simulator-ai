package mcpserver

// reactionTools are the legacy reaction tools (getReactions, createReaction, updateReaction,
// deleteReaction) on public methods; pinned reactions are out of scope (no such thing in sim-api).
var reactionTools = []parityTool{
	{Name: "reactions_list", Method: "reaction/list", Params: []string{"rootActorId", "view", "orderBy", "orderValue", "limit", "offset", "pagination", "cursor", "direction", "parentId", "types", "ownerIds", "from", "to", "with"}, Description: "Read the reactions under an actor (rootActorId): comments, done, sign, reject, rating and the rest, with readTime (when another participant last read them). view flat lists all in time order, tree top-level with replies, thread one thread (parentId); one page per call (limit/offset, or pagination cursor with cursor/direction); types and ownerIds filter (types with cursor paging only); with adds user (the author) and attachments. Needs view on the actor."},
	{Name: "reactions_count", Method: "reaction/count", Description: "Count the reactions reactions_list with cursor paging would return for the same filters (view defaults to flat)."},
	{Name: "reaction_create", Method: "reaction/create", Params: []string{"rootActorId", "type", "description", "data", "parentId", "attachmentIds", "hidden", "notify", "appId", "appSettings", "with"}, Description: "Post a reaction under an actor (rootActorId): type comment with description as its text, or done, reject, sign, ds, freeze, rating, view; parentId replies to a reaction; attachmentIds hangs uploaded files (attachment_upload) on it; hidden shows it only to users with execute on the actor; data.rid (a string) makes a retry idempotent. Participants are notified unless notify is false. Needs view on the actor plus the privilege the type demands (sign for sign, execute for done)."},
	{Name: "reaction_update", Method: "reaction/update", Description: "Change a reaction you posted (actorId under rootActorId). description and appId are written as sent, so resend the text you keep; data, appSettings, hidden and extra change only when sent (extra replaces the whole object)."},
	{Name: "reaction_delete", Method: "reaction/delete", Description: "Remove a reaction you posted (actorId under rootActorId). Removing a freeze unfreezes the actor; a ds can never be removed. Destructive: confirm with the user first."},
}

// userTools are the legacy getWorkspaces, getUsers, getUser and searchUsers on public methods.
var userTools = []parityTool{
	{Name: "users_search", Method: "user/list", Params: []string{"query", "limit", "offset"}, Description: "Find workspace members (people and API users) by name or login (query); without query, list them one page at a time (limit, default 20, max 200; offset). Each id is the userId of access rules, reactions and actor fields."},
	{Name: "users_list", Method: "user/list", Params: []string{"limit", "offset"}, Description: "List the workspace members (people and API users) one page at a time (limit, default 20, max 200; offset). To find someone use users_search."},
	{Name: "user_get", Method: "user/get", Description: "Read one workspace member by userId: nick, type, status, logins and roles."},
	{Name: "workspaces_list", Method: "workspace/list", Description: "List the workspaces you belong to (query finds them by name), one page at a time (limit, default 20, max 100; offset). Every other tool works in the host workspace only."},
}

// attachmentTools are the legacy getAttachments, getActorAttachments, addAttachments,
// updateAttachment and removeAttachments on public methods.
var attachmentTools = []parityTool{
	{Name: "attachments_list", Method: "attachment/list", Description: "List the files you can see anywhere in the workspace, newest first; people (uploader ids) and starred narrow it; one page per call (limit 1..100, offset). Each has id (for attach and reactions) and fileName (for attachment_read)."},
	{Name: "attachments_by_actor_list", Method: "attachment/by-actor/list", Description: "List the files attached to one actor (actorId), one page per call (limit/offset). Each has id and fileName (for attachment_read). Needs view on the actor."},
	{Name: "attachment_attach", Method: "attachment/attach", Description: "Hang already uploaded files (attachmentIds from attachment_upload or attachments_list) on an actor, or on a reaction (actorId = the reaction, rootActorId = its actor). A file already there changes nothing. Needs modify on the actor."},
	{Name: "attachment_update", Method: "attachment/update", Description: "Rename an uploaded file (attachmentId, title)."},
	{Name: "attachment_detach", Method: "attachment/detach", Description: "Take files off an actor or a reaction (actorId, rootActorId for a reaction); the files stay in the workspace. Destructive: confirm with the user first."},
}
