package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type chartResult struct {
	ChartID       string   `json:"chartId"`
	FilterActorID string   `json:"filterActorId"`
	SourceType    string   `json:"sourceType"`
	LaID          int64    `json:"laId"`
	Warnings      []string `json:"warnings"`
}

// charts (console-full): a manual chart of one account on a layer, read back as a chart placement
// with its source; turned into an actorFilter chart (a new ActorFilters actor) and resized; then
// back to manual, which reports the filter it no longer uses; and a link to it.
func charts(c *client, stamp string) error {
	var pair struct {
		NameID     string `json:"nameId"`
		CurrencyID int64  `json:"currencyId"`
	}
	if err := c.tool("account_pair_create", map[string]any{"accountName": "AI smoke chart " + stamp, "currencyName": "AI smoke chart unit " + stamp}, &pair); err != nil {
		return err
	}
	var layer, series id
	if err := c.tool("layer_create", map[string]any{"title": "AI smoke charts " + stamp}, &layer); err != nil {
		return err
	}
	if err := c.tool("layer_create", map[string]any{"title": "AI smoke chart series " + stamp}, &series); err != nil {
		return err
	}
	var actor struct {
		FormID int64 `json:"formId"`
	}
	if err := c.tool("actor_get", map[string]any{"actorId": series.ID, "fields": []string{"formId"}}, &actor); err != nil {
		return err
	}
	var ignored json.RawMessage
	if err := c.tool("account_create", map[string]any{"actorId": series.ID, "nameId": pair.NameID, "currencyId": pair.CurrencyID}, &ignored); err != nil {
		return err
	}
	account := map[string]any{"actorId": series.ID, "nameId": pair.NameID, "currencyId": pair.CurrencyID}
	var made chartResult
	if err := c.tool("chart_create", map[string]any{"layerId": layer.ID, "title": "Smoke chart " + stamp, "chartType": "bar", "range": "lastMonth",
		"position": map[string]any{"x": 300, "y": 200}, "accounts": []any{account}}, &made); err != nil {
		return err
	}
	if err := chartReadBack(c, layer.ID, made, "manual", 650); err != nil {
		return err
	}
	if err := chartData(c, made.ChartID, series.ID); err != nil {
		return err
	}
	var changed chartResult
	if err := c.tool("chart_update", map[string]any{"chartId": made.ChartID, "sourceFormId": actor.FormID, "accountNameId": pair.NameID, "currencyId": pair.CurrencyID, "top": 5,
		"layerId": layer.ID, "laId": made.LaID, "width": 800, "height": 500}, &changed); err != nil {
		return err
	}
	if changed.SourceType != "actorFilter" || changed.FilterActorID == "" || changed.LaID != made.LaID {
		return fmt.Errorf("chart_update to a filter returned %+v", changed)
	}
	if err := chartReadBack(c, layer.ID, changed, "actorFilter", 800); err != nil {
		return err
	}
	var back chartResult
	if err := c.tool("chart_update", map[string]any{"chartId": made.ChartID, "accounts": []any{account}}, &back); err != nil {
		return err
	}
	if back.SourceType != "manual" || len(back.Warnings) != 1 || !strings.Contains(back.Warnings[0], changed.FilterActorID) {
		return fmt.Errorf("chart_update back to manual returned %+v, want a warning naming %s", back, changed.FilterActorID)
	}
	var link struct {
		URL string `json:"url"`
	}
	if err := c.tool("build_link", map[string]any{"entity": "chart", "id": made.ChartID}, &link); err != nil {
		return err
	}
	fmt.Println("OK charts chart", made.ChartID, "filter", changed.FilterActorID, link.URL)
	return nil
}

// chartReadBack checks the chart actor's stored source and that its placement is drawn as a chart.
func chartReadBack(c *client, layerID string, r chartResult, sourceType string, width float64) error {
	var actor struct {
		Data struct {
			Source string `json:"source"`
		} `json:"data"`
	}
	if err := c.tool("actor_get", map[string]any{"actorId": r.ChartID, "fields": []string{"data"}}, &actor); err != nil {
		return err
	}
	var source struct {
		SourceType    string `json:"sourceType"`
		DynamicSource *struct {
			ID string `json:"id"`
		} `json:"dynamicSource"`
	}
	if err := json.Unmarshal([]byte(actor.Data.Source), &source); err != nil || source.SourceType != sourceType ||
		(sourceType == "actorFilter" && (source.DynamicSource == nil || source.DynamicSource.ID != r.FilterActorID)) {
		return fmt.Errorf("chart %s stores the source %q, want %s", r.ChartID, actor.Data.Source, sourceType)
	}
	var page struct {
		List []struct {
			LaID          int64 `json:"laId"`
			LayerSettings struct {
				ExpandType string  `json:"expandType"`
				Expand     bool    `json:"expand"`
				Width      float64 `json:"width"`
			} `json:"layerSettings"`
		} `json:"list"`
	}
	if err := c.tool("layer_placements_list", map[string]any{"actorId": layerID, "limit": 10}, &page); err != nil {
		return err
	}
	for _, p := range page.List {
		if p.LaID == r.LaID || r.LaID == 0 {
			if s := p.LayerSettings; s.ExpandType != "chart" || !s.Expand || s.Width != width {
				return fmt.Errorf("placement %d is drawn as %+v, want an open chart %v wide", p.LaID, s, width)
			}
			return nil
		}
	}
	return fmt.Errorf("layer %s has no placement %d", layerID, r.LaID)
}

// chartData reads the new manual bar chart's last day through chart_data_get: its one series (an
// account without movement, so 0) per day, and their total.
func chartData(c *client, chartID, actorID string) error {
	var shown struct {
		Data []struct {
			ActorID string  `json:"actorId"`
			Value   float64 `json:"value"`
		} `json:"data"`
		Total *struct {
			Value float64 `json:"value"`
		} `json:"total"`
	}
	to := time.Now().UnixMilli()
	if err := c.tool("chart_data_get", map[string]any{"chartId": chartID, "from": to - 86_400_000, "to": to, "interval": "hour",
		"with": []string{"series", "total"}}, &shown); err != nil {
		return err
	}
	if len(shown.Data) != 1 || shown.Data[0].ActorID != actorID || shown.Data[0].Value != 0 || shown.Total == nil || shown.Total.Value != 0 {
		return fmt.Errorf("chart_data_get of chart %s returned %+v, want one empty series of %s and a zero total", chartID, shown, actorID)
	}
	return nil
}
