// Package contracts contains the pinned public contract used by the Smart Forms profile.
package contracts

import _ "embed"

// SmartForms is a projection of the public OpenRPC, not an arbitrary RPC allow-list.
//
//go:embed smartforms-openrpc.json
var SmartForms []byte
