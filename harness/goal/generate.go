package goal

//go:generate go tool go-jsonschema -p goal --capitalization ID --tags json --schema-root-type https://github.com/lox/clyde/api/goal.schema.json=Goal -o goal.gen.go ../../api/goal.schema.json
