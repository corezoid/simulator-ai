package publicapi

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"runtime"
	"strings"
	"testing"
	"time"
)

// bounded runs f and fails when it takes longer than d or allocates more than limit bytes.
func bounded(t *testing.T, d time.Duration, limit uint64, f func()) {
	t.Helper()
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	start := time.Now()
	f()
	took := time.Since(start)
	runtime.ReadMemStats(&after)
	if took > d {
		t.Errorf("took %v, more than %v", took, d)
	}
	if alloc := after.TotalAlloc - before.TotalAlloc; alloc > limit {
		t.Errorf("allocated %d MiB, more than %d MiB", alloc>>20, limit>>20)
	}
}

func deflated(t *testing.T, n int) []byte {
	var b bytes.Buffer
	z, _ := zlib.NewWriterLevel(&b, zlib.BestCompression)
	if _, err := z.Write(make([]byte, n)); err != nil {
		t.Fatal(err)
	}
	z.Close()
	return b.Bytes()
}

// A PDF whose content stream inflates to 96 MiB is not handed to the parser.
func TestPDFFlateBombIsRefused(t *testing.T) {
	bomb := deflated(t, 96<<20)
	var doc bytes.Buffer
	fmt.Fprintf(&doc, "%%PDF-1.4\n1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n")
	fmt.Fprintf(&doc, "3 0 obj\n<< /Type /Page /Parent 2 0 R /Contents 4 0 R >>\nendobj\n4 0 obj\n<< /Length %d /Filter /FlateDecode >>\nstream\n", len(bomb))
	doc.Write(bomb)
	doc.WriteString("\nendstream\nendobj\ntrailer\n<< /Root 1 0 R >>\n%%EOF\n")
	var text, unread string
	bounded(t, 3*time.Second, 32<<20, func() { text, unread = pdfText(doc.Bytes()) })
	if text != "" || !strings.Contains(unread, "more than 64 MiB") {
		t.Fatalf("%q %q", text, unread)
	}
	if !inflatesWithin(pdfBytes("fine"), maxPDFInflate) {
		t.Fatal("a plain PDF was taken for a bomb")
	}
}

func TestPDFTextRefusals(t *testing.T) {
	for body, want := range map[string]string{
		"%PDF-1.4 " + strings.Repeat(" ", maxPDFTextBytes): "larger than 10 MiB",
		"%PDF-1.4\ntrailer\n<< /Encrypt 5 0 R >>":          "encrypted",
	} {
		if _, unread := pdfText([]byte(body)); !strings.Contains(unread, want) {
			t.Errorf("%.20q: %q, want %q", body, unread, want)
		}
	}
}

// A sheet whose cells sit at column XFD (16384) pads no more than maxColumns.
func TestXLSXFarColumnsAreBounded(t *testing.T) {
	var rows strings.Builder
	for i := 1; i <= 20000; i++ {
		fmt.Fprintf(&rows, `<row r="%d"><c r="XFD%d" t="inlineStr"><is><t>x</t></is></c><c r="A%d"><v>%d</v></c></row>`, i, i, i, i)
	}
	body := zipBytes(t, map[string]string{
		"xl/workbook.xml":            `<workbook xmlns:r="r"><sheets><sheet name="S" r:id="rId1"/></sheets></workbook>`,
		"xl/_rels/workbook.xml.rels": `<Relationships><Relationship Id="rId1" Target="worksheets/sheet1.xml"/></Relationships>`,
		"xl/worksheets/sheet1.xml":   `<worksheet><sheetData>` + rows.String() + `</sheetData></worksheet>`,
	})
	var text string
	bounded(t, 3*time.Second, 256<<20, func() {
		var err error
		if text, _, err = officeText("xlsx", body); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.HasPrefix(text, "## S\n1\n2\n") || strings.Contains(text, "\t") {
		t.Fatalf("the XFD cells were not dropped: %.60q", text)
	}
}

// A sheet part that unpacks past the file's budget stops there, keeping what was read, cut.
func TestOfficeInflateBudget(t *testing.T) {
	body := zipBytes(t, map[string]string{
		"xl/workbook.xml":            `<workbook xmlns:r="r"><sheets><sheet name="S" r:id="rId1"/></sheets></workbook>`,
		"xl/_rels/workbook.xml.rels": `<Relationships><Relationship Id="rId1" Target="worksheets/sheet1.xml"/></Relationships>`,
		"xl/worksheets/sheet1.xml":   `<worksheet><sheetData><row><c><v>1</v></c></row>` + strings.Repeat(" ", 48<<20) + `</sheetData></worksheet>`,
	})
	var text string
	var cut bool
	bounded(t, 5*time.Second, 256<<20, func() {
		var err error
		if text, cut, err = officeText("xlsx", body); err != nil {
			t.Fatal(err)
		}
	})
	if !cut || text != "## S\n1" {
		t.Fatalf("%v %q", cut, text)
	}
}

// One run sends at most maxImagesPerRun images; a scan gets the pages left, the rest is noted.
func TestImagesPerRunAreCapped(t *testing.T) {
	c, _ := New(Config{Origin: "http://127.0.0.1:1", Workspace: "ws", Profile: "console", Authorization: "Bearer x"})
	for i := 0; i < 6; i++ {
		img := AttachmentContent{Kind: "image", Inline: true, Data: []byte{1}}
		if c.limitImages(&img); !img.Inline || img.ImagesSkipped != 0 {
			t.Fatal(i, img)
		}
	}
	scan := AttachmentContent{Kind: "pdf", Inline: true, Pages: [][]byte{{1}, {2}, {3}, {4}}}
	if c.limitImages(&scan); len(scan.Pages) != 2 || scan.ImagesSkipped != 2 || !strings.Contains(scan.AttachmentNote(), "2 image(s) of this file were not sent") {
		t.Fatal(scan)
	}
	img := AttachmentContent{Kind: "image", Inline: true, Data: []byte{1}}
	if c.limitImages(&img); img.Inline || img.Data != nil || img.ImagesSkipped != 1 {
		t.Fatal(img)
	}
}
