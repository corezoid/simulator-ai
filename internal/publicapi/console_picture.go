package publicapi

import (
	"context"
	"errors"
	"fmt"
	"path"
	"strings"
)

// ActorPictureInput is legacy uploadActorPicture: an image's bytes (base64) to show as an actor's
// picture.
type ActorPictureInput struct {
	ActorID     string `json:"actorId"`
	FileName    string `json:"fileName"`
	Content     string `json:"content"`
	ContentType string `json:"contentType,omitempty"`
}

// ActorPicture names the uploaded file now set as the actor's picture.
type ActorPicture struct {
	ActorID string `json:"actorId"`
	Picture string `json:"picture"`
	FileID  int64  `json:"fileId"`
}

// pictureTypes are the image types the web app takes as a picture (simulator-ui-kit
// ACCEPTED_PICTURE_TYPES), by extension; SVG is refused there as a live document.
var pictureTypes = map[string]string{".png": "image/png", ".jpg": "image/jpeg", ".jpeg": "image/jpeg", ".gif": "image/gif", ".webp": "image/webp"}

type pictureUpdate struct {
	AccID   string   `json:"accId"`
	ActorID string   `json:"actorId"`
	Picture string   `json:"picture"`
	Fields  []string `json:"fields"`
}

// UploadActorPicture stores the image through the upload route (not attached to anything) and sets
// its fileName as the actor's picture, which is what the web apps write.
func (c *Client) UploadActorPicture(ctx context.Context, in ActorPictureInput) (ActorPicture, error) {
	if e := consoleOnly(c); e != nil {
		return ActorPicture{}, e
	}
	if !validID(in.ActorID) {
		return ActorPicture{}, errors.New("actorId required")
	}
	kind, ok := pictureTypes[strings.ToLower(path.Ext(in.FileName))]
	if !ok {
		return ActorPicture{}, errors.New("fileName must name a .png, .jpg, .jpeg, .gif or .webp image")
	}
	if in.ContentType != "" && in.ContentType != kind {
		return ActorPicture{}, fmt.Errorf("contentType %q does not match %s: leave it out or send %s", in.ContentType, in.FileName, kind)
	}
	f, err := c.UploadAttachment(ctx, AttachmentUploadInput{FileName: in.FileName, Content: in.Content, ContentType: kind})
	if err != nil {
		return ActorPicture{}, err
	}
	if _, err = call[pictureUpdate, Actor](ctx, c, "actor/update", pictureUpdate{AccID: c.config.Workspace, ActorID: in.ActorID, Picture: f.FileName, Fields: []string{"id"}}); err != nil {
		return ActorPicture{}, fmt.Errorf("%w (the image was uploaded as %s, file %d, and is not set)", err, f.FileName, f.ID)
	}
	return ActorPicture{ActorID: in.ActorID, Picture: f.FileName, FileID: f.ID}, nil
}
