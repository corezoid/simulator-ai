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

// pdfWith is a one-page PDF whose page content is object 4: dict, then data as its stream; extra
// objects follow from number 5. Offsets and the xref are counted.
func pdfWith(dict string, data []byte, extra ...string) []byte {
	objects := [][]byte{
		[]byte("<< /Type /Catalog /Pages 2 0 R >>"),
		[]byte("<< /Type /Pages /Kids [3 0 R] /Count 1 >>"),
		[]byte("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 4 0 R >>"),
		append(append([]byte(fmt.Sprintf("<< %s /Length %d >>\nstream\n", dict, len(data))), data...), "\nendstream"...),
	}
	for _, e := range extra {
		objects = append(objects, []byte(e))
	}
	var b bytes.Buffer
	b.WriteString("%PDF-1.4\n")
	offsets := make([]int, len(objects))
	for i, o := range objects {
		offsets[i] = b.Len()
		fmt.Fprintf(&b, "%d 0 obj\n", i+1)
		b.Write(o)
		b.WriteString("\nendobj\n")
	}
	xref := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", len(objects)+1)
	for _, off := range offsets {
		fmt.Fprintf(&b, "%010d 00000 n \n", off)
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objects)+1, xref)
	return b.Bytes()
}

// Every way to hide a filter bomb from a look at the dictionary stops at the read budget, since the
// budget sits in the stream reader itself; a predictor sized past reason is refused unallocated.
func TestPDFBombsStopAtTheLimit(t *testing.T) {
	bomb := deflated(t, 96<<20)
	var twice bytes.Buffer
	z := zlib.NewWriter(&twice)
	z.Write(bomb)
	z.Close()
	cases := map[string][]byte{
		"chain of two FlateDecode":  pdfWith("/Filter [/FlateDecode /FlateDecode]", twice.Bytes()),
		"name in hex":               pdfWith("/Filter /Fl#61teDecode", bomb),
		"indirect Filter":           pdfWith("/Filter 5 0 R", bomb, "/FlateDecode"),
		"dictionary past 4 KiB":     pdfWith("/Pad ("+strings.Repeat("x", 8192)+") /Filter /FlateDecode", bomb),
		"predictor of 2^40 columns": pdfWith("/Filter /FlateDecode /DecodeParms << /Predictor 12 /Columns 1099511627776 >>", deflated(t, 64)),
	}
	for name, doc := range cases {
		t.Run(name, func(t *testing.T) {
			var text, unread string
			bounded(t, 3*time.Second, 64<<20, func() { text, unread = pdfText(doc) })
			// Each bomb is read into until the budget stops it; the predictor is refused before.
			if text != "" || strings.HasPrefix(name, "predictor") != !strings.Contains(unread, "64 MiB") {
				t.Fatalf("text %.40q, unread %q", text, unread)
			}
		})
	}
	if text, unread := pdfText(pdfBytes("fine")); text != "fine" || unread != "" {
		t.Fatalf("a plain PDF: %q %q", text, unread)
	}
}

func TestPDFTextRefusesALargeFile(t *testing.T) {
	if _, unread := pdfText([]byte("%PDF-1.4 " + strings.Repeat(" ", maxPDFTextBytes))); !strings.Contains(unread, "larger than 10 MiB") {
		t.Fatal(unread)
	}
}
