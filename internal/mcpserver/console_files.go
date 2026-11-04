package mcpserver

import (
	"strconv"

	"github.com/corezoid/simulator-ai/internal/publicapi"
	"github.com/mark3labs/mcp-go/server"
)

// addConsoleFiles registers the file tools over sim-api's upload and download routes.
func addConsoleFiles(s *server.MCPServer, c *publicapi.Client) {
	consoleTool(s, "attachment_upload", "Upload one file (up to "+strconv.Itoa(publicapi.MaxUploadBytes>>10)+" KiB) into the workspace: content is its bytes in standard base64, fileName becomes its title, contentType defaults from the extension. actorId attaches it to an actor, or to a reaction with rootActorId its actor (needs modify; a refused attach is an error); ttl (seconds) makes it temporary, and a temporary file can be neither attached nor put in a reaction. Returns {id, title, type, size, fileName, attachedTo}: id for reaction_create attachmentIds and attachment_attach, fileName for attachment_read. The antivirus scans it before it can be read.",
		`{"type":"object","properties":{"fileName":{"type":"string","minLength":1,"maxLength":255},"content":{"type":"string","minLength":4,"maxLength":`+strconv.Itoa((publicapi.MaxUploadBytes+2)/3*4)+`},"contentType":{"type":"string","maxLength":255},"actorId":{"type":"string","minLength":1,"maxLength":255},"rootActorId":{"type":"string","minLength":1,"maxLength":255},"ttl":{"type":"integer","minimum":1}},"required":["fileName","content"],"additionalProperties":false}`, true, c.UploadAttachment)
	consoleTool(s, "attachment_read", "Read an uploaded file by its fileName (the storage key from attachment_upload, attachments_list or attachments_by_actor_list, not its title): contentType and size, and for a text file (text/*, JSON, XML, YAML, CSV) its text, the first 100 KiB (truncated when longer). A binary file returns only binary true, its type and size (sizeAtLeast when larger). A file still being scanned is retried for several seconds (a stand serving files from ufs answers not found until the scan ends); a quarantined one is never served.",
		`{"type":"object","properties":{"fileName":{"type":"string","minLength":1,"maxLength":255}},"required":["fileName"],"additionalProperties":false}`, false, c.ReadAttachment)
	consoleTool(s, "actor_picture_upload", "Set an actor's picture (its avatar) from an image (up to "+strconv.Itoa(publicapi.MaxUploadBytes>>10)+" KiB): content is the bytes in standard base64, fileName ends in .png, .jpg, .jpeg, .gif or .webp (no SVG: the web app refuses it as a picture), contentType if sent matches it. The image is uploaded as a file of the workspace (not attached) and its fileName written to the actor's picture, as the web app does; needs modify on the actor. Returns {actorId, picture, fileId}.",
		`{"type":"object","properties":{"actorId":{"type":"string","minLength":1,"maxLength":255},"fileName":{"type":"string","minLength":5,"maxLength":255},"content":{"type":"string","minLength":4,"maxLength":`+strconv.Itoa((publicapi.MaxUploadBytes+2)/3*4)+`},"contentType":{"type":"string","enum":["image/png","image/jpeg","image/gif","image/webp"]}},"required":["actorId","fileName","content"],"additionalProperties":false}`, true, c.UploadActorPicture)
}
