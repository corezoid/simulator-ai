package publicapi

import (
	"bytes"
	"image/jpeg"
	"regexp"
	"strconv"
)

// A scanned PDF has no text, but its pages are usually JPEG images: image XObjects whose stream is
// /DCTDecode, i.e. a JPEG file as it is. They are taken from the raw file (the pure-Go parser does
// not decode DCT streams), without rendering or cgo. A scan stored otherwise (CCITT, JBIG2, raw
// pixels) yields none, and the model is told its content is not available.

// imageObject finds an object dictionary up to the start of its stream; the dictionary may nest
// one level of << >> (DecodeParms).
var imageObject = regexp.MustCompile(`(?s)\d+\s+\d+\s+obj\s*<<((?:[^<>]|<<(?:[^<>]|<[^<>]*>)*>>|<[^<>]*>)*)>>\s*stream\r?\n`)

var streamLength = regexp.MustCompile(`/Length\s+(\d+)(\s+\d+\s+R)?`)

// scanPages returns up to max JPEG images of the PDF in file order, each a valid JPEG of at most
// MaxInlineBytes.
func scanPages(body []byte, max int) [][]byte {
	var pages [][]byte
	for _, m := range imageObject.FindAllSubmatchIndex(body, -1) {
		if len(pages) == max {
			break
		}
		dict := body[m[2]:m[3]]
		if !bytes.Contains(dict, []byte("/Image")) || !bytes.Contains(dict, []byte("/DCTDecode")) {
			continue
		}
		data := streamData(body[m[1]:], dict)
		if len(data) == 0 || len(data) > MaxInlineBytes {
			continue
		}
		if _, err := jpeg.DecodeConfig(bytes.NewReader(data)); err != nil {
			continue
		}
		pages = append(pages, data)
	}
	return pages
}

// streamData is the stream's bytes: its direct /Length, else up to endstream.
func streamData(rest, dict []byte) []byte {
	if l := streamLength.FindSubmatch(dict); l != nil && len(l[2]) == 0 {
		if n, err := strconv.Atoi(string(l[1])); err == nil && n > 0 && n <= len(rest) {
			return rest[:n]
		}
	}
	end := bytes.Index(rest, []byte("endstream"))
	if end < 0 {
		return nil
	}
	return bytes.TrimRight(rest[:end], "\r\n")
}
