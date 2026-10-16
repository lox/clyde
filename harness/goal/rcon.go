package goal

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Commander runs one server console command and returns its reply text.
// *rcon.Client implements it.
type Commander interface {
	Exec(command string) (string, error)
}

// RCON is an Observer backed by the server console. It reads the server's
// truth rather than a client's view and needs no player slot.
type RCON struct {
	Conn Commander
}

var (
	playerName  = regexp.MustCompile(`^[A-Za-z0-9_]{1,16}$`)
	foundItems  = regexp.MustCompile(`^Found (\d+) matching item`)
	entityPos   = regexp.MustCompile(`has the following entity data: \[(\S+)d, (\S+)d, (\S+)d\]`)
	passedCount = regexp.MustCompile(`^Test passed\. Count: (\d+)`)
)

// ItemCount uses `clear <player> <item> 0`, which counts without removing.
func (r RCON) ItemCount(ctx context.Context, player string, item ItemID) (int, error) {
	if err := checkPlayer(player); err != nil {
		return 0, err
	}
	out, err := r.run(ctx, "clear", player, string(item), "0")
	if err != nil {
		return 0, err
	}
	if m := foundItems.FindStringSubmatch(out); m != nil {
		return strconv.Atoi(m[1])
	}
	if strings.HasPrefix(out, "No items were found") || out == "No player was found" {
		return 0, nil
	}
	return 0, unexpected(out)
}

// Position uses `data get entity <player> Pos`.
func (r RCON) Position(ctx context.Context, player string) (Vec3, bool, error) {
	if err := checkPlayer(player); err != nil {
		return Vec3{}, false, err
	}
	out, err := r.run(ctx, "data", "get", "entity", player, "Pos")
	if err != nil {
		return Vec3{}, false, err
	}
	if out == "No entity was found" {
		return Vec3{}, false, nil
	}
	m := entityPos.FindStringSubmatch(out)
	if m == nil {
		return Vec3{}, false, unexpected(out)
	}
	var v [3]float64
	for i := range v {
		if v[i], err = strconv.ParseFloat(m[i+1], 64); err != nil {
			return Vec3{}, false, unexpected(out)
		}
	}
	return Vec3{X: v[0], Y: v[1], Z: v[2]}, true, nil
}

// BlockMatches uses `execute if block`.
func (r RCON) BlockMatches(ctx context.Context, pos BlockPos, block BlockID) (bool, error) {
	out, err := r.run(ctx, append(append([]string{"execute", "if", "block"}, xyz(pos)...), string(block))...)
	if err != nil {
		return false, err
	}
	switch out {
	case "Test passed":
		return true, nil
	case "Test failed":
		return false, nil
	}
	return false, unexpected(out)
}

// ContainerCount uses `execute if items block <pos> container.*`, which
// reports the matching count.
func (r RCON) ContainerCount(ctx context.Context, pos BlockPos, item ItemID) (int, error) {
	out, err := r.run(ctx, append(append([]string{"execute", "if", "items", "block"}, xyz(pos)...), "container.*", string(item))...)
	if err != nil {
		return 0, err
	}
	if m := passedCount.FindStringSubmatch(out); m != nil {
		return strconv.Atoi(m[1])
	}
	if out == "Test failed" {
		return 0, nil
	}
	return 0, unexpected(out)
}

// run joins words into one command. Words come from goal files, so it
// refuses anything that could smuggle in extra command words.
func (r RCON) run(ctx context.Context, words ...string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	for _, w := range words {
		if w == "" || strings.ContainsAny(w, " \t\r\n") {
			return "", fmt.Errorf("goal: invalid command word %q", w)
		}
	}
	return r.Conn.Exec(strings.Join(words, " "))
}

func checkPlayer(player string) error {
	if !playerName.MatchString(player) {
		return fmt.Errorf("goal: invalid player name %q", player)
	}
	return nil
}

func xyz(p BlockPos) []string {
	return []string{strconv.Itoa(p.X), strconv.Itoa(p.Y), strconv.Itoa(p.Z)}
}

func unexpected(out string) error {
	return fmt.Errorf("goal: unexpected server reply %q", out)
}
