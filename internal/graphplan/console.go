package graphplan

import (
	"encoding/json"

	"github.com/corezoid/simulator-ai/internal/publicapi"
)

// Block is one native flowchart block kind for the console profile. Pass FormID and Data to
// actor_create, then LayerSettings with the node placement, so Graph Editor draws the block shape.
// Data and LayerSettings are left out of a browse list without blockIds to keep it small.
type Block struct {
	FormID        int64                          `json:"formId"`
	Title         string                         `json:"title"`
	BlockID       string                         `json:"blockId"`
	Collection    string                         `json:"collection,omitempty"`
	Data          json.RawMessage                `json:"data,omitempty"`
	LayerSettings *publicapi.GraphNativeSettings `json:"layerSettings,omitempty"`
}

type Blocks struct {
	List []Block `json:"list"`
}

// ConsoleBlocks decodes catalog definitions; a malformed definition is skipped, never replaced
// by a generic shape.
func ConsoleBlocks(defs []publicapi.GraphBlock, full bool) Blocks {
	out := Blocks{List: []Block{}}
	for _, def := range defs {
		data, settings, err := decodeDefinition(def)
		if err != nil {
			continue
		}
		b := Block{FormID: def.ParentID, Title: def.Title, BlockID: def.BlockID, Collection: def.Collection}
		if full {
			b.Data, b.LayerSettings = data, &settings
		}
		out.List = append(out.List, b)
	}
	return out
}
