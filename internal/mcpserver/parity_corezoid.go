package mcpserver

// corezoidTools link a Smart Form to the Corezoid processes behind its pages.
var corezoidTools = []parityTool{
	{Name: "corezoid_projects_list", Method: "corezoid/project/list", Description: "List the workspace's Corezoid projects you can see: id and title. Pick one for corezoid_project_bind, then its stages and processes."},
	{Name: "corezoid_stages_list", Method: "corezoid/stage/list", Description: "List the stages of a Corezoid project of the workspace (e.g. develop and production): id and title."},
	{Name: "corezoid_processes_list", Method: "corezoid/process/list", Description: "List the processes of a stage of a Corezoid project, folders walked: id (the procId of corezoid_project_bind) and title."},
	{Name: "corezoid_project_get", Method: "corezoid/project/get", Description: "Read a Smart Form's Corezoid project link (actorId is the appId): status (ready once corezoid_project_create's task is done), project, stages and the processUrl of each stage's process."},
	{Name: "corezoid_project_create", Method: "corezoid/project/create", Destructive: true, Description: "Give a Smart Form without one a Corezoid project of its own (actorId is the appId): an import task creates a project with develop and production stages, each with a stub handler process, and rebinds both envs to them. Poll corezoid_project_get until status is ready. Refused when the form already has a project. Confirm with the user first."},
	{Name: "corezoid_project_bind", Method: "corezoid/project/bind", Destructive: true, Description: "Bind every env of a Smart Form (actorId is the appId) to an existing process: envs [{envTitle develop|production, stageId, procId}] of one project, all envs named once. The server shares each process with the form's API user (creating one if needed) and writes the envs' credentials; no secret is sent or returned. It replaces the current binding and is not undone in Corezoid: name the processes and get the user's yes first."},
}
