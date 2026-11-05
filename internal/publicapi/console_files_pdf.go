package publicapi

import (
	"bytes"
	"compress/zlib"
	"encoding/ascii85"
	"io"
	"regexp"
	"strings"
	"time"

	"github.com/ledongthuc/pdf"
)

// The text of a PDF, with the pure-Go parser kept from taking the server down: it inflates every
// stream it reads without a bound, so before it runs every FlateDecode stream of the file is
// inflated here, counted and stopped at maxPDFInflate in all. Then the text is read page by page,
// at most maxPDFPages pages and maxReadText of text, within pdfTimeout.

const (
	maxPDFTextBytes = 10 << 20 // a larger PDF is not parsed for its text
	maxPDFInflate   = 64 << 20 // all of its streams inflated together
	maxPDFPages     = 200
)

// pdfTimeout bounds the parse; a variable so a test can shorten it.
var pdfTimeout = 10 * time.Second

// pdfText is the PDF's plain text, or why it was not read ("" when it has none: a scan).
func pdfText(body []byte) (text, unread string) {
	switch {
	case len(body) > maxPDFTextBytes:
		return "", "the PDF is larger than 10 MiB, so its text is not read"
	case bytes.Contains(body, []byte("/Encrypt")):
		return "", "the PDF is encrypted, so its text is not read"
	case !inflatesWithin(body, maxPDFInflate):
		return "", "the PDF's streams unpack to more than 64 MiB (or cannot be measured), so its text is not read"
	}
	done := make(chan string, 1)
	go func() { done <- parsePDFText(body) }()
	select {
	case text := <-done:
		return text, ""
	case <-time.After(pdfTimeout):
		// The parse goroutine runs on, bounded by the inflate check above, and its result is dropped.
		return "", "reading the PDF's text took too long"
	}
}

func parsePDFText(body []byte) (text string) {
	defer func() {
		if recover() != nil { // a malformed file must not take the server down
			text = ""
		}
	}()
	r, err := pdf.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		return ""
	}
	var b strings.Builder
	fonts := map[string]*pdf.Font{}
	for i := 1; i <= min(r.NumPage(), maxPDFPages) && b.Len() <= maxReadText; i++ {
		p := r.Page(i)
		if p.V.IsNull() {
			continue
		}
		for _, name := range p.Fonts() {
			if _, ok := fonts[name]; !ok {
				f := p.Font(name)
				fonts[name] = &f
			}
		}
		if t, err := p.GetPlainText(fonts); err == nil {
			b.WriteString(t)
		}
	}
	return strings.TrimSpace(strings.ToValidUTF8(b.String(), ""))
}

// streamStart finds the start of each stream's data: the end of its dictionary, the keyword and its
// end of line.
var streamStart = regexp.MustCompile(`>>\s*stream\r?\n`)

// inflatesWithin inflates every FlateDecode stream of the file (after ASCII85 when listed), counting
// what comes out, and says whether all of it stays within budget; a stream that cannot be inflated
// counts as over, since the parser would meet it unbounded.
func inflatesWithin(body []byte, budget int64) bool {
	left := budget
	for _, m := range streamStart.FindAllIndex(body, -1) {
		from := max(0, m[0]-4096)
		at := bytes.LastIndex(body[from:m[0]], []byte("obj"))
		if at < 0 {
			continue
		}
		dict := body[from+at : m[0]+2]
		if !bytes.Contains(dict, []byte("/FlateDecode")) {
			continue
		}
		var r io.Reader = bytes.NewReader(streamData(body[m[1]:], dict))
		if bytes.Contains(dict, []byte("/ASCII85Decode")) {
			r = ascii85.NewDecoder(r)
		}
		zr, err := zlib.NewReader(r)
		if err != nil {
			return false
		}
		n, _ := io.CopyN(io.Discard, zr, left+1)
		if left -= n; left < 0 {
			return false
		}
	}
	return true
}
