package publicapi

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/ledongthuc/pdf"
)

// What attachment_read gives the model of a file (legacy readAttachment, and more): a text file's
// text; an image or a PDF itself, as an MCP content block, up to MaxInlineBytes; the text of a PDF,
// a .docx or a .xlsx, taken out without converters; of any other file its type and size.

// MaxInlineBytes bounds an image or a PDF sent along as a content block.
const MaxInlineBytes = 5 << 20

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
			out.ContentType, out.Data = imageType(media, body), body
		}
	case kind == "pdf" && whole:
		out.Kind, out.ContentType = "pdf", "application/pdf"
		out.Inline, out.TooLarge = len(body) <= MaxInlineBytes, len(body) > MaxInlineBytes
		if out.Inline {
			out.Data = body
		}
		out.Text, out.Truncated = cutText(pdfText(body))
	case (kind == "docx" || kind == "xlsx") && whole:
		text, err := officeText(kind, body)
		if err != nil {
			out.Kind, out.Binary = "binary", true
			return out, nil
		}
		out.Kind = kind
		out.Text, out.Truncated = cutText(text)
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
	case inlineImages[media] || media == "application/octet-stream" && inlineImages[sniffed] || media == "" && inlineImages[sniffed]:
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

func imageType(media string, body []byte) string {
	if inlineImages[media] {
		return media
	}
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

// pdfText is the plain text of a PDF, empty when it has none (a scan) or cannot be parsed; the
// parser is pure Go, and a malformed file must not take the server down with it.
func pdfText(body []byte) (text string) {
	defer func() {
		if recover() != nil {
			text = ""
		}
	}()
	r, err := pdf.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		return ""
	}
	plain, err := r.GetPlainText()
	if err != nil {
		return ""
	}
	b, err := io.ReadAll(io.LimitReader(plain, maxReadText+1))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(strings.ToValidUTF8(string(b), ""))
}

// AttachmentNote is the line an image or PDF block goes with, and what the model is told when the
// file was too large to send.
func (a AttachmentContent) AttachmentNote() string {
	switch {
	case a.Inline:
		return fmt.Sprintf("%s %q (%s, %d bytes) follows as a content block.", strings.ToUpper(a.Kind[:1])+a.Kind[1:], a.FileName, a.ContentType, a.Size)
	case a.TooLarge:
		return fmt.Sprintf("The %s is larger than the %d MiB inline limit, so it is not sent: ask the user to open it in Simulator or to share a smaller one.", a.Kind, MaxInlineBytes>>20)
	}
	return ""
}
