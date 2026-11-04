package simulation

import (
	"fmt"
	"strings"
	"sync"
	"unicode"
)

// The rule language is a subset of Python expression syntax (spec §8).

type ExprError struct{ msg string }

func (e *ExprError) Error() string { return e.msg }

func exprErr(format string, a ...any) error { return &ExprError{fmt.Sprintf(format, a...)} }

// ---- tokens ------------------------------------------------------------

type tokKind int

const (
	tEOF tokKind = iota
	tNum
	tStr
	tName
	tOp
)

type token struct {
	kind tokKind
	text string
}

func tokenize(src string) ([]token, error) {
	var out []token
	rs := []rune(src)
	for i := 0; i < len(rs); {
		c := rs[i]
		switch {
		case unicode.IsSpace(c):
			i++
		case unicode.IsDigit(c) || (c == '.' && i+1 < len(rs) && unicode.IsDigit(rs[i+1])):
			j := i
			for j < len(rs) && (unicode.IsDigit(rs[j]) || rs[j] == '.' || rs[j] == '_') {
				j++
			}
			if j < len(rs) && (rs[j] == 'e' || rs[j] == 'E') {
				k := j + 1
				if k < len(rs) && (rs[k] == '+' || rs[k] == '-') {
					k++
				}
				if k < len(rs) && unicode.IsDigit(rs[k]) {
					for k < len(rs) && unicode.IsDigit(rs[k]) {
						k++
					}
					j = k
				}
			}
			out = append(out, token{tNum, strings.ReplaceAll(string(rs[i:j]), "_", "")})
			i = j
		case c == '\'' || c == '"':
			var b strings.Builder
			j := i + 1
			for ; j < len(rs) && rs[j] != c; j++ {
				if rs[j] == '\\' && j+1 < len(rs) {
					j++
					switch rs[j] {
					case 'n':
						b.WriteRune('\n')
					case 't':
						b.WriteRune('\t')
					default:
						b.WriteRune(rs[j])
					}
					continue
				}
				b.WriteRune(rs[j])
			}
			if j >= len(rs) {
				return nil, exprErr("syntax error in %q: unterminated string", src)
			}
			out = append(out, token{tStr, b.String()})
			i = j + 1
		case unicode.IsLetter(c) || c == '_':
			j := i
			for j < len(rs) && (unicode.IsLetter(rs[j]) || unicode.IsDigit(rs[j]) || rs[j] == '_') {
				j++
			}
			out = append(out, token{tName, string(rs[i:j])})
			i = j
		default:
			two := ""
			if i+1 < len(rs) {
				two = string(rs[i : i+2])
			}
			switch two {
			case "==", "!=", "<=", ">=", "//", "**":
				out = append(out, token{tOp, two})
				i += 2
				continue
			}
			if strings.ContainsRune("+-*/%<>()[]{},:.=", c) {
				out = append(out, token{tOp, string(c)})
				i++
				continue
			}
			return nil, exprErr("syntax error in %q: unexpected %q", src, string(c))
		}
	}
	return append(out, token{tEOF, ""}), nil
}

// ---- AST ---------------------------------------------------------------

type node interface{}

type (
	nConst struct{ v any }
	nName  struct{ id string }
	nAttr  struct {
		obj  node
		attr string
	}
	nIndex struct{ obj, key node }
	nCall  struct {
		fn     node
		args   []node
		kwKeys []string
		kwVals []node
	}
	nBin struct {
		op   string
		l, r node
	}
	nUnary struct {
		op string
		x  node
	}
	nBool struct {
		op    string
		items []node
	}
	nCmp struct {
		first node
		ops   []string
		rest  []node
	}
	nIf   struct{ test, body, orelse node }
	nList struct{ items []node }
	nDict struct{ keys, vals []node }
)

type parser struct {
	toks []token
	pos  int
	src  string
}

func (p *parser) peek() token { return p.toks[p.pos] }
func (p *parser) next() token { t := p.toks[p.pos]; p.pos++; return t }

func (p *parser) isOp(s string) bool   { t := p.peek(); return t.kind == tOp && t.text == s }
func (p *parser) isName(s string) bool { t := p.peek(); return t.kind == tName && t.text == s }

func (p *parser) expect(s string) error {
	if !p.isOp(s) {
		return exprErr("syntax error in %q: expected %q", p.src, s)
	}
	p.pos++
	return nil
}

var keywords = map[string]bool{"and": true, "or": true, "not": true, "in": true, "is": true, "if": true,
	"else": true, "lambda": true, "for": true, "import": true, "True": true, "False": true, "None": true}

func (p *parser) expr() (node, error) {
	body, err := p.or()
	if err != nil {
		return nil, err
	}
	if p.isName("if") {
		p.pos++
		test, err := p.or()
		if err != nil {
			return nil, err
		}
		if !p.isName("else") {
			return nil, exprErr("syntax error in %q: expected else", p.src)
		}
		p.pos++
		orelse, err := p.expr()
		if err != nil {
			return nil, err
		}
		return nIf{test, body, orelse}, nil
	}
	return body, nil
}

func (p *parser) boolChain(op string, sub func() (node, error)) (node, error) {
	first, err := sub()
	if err != nil {
		return nil, err
	}
	items := []node{first}
	for p.isName(op) {
		p.pos++
		n, err := sub()
		if err != nil {
			return nil, err
		}
		items = append(items, n)
	}
	if len(items) == 1 {
		return first, nil
	}
	return nBool{op, items}, nil
}

func (p *parser) or() (node, error)  { return p.boolChain("or", p.and) }
func (p *parser) and() (node, error) { return p.boolChain("and", p.not) }

func (p *parser) not() (node, error) {
	if p.isName("not") {
		p.pos++
		x, err := p.not()
		if err != nil {
			return nil, err
		}
		return nUnary{"not", x}, nil
	}
	return p.comparison()
}

func (p *parser) comparison() (node, error) {
	first, err := p.arith()
	if err != nil {
		return nil, err
	}
	var ops []string
	var rest []node
	for {
		t := p.peek()
		op := ""
		switch {
		case t.kind == tOp && (t.text == "==" || t.text == "!=" || t.text == "<" || t.text == "<=" || t.text == ">" || t.text == ">="):
			op = t.text
			p.pos++
		case p.isName("in"):
			op = "in"
			p.pos++
		case p.isName("not") && p.toks[p.pos+1].kind == tName && p.toks[p.pos+1].text == "in":
			op = "not in"
			p.pos += 2
		case p.isName("is"):
			p.pos++
			op = "is"
			if p.isName("not") {
				p.pos++
				op = "is not"
			}
		}
		if op == "" {
			break
		}
		r, err := p.arith()
		if err != nil {
			return nil, err
		}
		ops = append(ops, op)
		rest = append(rest, r)
	}
	if len(ops) == 0 {
		return first, nil
	}
	return nCmp{first, ops, rest}, nil
}

func (p *parser) arith() (node, error) {
	l, err := p.term()
	if err != nil {
		return nil, err
	}
	for p.isOp("+") || p.isOp("-") {
		op := p.next().text
		r, err := p.term()
		if err != nil {
			return nil, err
		}
		l = nBin{op, l, r}
	}
	return l, nil
}

func (p *parser) term() (node, error) {
	l, err := p.factor()
	if err != nil {
		return nil, err
	}
	for p.isOp("*") || p.isOp("/") || p.isOp("//") || p.isOp("%") {
		op := p.next().text
		r, err := p.factor()
		if err != nil {
			return nil, err
		}
		l = nBin{op, l, r}
	}
	return l, nil
}

func (p *parser) factor() (node, error) {
	if p.isOp("-") || p.isOp("+") {
		op := p.next().text
		x, err := p.factor()
		if err != nil {
			return nil, err
		}
		return nUnary{op, x}, nil
	}
	if p.isOp("**") {
		return nil, exprErr("not allowed in %q: Pow", p.src)
	}
	return p.primary()
}

func (p *parser) primary() (node, error) {
	n, err := p.atom()
	if err != nil {
		return nil, err
	}
	for {
		switch {
		case p.isOp("."):
			p.pos++
			t := p.next()
			if t.kind != tName {
				return nil, exprErr("syntax error in %q: attribute name expected", p.src)
			}
			n = nAttr{n, t.text}
		case p.isOp("("):
			p.pos++
			call := nCall{fn: n}
			for !p.isOp(")") {
				if p.peek().kind == tName && p.toks[p.pos+1].kind == tOp && p.toks[p.pos+1].text == "=" {
					call.kwKeys = append(call.kwKeys, p.next().text)
					p.pos++
					v, err := p.expr()
					if err != nil {
						return nil, err
					}
					call.kwVals = append(call.kwVals, v)
				} else {
					a, err := p.expr()
					if err != nil {
						return nil, err
					}
					call.args = append(call.args, a)
				}
				if !p.isOp(",") {
					break
				}
				p.pos++
			}
			if err := p.expect(")"); err != nil {
				return nil, err
			}
			n = call
		case p.isOp("["):
			p.pos++
			k, err := p.expr()
			if err != nil {
				return nil, err
			}
			if err := p.expect("]"); err != nil {
				return nil, err
			}
			n = nIndex{n, k}
		default:
			return n, nil
		}
	}
}

func (p *parser) atom() (node, error) {
	t := p.next()
	switch t.kind {
	case tNum:
		r, ok := parseNum(t.text)
		if !ok {
			return nil, exprErr("syntax error in %q: bad number %q", p.src, t.text)
		}
		return nConst{r}, nil
	case tStr:
		s := t.text
		for p.peek().kind == tStr { // implicit concatenation
			s += p.next().text
		}
		return nConst{s}, nil
	case tName:
		switch t.text {
		case "True":
			return nConst{true}, nil
		case "False":
			return nConst{false}, nil
		case "None":
			return nConst{nil}, nil
		case "lambda", "for", "import":
			return nil, exprErr("not allowed in %q: %s", p.src, t.text)
		}
		if keywords[t.text] {
			return nil, exprErr("syntax error in %q: unexpected %q", p.src, t.text)
		}
		return nName{t.text}, nil
	case tOp:
		switch t.text {
		case "(":
			if p.isOp(")") {
				p.pos++
				return nList{}, nil
			}
			first, err := p.expr()
			if err != nil {
				return nil, err
			}
			if p.isOp(",") {
				items := []node{first}
				for p.isOp(",") {
					p.pos++
					if p.isOp(")") {
						break
					}
					e, err := p.expr()
					if err != nil {
						return nil, err
					}
					items = append(items, e)
				}
				if err := p.expect(")"); err != nil {
					return nil, err
				}
				return nList{items}, nil
			}
			if err := p.expect(")"); err != nil {
				return nil, err
			}
			return first, nil
		case "[":
			var items []node
			for !p.isOp("]") {
				e, err := p.expr()
				if err != nil {
					return nil, err
				}
				items = append(items, e)
				if !p.isOp(",") {
					break
				}
				p.pos++
			}
			if err := p.expect("]"); err != nil {
				return nil, err
			}
			return nList{items}, nil
		case "{":
			d := nDict{}
			for !p.isOp("}") {
				k, err := p.expr()
				if err != nil {
					return nil, err
				}
				if err := p.expect(":"); err != nil {
					return nil, err
				}
				v, err := p.expr()
				if err != nil {
					return nil, err
				}
				d.keys, d.vals = append(d.keys, k), append(d.vals, v)
				if !p.isOp(",") {
					break
				}
				p.pos++
			}
			if err := p.expect("}"); err != nil {
				return nil, err
			}
			return d, nil
		}
	}
	return nil, exprErr("syntax error in %q", p.src)
}

// exprCache holds compiled expressions; it is emptied when it reaches maxCachedExprs so a
// long-lived server does not grow with every model it has seen.
var (
	exprMu    sync.Mutex
	exprCache = map[string]node{}
)

const maxCachedExprs = 10_000

func compileExpr(src string) (node, error) {
	src = strings.TrimSpace(src)
	exprMu.Lock()
	n, ok := exprCache[src]
	exprMu.Unlock()
	if ok {
		return n, nil
	}
	toks, err := tokenize(src)
	if err != nil {
		return nil, err
	}
	p := &parser{toks: toks, src: src}
	n, err = p.expr()
	if err != nil {
		return nil, err
	}
	if p.peek().kind != tEOF {
		return nil, exprErr("syntax error in %q: unexpected %q", src, p.peek().text)
	}
	exprMu.Lock()
	if len(exprCache) >= maxCachedExprs {
		clear(exprCache)
	}
	exprCache[src] = n
	exprMu.Unlock()
	return n, nil
}
