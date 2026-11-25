package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/corezoid/simulator-ai/internal/publicapi"
)

// sim-api reads transactions and transfers in a window of from and to (Unix ms) of at most 366
// days and refuses a longer one. "Who paid X over all time" asked for more and failed (eval
// 2026-11-24): the plugin splits a longer period into windows, newest first, up to maxWindows of
// them (about five years), and sums counts or joins pages.

const (
	// maxWindowMs is sim-api's MaxFilterPeriodMs: 366 days.
	maxWindowMs = int64(366 * 24 * time.Hour / time.Millisecond)
	// maxWindows bounds one call: about five years back from to.
	maxWindows = 5
	// defaultWindowRows is the backend's page size when the call sends no limit.
	defaultWindowRows = 50
)

// windowNote is the description of from and to on the windowed tools.
var windowNote = map[string]string{
	"from": "Window start, Unix ms, sent with to. sim-api reads at most 366 days per window: a longer period (all time: from 0) is read in 366-day windows newest first, at most 5 years back from to.",
	"to":   "Window end, Unix ms, sent with from (without both: this month).",
}

// transferWindowNote adds what the window of a transfer search bounds: sim-api's dateBy.
var transferWindowNote = map[string]string{
	"from": windowNote["from"],
	"to":   windowNote["to"],
	"dateBy": "What from/to bound: createdAt (default) when the transfer was entered; originalDate its business date — a transfer with a leg " +
		"whose originalDate is in the window, ordered by it and answered as originalDate. A month or date of payment (\"in April\") is originalDate. " +
		"Refused (-32602 invalid params) with formFields or kind formFields: a form-field transfer has no business date.",
}

type period struct{ from, to int64 }

// halfOpenMethods are the methods whose window is from <= t < to (the filters and their counts); the
// lists read from <= t <= to (BETWEEN). Windows must meet exactly on either kind: no row lost at a
// boundary, none read twice.
var halfOpenMethods = map[string]bool{"transaction/filter": true, "transaction/filter/count": true, "transfer/filter": true, "transfer/filter/count": true}

// windows splits the period into windows of at most maxWindowMs, newest first, and says whether
// the ceiling cut the oldest part off. A half-open window [start, end) is followed by one ending at
// start; a closed one [start, end] by one ending at start - 1.
func windows(from, to int64, halfOpen bool) ([]period, bool) {
	step, last := maxWindowMs+1, from
	if halfOpen {
		step, last = maxWindowMs, from+1
	}
	var out []period
	for end := to; end >= last; end -= step {
		if len(out) == maxWindows {
			return out, true
		}
		out = append(out, period{max(from, end-maxWindowMs), end})
	}
	return out, false
}

// longPeriod reads from and to; ok is false unless both are sent and span more than one window.
func longPeriod(args map[string]json.RawMessage) (from, to int64, ok bool, err error) {
	if args["from"] == nil || args["to"] == nil {
		return 0, 0, false, nil
	}
	if json.Unmarshal(args["from"], &from) != nil || json.Unmarshal(args["to"], &to) != nil {
		return 0, 0, false, nil // the backend names a bad value itself
	}
	if to < from {
		return 0, 0, false, errors.New(`"to" is before "from"`)
	}
	return from, to, to-from > maxWindowMs, nil
}

func day(ms int64) string { return time.UnixMilli(ms).UTC().Format("2006-01-02") }

// cutNote says which part of the period the ceiling left unread.
func cutNote(ws []period, from int64) string {
	return fmt.Sprintf("read %d windows of 366 days back to %s; the period before it (from %s) was not read: ask for it with to before %s", len(ws), day(ws[len(ws)-1].from), day(from), day(ws[len(ws)-1].from))
}

// windowedCount sums a count method over the windows of a long period.
func windowedCount(ctx context.Context, c *publicapi.Client, method string, args, _ map[string]json.RawMessage) (json.RawMessage, error) {
	from, to, long, err := longPeriod(args)
	if err != nil || !long {
		if err != nil {
			return nil, err
		}
		return c.Pass(ctx, method, args)
	}
	ws, cut := windows(from, to, halfOpenMethods[method])
	total := int64(0)
	for _, w := range ws {
		args["from"], args["to"] = jsonInt(w.from), jsonInt(w.to)
		v, err := c.Pass(ctx, method, args)
		if err != nil {
			return nil, fmt.Errorf("window %s..%s: %w", day(w.from), day(w.to), err)
		}
		var got struct {
			Count int64 `json:"count"`
		}
		if err = json.Unmarshal(v, &got); err != nil {
			return nil, fmt.Errorf("window %s..%s: %w", day(w.from), day(w.to), err)
		}
		total += got.Count
	}
	out := map[string]any{"count": total, "windows": len(ws)}
	if cut {
		out["note"] = cutNote(ws, from)
	}
	return json.Marshal(out)
}

// windowedList joins the pages of a long period, newest window first, up to the call's limit. A
// window with more rows than fit stops the walk: next names the call that reads on (the same to
// and from, with nextCursor), and the windows past it follow from there. offset does not page
// across windows; a cursor applies to the newest window (the one a next names).
func windowedList(ctx context.Context, c *publicapi.Client, method string, args, _ map[string]json.RawMessage) (json.RawMessage, error) {
	from, to, long, err := longPeriod(args)
	if err != nil || !long {
		if err != nil {
			return nil, err
		}
		return c.Pass(ctx, method, args)
	}
	if args["offset"] != nil {
		return nil, errors.New("a period over 366 days is read in windows: page it with cursor (from next), not offset")
	}
	limit := int64(defaultWindowRows)
	if args["limit"] != nil {
		if err = json.Unmarshal(args["limit"], &limit); err != nil || limit < 1 {
			return nil, errors.New("limit must be a positive integer")
		}
	}
	ws, cut := windows(from, to, halfOpenMethods[method])
	var rows []json.RawMessage
	out := map[string]any{}
	for i, w := range ws {
		args["from"], args["to"] = jsonInt(w.from), jsonInt(w.to)
		args["limit"] = jsonInt(limit - int64(len(rows)))
		if i > 0 {
			delete(args, "cursor")
		}
		v, err := c.Pass(ctx, method, args)
		if err != nil {
			return nil, fmt.Errorf("window %s..%s: %w", day(w.from), day(w.to), err)
		}
		var page struct {
			List       []json.RawMessage `json:"list"`
			HasMore    bool              `json:"hasMore"`
			NextCursor string            `json:"nextCursor"`
		}
		if err = json.Unmarshal(v, &page); err != nil {
			return nil, fmt.Errorf("window %s..%s: %w", day(w.from), day(w.to), err)
		}
		if i == 0 {
			keepPageKeys(v, out) // e.g. appliedAccounts of transaction/filter
		}
		rows = append(rows, page.List...)
		if page.HasMore {
			out["next"] = map[string]any{"from": from, "to": w.to, "cursor": page.NextCursor}
			break
		}
		if int64(len(rows)) >= limit && i+1 < len(ws) {
			out["next"] = map[string]any{"from": from, "to": ws[i+1].to}
			break
		}
	}
	if rows == nil {
		rows = []json.RawMessage{}
	}
	if next, ok := out["next"].(map[string]any); ok && args["dateBy"] != nil {
		next["dateBy"] = args["dateBy"] // the windows are on that date: reading on keeps it
	}
	out["list"], out["hasMore"] = rows, out["next"] != nil
	if cut && out["next"] == nil {
		out["note"] = cutNote(ws, from)
	}
	return json.Marshal(out)
}

func jsonInt(n int64) json.RawMessage { return json.RawMessage(fmt.Sprint(n)) }

// keepPageKeys copies a page's own keys other than the rows and their paging into out.
func keepPageKeys(page json.RawMessage, out map[string]any) {
	var doc map[string]json.RawMessage
	if json.Unmarshal(page, &doc) != nil {
		return
	}
	for k, v := range doc {
		if k != "list" && k != "hasMore" && k != "nextCursor" {
			out[k] = v
		}
	}
}
