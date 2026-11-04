package contracts

import _ "embed"

// CDUComponents is a compact projection of the sim-api CDU 2.0 component catalog
// (cdu-components-source.json pins it): every component type with its palette category, props
// (type, enum, description), event names and slots. A page config may only use these types and
// props; sim-api refuses anything else when a page file is saved.
//
//go:embed cdu-components.json
var CDUComponents []byte
