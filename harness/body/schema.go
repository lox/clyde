package body

import _ "embed"

// ActionSchema is a copy of api/action.schema.json, refreshed by go generate.
//
//go:embed action.schema.json
var ActionSchema []byte
