package mcpserver

// accessRequestTools are the legacy requestAccess and its waiting room on public methods: someone
// who cannot see an actor asks its managers, who confirm or turn the request down.
var accessRequestTools = []parityTool{
	{Name: "access_request_create", Method: "access_rule/actor/request/create", Description: "Ask the people who manage an actor you cannot see (an actor read failed for lack of access) for view, or view and modify with modify true. It grants nothing: they confirm or turn it down. Asking again while a request is pending returns that request; with view already it is refused. sendEvent false skips the event they would sign or reject. Returns the pending request (invite.id)."},
	{Name: "access_requests_list", Method: "access_rule/actor/request/list", Description: "List the pending access requests to an actor (its waiting room): who asked, for modify too or not, when; oldest first, at most 200 (hasMore). Needs modify on the actor."},
	{Name: "access_request_confirm", Method: "access_rule/actor/request/confirm", Description: "Grant a pending access request (inviteId from access_requests_list): the person gets view, and modify if they asked for it, and is notified; mirrorSign false skips the sign reaction on the request's event. Needs modify on the actor. Confirm with the user first."},
	{Name: "access_request_cancel", Method: "access_rule/actor/request/cancel", Description: "Withdraw your own pending access request or turn down someone's (inviteId); nothing is granted, and mirrorSign false skips the reject reaction on the request's event. Turning one down needs modify on the actor."},
}
