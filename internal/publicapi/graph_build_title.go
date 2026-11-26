package publicapi

import (
	"context"
	"fmt"
	"strings"
)

// A repeated "build the org chart on a new layer" in a new turn comes with a new operationId, so
// the operation's own ref finds nothing and a second layer of the same title was made with the same
// nodes (eval build-01). Before a new layer is created, the layers the caller can see are searched
// for the title: one that exists is answered (existingLayerId) and nothing is built, unless the
// call sets allowDuplicateTitle.

// LayerTitleTaken says a layer of the title of a new layer exists already.
type LayerTitleTaken struct {
	Title   string
	LayerID string
	Count   int
}

func (e *LayerTitleTaken) Error() string {
	more := ""
	if e.Count > 1 {
		more = fmt.Sprintf(" (%d layers have it; this is the newest)", e.Count)
	}
	return fmt.Sprintf("a layer titled %q exists already: %s%s. Nothing was built. To build on it (a repeat, \"build it again\"), call again with layerId %s and existing/use for what is there; "+
		"for a separate layer give newLayer another title, or set allowDuplicateTitle true only when the person wants a second layer of the same title", e.Title, e.LayerID, more, e.LayerID)
}

// layerTitleSearch bounds the search: the first page of layers matching the title's words.
const layerTitleSearch = 50

// refuseTakenTitle answers a LayerTitleTaken when a layer the caller can see has this title (spaces
// and case aside); a failed search does not stop the build.
func (c *Client) refuseTakenTitle(ctx context.Context, formID int64, title string) error {
	want := strings.Join(strings.Fields(title), " ")
	page, err := call[actorSearchParams, struct {
		List []struct {
			ID        string `json:"id"`
			Title     string `json:"title"`
			CreatedAt int64  `json:"createdAt"`
		} `json:"list"`
	}](ctx, c, "actor/search/list", actorSearchParams{AccID: c.workspace(), Query: want, SearchType: "text", FormID: formID,
		Fields: []string{"id", "title", "createdAt"}, Limit: layerTitleSearch})
	if err != nil {
		return nil
	}
	taken := &LayerTitleTaken{Title: want}
	newest := int64(-1)
	for _, l := range page.List {
		if l.ID == "" || !strings.EqualFold(strings.Join(strings.Fields(l.Title), " "), want) {
			continue
		}
		taken.Count++
		if l.CreatedAt > newest {
			newest, taken.LayerID = l.CreatedAt, l.ID
		}
	}
	if taken.Count == 0 {
		return nil
	}
	return taken
}
