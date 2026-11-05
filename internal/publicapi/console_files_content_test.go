package publicapi

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func pngBytes(t *testing.T) []byte {
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// pdfBytes is a one-page PDF whose text layer says text, its xref offsets counted.
func pdfBytes(text string) []byte {
	stream := fmt.Sprintf("BT /F1 12 Tf 72 720 Td (%s) Tj ET", text)
	objects := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 4 0 R /Resources << /Font << /F1 5 0 R >> >> >>",
		fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(stream), stream),
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>",
	}
	var b bytes.Buffer
	b.WriteString("%PDF-1.4\n")
	offsets := make([]int, len(objects))
	for i, o := range objects {
		offsets[i] = b.Len()
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", i+1, o)
	}
	xref := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", len(objects)+1)
	for _, off := range offsets {
		fmt.Fprintf(&b, "%010d 00000 n \n", off)
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objects)+1, xref)
	return b.Bytes()
}

func zipBytes(t *testing.T, parts map[string]string) []byte {
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	for name, body := range parts {
		w, err := z.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		fmt.Fprint(w, body)
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func docxBytes(t *testing.T) []byte {
	return zipBytes(t, map[string]string{"word/document.xml": `<?xml version="1.0"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>` +
		`<w:p><w:r><w:t>Договір</w:t></w:r><w:r><w:tab/><w:t xml:space="preserve"> № 7</w:t></w:r></w:p><w:p><w:r><w:t>Сума: 100</w:t><w:br/><w:t>USD</w:t></w:r></w:p></w:body></w:document>`})
}

func xlsxBytes(t *testing.T) []byte {
	return zipBytes(t, map[string]string{
		"xl/workbook.xml": `<?xml version="1.0"?><workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">` +
			`<sheets><sheet name="Orders" sheetId="1" r:id="rId1"/><sheet name="Empty" sheetId="2" r:id="rId2"/></sheets></workbook>`,
		"xl/_rels/workbook.xml.rels": `<?xml version="1.0"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
			`<Relationship Id="rId1" Type="worksheet" Target="worksheets/sheet1.xml"/><Relationship Id="rId2" Type="worksheet" Target="/xl/worksheets/sheet2.xml"/></Relationships>`,
		"xl/sharedStrings.xml": `<?xml version="1.0"?><sst xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><si><t>Client</t></si><si><r><t>Total</t></r><r><t> USD</t></r></si><si><t>Acme</t><rPh><t>あ</t></rPh></si></sst>`,
		"xl/worksheets/sheet1.xml": `<?xml version="1.0"?><worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData>` +
			`<row r="1"><c r="A1" t="s"><v>0</v></c><c r="C1" t="s"><v>1</v></c></row>` +
			`<row r="2"><c r="A2" t="s"><v>2</v></c><c r="B2" t="inlineStr"><is><t>new</t></is></c><c r="C2"><v>12.5</v></c></row></sheetData></worksheet>`,
		"xl/worksheets/sheet2.xml": `<?xml version="1.0"?><worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData/></worksheet>`,
	})
}

func TestReadAttachmentContentByKind(t *testing.T) {
	pic, doc := pngBytes(t), pdfBytes("Invoice 42 paid")
	files := map[string]struct {
		contentType string
		body        []byte
	}{
		"png":      {"image/png", pic},
		"sniffed":  {"application/octet-stream", pic},
		"huge":     {"image/jpeg", append([]byte("\xff\xd8\xff"), make([]byte, MaxInlineBytes)...)},
		"pdf":      {"application/pdf", doc},
		"brokenpd": {"application/pdf", []byte("%PDF-1.4 not really")},
		"docx":     {docxType, docxBytes(t)},
		"barezip":  {"application/zip", xlsxBytes(t)},
		"csv":      {"text/csv", []byte("a,b\n1,2\n")},
		"zip":      {"application/zip", zipBytes(t, map[string]string{"a.txt": "x"})},
		"exe":      {"application/x-msdownload", []byte("MZ\x90\x00")},
	}
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f := files[strings.TrimPrefix(r.URL.Path, "/api/1.0/download/")]
		w.Header().Set("Content-Type", f.contentType)
		w.Write(f.body)
	}))
	defer api.Close()
	c, _ := New(Config{Origin: api.URL, Workspace: "ws", Profile: "console", Authorization: "Simulator run"})
	read := func(name string) AttachmentContent {
		t.Helper()
		got, err := c.ReadAttachment(context.Background(), AttachmentReadInput{FileName: name})
		if err != nil {
			t.Fatal(name, err)
		}
		return got
	}
	for _, name := range []string{"png", "sniffed"} {
		if got := read(name); got.Kind != "image" || !got.Inline || got.ContentType != "image/png" || !bytes.Equal(got.Data, pic) || got.Binary {
			t.Errorf("%s: %+v", name, got)
		}
	}
	if got := read("huge"); got.Kind != "image" || got.Inline || !got.TooLarge || got.Data != nil || !strings.Contains(got.AttachmentNote(), "5 MiB") {
		t.Errorf("huge: %+v", got)
	}
	if got := read("pdf"); got.Kind != "pdf" || !got.Inline || !bytes.Equal(got.Data, doc) || !strings.Contains(got.Text, "Invoice 42 paid") {
		t.Errorf("pdf: %+v %q", got.Kind, got.Text)
	}
	if got := read("brokenpd"); got.Kind != "pdf" || !got.Inline || got.Text != "" {
		t.Errorf("a PDF without a readable text layer still goes as a document: %+v", got)
	}
	if got := read("docx"); got.Kind != "docx" || got.Text != "Договір\t № 7\nСума: 100\nUSD" || got.Data != nil {
		t.Errorf("docx: %q", got.Text)
	}
	if got := read("barezip"); got.Kind != "xlsx" || got.Text != "## Orders\nClient\t\tTotal USD\nAcme\tnew\t12.5\n\n## Empty" {
		t.Errorf("xlsx: %q", got.Text)
	}
	if got := read("csv"); got.Kind != "text" || got.Text != "a,b\n1,2\n" {
		t.Errorf("csv: %+v", got)
	}
	for _, name := range []string{"zip", "exe"} {
		if got := read(name); got.Kind != "binary" || !got.Binary || got.Text != "" || got.Data != nil {
			t.Errorf("%s: %+v", name, got)
		}
	}
}

func TestOfficeTextIsBounded(t *testing.T) {
	var row strings.Builder
	for i := 0; i < 20000; i++ {
		fmt.Fprintf(&row, `<w:p><w:r><w:t>line %d</w:t></w:r></w:p>`, i)
	}
	body := zipBytes(t, map[string]string{"word/document.xml": `<w:document xmlns:w="w"><w:body>` + row.String() + `</w:body></w:document>`})
	text, err := officeText("docx", body)
	if err != nil {
		t.Fatal(err)
	}
	cut, truncated := cutText(text)
	if !truncated || len(cut) != maxReadText || !strings.HasPrefix(cut, "line 0\nline 1\n") {
		t.Fatal(len(cut), truncated)
	}
	if columnIndex("C7") != 2 || columnIndex("AA1") != 26 || columnIndex("7") != -1 {
		t.Fatal("column index")
	}
}
