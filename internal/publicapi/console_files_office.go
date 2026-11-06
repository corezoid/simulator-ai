package publicapi

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"path"
	"strconv"
	"strings"
	"time"
)

// The text of a .docx (its paragraphs) or a .xlsx (each sheet's rows, cells tab-separated), read
// from the zip and XML parts with the standard library. One budget bounds the whole file: the
// bytes unpacked from all its parts (a zip bomb stops at maxOfficeInflate), the time (officeTimeout),
// the sheets read and the columns of a row; text stops a little past what cutText keeps. Running out
// of the budget keeps the text read so far, marked cut.

const (
	maxOfficeInflate = 32 << 20
	maxSheets        = 32
	maxColumns       = 512
)

// officeTimeout bounds one file's text; a variable so a test can shorten it.
var officeTimeout = 5 * time.Second

var errBudget = errors.New("office text budget spent")

// officeBudget is what reading one file may still spend.
type officeBudget struct {
	left     int64
	deadline time.Time
	tokens   int
}

// spent says the time is up, looked at every 1024 XML tokens.
func (b *officeBudget) spent() bool {
	b.tokens++
	return b.tokens%1024 == 0 && time.Now().After(b.deadline)
}

// Read counts the unpacked bytes of every part against the one budget.
type budgetReader struct {
	r io.Reader
	b *officeBudget
}

func (r budgetReader) Read(p []byte) (int, error) {
	if r.b.left <= 0 {
		return 0, errBudget
	}
	if int64(len(p)) > r.b.left {
		p = p[:r.b.left]
	}
	n, err := r.r.Read(p)
	r.b.left -= int64(n)
	return n, err
}

// officeKind tells a .docx or .xlsx sent as a bare zip by the part it must have.
func officeKind(body []byte) string {
	z, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		return "binary"
	}
	for _, f := range z.File {
		switch f.Name {
		case "word/document.xml":
			return "docx"
		case "xl/workbook.xml":
			return "xlsx"
		}
	}
	return "binary"
}

// officeText is the file's text and whether the budget cut it short; an error means the file is not
// the Office document it claimed to be.
func officeText(kind string, body []byte) (string, bool, error) {
	z, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		return "", false, err
	}
	parts := map[string]*zip.File{}
	for _, f := range z.File {
		parts[f.Name] = f
	}
	budget := &officeBudget{left: maxOfficeInflate, deadline: time.Now().Add(officeTimeout)}
	var text string
	if kind == "docx" {
		text, err = docxText(parts, budget)
	} else {
		text, err = xlsxText(parts, budget)
	}
	if errors.Is(err, errBudget) {
		return text, true, nil
	}
	return text, false, err
}

func openPart(parts map[string]*zip.File, name string, budget *officeBudget) (*xml.Decoder, func(), error) {
	f, ok := parts[name]
	if !ok {
		return nil, nil, fmt.Errorf("%s missing", name)
	}
	rc, err := f.Open()
	if err != nil {
		return nil, nil, err
	}
	d := xml.NewDecoder(budgetReader{rc, budget})
	d.Strict = false
	return d, func() { _ = rc.Close() }, nil
}

// token is the next XML token, or errBudget once the time is up; a budget error of the reader
// comes through the decoder wrapped, and is told apart here.
func token(d *xml.Decoder, budget *officeBudget) (xml.Token, error) {
	if budget.spent() {
		return nil, errBudget
	}
	tok, err := d.Token()
	if err != nil && budget.left <= 0 {
		return nil, errBudget
	}
	return tok, err
}

// docxText walks word/document.xml: w:t is text, w:tab a tab, w:br/w:cr a line break, the end of a
// w:p a new line.
func docxText(parts map[string]*zip.File, budget *officeBudget) (string, error) {
	d, done, err := openPart(parts, "word/document.xml", budget)
	if err != nil {
		return "", err
	}
	defer done()
	var b strings.Builder
	inText := false
	for b.Len() <= maxReadText {
		tok, err := token(d, budget)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return strings.TrimSpace(b.String()), err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "t":
				inText = true
			case "tab":
				b.WriteByte('\t')
			case "br", "cr":
				b.WriteByte('\n')
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "t":
				inText = false
			case "p":
				b.WriteByte('\n')
			}
		case xml.CharData:
			if inText {
				b.Write(t)
			}
		}
	}
	return strings.TrimSpace(b.String()), nil
}

// xlsxText reads the shared strings, then each sheet of the workbook in order, named, a row per
// line with its cells in their columns.
func xlsxText(parts map[string]*zip.File, budget *officeBudget) (string, error) {
	shared, err := sharedStrings(parts, budget)
	if err != nil {
		return "", err
	}
	sheets, err := workbookSheets(parts, budget)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for i, s := range sheets {
		if b.Len() > maxReadText || i == maxSheets {
			break
		}
		fmt.Fprintf(&b, "## %s\n", s.name)
		if err := sheetText(parts, s.part, shared, &b, budget); err != nil {
			return strings.TrimSpace(b.String()), err
		}
		b.WriteByte('\n')
	}
	return strings.TrimSpace(b.String()), nil
}

func sharedStrings(parts map[string]*zip.File, budget *officeBudget) ([]string, error) {
	if _, ok := parts["xl/sharedStrings.xml"]; !ok {
		return nil, nil
	}
	d, done, err := openPart(parts, "xl/sharedStrings.xml", budget)
	if err != nil {
		return nil, err
	}
	defer done()
	var out []string
	var cur strings.Builder
	inText := false
	for {
		tok, err := token(d, budget)
		if errors.Is(err, io.EOF) {
			return out, nil
		}
		if err != nil {
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "si":
				cur.Reset()
			case "t":
				inText = true
			case "rPh": // phonetic hints are not the cell's text
				if err := d.Skip(); err != nil {
					return nil, err
				}
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "t":
				inText = false
			case "si":
				out = append(out, cur.String())
			}
		case xml.CharData:
			if inText {
				cur.Write(t)
			}
		}
	}
}

type sheetRef struct{ name, part string }

// workbookSheets lists the sheets in workbook order with their parts, through the workbook rels.
func workbookSheets(parts map[string]*zip.File, budget *officeBudget) ([]sheetRef, error) {
	var rels struct {
		List []struct {
			ID     string `xml:"Id,attr"`
			Target string `xml:"Target,attr"`
		} `xml:"Relationship"`
	}
	d, done, err := openPart(parts, "xl/_rels/workbook.xml.rels", budget)
	if err != nil {
		return nil, err
	}
	err = d.Decode(&rels)
	done()
	if err != nil {
		return nil, err
	}
	targets := map[string]string{}
	for _, r := range rels.List {
		target := strings.TrimPrefix(r.Target, "/")
		if !strings.HasPrefix(target, "xl/") {
			target = path.Join("xl", target)
		}
		targets[r.ID] = target
	}
	// Each sheet's name and r:id, read token by token: the prefix of r:id varies.
	d, done, err = openPart(parts, "xl/workbook.xml", budget)
	if err != nil {
		return nil, err
	}
	defer done()
	var out []sheetRef
	for len(out) < maxSheets {
		tok, err := token(d, budget)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		if s, ok := tok.(xml.StartElement); ok && s.Name.Local == "sheet" {
			var name, id string
			for _, a := range s.Attr {
				switch {
				case a.Name.Local == "name":
					name = a.Value
				case a.Name.Local == "id" && a.Name.Space != "":
					id = a.Value
				}
			}
			if part, ok := targets[id]; ok {
				out = append(out, sheetRef{name: name, part: part})
			}
		}
	}
	if len(out) == 0 {
		return nil, errors.New("workbook has no sheets")
	}
	return out, nil
}

// sheetText writes a sheet's rows: each cell at its column (from its A1 reference), tab-separated.
func sheetText(parts map[string]*zip.File, part string, shared []string, b *strings.Builder, budget *officeBudget) error {
	d, done, err := openPart(parts, part, budget)
	if err != nil {
		return err
	}
	defer done()
	var row []string
	var cellType, value string
	col, inValue := 0, false
	for b.Len() <= maxReadText {
		tok, err := token(d, budget)
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "row":
				row = row[:0]
			case "c":
				cellType, value, col = "", "", len(row)
				for _, a := range t.Attr {
					switch a.Name.Local {
					case "t":
						cellType = a.Value
					case "r":
						if c := columnIndex(a.Value); c >= 0 {
							col = c
						}
					}
				}
			case "v", "t":
				inValue = true
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "v", "t":
				inValue = false
			case "c":
				if cellType == "s" {
					if i, err := strconv.Atoi(value); err == nil && i >= 0 && i < len(shared) {
						value = shared[i]
					}
				}
				// Cells past maxColumns are dropped, so a far reference (XFD) pads no more than that.
				if col < maxColumns {
					for len(row) < col {
						row = append(row, "")
					}
					row = append(row, value)
				}
			case "row":
				b.WriteString(strings.TrimRight(strings.Join(row, "\t"), "\t"))
				b.WriteByte('\n')
			}
		case xml.CharData:
			if inValue && len(value) < maxReadText { // one cell never grows past what is kept
				value += string(t)
			}
		}
	}
	return nil
}

// columnIndex is the 0-based column of an A1 reference ("C7" is 2), -1 when it has none.
func columnIndex(ref string) int {
	col := 0
	n := 0
	for _, r := range ref {
		if r < 'A' || r > 'Z' {
			break
		}
		col, n = col*26+int(r-'A'+1), n+1
	}
	if n == 0 {
		return -1
	}
	return col - 1
}
