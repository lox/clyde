// Package goal evaluates a goal's until predicate against the world through
// an Observer. The gym scores tasks with it and the executor uses the same
// predicates as stop conditions.
package goal

import (
	"context"
	"errors"
	"fmt"
	"math"
)

// Observer answers the questions predicates need about the world.
type Observer interface {
	// ItemCount returns how many of item (an id or #tag) the player holds.
	// An offline player holds nothing.
	ItemCount(ctx context.Context, player string, item ItemID) (int, error)
	// Position returns the player's feet position; ok is false when the
	// player is not online.
	Position(ctx context.Context, player string) (pos Vec3, ok bool, err error)
	// BlockMatches reports whether the block at pos matches block.
	BlockMatches(ctx context.Context, pos BlockPos, block BlockID) (bool, error)
	// ContainerCount returns how many of item the container at pos holds.
	ContainerCount(ctx context.Context, pos BlockPos, item ItemID) (int, error)
}

// Result is one evaluation of a predicate.
type Result struct {
	Satisfied bool   `json:"satisfied"`
	Detail    string `json:"detail"`
}

// ErrJudged is returned for judged predicates, which need the judge.
var ErrJudged = errors.New("goal: judged predicates are not supported yet")

// Kind returns the name of the predicate's single kind, or an error unless
// exactly one kind is set.
func Kind(p Predicate) (string, error) {
	var kinds []string
	if p.Items != nil {
		kinds = append(kinds, "items")
	}
	if p.Arrived != nil {
		kinds = append(kinds, "arrived")
	}
	if p.Built != nil {
		kinds = append(kinds, "built")
	}
	if p.Delivered != nil {
		kinds = append(kinds, "delivered")
	}
	if p.Judged != nil {
		kinds = append(kinds, "judged")
	}
	if len(kinds) != 1 {
		return "", fmt.Errorf("goal: predicate must have exactly one kind, got %v", kinds)
	}
	return kinds[0], nil
}

// Evaluate checks p for player.
func Evaluate(ctx context.Context, obs Observer, player string, p Predicate) (Result, error) {
	kind, err := Kind(p)
	if err != nil {
		return Result{}, err
	}
	switch kind {
	case "items":
		n, err := obs.ItemCount(ctx, player, p.Items.Item)
		if err != nil {
			return Result{}, err
		}
		return Result{n >= p.Items.Count, fmt.Sprintf("%d/%d %s", n, p.Items.Count, p.Items.Item)}, nil

	case "arrived":
		pos, ok, err := obs.Position(ctx, player)
		if err != nil {
			return Result{}, err
		}
		if !ok {
			return Result{false, player + " is offline"}, nil
		}
		at := p.Arrived.At
		d := math.Sqrt(sq(pos.X-at.X) + sq(pos.Y-at.Y) + sq(pos.Z-at.Z))
		return Result{d <= p.Arrived.Radius, fmt.Sprintf("%.2f blocks away (radius %g)", d, p.Arrived.Radius)}, nil

	case "built":
		matched := 0
		for _, b := range p.Built.Blocks {
			ok, err := obs.BlockMatches(ctx, b.Pos, b.Block)
			if err != nil {
				return Result{}, err
			}
			if ok {
				matched++
			}
		}
		n := len(p.Built.Blocks)
		return Result{n > 0 && matched == n, fmt.Sprintf("%d/%d blocks match", matched, n)}, nil

	case "delivered":
		d := p.Delivered
		n, err := obs.ContainerCount(ctx, d.Container, d.Item)
		if err != nil {
			return Result{}, err
		}
		return Result{n >= d.Count, fmt.Sprintf("%d/%d %s in container at %d %d %d", n, d.Count, d.Item, d.Container.X, d.Container.Y, d.Container.Z)}, nil
	}
	return Result{}, ErrJudged
}

func sq(f float64) float64 { return f * f }
