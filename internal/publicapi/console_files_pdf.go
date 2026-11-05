package publicapi

import (
	"bytes"
	"errors"
	"strings"
	"time"

	"github.com/corezoid/simulator-ai/internal/third_party/pdf"
)

// The text of a PDF, read by the pure-Go parser forked into internal/third_party/pdf with limits:
// every byte any stream filter layer produces counts against maxPDFInflate for the whole file, and
// every stream read and every read of the file checks the deadline, so a filter bomb or a parse
// that would not end stops where the limit is, wherever the PDF hides it. Text is read page by
// page, at most maxPDFPages pages and maxReadText of text.

const (
	maxPDFTextBytes = 10 << 20 // a larger PDF is not parsed for its text
	maxPDFInflate   = 64 << 20 // all of its streams decoded together
	maxPDFPages     = 200
)

// pdfTimeout bounds the parse; a variable so a test can shorten it.
var pdfTimeout = 10 * time.Second

// pdfText is the PDF's plain text and, when it was cut short or not read, why ("" with no reason:
// the PDF has no text, a scan).
func pdfText(body []byte) (text, unread string) {
	if len(body) > maxPDFTextBytes {
		return "", "the PDF is larger than 10 MiB, so its text is not read"
	}
	limits := &pdf.Limits{Bytes: maxPDFInflate, Deadline: time.Now().Add(pdfTimeout)}
	r, err := openPDF(body, limits)
	if err != nil {
		return "", limitReason(err)
	}
	var b strings.Builder
	fonts := map[string]*pdf.Font{}
	for i := 1; i <= min(r.NumPage(), maxPDFPages) && b.Len() <= maxReadText; i++ {
		t, err := pageText(r, i, fonts)
		if errors.Is(err, pdf.ErrLimit) {
			unread = limitReason(err)
			break
		}
		b.WriteString(t)
	}
	return strings.TrimSpace(strings.ToValidUTF8(b.String(), "")), unread
}

// openPDF opens the file within the limits; a malformed file is an error, never a panic.
func openPDF(body []byte, limits *pdf.Limits) (r *pdf.Reader, err error) {
	defer func() {
		if x := recover(); x != nil {
			r, err = nil, panicError(x)
		}
	}()
	return pdf.NewReaderLimited(deadlineReader{bytes.NewReader(body), limits}, int64(len(body)), limits)
}

// pageText is one page's text; a page that cannot be read gives none, and a spent limit stops.
func pageText(r *pdf.Reader, i int, fonts map[string]*pdf.Font) (text string, err error) {
	defer func() {
		if x := recover(); x != nil {
			text, err = "", panicError(x)
		}
	}()
	p := r.Page(i)
	if p.V.IsNull() {
		return "", nil
	}
	for _, name := range p.Fonts() {
		if _, ok := fonts[name]; !ok {
			f := p.Font(name)
			fonts[name] = &f
		}
	}
	text, err = p.GetPlainText(fonts)
	if err != nil && !errors.Is(err, pdf.ErrLimit) {
		return "", nil // a malformed page: the next one may still read
	}
	return text, err
}

func panicError(x any) error {
	if e, ok := x.(error); ok {
		return e
	}
	return errors.New("malformed PDF")
}

func limitReason(err error) string {
	if errors.Is(err, pdf.ErrLimit) {
		return "the PDF's streams unpack past 64 MiB or reading it took too long, so its text is not read further"
	}
	return ""
}

// deadlineReader checks the deadline on every read of the file, so the parser's own loops over the
// file (not only its streams) stop too.
type deadlineReader struct {
	r      *bytes.Reader
	limits *pdf.Limits
}

func (d deadlineReader) ReadAt(p []byte, off int64) (int, error) {
	if !d.limits.Deadline.IsZero() && time.Now().After(d.limits.Deadline) {
		panic(pdf.ErrLimit)
	}
	return d.r.ReadAt(p, off)
}
