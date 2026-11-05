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
)

// The text of a .docx (its paragraphs) or a .xlsx (each sheet's rows, cells tab-separated), read
// from the zip and XML parts with the standard library. Every part read is bounded, so a zip bomb
// stops at maxPartBytes, and text stops a little past what cutText keeps.

const maxPartBytes = 16 << 20

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

func officeText(kind string, body []byte) (string, error) {
	z, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		return "", err
	}
	parts := map[string]*zip.File{}
	for _, f := range z.File {
		parts[f.Name] = f
	}
	if kind == "docx" {
		return docxText(parts)
	}
	return xlsxText(parts)
}

func openPart(parts map[string]*zip.File, name string) (*xml.Decoder, func(), error) {
	f, ok := parts[name]
	if !ok {
		return nil, nil, fmt.Errorf("%s missing", name)
	}
	rc, err := f.Open()
	if err != nil {
		return nil, nil, err
	}
	d := xml.NewDecoder(io.LimitReader(rc, maxPartBytes))
	d.Strict = false
	return d, func() { rc.Close() }, nil
}

// docxText walks word/document.xml: w:t is text, w:tab a tab, w:br/w:cr a line break, the end of a
// w:p a new line.
func docxText(parts map[string]*zip.File) (string, error) {
	d, done, err := openPart(parts, "word/document.xml")
	if err != nil {
		return "", err
	}
	defer done()
	var b strings.Builder
	inText := false
	for b.Len() <= maxReadText {
		tok, err := d.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
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
func xlsxText(parts map[string]*zip.File) (string, error) {
	shared, err := sharedStrings(parts)
	if err != nil {
		return "", err
	}
	sheets, err := workbookSheets(parts)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for _, s := range sheets {
		if b.Len() > maxReadText {
			break
		}
		fmt.Fprintf(&b, "## %s\n", s.name)
		if err := sheetText(parts, s.part, shared, &b); err != nil {
			return "", err
		}
		b.WriteByte('\n')
	}
	return strings.TrimSpace(b.String()), nil
}

func sharedStrings(parts map[string]*zip.File) ([]string, error) {
	if _, ok := parts["xl/sharedStrings.xml"]; !ok {
		return nil, nil
	}
	d, done, err := openPart(parts, "xl/sharedStrings.xml")
	if err != nil {
		return nil, err
	}
	defer done()
	var out []string
	var cur strings.Builder
	inText := false
	for {
		tok, err := d.Token()
		if err == io.EOF {
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
func workbookSheets(parts map[string]*zip.File) ([]sheetRef, error) {
	var rels struct {
		List []struct {
			ID     string `xml:"Id,attr"`
			Target string `xml:"Target,attr"`
		} `xml:"Relationship"`
	}
	d, done, err := openPart(parts, "xl/_rels/workbook.xml.rels")
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
	d, done, err = openPart(parts, "xl/workbook.xml")
	if err != nil {
		return nil, err
	}
	defer done()
	var out []sheetRef
	for {
		tok, err := d.Token()
		if err == io.EOF {
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
func sheetText(parts map[string]*zip.File, part string, shared []string, b *strings.Builder) error {
	d, done, err := openPart(parts, part)
	if err != nil {
		return err
	}
	defer done()
	var row []string
	var cellType, value string
	col, inValue := 0, false
	for b.Len() <= maxReadText {
		tok, err := d.Token()
		if err == io.EOF {
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
				for len(row) < col && len(row) < 16384 {
					row = append(row, "")
				}
				if col < 16384 {
					row = append(row, value)
				}
			case "row":
				b.WriteString(strings.TrimRight(strings.Join(row, "\t"), "\t"))
				b.WriteByte('\n')
			}
		case xml.CharData:
			if inValue {
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
