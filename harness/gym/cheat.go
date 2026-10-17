package gym

import (
	"fmt"
	"strings"

	"github.com/lox/clyde/harness/goal"
)

// CheatCommands returns console commands that satisfy g for player without
// playing. The cheat agent uses them to prove each checker can pass.
func CheatCommands(player string, g goal.Goal) ([]string, error) {
	kind, err := goal.Kind(g.Until)
	if err != nil {
		return nil, err
	}
	p := g.Until
	switch kind {
	case "items":
		if err := noTag(string(p.Items.Item)); err != nil {
			return nil, err
		}
		return []string{fmt.Sprintf("give %s %s %d", player, p.Items.Item, p.Items.Count)}, nil
	case "arrived":
		a := p.Arrived.At
		return []string{fmt.Sprintf("tp %s %g %g %g", player, a.X, a.Y, a.Z)}, nil
	case "built":
		var cmds []string
		for _, b := range p.Built.Blocks {
			if err := noTag(string(b.Block)); err != nil {
				return nil, err
			}
			cmds = append(cmds, fmt.Sprintf("setblock %d %d %d %s", b.Pos.X, b.Pos.Y, b.Pos.Z, b.Block))
		}
		return cmds, nil
	case "delivered":
		d := p.Delivered
		if err := noTag(string(d.Item)); err != nil {
			return nil, err
		}
		// Fill slots in stacks of up to 64; items with smaller stacks are
		// not supported.
		var cmds []string
		for slot, left := 0, d.Count; left > 0; slot++ {
			n := min(left, 64)
			cmds = append(cmds, fmt.Sprintf("item replace block %d %d %d container.%d with %s %d",
				d.Container.X, d.Container.Y, d.Container.Z, slot, d.Item, n))
			left -= n
		}
		return cmds, nil
	}
	return nil, goal.ErrJudged
}

func noTag(id string) error {
	if strings.HasPrefix(id, "#") {
		return fmt.Errorf("cheat cannot create %s: tags have no single item or block", id)
	}
	return nil
}

// CommandFailed reports whether a console reply is an error. RCON returns
// errors as ordinary reply text. "No blocks were filled" and "Could not set
// the block" mean nothing changed and are not errors.
func CommandFailed(reply string) bool {
	for _, s := range []string{
		"<--[HERE]",
		"Unknown ",
		"Incorrect argument",
		"That position is",
		"No player was found",
		"No entity was found",
		"is not a container",
	} {
		if strings.Contains(reply, s) {
			return true
		}
	}
	return false
}
