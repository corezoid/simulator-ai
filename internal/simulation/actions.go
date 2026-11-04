package simulation

import (
	"fmt"
	"math/big"
	"sort"
)

// ---- actions (spec §6) -----------------------------------------------------------

func need(spec *OMap, action string, fields ...string) error {
	for _, f := range fields {
		if !spec.Has(f) {
			return ruleErr("action %q is missing field %q: %s", action, f, show(spec))
		}
	}
	return nil
}

func runActions(c *StepContext, actions any) error {
	if actions == nil {
		return nil
	}
	list, ok := actions.([]any)
	if !ok {
		return ruleErr("actions must be a list: %s", show(actions))
	}
	for _, it := range list {
		a, ok := it.(*OMap)
		if !ok || a.Len() == 0 {
			return ruleErr("bad action %s", show(it))
		}
		if a.Has("if") {
			cond, _ := a.Get("if")
			v, err := c.ev(cond)
			if err != nil {
				return err
			}
			branch := "else"
			if truthy(v) {
				branch = "then"
			}
			sub, _ := a.Get(branch)
			if err := runActions(c, sub); err != nil {
				return err
			}
			continue
		}
		name := a.keys[0]
		spec := a.m[name]
		if err := doAction(c, name, spec); err != nil {
			return err
		}
	}
	return nil
}

func accountRef(c *StepContext, spec any) (accKey, error) {
	m, ok := spec.(*OMap)
	if !ok || !m.Has("account") {
		return accKey{}, ruleErr("account reference needs {actor, account}: %s", repr(spec))
	}
	src, has := m.Get("actor")
	if !has {
		src = "self"
	}
	actor, err := c.evActor(src)
	if err != nil {
		return accKey{}, err
	}
	return accKey{actor, getString(m, "account")}, nil
}

func (c *StepContext) timeValue(raw any) (int64, error) {
	switch x := raw.(type) {
	case *big.Rat:
		return parseDuration(x)
	case string:
		if durationRe.MatchString(x) {
			return parseDuration(x)
		}
	}
	v, err := c.ev(raw)
	if err != nil {
		return 0, err
	}
	d, err := parseDuration(v)
	if err != nil {
		return 0, ruleErr("%v", err)
	}
	return d, nil
}

func intField(m *OMap, k string, def int64) int64 {
	v, ok := m.Get(k)
	if !ok || v == nil {
		return def
	}
	n, err := toNum(v)
	if err != nil {
		return def
	}
	return truncInt(n).Int64()
}

func doAction(c *StepContext, name string, raw any) error {
	if name == "log" {
		v, err := c.val(raw)
		if err != nil {
			return err
		}
		c.notes = append(c.notes, Note{Op: "log", Msg: show(v)})
		return nil
	}
	spec, ok := raw.(*OMap)
	if !ok {
		if raw == nil && (name == "release" || name == "set") {
			spec = NewOMap()
		} else {
			return ruleErr("action %q expects a mapping: %s", name, show(raw))
		}
	}
	switch name {
	case "set":
		for _, k := range spec.Keys() {
			v, err := c.val(spec.m[k])
			if err != nil {
				return err
			}
			if err := c.s.setField(c.self, k, v); err != nil {
				return err
			}
		}
	case "set_on":
		if err := need(spec, name, "actor"); err != nil {
			return err
		}
		target, err := c.evActor(spec.m["actor"])
		if err != nil {
			return err
		}
		if fields := getMap(spec, "fields"); fields != nil {
			for _, k := range fields.Keys() {
				v, err := c.val(fields.m[k])
				if err != nil {
					return err
				}
				if err := c.s.setField(target, k, v); err != nil {
					return err
				}
			}
		}
	case "create":
		if err := need(spec, name, "type"); err != nil {
			return err
		}
		typ := show(spec.m["type"])
		as := getString(spec, "as")
		ordinal := 0
		for _, n := range c.notes {
			if n.Op == "create" {
				ordinal++
			}
		}
		label := as
		if label == "" {
			label = typ
		}
		logical := fmt.Sprintf("%s:%s#%d", c.event.Key, label, ordinal)
		titleSpec, has := spec.Get("title")
		if !has {
			titleSpec = typ
		}
		title, err := c.val(titleSpec)
		if err != nil {
			return err
		}
		data := NewOMap()
		if dm := getMap(spec, "data"); dm != nil {
			v, err := c.val(dm)
			if err != nil {
				return err
			}
			data = v.(*OMap)
		}
		id, err := c.s.createActor(typ, show(title), data, c.event.Key, logical)
		if err != nil {
			return err
		}
		c.notes = append(c.notes, Note{Op: "create", ID: id, Extra: map[string]any{"logical_id": logical}})
		if lf, ok := spec.Get("link_from"); ok {
			src, err := c.evActor(lf)
			if err != nil {
				return err
			}
			et := getString(spec, "edge_type")
			if et == "" {
				et = hierarchy
			}
			if _, err := c.s.createLink(src, id, et); err != nil {
				return err
			}
		}
		for _, acc := range getList(spec, "accounts") {
			am, _ := acc.(*OMap)
			if _, err := c.s.ensureAccount(id, getString(am, "name"), getString(am, "value_type")); err != nil {
				return err
			}
		}
		if as != "" {
			c.vars[as] = &ActorView{c.s, id}
		}
	case "link":
		if err := need(spec, name, "from", "to"); err != nil {
			return err
		}
		src, err := c.evActor(spec.m["from"])
		if err != nil {
			return err
		}
		dst, err := c.evActor(spec.m["to"])
		if err != nil {
			return err
		}
		et := getString(spec, "edge_type")
		if et == "" {
			et = hierarchy
		}
		_, err = c.s.createLink(src, dst, et)
		return err
	case "transfer":
		if err := need(spec, name, "amount", "from", "to"); err != nil {
			return err
		}
		amount, err := c.evNum(spec.m["amount"])
		if err != nil {
			return err
		}
		if amount.Sign() == 0 {
			return nil
		}
		src, err := accountRef(c, spec.m["from"])
		if err != nil {
			return err
		}
		dst, err := accountRef(c, spec.m["to"])
		if err != nil {
			return err
		}
		return c.s.transfer(src, dst, amount)
	case "add", "set_account":
		field := "amount"
		if name == "set_account" {
			field = "value"
		}
		ref, err := accountRef(c, spec)
		if err != nil {
			return err
		}
		if err := need(spec, name, field); err != nil {
			return err
		}
		n, err := c.evNum(spec.m[field])
		if err != nil {
			return err
		}
		if name == "add" {
			return c.s.add(ref, n, getString(spec, "value_type"))
		}
		return c.s.setAccount(ref, n, getString(spec, "value_type"))
	case "schedule":
		if err := need(spec, name, "event"); err != nil {
			return err
		}
		tsrc, has := spec.Get("target")
		if !has {
			tsrc = "self"
		}
		target, err := c.evActor(tsrc)
		if err != nil {
			return err
		}
		var after int64
		if a, ok := spec.Get("after"); ok {
			if after, err = c.timeValue(a); err != nil {
				return err
			}
		}
		at := c.now + after
		if a, ok := spec.Get("at"); ok {
			if at, err = c.timeValue(a); err != nil {
				return err
			}
		}
		if at < c.now {
			return ruleErr("cannot schedule %s in the past (%d < %d)", show(spec.m["event"]), at, c.now)
		}
		payload, err := c.payload(spec)
		if err != nil {
			return err
		}
		c.schedule(scheduled{show(spec.m["event"]), target, at, intField(spec, "priority", 30), payload})
	case "enqueue":
		if err := need(spec, name, "resource", "event"); err != nil {
			return err
		}
		resource, err := c.evActor(spec.m["resource"])
		if err != nil {
			return err
		}
		tsrc, has := spec.Get("target")
		if !has {
			tsrc = "self"
		}
		target, err := c.evActor(tsrc)
		if err != nil {
			return err
		}
		res, err := c.s.actor(resource)
		if err != nil {
			return err
		}
		if acc, ok := spec.Get("account"); ok {
			cur, _ := res.Data.Get("_capacity_account")
			if !equal(cur, acc) {
				if err := c.s.setField(resource, "_capacity_account", acc); err != nil {
					return err
				}
			}
		}
		amtSrc, has := spec.Get("amount")
		if !has {
			amtSrc = ratOne
		}
		amt, err := c.ev(amtSrc)
		if err != nil {
			return err
		}
		payload, err := c.payload(spec)
		if err != nil {
			return err
		}
		job := NewOMap()
		job.Set("target", target)
		job.Set("event", show(spec.m["event"]))
		job.Set("amount", show(amt))
		job.Set("priority", ratInt(intField(spec, "priority", 40)))
		job.Set("payload", payload)
		queue := append(queueOf(res), job)
		if err := c.s.setField(resource, "_queue", queue); err != nil {
			return err
		}
		return c.dispatch(resource)
	case "release":
		tsrc, has := spec.Get("target")
		if !has {
			tsrc = "self"
		}
		target, err := c.evActor(tsrc)
		if err != nil {
			return err
		}
		a, err := c.s.actor(target)
		if err != nil {
			return err
		}
		hv, _ := a.Data.Get("_holding")
		hold, _ := hv.(*OMap)
		if hold == nil || !truthy(hold) {
			return nil
		}
		amt, err := toNum(getString(hold, "amount"))
		if err != nil {
			return ruleErr("%v", err)
		}
		account, resource := getString(hold, "account"), getString(hold, "resource")
		if err := c.s.transfer(accKey{target, account}, accKey{resource, account}, amt); err != nil {
			return err
		}
		if err := c.s.setField(target, "_holding", nil); err != nil {
			return err
		}
		return c.dispatch(resource)
	case "dequeue":
		if err := need(spec, name, "resource"); err != nil {
			return err
		}
		resource, err := c.evActor(spec.m["resource"])
		if err != nil {
			return err
		}
		tsrc, has := spec.Get("target")
		if !has {
			tsrc = "self"
		}
		target, err := c.evActor(tsrc)
		if err != nil {
			return err
		}
		res, err := c.s.actor(resource)
		if err != nil {
			return err
		}
		queue := queueOf(res)
		kept := []any{}
		for _, j := range queue {
			if getString(j.(*OMap), "target") != target {
				kept = append(kept, j)
			}
		}
		if len(kept) != len(queue) {
			return c.s.setField(resource, "_queue", kept)
		}
	case "decide":
		if err := need(spec, name, "options"); err != nil {
			return err
		}
		var options []string
		for _, o := range getList(spec, "options") {
			options = append(options, show(o))
		}
		varName := getString(spec, "var")
		if varName == "" {
			varName = "choice"
		}
		var ruleChoice *string
		if r, ok := spec.Get("rule"); ok {
			v, err := c.ev(r)
			if err != nil {
				return err
			}
			s := show(v)
			if !contains(options, s) {
				return ruleErr("rule returned %s, not one of %s", repr(s), show(toAny(options)))
			}
			ruleChoice = &s
		}
		choice, record, err := c.decider(c, spec, options, ruleChoice)
		if err != nil {
			return err
		}
		if !contains(options, choice) {
			return ruleErr("decision %s is not one of %s", repr(choice), show(toAny(options)))
		}
		c.vars[varName] = choice
		src, _ := record["source"].(string)
		c.notes = append(c.notes, Note{Op: "decide", Var: varName, Choice: choice, Source: src, Extra: record})
	default:
		known := []string{"add", "create", "decide", "dequeue", "enqueue", "link", "log", "release", "schedule", "set", "set_account", "set_on", "transfer"}
		sort.Strings(known)
		return ruleErr("unknown action %q", name)
	}
	return nil
}

func (c *StepContext) payload(spec *OMap) (*OMap, error) {
	p := getMap(spec, "payload")
	if p == nil {
		return NewOMap(), nil
	}
	v, err := c.val(p)
	if err != nil {
		return nil, err
	}
	return v.(*OMap), nil
}

func queueOf(a *Actor) []any {
	v, _ := a.Data.Get("_queue")
	l, _ := v.([]any)
	return append([]any{}, l...)
}

// dispatch gives free capacity to waiting jobs in FIFO order (spec §6, resource queue).
func (c *StepContext) dispatch(resource string) error {
	res, err := c.s.actor(resource)
	if err != nil {
		return err
	}
	capAcc := "capacity"
	if v, ok := res.Data.Get("_capacity_account"); ok && v != nil {
		capAcc = show(v)
	}
	queue := queueOf(res)
	changed := false
	for len(queue) > 0 {
		job := queue[0].(*OMap)
		needAmt := ratOne
		if a, ok := job.Get("amount"); ok {
			if needAmt, err = toNum(a); err != nil {
				return ruleErr("%v", err)
			}
		}
		free := c.s.account(resource, capAcc)
		if free == nil || free.Value.Cmp(needAmt) < 0 {
			break
		}
		queue = queue[1:]
		changed = true
		target := getString(job, "target")
		if err := c.s.transfer(accKey{resource, capAcc}, accKey{target, capAcc}, needAmt); err != nil {
			return err
		}
		hold := NewOMap()
		hold.Set("resource", resource)
		hold.Set("account", capAcc)
		hold.Set("amount", numString(needAmt))
		if err := c.s.setField(target, "_holding", hold); err != nil {
			return err
		}
		payload := getMap(job, "payload")
		if payload == nil {
			payload = NewOMap()
		}
		c.schedule(scheduled{getString(job, "event"), target, c.now, intField(job, "priority", 40), deepCopy(payload).(*OMap)})
	}
	if changed {
		return c.s.setField(resource, "_queue", queue)
	}
	return nil
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

func toAny(xs []string) []any {
	out := make([]any, len(xs))
	for i, x := range xs {
		out[i] = x
	}
	return out
}

// RuleDecider: the rule if present, else a uniform pick with a stable stream (spec §7).
func RuleDecider(c *StepContext, spec *OMap, options []string, ruleChoice *string) (string, map[string]any, error) {
	if ruleChoice != nil {
		return *ruleChoice, map[string]any{"source": "rule"}, nil
	}
	varName := getString(spec, "var")
	if varName == "" {
		varName = "choice"
	}
	u := stableUniform(c.seed, "decide", c.logicalSelf(), c.event.Key, varName)
	idx := truncInt(new(big.Rat).Mul(u, ratInt(int64(len(options))))).Int64()
	if idx > int64(len(options)-1) {
		idx = int64(len(options) - 1)
	}
	return options[idx], map[string]any{"source": "uniform", "u": numString(u)}, nil
}
