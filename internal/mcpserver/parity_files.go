package mcpserver

// attachmentTools are the legacy getAttachments, getActorAttachments, addAttachments,
// updateAttachment and removeAttachments on public methods.
var attachmentTools = []parityTool{
	{Name: "attachments_list", Method: "attachment/list", Description: "List the files you can see anywhere in the workspace, newest first; people (uploader ids) and starred narrow it; one page per call (limit 1..100, offset). Each has id (for attach and reactions) and fileName (for attachment_read)."},
	{Name: "attachments_by_actor_list", Method: "attachment/by-actor/list", Description: "List the files attached to one actor (actorId), one page per call (limit/offset). Each has id and fileName (for attachment_read). Needs view on the actor."},
	{Name: "attachment_attach", Method: "attachment/attach", Description: "Hang already uploaded files (attachmentIds from attachment_upload or attachments_list) on an actor, or on a reaction (actorId = the reaction, rootActorId = its actor). A file already there changes nothing. Needs modify on the actor."},
	{Name: "attachment_update", Method: "attachment/update", Description: "Rename an uploaded file (attachmentId, title)."},
	{Name: "attachment_detach", Method: "attachment/detach", Description: "Take files off an actor or a reaction (actorId, rootActorId for a reaction); the files stay in the workspace. Destructive: confirm with the user first."},
}
