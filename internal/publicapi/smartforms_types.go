package publicapi

// These named projections follow contracts/smartforms-openrpc.json. Credentials,
// application.data and arbitrary backend fields never enter a tool result.

// Workspace is injected by Client, never taken from tool arguments.
type Workspace struct {
	AccID string `json:"accId"`
}

// Operation identifies a principal/workspace/method-scoped receipt.
type Operation struct {
	OperationID string `json:"operationId"`
}

// AppTarget is the public actorId binding of the Smart Form methods.
type AppTarget struct {
	ActorID string `json:"actorId"`
}

// CreateApplicationInput projects application/create; CDU v2 is fixed by Client.
type CreateApplicationInput struct {
	Workspace
	Operation
	Ref         string `json:"ref"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	CduVersion  int64  `json:"cduVersion"`
}

// ApplicationOperationInput follows application/operation/get.
type ApplicationOperationInput struct {
	Workspace
	Operation
}

// AppInput follows application/env/list and route/key/get.
type AppInput struct {
	Workspace
	AppTarget
}

// SnapshotInput follows application/snapshot/get.
type SnapshotInput struct {
	Workspace
	AppTarget
	EnvID int64 `json:"envId"`
}

// Environment projects public application/env/list and application/create envs.
type Environment struct {
	ID           int64  `json:"id"`
	ActorID      string `json:"actorId"`
	Title        string `json:"title"`
	RootFolderID int64  `json:"rootFolderId"`
	Readonly     bool   `json:"readonly"`
}

// CreatedApplication projects application/create and its receipt readback.
type CreatedApplication struct {
	Application Actor         `json:"application"`
	Envs        []Environment `json:"envs"`
}

// Environments projects application/env/list without Corezoid credentials.
type Environments struct {
	CanModify bool          `json:"canModify"`
	Envs      []Environment `json:"envs"`
}

// ContentNode follows application/snapshot/get tree. Children recursively use
// the same tree node shape (backend rpc_application.structNode), although the
// OpenRPC recursion boundary describes items only as objects.
type ContentNode struct {
	ID         int64         `json:"id"`
	ObjType    string        `json:"objType"`
	Title      string        `json:"title"`
	Type       string        `json:"type,omitempty"`
	Source     *string       `json:"source,omitempty"`
	SourceHash string        `json:"sourceHash,omitempty"`
	IsStatic   bool          `json:"isStatic,omitempty"`
	IsSystem   bool          `json:"isSystem,omitempty"`
	Children   []ContentNode `json:"children,omitempty"`
}

// Snapshot couples the live tree with its authoritative content fingerprint.
type Snapshot struct {
	EnvironmentHash string      `json:"environmentHash"`
	Tree            ContentNode `json:"tree"`
}

// FolderItem follows app_folder/create items.
type FolderItem struct {
	FolderID int64  `json:"folderId"`
	Title    string `json:"title"`
	IsStatic bool   `json:"isStatic,omitempty"`
}

// CreateFoldersInput follows app_folder/create with a mandatory receipt key.
type CreateFoldersInput struct {
	Workspace
	AppTarget
	Operation
	Items []FolderItem `json:"items"`
}

// Folder follows app_folder/create and app_folder/operation/get folders.
type Folder struct {
	ID       int64  `json:"id"`
	FolderID int64  `json:"folderId"`
	Title    string `json:"title"`
	ObjType  string `json:"objType"`
	IsStatic bool   `json:"isStatic"`
}
type Folders struct {
	Folders []Folder `json:"folders"`
}

// FileItem follows app_file/create items; source is always explicit in this profile.
type FileItem struct {
	FolderID int64  `json:"folderId"`
	Title    string `json:"title"`
	Type     string `json:"type"`
	Source   string `json:"source"`
}
type CreateFilesInput struct {
	Workspace
	AppTarget
	Operation
	Items []FileItem `json:"items"`
}

// FileUpdate follows app_file/update items. Pointer fields preserve explicit clearing.
type FileUpdate struct {
	ID          int64   `json:"id"`
	IfMatchHash string  `json:"ifMatchHash"`
	FolderID    *int64  `json:"folderId,omitempty"`
	Title       string  `json:"title"`
	Type        string  `json:"type"`
	Source      *string `json:"source,omitempty"`
}

// EnvironmentPrecondition follows app_file/update environmentPreconditions.
type EnvironmentPrecondition struct {
	EnvID           int64  `json:"envId"`
	EnvironmentHash string `json:"environmentHash"`
}
type UpdateFilesInput struct {
	Workspace
	AppTarget
	Operation
	Items                    []FileUpdate              `json:"items"`
	EnvironmentPreconditions []EnvironmentPrecondition `json:"environmentPreconditions"`
}

// File follows app_file/create, update and operation/get files.
type File struct {
	ID         int64  `json:"id"`
	FolderID   int64  `json:"folderId"`
	ObjType    string `json:"objType"`
	Title      string `json:"title"`
	Type       string `json:"type"`
	Source     string `json:"source"`
	SourceHash string `json:"sourceHash"`
}
type Files struct {
	Files []File `json:"files"`
}

// AppOperationInput follows app_folder/operation/get and release/operation/get.
type AppOperationInput struct {
	Workspace
	AppTarget
	Operation
}

// FileOperationInput selects only the public file create/update receipt methods.
type FileOperationInput struct {
	Workspace
	AppTarget
	Operation
	Method string `json:"method"`
}

// CreateReleaseInput follows the keyed release/create contract.
type CreateReleaseInput struct {
	Workspace
	AppTarget
	Operation
	SourceEnvID                   int64  `json:"sourceEnvId"`
	TargetEnvID                   int64  `json:"targetEnvId"`
	ExpectedSourceEnvironmentHash string `json:"expectedSourceEnvironmentHash"`
	ExpectedTargetEnvironmentHash string `json:"expectedTargetEnvironmentHash"`
}
type ReleaseInput struct {
	Workspace
	AppTarget
	ReleaseID int64 `json:"releaseId"`
}

// Release projects release/create, release/get and release/operation/get.
// Receipt status/retention are historical; GetRelease reads current state.
type Release struct {
	ID                int64  `json:"id"`
	ActorID           string `json:"actorId"`
	EnvID             int64  `json:"envId"`
	SourceEnvID       int64  `json:"sourceEnvId"`
	ReleaseNumber     int64  `json:"releaseNumber"`
	Status            string `json:"status"`
	PrunedByRetention bool   `json:"prunedByRetention"`
}

// RouteKey follows public route/key/get. It never grants viewer access.
type RouteKey struct {
	AppKey          string `json:"appKey"`
	ActiveReleaseID int64  `json:"activeReleaseId"`
	SuggestedAppKey string `json:"suggestedAppKey"`
}
