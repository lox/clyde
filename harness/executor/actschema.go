package executor

import (
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/lox/clyde/harness/body"
)

// actSchema and actDescription flatten api/action.schema.json into one
// object, because tool schemas must be a single object without a top-level
// oneOf. act validates each request against its own type before sending it.
var actSchema, actDescription = flattenActions(body.ActionSchema)

func flattenActions(schemaJSON []byte) (json.RawMessage, string) {
	var s struct {
		Definitions map[string]struct {
			Description string                    `json:"description"`
			Required    []string                  `json:"required"`
			Properties  map[string]map[string]any `json:"properties"`
		} `json:"definitions"`
	}
	if err := json.Unmarshal(schemaJSON, &s); err != nil {
		panic(fmt.Sprintf("action schema: %v", err))
	}
	type field struct {
		typ   string
		users []string
	}
	fields := map[string]*field{}
	var types, lines []string
	for _, def := range s.Definitions {
		typ, _ := def.Properties["type"]["const"].(string)
		types = append(types, typ)
		var params []string
		for name, p := range def.Properties {
			if name == "type" {
				continue
			}
			t, _ := p["type"].(string)
			f := fields[name]
			if f == nil {
				f = &field{typ: t}
				fields[name] = f
			} else if f.typ != t {
				f.typ = "number" // integer in one type, number in another
			}
			f.users = append(f.users, typ)
			param := name
			if !slices.Contains(def.Required, name) {
				param += "?"
				if d, ok := p["default"]; ok {
					param += fmt.Sprintf("=%v", d)
				}
			}
			params = append(params, param)
		}
		sort.Strings(params)
		desc := def.Description
		if typ == "stop" {
			desc = "Abort the running action and stand still."
		}
		lines = append(lines, fmt.Sprintf("- %s(%s): %s", typ, strings.Join(params, ", "), desc))
	}
	sort.Strings(types)
	sort.Strings(lines)

	props := map[string]any{
		"type": map[string]any{"type": "string", "enum": types},
		"wait": map[string]any{"type": "number", "minimum": 0, "maximum": maxActWait,
			"description": fmt.Sprintf("Seconds to wait for the action to finish before returning. Default %g.", defaultActWait)},
	}
	for name, f := range fields {
		sort.Strings(f.users)
		props[name] = map[string]any{"type": f.typ, "description": "For " + strings.Join(f.users, ", ") + "."}
	}
	b, err := json.Marshal(map[string]any{"type": "object", "properties": props, "required": []string{"type"}})
	if err != nil {
		panic(err)
	}
	desc := "Start one body action. It replaces any running action. Waits until the action finishes or wait seconds pass, " +
		"then returns its final status (result or error) or its progress so far. Actions (? marks optional fields):\n" +
		strings.Join(lines, "\n")
	return b, desc
}
