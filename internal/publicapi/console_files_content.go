package publicapi

import (
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"
	"unicode/utf8"
)

// What attachment_read gives the model of a file (legacy readAttachment, and more): a text file's
// text; an image itself, as an MCP image block, up to MaxInlineBytes; the text of a PDF, a .docx or
// a .xlsx, taken out without converters, and of a scanned PDF its page images; of any other file
// its type and size. Hermes passes image blocks to the model and nothing else binary (an embedded
// PDF it parks in a file the agent cannot read), so documents become text and images.

// MaxInlineBytes bounds an image sent along as a content block; maxPageImages the pages of a scan
// (Hermes passes up to four images of one tool result to the model).
const (
	MaxInlineBytes  = 5 << 20
	maxPageImages   = 4
	maxImagesPerRun = 8
)

const (
	docxType = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	xlsxType = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
)

var inlineImages = map[string]bool{"image/png": true, "image/jpeg": true, "image/gif": true, "image/webp": true}

func readContent(name string, resp *http.Response) (AttachmentContent, error) {
	out := AttachmentContent{FileName: name, ContentType: resp.Header.Get("Content-Type"), Size: resp.ContentLength}
	media, _, _ := mime.ParseMediaType(out.ContentType)
	if textual(out.ContentType) {
		return readText(out, resp.Body)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxDownloadBytes+1))
	if err != nil {
		return AttachmentContent{}, errors.New("cannot read the file")
	}
	out.Size, out.SizeAtLeast = min(int64(len(body)), maxDownloadBytes), len(body) > maxDownloadBytes
	whole := !out.SizeAtLeast
	switch kind := kindOf(media, body); {
	case kind == "image":
		out.Kind = "image"
		out.Inline, out.TooLarge = whole && len(body) <= MaxInlineBytes, !whole || len(body) > MaxInlineBytes
		if out.Inline {
			out.ContentType, out.Data = imageType(body), body
		}
	case kind == "pdf" && whole:
		// Hermes hands the model an image block, never a PDF one: a PDF is its text, and a scan
		// without text its page images.
		out.Kind, out.ContentType = "pdf", "application/pdf"
		text, unread := pdfText(body)
		out.Text, out.Truncated = cutText(text)
		if out.Text == "" {
			out.Pages = scanPages(body, maxPageImages)
			out.Inline, out.Scanned, out.Note = len(out.Pages) > 0, unread == "", unread
		}
	case (kind == "docx" || kind == "xlsx") && whole:
		text, cut, err := officeText(kind, body)
		if err != nil {
			out.Kind, out.Binary = "binary", true
			return out, nil
		}
		out.Kind = kind
		out.Text, out.Truncated = cutText(text)
		out.Truncated = out.Truncated || cut
	default:
		out.Kind, out.Binary = "binary", true
	}
	return out, nil
}

func readText(out AttachmentContent, body io.Reader) (AttachmentContent, error) {
	out.Kind = "text"
	head, err := io.ReadAll(io.LimitReader(body, maxReadText+1))
	if err != nil {
		return AttachmentContent{}, errors.New("cannot read the file")
	}
	if out.Size < 0 {
		rest, err := io.Copy(io.Discard, io.LimitReader(body, maxDownloadBytes+1))
		if err != nil {
			return AttachmentContent{}, errors.New("cannot read the file")
		}
		n := int64(len(head)) + rest
		out.Size, out.SizeAtLeast = min(n, maxDownloadBytes), n > maxDownloadBytes
	}
	if len(head) > maxReadText {
		head, out.Truncated = head[:maxReadText], true
		// The cut may split the last character.
		for i := 0; i < 3 && !utf8.Valid(head); i++ {
			head = head[:len(head)-1]
		}
	}
	if !utf8.Valid(head) {
		out.Kind, out.Binary = "binary", true
		return out, nil
	}
	out.Text = string(head)
	return out, nil
}

// kindOf names the file by its declared type, else by its first bytes (a store may answer
// application/octet-stream); an Office file is told apart by the parts inside its zip.
func kindOf(media string, body []byte) string {
	sniffed, _, _ := mime.ParseMediaType(http.DetectContentType(body))
	switch {
	// An image is one by its bytes too: a declared image/png that is not one is no image block.
	case inlineImages[sniffed] && (inlineImages[media] || media == "application/octet-stream" || media == ""):
		return "image"
	case media == "application/pdf" || sniffed == "application/pdf":
		return "pdf"
	case media == docxType:
		return "docx"
	case media == xlsxType:
		return "xlsx"
	case sniffed == "application/zip":
		return officeKind(body)
	}
	return "binary"
}

// imageType is the type the image's bytes say it is.
func imageType(body []byte) string {
	sniffed, _, _ := mime.ParseMediaType(http.DetectContentType(body))
	return sniffed
}

// cutText bounds extracted text to maxReadText, on a character boundary.
func cutText(text string) (string, bool) {
	if len(text) <= maxReadText {
		return text, false
	}
	cut := text[:maxReadText]
	for i := 0; i < 3 && !utf8.ValidString(cut); i++ {
		cut = cut[:len(cut)-1]
	}
	return cut, true
}

// AttachmentNote is the line the image blocks go with, and what the model is told when nothing of
// the file could be sent.
func (a AttachmentContent) AttachmentNote() string {
	var note string
	switch {
	case a.Inline && a.Kind == "image":
		note = fmt.Sprintf("Image %q (%s, %d bytes) follows as an image block.", a.FileName, a.ContentType, a.Size)
	case a.Inline:
		why := "The PDF has no text layer (a scan)"
		if a.Note != "" {
			why = strings.ToUpper(a.Note[:1]) + a.Note[1:]
		}
		note = fmt.Sprintf("%s: its first %d page images follow as image blocks.", why, len(a.Pages))
	case a.TooLarge:
		note = fmt.Sprintf("The image is larger than the %d MiB inline limit, so it is not sent: ask the user to open it in Simulator or to share a smaller one.", MaxInlineBytes>>20)
	case a.Kind == "pdf" && a.Text == "" && a.ImagesSkipped == 0:
		why := "The PDF has no text layer (a scan) and its pages are not JPEG images that can be sent"
		if a.Note != "" {
			why = strings.ToUpper(a.Note[:1]) + a.Note[1:]
		}
		note = why + ", so its content is not available: ask the user for a text version or to describe it."
	}
	if a.ImagesSkipped > 0 {
		note = strings.TrimSpace(note + fmt.Sprintf(" %d image(s) of this file were not sent: this request already sent %d images, the most one request may; ask the user which to look at and read it again in a new request.", a.ImagesSkipped, maxImagesPerRun))
	}
	return note
}

// limitImages holds back the image blocks past maxImagesPerRun in this process (one Console run):
// every image goes again with each later step of the run, so they are not let pile up.
func (c *Client) limitImages(a *AttachmentContent) {
	n := len(a.Pages)
	if a.Kind == "image" && a.Inline {
		n = 1
	}
	if n == 0 {
		return
	}
	over := min(int(c.imagesSent.Add(int64(n))-maxImagesPerRun), n)
	if over <= 0 {
		return
	}
	c.imagesSent.Add(-int64(over))
	a.ImagesSkipped = over
	if a.Kind == "image" {
		a.Data, a.Inline = nil, false
		return
	}
	a.Pages = a.Pages[:n-over]
	a.Inline = len(a.Pages) > 0
}
