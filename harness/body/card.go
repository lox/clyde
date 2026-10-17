// Package body is a Go client for the body's HTTP API (tier 0). Its types
// are generated from api/state.schema.json and api/action.schema.json with
// `go generate ./body`.
package body

import "encoding/json"

// Card is a state card with action and lastAction typed. The generator
// turns the schema's "ActionStatus or null" into interface{}, so Card
// decodes those two fields itself.
type Card struct {
	StateCard
	Action     *ActionStatus `json:"action"`
	LastAction *ActionStatus `json:"lastAction"`
}

// UnmarshalJSON validates the card with the generated decoder, then decodes
// action and lastAction as *ActionStatus.
func (c *Card) UnmarshalJSON(b []byte) error {
	var sc StateCard
	if err := json.Unmarshal(b, &sc); err != nil {
		return err
	}
	var actions struct {
		Action     *ActionStatus `json:"action"`
		LastAction *ActionStatus `json:"lastAction"`
	}
	if err := json.Unmarshal(b, &actions); err != nil {
		return err
	}
	sc.Action, sc.LastAction = nil, nil
	*c = Card{StateCard: sc, Action: actions.Action, LastAction: actions.LastAction}
	return nil
}
