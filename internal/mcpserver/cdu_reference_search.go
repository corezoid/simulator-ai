package mcpserver

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

const (
	grepMaxChars     = 6000 // one grep answer, well under what Hermes spills
	readMaxChars     = 8000 // one read answer
	grepLineRunes    = 240  // a line longer than this is cut in a grep answer
	readLineRunes    = 2000 // and in a read answer
	defaultGrepLimit = 40   // matching lines shown
	defaultReadLimit = 120
)

// docWindow is the part of a document a call works on: all of it, or one section (doc/<section>).
type docWindow struct {
	doc     referenceDoc
	section string
	lo, hi  int // line indexes [lo, hi)
}

func (w docWindow) name() string {
	if w.section == "" {
		return w.doc.Name
	}
	return w.doc.Name + "/" + w.section
}

// headingLevel is the number of #s of a "## " or "### " heading line, else 0.
func headingLevel(line string) int {
	n := len(line) - len(strings.TrimLeft(line, "#"))
	if n == 0 || n > 3 || !strings.HasPrefix(line[n:], " ") {
		return 0
	}
	return n
}

// sectionID is the first word of a heading: the Type of a components section, an action type of
// protocol, an example's id.
func sectionID(line string) string {
	fields := strings.Fields(strings.TrimLeft(line, "#"))
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

// section is one ## or ### heading of a document: name is its first word, path its unique address
// (a ### heading under its ## one: actions/notify, responses/notify).
type section struct {
	name, path string
	lo, hi     int
}

// sections lists a document's ## and ### headings with the lines each one covers.
func sections(d referenceDoc) []section {
	var out []section
	parent := ""
	for i, line := range d.Lines {
		level := headingLevel(line)
		if level < 2 {
			continue
		}
		s := section{name: sectionID(line), path: sectionID(line), lo: i}
		if level == 2 {
			parent = s.name
		} else if parent != "" {
			s.path = parent + "/" + s.name
		}
		s.hi = i + 1
		for s.hi < len(d.Lines) && (headingLevel(d.Lines[s.hi]) == 0 || headingLevel(d.Lines[s.hi]) > level) {
			s.hi++
		}
		out = append(out, s)
	}
	return out
}

// window resolves "doc", "doc/<section>" or "doc/<section>/<subsection>" (any case). A bare name
// shared by two subsections (protocol/notify: a client and a response action) is refused with
// their full addresses.
func window(docs []referenceDoc, name string) (docWindow, error) {
	docName, want, _ := strings.Cut(name, "/")
	for _, d := range docs {
		if d.Name != docName {
			continue
		}
		if want == "" {
			return docWindow{d, "", 0, len(d.Lines)}, nil
		}
		all := sections(d)
		var named []section
		for _, s := range all {
			if strings.EqualFold(s.path, want) {
				return docWindow{d, s.path, s.lo, s.hi}, nil
			}
			if strings.EqualFold(s.name, want) {
				named = append(named, s)
			}
		}
		switch len(named) {
		case 0:
			return docWindow{}, fmt.Errorf("%s has no section %s: grep \"^## \" with doc %q lists its sections", docName, want, docName)
		case 1:
			return docWindow{d, named[0].path, named[0].lo, named[0].hi}, nil
		}
		paths := make([]string, len(named))
		for i, s := range named {
			paths[i] = docName + "/" + s.path
		}
		return docWindow{}, fmt.Errorf("%s/%s names %d sections: use %s", docName, want, len(named), strings.Join(paths, " or "))
	}
	return docWindow{}, fmt.Errorf("no document %q: the documents are components, protocol and examples (doc/<section> reads one section)", docName)
}

// grepReference answers the lines of the windows matching pattern (case-insensitive RE2) with
// context lines around them, grouped per document and capped at grepMaxChars: a hit is
// doc:line: text, a context line doc-line- text and the heading of the section a block sits in
// doc=line= heading. unavailable are notes on documents that could not be built.
func grepReference(windows []docWindow, unavailable []string, pattern string, context, limit int) (string, error) {
	re, err := regexp.Compile("(?i)" + pattern)
	if err != nil {
		return "", fmt.Errorf("invalid pattern %q: %w (RE2 syntax: no lookaround or backreferences; escape . ( ) [ ] { } * + ? | ^ $ with \\ to match them)", pattern, err)
	}
	if limit <= 0 {
		limit = defaultGrepLimit
	}
	type docHits struct {
		w    docWindow
		hits []int
	}
	var found []docHits
	total := 0
	for _, w := range windows {
		var hits []int
		for i := w.lo; i < w.hi; i++ {
			if re.MatchString(w.doc.Lines[i]) {
				hits = append(hits, i)
			}
		}
		if len(hits) > 0 {
			found = append(found, docHits{w, hits})
			total += len(hits)
		}
	}
	names := make([]string, len(windows))
	for i, w := range windows {
		names[i] = w.name()
	}
	b := &strings.Builder{}
	for _, u := range unavailable {
		b.WriteString(u + "\n")
	}
	if total == 0 {
		fmt.Fprintf(b, "no hits for /%s/ in %s. Try other words for the idea (gap|spacing|margin), a shorter stem, or \"^## \" for a document's sections.", pattern, strings.Join(names, ", "))
		return b.String(), nil
	}
	fmt.Fprintf(b, "%d hits for /%s/ in %s\n", total, pattern, strings.Join(names, ", "))
	shown, budget := 0, grepMaxChars-b.Len()
	emit := func(s string) bool {
		if len(s)+1 > budget {
			return false
		}
		b.WriteString(s + "\n")
		budget -= len(s) + 1
		return true
	}
	full := false
	for _, f := range found {
		if full || !emit(fmt.Sprintf("--- %s (%d hits)", f.w.name(), len(f.hits))) {
			break
		}
		lastEnd, lastHeading := -1, -1
		for k := 0; k < len(f.hits) && !full; {
			start, end := max(f.w.lo, f.hits[k]-context), min(f.w.hi-1, f.hits[k]+context)
			next := k + 1
			for next < len(f.hits) && f.hits[next]-context <= end+1 {
				end = min(f.w.hi-1, f.hits[next]+context)
				next++
			}
			if lastEnd >= 0 && context > 0 && !emit("--") {
				full = true
				break
			}
			// the heading of the section the first hit sits in, unless the block or the last one shown has it
			if h := sectionOf(f.w.doc.Lines, f.hits[k]); h >= 0 && h != lastHeading && h < start {
				full = !emit(fmt.Sprintf("%s=%d= %s", f.w.doc.Name, h+1, clip(f.w.doc.Lines[h], grepLineRunes)))
				lastHeading = h
			}
			for i := start; i <= end && !full; i++ {
				sep := "-"
				if headingLevel(f.w.doc.Lines[i]) > 0 {
					lastHeading = i
				}
				if re.MatchString(f.w.doc.Lines[i]) {
					if shown == limit {
						full = true
						break
					}
					sep = ":"
					shown++
				}
				full = !emit(fmt.Sprintf("%s%s%d%s %s", f.w.doc.Name, sep, i+1, sep, clip(f.w.doc.Lines[i], grepLineRunes)))
				if full && sep == ":" {
					shown--
				}
			}
			lastEnd, k = end, next
		}
	}
	if shown < total {
		fmt.Fprintf(b, "… %d of %d hits not shown: narrow the pattern, pass doc (components, protocol, examples or doc/<section>), context 0 or a higher limit (≤ 200), or read the lines with smart_form_reference_read.", total-shown, total)
	}
	return strings.TrimRight(b.String(), "\n"), nil
}

// sectionOf is the index of the nearest heading at or before line i, or -1.
func sectionOf(lines []string, i int) int {
	for ; i >= 0; i-- {
		if headingLevel(lines[i]) > 0 {
			return i
		}
	}
	return -1
}

// readReference answers a window's lines from offset (a line number of the document; default the
// window's first line), at most limit lines and readMaxChars, with the next offset when more follow.
func readReference(w docWindow, offset, limit int) (string, error) {
	if offset == 0 {
		offset = w.lo + 1
	}
	if limit <= 0 {
		limit = defaultReadLimit
	}
	if offset < w.lo+1 || offset > w.hi {
		return "", fmt.Errorf("offset %d is outside %s (lines %d–%d)", offset, w.name(), w.lo+1, w.hi)
	}
	b := &strings.Builder{}
	last := min(w.hi, offset-1+limit)
	head := fmt.Sprintf("%s: lines %d–%%d of %d", w.doc.Name, offset, len(w.doc.Lines))
	if w.section != "" {
		head += fmt.Sprintf(" (section %s: lines %d–%d)", w.name(), w.lo+1, w.hi)
	}
	var body strings.Builder
	end := offset - 1
	for i := offset - 1; i < last; i++ {
		line := fmt.Sprintf("%d: %s\n", i+1, clip(w.doc.Lines[i], readLineRunes))
		if body.Len()+len(line) > readMaxChars-len(head)-120 && i > offset-1 {
			break
		}
		body.WriteString(line)
		end = i + 1
	}
	fmt.Fprintf(b, head+"\n", end)
	b.WriteString(body.String())
	if end < w.hi {
		fmt.Fprintf(b, "more: {doc: %q, offset: %d}", w.name(), end+1)
	}
	return strings.TrimRight(b.String(), "\n"), nil
}

// listReference answers the documents with their sizes and sections.
func listReference(docs []referenceDoc, unavailable []string) string {
	b := &strings.Builder{}
	b.WriteString("CDU v2 reference documents (read: {doc, offset?, limit?}; one section: {doc: \"<doc>/<section>\"}; search: smart_form_reference_grep):\n")
	for _, d := range docs {
		size := 0
		var sections []string
		for _, line := range d.Lines {
			size += len(line) + 1
			if headingLevel(line) == 2 {
				sections = append(sections, sectionID(line))
			}
		}
		fmt.Fprintf(b, "- %s — %d lines, %d chars. %s\n", d.Name, len(d.Lines), size, d.Summary)
		if d.Name == "components" {
			fmt.Fprintf(b, "  sections: %d types (grep \"^## \" lists them with kind and category)\n", len(sections))
		} else {
			fmt.Fprintf(b, "  sections: %s\n", strings.Join(sections, ", "))
		}
	}
	for _, u := range unavailable {
		fmt.Fprintf(b, "- %s\n", u)
	}
	return strings.TrimRight(b.String(), "\n")
}

func clip(s string, runes int) string {
	if utf8.RuneCountInString(s) <= runes {
		return s
	}
	return string([]rune(s)[:runes-1]) + "…"
}
