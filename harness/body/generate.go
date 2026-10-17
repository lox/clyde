package body

//go:generate go tool go-jsonschema -p body --capitalization ID --tags json --schema-root-type state.schema.json=StateCard -o state.gen.go ../../api/state.schema.json
//go:generate go tool go-jsonschema -p body --capitalization ID --tags json --schema-root-type action.schema.json=ActionRequest -o action.gen.go ../../api/action.schema.json
//go:generate cp ../../api/action.schema.json action.schema.json
