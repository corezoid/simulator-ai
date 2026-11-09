package mcpserver

// formTools are the legacy form tools (getForm, getForms, searchForms, createForm, updateForm,
// deleteForm, setFormStatus, getFormAccounts, removeFormAccount) on public methods;
// createFormAccount is the console form_account_create.
var formTools = []parityTool{
	{Name: "form_get", Method: "form/get", Description: "Read an account template (form) with its full body: sections and every field (a field id is its key in actor data). with editor adds canModify and the editToken a template update needs; with parents adds the ancestor chain. Needs view access."},
	{Name: "forms_tree_get", Method: "form/tree/get", Description: "Read the whole nesting tree of account templates that formId belongs to (root and every descendant), with parentId per node; a template you cannot view comes back as a stub {id, parentId, accessDenied}. This is getFormsTree; form_linked_get gives one template's children or parents."},
	{Name: "forms_list", Method: "form/list", Local: fullLocal, Shape: compactUnlessFull(formRows), Description: "List account templates one page at a time (limit up to 100): custom (the workspace's own) by default, type system/connector/all to switch; query matches title, ref or description (a number also matches the id); filter by status, owners, dates; sort with order/dir. Rows: id, title, type (with type all), isDefault; full adds owner, permission flags and dates. Rows have no body; read one with form_get. How many: forms_count with the same filters, never a count of pages; to name a few, ask only for that many (limit)."},
	{Name: "forms_count", Method: "form/count", Description: "Count exactly the templates forms_list would return for the same filters: custom (the workspace's own) by default, type all with system and connector ones. Answers how many in one call."},
	{Name: "form_create", Method: "form/create", Description: "Create an account template: title, sections with fields, optional parentId (inheritance), ref, color, settings, tags. isTemplate false makes a private draft that lists do not return; keep its id."},
	{Name: "form_update", Method: "form/update", Description: "Change an account template. expectedEditToken is the editToken from form_get with editor: a stale token is refused instead of overwriting another edit. Sections replace the field structure; field values of removed fields stay in actor data."},
	{Name: "form_status_update", Method: "form/status/update", Description: "Set a template's status (verified, rejected or pending)."},
	{Name: "form_delete", Method: "form/delete", Description: "Delete an account template. Destructive: confirm with the user first."},
	{Name: "form_accounts_list", Method: "account_form/list", Description: "List the account definitions of a template (account name, currency, type) that every actor of the template gets."},
	{Name: "form_account_delete", Method: "account_form/delete", Description: "Remove an account definition (id from form_accounts_list) from a template. Destructive: confirm with the user first."},
}
