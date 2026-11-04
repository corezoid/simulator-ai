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

var pictureExtensions = map[string]bool{".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".webp": true, ".svg": true}

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
	if !pictureExtensions[strings.ToLower(path.Ext(in.FileName))] {
		return ActorPicture{}, errors.New("fileName must name an image: .png, .jpg, .jpeg, .gif, .webp or .svg")
	}
	f, err := c.UploadAttachment(ctx, AttachmentUploadInput{FileName: in.FileName, Content: in.Content, ContentType: in.ContentType})
	if err != nil {
		return ActorPicture{}, err
	}
	if _, err = call[pictureUpdate, Actor](ctx, c, "actor/update", pictureUpdate{AccID: c.config.Workspace, ActorID: in.ActorID, Picture: f.FileName, Fields: []string{"id"}}); err != nil {
		return ActorPicture{}, fmt.Errorf("%w (the image was uploaded as %s, file %d, and is not set)", err, f.FileName, f.ID)
	}
	return ActorPicture{ActorID: in.ActorID, Picture: f.FileName, FileID: f.ID}, nil
}
