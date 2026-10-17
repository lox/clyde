package body

import (
	"context"
	"fmt"
	"strings"

	"github.com/lox/clyde/harness/goal"
)

// Observer answers goal predicates from the body's own state card, for
// when there is no RCON. It sees only what the bot sees: its own inventory
// and position. Built and delivered predicates need the RCON observer.
type Observer struct {
	Client *Client
}

var _ goal.Observer = Observer{}

// ItemCount counts item in the bot's inventory. player must be the bot.
func (o Observer) ItemCount(ctx context.Context, player string, item goal.ItemID) (int, error) {
	card, err := o.card(ctx, player)
	if err != nil {
		return 0, err
	}
	if strings.HasPrefix(string(item), "#") {
		return 0, fmt.Errorf("body observer: item tags such as %s need the RCON observer", item)
	}
	name := strings.TrimPrefix(string(item), "minecraft:")
	n := 0
	for _, it := range card.Inventory.Items {
		if it.Name == name {
			n += it.Count
		}
	}
	return n, nil
}

// Position returns the bot's position. player must be the bot.
func (o Observer) Position(ctx context.Context, player string) (goal.Vec3, bool, error) {
	card, err := o.card(ctx, player)
	if err != nil {
		return goal.Vec3{}, false, err
	}
	p := card.Bot.Position
	return goal.Vec3{X: p.X, Y: p.Y, Z: p.Z}, true, nil
}

func (o Observer) BlockMatches(context.Context, goal.BlockPos, goal.BlockID) (bool, error) {
	return false, fmt.Errorf("body observer: built predicates need the RCON observer")
}

func (o Observer) ContainerCount(context.Context, goal.BlockPos, goal.ItemID) (int, error) {
	return 0, fmt.Errorf("body observer: delivered predicates need the RCON observer")
}

func (o Observer) card(ctx context.Context, player string) (Card, error) {
	card, err := o.Client.State(ctx)
	if err != nil {
		return card, err
	}
	if card.Bot.Name != player {
		return card, fmt.Errorf("body observer: can only observe the bot %s, not %s", card.Bot.Name, player)
	}
	return card, nil
}
