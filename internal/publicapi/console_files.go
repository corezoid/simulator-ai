package publicapi

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"
)

// Files move over sim-api's HTTP routes, not JSON-RPC: POST /api/2.0/rest/upload (multipart, ufs
// limits and antivirus) stores a file as an attachment, GET /api/1.0/download/<fileName> serves it.
// The tool carries the bytes as base64 in its arguments, so a run in a container needs no file path.

// MaxUploadBytes keeps a file's base64 under the console's 960 KiB argument bound.
const MaxUploadBytes = 700 << 10

// maxReadText is how much of a text file attachment_read returns; maxDownloadBytes bounds a read.
const (
	maxReadText      = 100 << 10
	maxDownloadBytes = 32 << 20
)

// scanWait is the first wait for a file ufs is still scanning; each retry doubles it: four for a
// 425, two (about 3 s) for a 404, which is also a file that does not exist.
var scanWait = time.Second

// The media types of common files, so a file's type does not depend on the host's mime.types. They
// are bare: a stand's allow-list (fileMimeTypes) compares the type exactly. Registering them changes
// the process-wide mime table on purpose; this binary has no other user of it.
func init() {
	for ext, kind := range map[string]string{
		".txt": "text/plain", ".md": "text/markdown", ".csv": "text/csv",
		".json": "application/json", ".yaml": "application/yaml", ".yml": "application/yaml", ".xml": "application/xml",
		".pdf": "application/pdf", ".zip": "application/zip", ".png": "image/png", ".jpg": "image/jpeg", ".jpeg": "image/jpeg",
		".docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		".xlsx": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
	} {
		_ = mime.AddExtensionType(ext, kind)
	}
}

type AttachmentUploadInput struct {
	FileName    string `json:"fileName"`
	Content     string `json:"content"` // base64 (standard, padded)
	ContentType string `json:"contentType,omitempty"`
	ActorID     string `json:"actorId,omitempty"`
	RootActorID string `json:"rootActorId,omitempty"`
	TTL         int64  `json:"ttl,omitempty"`
}

// Attachment is an uploaded file: id for attachment/attach and reaction attachmentIds, fileName
// for the download; attachedTo is the actor (or reaction) it now hangs on.
type Attachment struct {
	ID         int64  `json:"id"`
	Title      string `json:"title"`
	Type       string `json:"type"`
	Size       int64  `json:"size"`
	FileName   string `json:"fileName"`
	AttachedTo string `json:"attachedTo,omitempty"`
}

// UploadAttachment stores one file in the workspace, optionally attached to an actor or reaction.
func (c *Client) UploadAttachment(ctx context.Context, in AttachmentUploadInput) (Attachment, error) {
	if e := consoleOnly(c); e != nil {
		return Attachment{}, e
	}
	name := strings.TrimSpace(in.FileName)
	if name == "" || len(name) > 255 || strings.ContainsAny(name, "/\\\r\n\"") || len(in.ActorID) > 255 || len(in.RootActorID) > 255 || in.RootActorID != "" && in.ActorID == "" || in.TTL < 0 {
		return Attachment{}, errors.New("a fileName without path separators or quotes, and rootActorId only with actorId, required")
	}
	if in.TTL > 0 && in.ActorID != "" {
		// sim-api never finds a temporary file to attach, in the upload route or attachment/attach.
		return Attachment{}, errors.New("a temporary file (ttl) cannot be attached: drop ttl or actorId")
	}
	if base64.StdEncoding.DecodedLen(len(in.Content)) > MaxUploadBytes+2 {
		return Attachment{}, fmt.Errorf("content over %d KiB: upload larger files from the Simulator UI", MaxUploadBytes>>10)
	}
	data, err := base64.StdEncoding.DecodeString(in.Content)
	if err != nil || len(data) == 0 || len(data) > MaxUploadBytes {
		return Attachment{}, fmt.Errorf("content must be standard base64 of 1 byte..%d KiB", MaxUploadBytes>>10)
	}
	kind := in.ContentType
	if kind == "" {
		kind = mime.TypeByExtension(path.Ext(name))
	}
	if kind == "" {
		kind = "application/octet-stream"
	}
	media, _, err := mime.ParseMediaType(kind)
	if kind = mime.FormatMediaType(media, nil); err != nil || kind == "" {
		return Attachment{}, errors.New("contentType must be a media type such as text/plain")
	}
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	// The route attaches to actorId itself and refuses an attach it may not make before storing the
	// file (sim-api 606ff98e); an older stand stored the file and only logged the failure.
	fields := [][2]string{{"accId", c.config.Workspace}, {"actorId", in.ActorID}, {"rootActorId", in.RootActorID}}
	if in.TTL > 0 {
		fields = append(fields, [2]string{"ttl", strconv.FormatInt(in.TTL, 10)})
	}
	for _, f := range fields {
		if f[1] != "" {
			_ = form.WriteField(f[0], f[1])
		}
	}
	h := textproto.MIMEHeader{}
	h.Set("Content-Disposition", mime.FormatMediaType("form-data", map[string]string{"name": "file", "filename": name}))
	h.Set("Content-Type", kind)
	part, _ := form.CreatePart(h)
	_, _ = part.Write(data)
	if err = form.Close(); err != nil {
		return Attachment{}, errors.New("cannot encode the upload")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.config.Origin, "/")+"/api/2.0/rest/upload", &body)
	if err != nil {
		return Attachment{}, errors.New("invalid request")
	}
	req.Header.Set("Authorization", c.config.authorization())
	req.Header.Set("Content-Type", form.FormDataContentType())
	resp, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return Attachment{}, ctx.Err()
		}
		return Attachment{}, errors.New("upload transport unavailable; outcome unknown; retry the upload (a duplicate is a harmless unattached file)")
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if resp.StatusCode != http.StatusOK {
		return Attachment{}, fmt.Errorf("upload refused (http %d): %s", resp.StatusCode, serverText(b))
	}
	var a Attachment
	if json.Unmarshal(b, &a) != nil || a.ID < 1 || a.FileName == "" {
		return Attachment{}, errors.New("upload returned no attachment")
	}
	if in.ActorID == "" {
		return a, nil
	}
	// The route checks the attach before storing the file and refuses it with a 400 (sim-api
	// 606ff98e, in every revision these tools need), so a 200 means the file hangs there.
	a.AttachedTo = in.ActorID
	return a, nil
}

type AttachmentReadInput struct {
	FileName string `json:"fileName"`
}

// AttachmentContent is a downloaded file: its type and size, and the text of a text file.
type AttachmentContent struct {
	FileName    string `json:"fileName"`
	ContentType string `json:"contentType"`
	Size        int64  `json:"size"`
	SizeAtLeast bool   `json:"sizeAtLeast,omitempty"` // the file is larger than size
	// Kind is what was read: text, image, pdf, docx, xlsx or binary (type and size only).
	Kind      string `json:"kind"`
	Text      string `json:"text,omitempty"` // a text file, or the text taken out of a pdf, docx or xlsx
	Truncated bool   `json:"truncated,omitempty"`
	Binary    bool   `json:"binary,omitempty"`
	// Inline says image blocks follow: the image (Data), or a scanned PDF's page images (Pages).
	// TooLarge: an image over the inline limit. Scanned: a PDF without a text layer.
	Inline   bool `json:"inline,omitempty"`
	TooLarge bool `json:"tooLarge,omitempty"`
	Scanned  bool `json:"scanned,omitempty"`
	// Note says why a PDF's text was not read, or not all of it (too large, a stream bomb, too slow).
	Note  string   `json:"note,omitempty"`
	Data  []byte   `json:"-"`
	Pages [][]byte `json:"-"` // JPEG
}

// ReadAttachment downloads a file by its storage key (fileName). A file ufs is still scanning is
// retried with backoff: sim-api's own route answers 425, a stand serving the route from ufs 404
// (also for a quarantined file); sim-api answers 403 for a quarantined one, never served.
func (c *Client) ReadAttachment(ctx context.Context, in AttachmentReadInput) (AttachmentContent, error) {
	if e := consoleOnly(c); e != nil {
		return AttachmentContent{}, e
	}
	if in.FileName == "" || len(in.FileName) > 255 || strings.ContainsAny(in.FileName, `/\`) || strings.Contains(in.FileName, "..") {
		return AttachmentContent{}, errors.New("fileName: the storage key from an attachment (fileName), not a title")
	}
	target := strings.TrimRight(c.config.Origin, "/") + "/api/1.0/download/" + url.PathEscape(in.FileName)
	for wait, tries := scanWait, 0; ; wait, tries = wait*2, tries+1 {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
		if err != nil {
			return AttachmentContent{}, errors.New("invalid request")
		}
		resp, err := c.http.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return AttachmentContent{}, ctx.Err()
			}
			return AttachmentContent{}, errors.New("download transport unavailable")
		}
		if resp.StatusCode == http.StatusTooEarly && tries < 4 || resp.StatusCode == http.StatusNotFound && tries < 2 {
			_ = resp.Body.Close()
			select {
			case <-ctx.Done():
				return AttachmentContent{}, ctx.Err()
			case <-time.After(wait):
			}
			continue
		}
		defer func() { _ = resp.Body.Close() }()
		switch resp.StatusCode {
		case http.StatusOK:
			return readContent(in.FileName, resp)
		case http.StatusNotFound:
			return AttachmentContent{}, errors.New("no such file, or its antivirus scan has not finished (or it is quarantined) on a stand serving files from ufs; try again later")
		case http.StatusTooEarly:
			return AttachmentContent{}, errors.New("the antivirus scan of the file has not finished; try again later")
		case http.StatusForbidden:
			return AttachmentContent{}, errors.New("the file is quarantined by the antivirus and is never served")
		}
		return AttachmentContent{}, fmt.Errorf("download failed (http %d)", resp.StatusCode)
	}
}

func textual(contentType string) bool {
	t, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return false
	}
	switch {
	case strings.HasPrefix(t, "text/"), strings.HasSuffix(t, "+json"), strings.HasSuffix(t, "+xml"):
		return true
	}
	switch t {
	case "application/json", "application/xml", "application/javascript", "application/x-yaml", "application/yaml", "application/csv":
		return true
	}
	return false
}

// serverText is the bounded message of a refused upload: the JSON message or the plain text.
func serverText(b []byte) string {
	var m struct {
		Message string `json:"message"`
	}
	s := strings.TrimSpace(string(b))
	if json.Unmarshal(b, &m) == nil && m.Message != "" {
		s = m.Message
	}
	if len(s) > 500 {
		s = strings.ToValidUTF8(s[:500], "") + "…"
	}
	return s
}
