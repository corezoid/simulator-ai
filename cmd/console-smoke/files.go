package main

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"slices"
	"strings"
	"time"
)

type fileRead struct {
	Kind     string `json:"kind"`
	Text     string `json:"text"`
	Inline   bool   `json:"inline"`
	TooLarge bool   `json:"tooLarge"`
	Scanned  bool   `json:"scanned"`
}

// coreFiles uploads a PNG, a PDF with a text layer and a scanned PDF (a JPEG page) onto an actor
// and reads each back: the image as an image block, the PDF as its text, the scan as its page.
func coreFiles(c *client, actor, stamp string) error {
	var pic, page bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	img.Set(1, 1, color.RGBA{R: 255, A: 255})
	if err := png.Encode(&pic, img); err != nil {
		return err
	}
	if err := jpeg.Encode(&page, img, nil); err != nil {
		return err
	}
	phrase := "Smoke invoice " + stamp
	for _, f := range []struct {
		name  string
		body  []byte
		kind  string
		block string // the content block after the structured answer, none for text only
	}{
		{"smoke-" + stamp + ".png", pic.Bytes(), "image", "image"},
		{"smoke-" + stamp + ".pdf", smokePDF(phrase), "pdf", ""},
		{"smoke-scan-" + stamp + ".pdf", smokeScan(page.Bytes()), "pdf", "image"},
	} {
		var up struct {
			FileName string `json:"fileName"`
		}
		if err := c.tool("attachment_upload", map[string]any{"fileName": f.name, "content": base64.StdEncoding.EncodeToString(f.body), "actorId": actor}, &up); err != nil {
			return err
		}
		read, err := readScanned(c, up.FileName)
		if err != nil {
			return err
		}
		inline := f.block != ""
		if read.Kind != f.kind || read.Inline != inline || inline && !slices.Contains(c.blocks, f.block) || slices.Contains(c.blocks, "resource") {
			return fmt.Errorf("attachment_read %s: kind %s inline %v blocks %v, want %s with block %q", f.name, read.Kind, read.Inline, c.blocks, f.kind, f.block)
		}
		if f.kind == "pdf" && !inline && !strings.Contains(read.Text, phrase) || inline && f.kind == "pdf" && !read.Scanned {
			return fmt.Errorf("attachment_read %s: text %q scanned %v", f.name, read.Text, read.Scanned)
		}
	}
	return nil
}

// readScanned reads a just-uploaded file, waiting out its antivirus scan for up to half a minute.
func readScanned(c *client, fileName string) (fileRead, error) {
	var read fileRead
	for try := 0; ; try++ {
		err := c.tool("attachment_read", map[string]any{"fileName": fileName}, &read)
		if err == nil || try == 5 || !strings.Contains(err.Error(), "scan has not finished") {
			return read, err
		}
		time.Sleep(5 * time.Second)
	}
}

// smokeScan is a one-page PDF whose page is a JPEG image and nothing else.
func smokeScan(page []byte) []byte {
	var b bytes.Buffer
	b.WriteString("%PDF-1.4\n1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n")
	b.WriteString("3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 8 8] /Resources << /XObject << /Im1 4 0 R >> >> >>\nendobj\n")
	fmt.Fprintf(&b, "4 0 obj\n<< /Type /XObject /Subtype /Image /Width 8 /Height 8 /ColorSpace /DeviceRGB /BitsPerComponent 8 /Filter /DCTDecode /Length %d >>\nstream\n", len(page))
	b.Write(page)
	b.WriteString("\nendstream\nendobj\ntrailer\n<< /Root 1 0 R >>\n%%EOF\n")
	return b.Bytes()
}

// smokePDF is a one-page PDF whose text layer says text.
func smokePDF(text string) []byte {
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
