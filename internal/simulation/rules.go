package simulation

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math/big"
	"strings"
)

type RuleError struct{ msg string }

func (e *RuleError) Error() string { return e.msg }

func ruleErr(format string, a ...any) error { return &RuleError{fmt.Sprintf(format, a...)} }

// Model is a parsed model.yaml (spec §5).
type Model struct {
	ValueTypes    map[string]ValueType
	vtOrder       []string
	Refs          *OMap
	Params        *OMap
	Horizon       int64
	InitialEvents []*OMap
	Behaviors     *OMap // type -> (event -> []action)
	Metrics       *OMap // name -> expression
	Goals         *OMap
	TypeField     string
	Actors        []*OMap
	Raw           *OMap
}

func LoadModel(src []byte) (*Model, error) {
	raw, err := parseYAML(src)
	if err != nil {
		return nil, fmt.Errorf("model: %w", err)
	}
	d, _ := raw.(*OMap)
	if d == nil {
		d = NewOMap()
	}
	m := &Model{ValueTypes: map[string]ValueType{}, Raw: d}
	if vts := getMap(d, "value_types"); vts != nil {
		for _, name := range vts.Keys() {
			spec, _ := vts.m[name].(*OMap)
			vt, err := valueTypeFrom(name, spec)
			if err != nil {
				return nil, err
			}
			m.ValueTypes[name] = vt
			m.vtOrder = append(m.vtOrder, name)
		}
	}
	orEmpty := func(k string) *OMap {
		if v := getMap(d, k); v != nil {
			return v
		}
		return NewOMap()
	}
	m.Refs, m.Params, m.Behaviors, m.Metrics, m.Goals = orEmpty("refs"), orEmpty("params"), orEmpty("behaviors"), orEmpty("metrics"), orEmpty("goals")
	h, _ := d.Get("horizon")
	if h == nil {
		h = ratZero
	}
	if m.Horizon, err = parseDuration(h); err != nil {
		return nil, fmt.Errorf("horizon: %w", err)
	}
	for i, e := range getList(d, "initial_events") {
		em, ok := e.(*OMap)
		if !ok {
			return nil, fmt.Errorf("initial_events[%d]: mapping expected", i)
		}
		m.InitialEvents = append(m.InitialEvents, em)
	}
	for i, a := range getList(d, "actors") {
		am, ok := a.(*OMap)
		if !ok {
			return nil, fmt.Errorf("actors[%d]: mapping expected", i)
		}
		m.Actors = append(m.Actors, am)
	}
	m.TypeField = getString(d, "type_field")
	return m, nil
}

// stableUniform is U(parts) of spec §7: SHA-256 of the joined parts, first 8 bytes / 2^64.
func stableUniform(parts ...string) *big.Rat {
	h := sha256.Sum256([]byte(strings.Join(parts, "|")))
	n := new(big.Int).SetUint64(binary.BigEndian.Uint64(h[:8]))
	return new(big.Rat).SetFrac(n, new(big.Int).Lsh(big.NewInt(1), 64))
}

// ---- views ---------------------------------------------------------------

type ActorView struct {
	s  *store
	id string
}

func (v *ActorView) actor() (*Actor, error) { return v.s.actor(v.id) }

func (v *ActorView) title() string {
	if a, err := v.actor(); err == nil {
		return a.Title
	}
	return ""
}

func (v *ActorView) label() string {
	if t := v.title(); t != "" {
		return t
	}
	return v.id
}

func (v *ActorView) attr(name string) (any, error) {
	a, err := v.actor()
	if err != nil {
		return nil, err
	}
	switch name {
	case "id":
		return v.id, nil
	case "type":
		return a.Type, nil
	case "title":
		return a.Title, nil
	case "data":
		return a.Data, nil
	case "parent":
		p := v.s.g.parents(v.id, "")
		if len(p) == 0 {
			return nil, nil
		}
		return &ActorView{v.s, p[0]}, nil
	}
	if val, ok := a.Data.Get(name); ok {
		return val, nil
	}
	return nil, exprErr("actor %s has no field %q", v.label(), name)
}

func (v *ActorView) related(ids []string, typ any) []any {
	out := []any{}
	for _, id := range ids {
		if typ != nil {
			a := v.s.g.actorIdx[id]
			if a == nil || a.Type != show(typ) {
				continue
			}
		}
		out = append(out, &ActorView{v.s, id})
	}
	return out
}

func (v *ActorView) method(name string, args []any, kw *OMap) (any, error) {
	a, err := v.actor()
	if err != nil {
		return nil, err
	}
	switch name {
	case "acc":
		acc := v.s.account(v.id, show(argOr(args, kw, 0, "name", nil)))
		if acc == nil {
			return new(big.Rat), nil
		}
		return acc.Value, nil
	case "sum_accounts":
		vt := argOr(args, kw, 0, "value_type", nil)
		sum := new(big.Rat)
		for _, acc := range v.s.g.accounts {
			if acc.ActorID == v.id && (vt == nil || acc.ValueType == show(vt)) {
				sum.Add(sum, acc.Value)
			}
		}
		return sum, nil
	case "has":
		return a.Data.Has(show(argOr(args, kw, 0, "key", nil))), nil
	case "get":
		if val, ok := a.Data.Get(show(argOr(args, kw, 0, "key", nil))); ok {
			return val, nil
		}
		return argOr(args, kw, 1, "default", nil), nil
	case "children", "parents":
		et := ""
		if e := argOr(args, kw, 1, "edge_type", nil); e != nil {
			et = show(e)
		}
		ids := v.s.g.children(v.id, et)
		if name == "parents" {
			ids = v.s.g.parents(v.id, et)
		}
		return v.related(ids, argOr(args, kw, 0, "type", nil)), nil
	}
	return nil, exprErr("no method %q on actor %s", name, v.label())
}

type Namespace struct {
	data *OMap
	name string
}

func (n *Namespace) attr(k string) (any, error) {
	if v, ok := n.data.Get(k); ok {
		return v, nil
	}
	return nil, exprErr("%s has no field %q", n.name, k)
}

func actorIDOf(v any) (string, error) {
	switch x := v.(type) {
	case *ActorView:
		return x.id, nil
	case string:
		return x, nil
	}
	return "", ruleErr("expected an actor, got %s", repr(v))
}

// ---- step context ------------------------------------------------------------

type Event struct {
	Time     int64
	Priority int64
	Seq      int64
	Kind     string
	Target   string
	Payload  *OMap
	Key      string
	// a recurring initial event keeps only its next occurrence queued
	chain *chain
	n     int64
}

// chain is one recurring initial event on one target.
type chain struct {
	every, last int64 // period; index of the last occurrence within the horizon
	keyPrefix   string
}

type scheduled struct {
	kind, target string
	time, prio   int64
	payload      *OMap
}

type Note struct {
	Op     string         `json:"op"`
	Var    string         `json:"var,omitempty"`
	Choice string         `json:"choice,omitempty"`
	Source string         `json:"source,omitempty"`
	ID     string         `json:"id,omitempty"`
	Msg    string         `json:"message,omitempty"`
	Extra  map[string]any `json:"extra,omitempty"`
}

// Decider picks an option of a decide action.
type Decider func(ctx *StepContext, spec *OMap, options []string, ruleChoice *string) (string, map[string]any, error)

type StepContext struct {
	s        *store
	model    *Model
	params   *OMap
	refs     *OMap // name -> actor id
	now      int64
	event    *Event
	self     string
	seed     string
	schedule func(scheduled)
	decider  Decider
	vars     map[string]any
	notes    []Note
	draws    int
}

func (c *StepContext) actorType(id string) string {
	a := c.s.g.actorIdx[id]
	if a == nil {
		return ""
	}
	if c.model.TypeField != "" {
		if v, ok := a.Data.Get(c.model.TypeField); ok {
			return show(v)
		}
	}
	return a.Type
}

func (c *StepContext) logicalSelf() string {
	a := c.s.g.actorIdx[c.self]
	if a == nil {
		return c.self
	}
	if v, ok := a.Data.Get("_logical_id"); ok && truthy(v) {
		return show(v)
	}
	if a.OriginID != "" {
		return a.OriginID
	}
	return a.ID
}

func (c *StepContext) names() map[string]any {
	refs := NewOMap()
	for _, k := range c.refs.Keys() {
		id, _ := c.refs.Get(k)
		refs.Set(k, &ActorView{c.s, show(id)})
	}
	payload := c.event.Payload
	if payload == nil {
		payload = NewOMap()
	}
	n := map[string]any{
		"self":   &ActorView{c.s, c.self},
		"event":  &Namespace{payload, "event"},
		"params": &Namespace{c.params, "params"},
		"refs":   &Namespace{refs, "refs"},
		"now":    ratInt(c.now),
	}
	for k, v := range c.vars {
		n[k] = v
	}
	return n
}

func (c *StepContext) rand() *big.Rat {
	c.draws++
	return stableUniform(c.seed, "rand", c.logicalSelf(), c.event.Key, fmt.Sprint(c.draws))
}

func (c *StepContext) filterActors(args []any, kw *OMap, typeArgIndex int) []*ActorView {
	typ := argOr(args, nil, typeArgIndex, "", nil)
	if t, ok := kw.Get("type"); ok {
		typ = t
	}
	var out []*ActorView
	for _, a := range c.s.g.actors {
		if typ != nil && c.actorType(a.ID) != show(typ) {
			continue
		}
		ok := true
		for _, k := range kw.Keys() {
			if k == "type" {
				continue
			}
			want, _ := kw.Get(k)
			got, _ := a.Data.Get(k)
			if !equal(got, want) {
				ok = false
				break
			}
		}
		if ok {
			out = append(out, &ActorView{c.s, a.ID})
		}
	}
	return out
}

func numArg(v any, fn string) (*big.Rat, error) {
	n, err := toNum(v)
	if err != nil {
		return nil, exprErr("%s: %v", fn, err)
	}
	return n, nil
}

func (c *StepContext) funcs() map[string]Func {
	listOf := func(vs []*ActorView) []any {
		out := make([]any, len(vs))
		for i, v := range vs {
			out[i] = v
		}
		return out
	}
	extreme := func(name string, sign int) Func {
		return func(args []any, kw *OMap) (any, error) {
			items := args
			if len(args) == 1 {
				if l, ok := args[0].([]any); ok {
					items = l
				}
			}
			if len(items) == 0 {
				return nil, exprErr("%s() arg is an empty sequence", name)
			}
			best := items[0]
			env := &Env{src: name}
			for _, it := range items[1:] {
				less, err := env.compare("<", it, best)
				if err != nil {
					return nil, err
				}
				if (sign < 0 && less) || (sign > 0 && !less && !equal(it, best)) {
					best = it
				}
			}
			return best, nil
		}
	}
	return map[string]Func{
		"actor": func(args []any, kw *OMap) (any, error) {
			id, err := actorIDOf(argOr(args, kw, 0, "x", nil))
			if err != nil {
				return nil, err
			}
			return &ActorView{c.s, id}, nil
		},
		"actors": func(args []any, kw *OMap) (any, error) { return listOf(c.filterActors(args, kw, 0)), nil },
		"count": func(args []any, kw *OMap) (any, error) {
			return ratInt(int64(len(c.filterActors(args, kw, 0)))), nil
		},
		"total": func(args []any, kw *OMap) (any, error) {
			account := show(argOr(args, kw, 0, "account", nil))
			rest := NewOMap()
			for _, k := range kw.Keys() {
				if k != "account" {
					v, _ := kw.Get(k)
					rest.Set(k, v)
				}
			}
			var typeArgs []any
			if len(args) > 1 {
				typeArgs = args[1:]
			}
			sum := new(big.Rat)
			for _, v := range c.filterActors(typeArgs, rest, 0) {
				if acc := c.s.account(v.id, account); acc != nil {
					sum.Add(sum, acc.Value)
				}
			}
			return sum, nil
		},
		"dur": func(args []any, kw *OMap) (any, error) {
			d, err := parseDuration(argOr(args, kw, 0, "v", nil))
			if err != nil {
				return nil, &ExprError{err.Error()}
			}
			return ratInt(d), nil
		},
		"dec": func(args []any, kw *OMap) (any, error) { return numArg(argOr(args, kw, 0, "x", nil), "dec") },
		"min": extreme("min", -1),
		"max": extreme("max", 1),
		"abs": func(args []any, kw *OMap) (any, error) {
			n, err := numArg(argOr(args, kw, 0, "x", nil), "abs")
			if err != nil {
				return nil, err
			}
			return new(big.Rat).Abs(n), nil
		},
		"len": func(args []any, kw *OMap) (any, error) {
			switch x := argOr(args, kw, 0, "x", nil).(type) {
			case []any:
				return ratInt(int64(len(x))), nil
			case string:
				return ratInt(int64(len([]rune(x)))), nil
			case *OMap:
				return ratInt(int64(x.Len())), nil
			}
			return nil, exprErr("len() of unsized value")
		},
		"round": func(args []any, kw *OMap) (any, error) {
			n, err := numArg(argOr(args, kw, 0, "x", nil), "round")
			if err != nil {
				return nil, err
			}
			places := int64(0)
			if p := argOr(args, kw, 1, "n", nil); p != nil {
				pn, err := numArg(p, "round")
				if err != nil {
					return nil, err
				}
				places = truncInt(pn).Int64()
			}
			return quantize(n, int(places)), nil
		},
		"str":  func(args []any, kw *OMap) (any, error) { return show(argOr(args, kw, 0, "x", nil)), nil },
		"rand": func(args []any, kw *OMap) (any, error) { return c.rand(), nil },
		"pick": func(args []any, kw *OMap) (any, error) {
			items, _ := argOr(args, kw, 0, "items", nil).([]any)
			if len(items) == 0 {
				return nil, nil
			}
			u := c.rand()
			idx := truncInt(new(big.Rat).Mul(u, ratInt(int64(len(items))))).Int64()
			if idx > int64(len(items)-1) {
				idx = int64(len(items) - 1)
			}
			return items[idx], nil
		},
		"where": func(args []any, kw *OMap) (any, error) {
			items, _ := argOr(args, kw, 0, "items", nil).([]any)
			has, hasOK := kw.Get("title_has")
			not, notOK := kw.Get("title_not")
			leaf, leafOK := kw.Get("leaf")
			out := []any{}
			for _, it := range items {
				v, ok := it.(*ActorView)
				if !ok {
					continue
				}
				t := strings.ToLower(v.title())
				if hasOK && has != nil && !strings.Contains(t, strings.ToLower(show(has))) {
					continue
				}
				if notOK && not != nil && strings.Contains(t, strings.ToLower(show(not))) {
					continue
				}
				if leafOK && leaf != nil && (len(c.s.g.children(v.id, "")) == 0) != truthy(leaf) {
					continue
				}
				out = append(out, v)
			}
			return out, nil
		},
		"chance": func(args []any, kw *OMap) (any, error) {
			p, err := numArg(argOr(args, kw, 0, "p", nil), "chance")
			if err != nil {
				return nil, err
			}
			return c.rand().Cmp(p) < 0, nil
		},
	}
}

func (c *StepContext) ev(src any) (any, error) { return evaluate(src, c.names(), c.funcs()) }

func (c *StepContext) val(spec any) (any, error) { return valueOf(spec, c.names(), c.funcs()) }

func (c *StepContext) evActor(src any) (string, error) {
	v, err := c.ev(src)
	if err != nil {
		return "", err
	}
	return actorIDOf(v)
}

func (c *StepContext) evNum(src any) (*big.Rat, error) {
	v, err := c.ev(src)
	if err != nil {
		return nil, err
	}
	n, err := toNum(v)
	if err != nil {
		return nil, ruleErr("%v", err)
	}
	return n, nil
}
