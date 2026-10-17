// Command gomclib is a throwaway spike: a go-mclib client that joins a local
// Paper server, walks to a point, digs a block, collects a log, and chats,
// timing each step and counting server position corrections (rubber-banding).
// See docs/plans/spike-go-mclib.md on main for the findings.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"runtime"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/go-mclib/client/pkg/client"
	"github.com/go-mclib/client/pkg/client/modules/chat"
	"github.com/go-mclib/client/pkg/client/modules/collisions"
	"github.com/go-mclib/client/pkg/client/modules/entities"
	"github.com/go-mclib/client/pkg/client/modules/inventory"
	"github.com/go-mclib/client/pkg/client/modules/pathfinding"
	"github.com/go-mclib/client/pkg/client/modules/physics"
	"github.com/go-mclib/client/pkg/client/modules/playerlist"
	"github.com/go-mclib/client/pkg/client/modules/protocol"
	"github.com/go-mclib/client/pkg/client/modules/self"
	"github.com/go-mclib/client/pkg/client/modules/world"
	"github.com/go-mclib/data/pkg/data/blocks"
	dataEntities "github.com/go-mclib/data/pkg/data/entities"
	"github.com/go-mclib/data/pkg/data/items"
	"github.com/go-mclib/data/pkg/data/packet_ids"
	"github.com/go-mclib/data/pkg/packets"
	jp "github.com/go-mclib/protocol/java_protocol"
	ns "github.com/go-mclib/protocol/java_protocol/net_structures"

	"github.com/lox/clyde/harness/rcon"
)

type step struct {
	Name    string  `json:"name"`
	OK      bool    `json:"ok"`
	Seconds float64 `json:"seconds"`
	Detail  string  `json:"detail"`
}

type report struct {
	JoinSeconds  float64 `json:"joinSeconds"`
	Steps        []step  `json:"steps"`
	Corrections  int64   `json:"serverPositionCorrections"`
	Disconnect   string  `json:"disconnect,omitempty"`
	ServerLogs   string  `json:"serverInventory,omitempty"`
	WalkedMetres float64 `json:"walkedMetres"`
	TotalSeconds float64 `json:"totalSeconds"`
}

type bot struct {
	c    *client.Client
	s    *self.Module
	w    *world.Module
	col  *collisions.Module
	ents *entities.Module
	inv  *inventory.Module
	pf   *pathfinding.Module
	ch   *chat.Module

	navDone  chan bool
	blockUpd chan [4]int
	spawned  chan struct{}

	corrections atomic.Int64
	walked      float64
	rep         report
}

func main() {
	addr := flag.String("s", "127.0.0.1:25566", "server address")
	name := flag.String("u", "Clyde", "offline username")
	rconAddr := flag.String("rcon", "127.0.0.1:25576", "RCON address for server-side checks")
	rconPass := flag.String("rcon-password", "spike", "RCON password")
	walks := flag.Int("walks", 4, "extra waypoint walks for the rubber-band soak")
	fixture := flag.Bool("fixture", false, "prepare an isolated survival platform using RCON")
	wall := flag.Bool("wall", false, "after collecting, push against and slide along a fixture wall")
	flag.Parse()

	if *fixture {
		commands(*rconAddr, *rconPass, "forceload add -32 -32 48 48")
		time.Sleep(5 * time.Second)
		commands(*rconAddr, *rconPass,
			"difficulty peaceful", "time set day",
			"kill @e[type=item]",
			"fill -32 160 -32 48 160 48 grass_block",
			"fill -32 161 -32 48 164 48 air",
			"setblock 17 161 8 oak_log",
			"setworldspawn 0 161 0")
	}
	b := newBot(*addr, *name)
	b.c.Verbose = os.Getenv("SPIKE_VERBOSE") != ""
	go memoryWatchdog(512 << 20)
	start := time.Now()
	connectionDone := make(chan error, 1)
	go func() {
		connectionDone <- b.c.ConnectAndStart(context.Background())
	}()

	select {
	case <-b.spawned:
	case err := <-connectionDone:
		log.Fatalf("connection failed before spawn: %v", err)
	case <-time.After(30 * time.Second):
		log.Fatal("no spawn after 30s")
	}
	b.rep.JoinSeconds = secs(time.Since(start))
	if *fixture {
		commands(*rconAddr, *rconPass, "gamemode survival "+*name,
			"clear "+*name, "tp "+*name+" 0.5 161 0.5")
		time.Sleep(2 * time.Second)
	}
	b.waitForChunks()
	b.corrections.Store(0)
	b.script(*walks)
	if *wall {
		commands(*rconAddr, *rconPass, "fill 20 161 -2 20 163 20 stone",
			"tp "+*name+" 17.5 161 0.5")
		time.Sleep(time.Second)
		b.corrections.Store(0)
		b.run("wall_slide", func() (string, error) {
			p := physics.From(b.c)
			b.s.SetRotation(-90, 0)
			p.SetInput(1, 0, false)
			time.Sleep(2 * time.Second)
			x1, y1, z1 := b.s.Position()
			b.s.SetRotation(0, 0)
			time.Sleep(3 * time.Second)
			p.SetInput(0, 0, false)
			x2, y2, z2 := b.s.Position()
			return fmt.Sprintf("wall contact %.5f %.2f %.2f -> slide %.5f %.2f %.2f; corrections=%d", x1, y1, z1, x2, y2, z2, b.corrections.Load()), nil
		})
	}
	select {
	case err := <-connectionDone:
		b.rep.Disconnect = fmt.Sprint(err)
	default:
	}
	b.finish(start, *rconAddr, *rconPass, *name)
	_ = b.c.Disconnect(true)
	for _, st := range b.rep.Steps {
		if !st.OK {
			os.Exit(1)
		}
	}
}

func newBot(addr, name string) *bot {
	c := client.New(addr, name, false)
	c.MaxReconnectAttempts = 0
	c.Logger = log.New(os.Stderr, "[mc] ", log.Ltime|log.Lmicroseconds)
	if dir := os.Getenv("SPIKE_DUMP"); dir != "" {
		c.Register(&dumper{c: c, dir: dir})
	}
	c.Register(protocol.New())
	c.Register(self.New())
	c.Register(world.New())
	c.Register(chat.New())
	c.Register(playerlist.New())
	c.Register(collisions.New())
	c.Register(physics.New())
	c.Register(entities.New())
	c.Register(inventory.New())
	c.Register(pathfinding.New())

	b := &bot{
		c: c, s: self.From(c), w: world.From(c), col: collisions.From(c),
		ents: entities.From(c), inv: inventory.From(c), pf: pathfinding.From(c), ch: chat.From(c),
		navDone:  make(chan bool, 1),
		blockUpd: make(chan [4]int, 64),
		spawned:  make(chan struct{}),
	}
	var once atomic.Bool
	b.s.OnSpawn(func() {
		if once.CompareAndSwap(false, true) {
			close(b.spawned)
		}
	})
	// Called with the pathfinder's lock held: never call back into it here.
	b.pf.OnNavigationComplete(func(reached bool) {
		select {
		case b.navDone <- reached:
		default:
		}
	})
	b.w.OnBlockUpdate(func(x, y, z int, state int32) {
		select {
		case b.blockUpd <- [4]int{x, y, z, int(state)}:
		default:
		}
	})
	b.ch.OnSystemChat(func(msg string, overlay bool) {
		if !overlay {
			log.Printf("system chat: %s", msg)
		}
	})
	// A Player Position packet after spawn is the server correcting us.
	c.RegisterHandler(func(c *client.Client, pkt *jp.WirePacket) {
		if c.State() != jp.StatePlay {
			return
		}
		switch pkt.PacketID {
		case packet_ids.S2CSetEntityDataID:
			var d packets.S2CSetEntityData
			if err := pkt.ReadInto(&d); err != nil {
				log.Printf("metadata decode error: %v", err)
			} else if raw := d.Metadata.Get(dataEntities.ItemEntityIndexItem); raw != nil {
				it, err := items.ReadSlot(ns.NewReader(raw))
				log.Printf("item metadata entity=%d item=%v decode=%v", d.EntityId, it, err)
			}
		case packet_ids.S2CAddEntityID:
			var d packets.S2CAddEntity
			err := pkt.ReadInto(&d)
			log.Printf("add entity id=%d type=%d at %.2f %.2f %.2f decode=%v", d.EntityId, d.Type, d.X, d.Y, d.Z, err)
		case packet_ids.S2CPlayerPositionID:
			select {
			case <-b.spawned:
				n := b.corrections.Add(1)
				var d packets.S2CPlayerPosition
				_ = pkt.ReadInto(&d)
				log.Printf("server position correction #%d to %.2f %.2f %.2f", n, d.X, d.Y, d.Z)
			default:
			}
		case packet_ids.S2CDisconnectPlayID:
			log.Printf("disconnect packet received")
		}
	})
	return b
}

func (b *bot) waitForChunks() {
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		x, _, z := b.s.Position()
		cx, cz := int32(math.Floor(x))>>4, int32(math.Floor(z))>>4
		if b.w.GetLoadedChunkCount() >= 50 && b.w.IsChunkLoaded(cx, cz) && physics.From(b.c).IsOnGround() {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	log.Printf("chunks not settled after 20s (%d loaded)", b.w.GetLoadedChunkCount())
}

func (b *bot) script(walks int) {
	x, y, z := b.s.Position()
	log.Printf("spawned at %.1f %.1f %.1f, %d chunks", x, y, z, b.w.GetLoadedChunkCount())

	b.run("chat", func() (string, error) {
		return "sent", b.ch.SendMessage("hello from go-mclib")
	})

	b.run("goto", func() (string, error) {
		tx, ty, tz, ok := b.surfaceNear(int(x)+12, int(z)+8, 6)
		if !ok {
			return "", fmt.Errorf("no standable surface near target")
		}
		return b.goTo(float64(tx)+0.5, float64(ty), float64(tz)+0.5, 60*time.Second)
	})

	b.run("dig", func() (string, error) {
		return b.digAdjacentGround()
	})

	b.run("collect_log", func() (string, error) {
		return b.collectLog()
	})

	for i := 0; i < walks; i++ {
		angle := float64(i) * math.Pi / 2
		b.run(fmt.Sprintf("walk_%d", i+1), func() (string, error) {
			px, _, pz := b.s.Position()
			tx, ty, tz, ok := b.surfaceNear(int(px+20*math.Cos(angle)), int(pz+20*math.Sin(angle)), 8)
			if !ok {
				return "", fmt.Errorf("no standable surface near waypoint")
			}
			return b.goTo(float64(tx)+0.5, float64(ty), float64(tz)+0.5, 60*time.Second)
		})
	}

	b.run("chat_done", func() (string, error) {
		return "sent", b.ch.SendMessage(fmt.Sprintf("spike done: %d corrections", b.corrections.Load()))
	})
}

func (b *bot) run(name string, fn func() (string, error)) {
	start := time.Now()
	detail, err := fn()
	st := step{Name: name, OK: err == nil, Seconds: secs(time.Since(start)), Detail: detail}
	if err != nil {
		st.Detail = strings.TrimSpace(detail + " " + err.Error())
	}
	log.Printf("step %s ok=%v %.2fs %s", name, st.OK, st.Seconds, st.Detail)
	b.rep.Steps = append(b.rep.Steps, st)
}

// surfaceNear finds a standable block column near (x, z): air for the body,
// something solid underneath, searching outwards up to r blocks.
func (b *bot) surfaceNear(x, z, r int) (int, int, int, bool) {
	for d := 0; d <= r; d++ {
		for dx := -d; dx <= d; dx++ {
			for dz := -d; dz <= d; dz++ {
				if max(abs(dx), abs(dz)) != d {
					continue
				}
				if y, ok := b.surfaceY(x+dx, z+dz); ok {
					return x + dx, y, z + dz, true
				}
			}
		}
	}
	return 0, 0, 0, false
}

func (b *bot) surfaceY(x, z int) (int, bool) {
	for y := 200; y > -60; y-- {
		state := b.w.GetBlock(x, y-1, z)
		if state == 0 {
			continue
		}
		n := blockName(state)
		if strings.Contains(n, "leaves") || strings.Contains(n, "water") || strings.Contains(n, "lava") {
			return 0, false
		}
		if _, ok := b.col.SupportSurfaceY(x, y, z, 0.6); ok && b.col.CanFitAt(float64(x)+0.5, float64(y), float64(z)+0.5, 0.6, 1.8) {
			return y, true
		}
	}
	return 0, false
}

func (b *bot) goTo(x, y, z float64, timeout time.Duration) (string, error) {
	sx, sy, sz := b.s.Position()
	select {
	case <-b.navDone:
	default:
	}
	pathStart := time.Now()
	path, err := b.pf.FindPath(x, y, z)
	if err != nil {
		return "", fmt.Errorf("find path: %w", err)
	}
	planned := time.Since(pathStart)
	if err := b.pf.NavigateTo(x, y, z); err != nil {
		return "", fmt.Errorf("navigate: %w", err)
	}
	var reached bool
	select {
	case reached = <-b.navDone:
	case <-time.After(timeout):
		b.pf.Stop()
		return "", fmt.Errorf("timeout after %s", timeout)
	}
	ex, ey, ez := b.s.Position()
	moved := dist(sx, sy, sz, ex, ey, ez)
	b.walked += moved
	detail := fmt.Sprintf("to %.1f,%.1f,%.1f; path %d nodes planned in %dms; moved %.1fm; %.2fm from goal",
		x, y, z, len(path), planned.Milliseconds(), moved, dist(ex, ey, ez, x, y, z))
	if !reached {
		return detail, fmt.Errorf("navigation reported not reached")
	}
	return detail, nil
}

// digAdjacentGround digs the ground block one step east (or another side) of the bot.
func (b *bot) digAdjacentGround() (string, error) {
	x, y, z := b.s.Position()
	bx, by, bz := int(math.Floor(x)), int(math.Floor(y))-1, int(math.Floor(z))
	for _, off := range [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
		tx, tz := bx+off[0], bz+off[1]
		n := blockName(b.w.GetBlock(tx, by, tz))
		if n == "minecraft:grass_block" || n == "minecraft:dirt" || n == "minecraft:sand" {
			return b.dig(tx, by, tz)
		}
	}
	return "", fmt.Errorf("no dirt, grass, or sand next to the bot")
}

// dig breaks a block bare-handed, waiting the vanilla break time, and waits
// for the server's block update.
func (b *bot) dig(x, y, z int) (string, error) {
	state := b.w.GetBlock(x, y, z)
	name := blockName(state)
	hardness := blocks.BlockHardness(strings.TrimPrefix(name, "minecraft:"))
	if hardness < 0 {
		hardness = blocks.BlockHardness(name)
	}
	// Bare hand, block harvestable without a tool: damage per tick is
	// 1/hardness/30, so ticks = ceil(30*hardness). Tool and harvest rules
	// are what a real body would add.
	ticks := int(math.Ceil(30 * float64(hardness)))
	b.s.LookAt(float64(x)+0.5, float64(y)+0.5, float64(z)+0.5)
	time.Sleep(100 * time.Millisecond)
	face := b.faceToward(x, y, z)
	drain(b.blockUpd)
	if err := b.c.BreakBlock(x, y, z, face, true); err != nil {
		return "", err
	}
	deadline := time.Now().Add(time.Duration(ticks)*50*time.Millisecond + 50*time.Millisecond)
	for time.Now().Before(deadline) {
		_ = b.c.SwingArm(0)
		time.Sleep(250 * time.Millisecond)
	}
	if err := b.c.BreakBlock(x, y, z, face, false); err != nil {
		return "", err
	}
	timeout := time.After(3 * time.Second)
	for {
		select {
		case u := <-b.blockUpd:
			if u[0] == x && u[1] == y && u[2] == z {
				after := blockName(int32(u[3]))
				detail := fmt.Sprintf("%s at %d,%d,%d (hardness %.1f, %d ticks) -> %s", name, x, y, z, hardness, ticks, after)
				if strings.HasSuffix(after, "air") {
					return detail, nil
				}
				return detail, fmt.Errorf("server kept the block")
			}
		case <-timeout:
			after := blockName(b.w.GetBlock(x, y, z))
			return fmt.Sprintf("%s at %d,%d,%d, now %s", name, x, y, z, after), fmt.Errorf("no block update within 3s")
		}
	}
}

func (b *bot) faceToward(x, y, z int) int8 {
	ex, ey, ez := b.s.Position()
	ey += 1.62
	dx, dy, dz := ex-(float64(x)+0.5), ey-(float64(y)+0.5), ez-(float64(z)+0.5)
	switch {
	case math.Abs(dy) >= math.Abs(dx) && math.Abs(dy) >= math.Abs(dz):
		if dy > 0 {
			return 1 // top
		}
		return 0
	case math.Abs(dx) >= math.Abs(dz):
		if dx > 0 {
			return 5 // east
		}
		return 4
	default:
		if dz > 0 {
			return 3 // south
		}
		return 2
	}
}

// collectLog finds the nearest reachable log, walks to it, digs it, and walks
// onto the drop until a log is in the inventory.
func (b *bot) collectLog() (string, error) {
	var logIDs []int32
	for _, wood := range []string{"oak", "birch", "spruce", "jungle", "acacia", "dark_oak", "mangrove", "cherry", "pale_oak"} {
		if id := blocks.BlockID("minecraft:" + wood + "_log"); id >= 0 {
			logIDs = append(logIDs, id)
		}
	}
	x, y, z := b.s.Position()
	var found [][3]int
	b.w.FindBlocks(logIDs, func(bx, by, bz int, _ int32) bool {
		if dist(x, y, z, float64(bx), float64(by), float64(bz)) <= 48 {
			found = append(found, [3]int{bx, by, bz})
		}
		return true
	})
	if len(found) == 0 {
		return "", fmt.Errorf("no logs within 48 blocks")
	}
	sort.Slice(found, func(i, j int) bool {
		a, c := found[i], found[j]
		return dist(x, y, z, float64(a[0]), float64(a[1]), float64(a[2])) < dist(x, y, z, float64(c[0]), float64(c[1]), float64(c[2]))
	})
	before := b.logCount()
	tried := 0
	for _, t := range found {
		if tried >= 5 {
			break
		}
		px, py, pz := b.s.Position()
		sx, sy, sz, ok := pathfinding.FindReachablePosition(b.col, px, py, pz, t[0], t[1], t[2], 4.0)
		if !ok {
			continue
		}
		tried++
		walk, err := b.goTo(float64(sx)+0.5, float64(sy), float64(sz)+0.5, 45*time.Second)
		if err != nil {
			log.Printf("log at %v: %s %v", t, walk, err)
			continue
		}
		dug, err := b.dig(t[0], t[1], t[2])
		if err != nil {
			log.Printf("log at %v: %s %v", t, dug, err)
			continue
		}
		pickup, err := b.pickUpNear(t, before)
		detail := fmt.Sprintf("%s; %s; %s; inventory logs %d -> %d", walk, dug, pickup, before, b.logCount())
		return detail, err
	}
	return "", fmt.Errorf("none of the %d nearest logs was reachable and diggable (%d tried)", min(len(found), 5), tried)
}

// pickUpNear waits for the dropped item and walks onto it until a log arrives.
func (b *bot) pickUpNear(t [3]int, before int) (string, error) {
	// Let the new item settle before choosing a standable pickup position.
	time.Sleep(800 * time.Millisecond)
	deadline := time.Now().Add(15 * time.Second)
	walked := false
	for time.Now().Before(deadline) {
		if n := b.logCount(); n > before {
			return fmt.Sprintf("picked up (walked to drop: %v)", walked), nil
		}
		var drop *entities.Entity
		for _, e := range b.ents.GetNearbyEntities(float64(t[0])+0.5, float64(t[1]), float64(t[2])+0.5, 8) {
			if e.TypeName == "minecraft:item" || e.TypeName == "item" {
				if raw := e.Metadata.Get(dataEntities.ItemEntityIndexItem); raw != nil {
					it, err := items.ReadSlot(ns.NewReader(raw))
					if err == nil && it != nil && strings.HasSuffix(items.ItemName(it.ID), "_log") {
						drop = e
						break
					}
				}
			}
		}
		if drop != nil && !walked {
			walked = true
			sx, sy, sz, ok := b.surfaceNear(int(math.Floor(drop.X)), int(math.Floor(drop.Z)), 2)
			if ok {
				detail, err := b.goTo(float64(sx)+0.5, float64(sy), float64(sz)+0.5, 15*time.Second)
				log.Printf("pickup walk: %s error=%v", detail, err)
			} else {
				log.Printf("pickup has no standable surface at %.2f %.2f", drop.X, drop.Z)
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	return "", fmt.Errorf("no log in inventory 15s after digging")
}

func (b *bot) logCount() int {
	n := 0
	for i := 0; i < inventory.TotalSlots; i++ {
		if it := b.inv.GetSlot(i); it != nil && !it.IsEmpty() && strings.HasSuffix(items.ItemName(it.ID), "_log") {
			n += int(it.Count)
		}
	}
	return n
}

func (b *bot) finish(start time.Time, rconAddr, rconPass, name string) {
	b.rep.Corrections = b.corrections.Load()
	b.rep.WalkedMetres = math.Round(b.walked*10) / 10
	b.rep.TotalSeconds = secs(time.Since(start))
	if rc, err := rcon.Dial(context.Background(), rconAddr, rconPass); err == nil {
		out, _ := rc.Exec("clear " + name + " #minecraft:logs 0")
		b.rep.ServerLogs = out
		_ = rc.Close()
	} else {
		b.rep.ServerLogs = "rcon: " + err.Error()
	}
	out, _ := json.MarshalIndent(b.rep, "", "  ")
	fmt.Println(string(out))
}

func commands(addr, password string, cmds ...string) {
	rc, err := rcon.Dial(context.Background(), addr, password)
	if err != nil {
		log.Fatal(err)
	}
	defer rc.Close()
	for _, cmd := range cmds {
		out, err := rc.Exec(cmd)
		log.Printf("RCON %s: %s (%v)", cmd, out, err)
		if err != nil || strings.Contains(out, "not loaded") {
			log.Fatal("fixture command failed")
		}
	}
}

// dumper writes every configuration-state packet to dir, for debugging
// decoding failures offline. Registered before the protocol module.
type dumper struct {
	c   *client.Client
	dir string
	n   int
}

func (d *dumper) Name() string        { return "spike-dumper" }
func (d *dumper) Init(*client.Client) {}
func (d *dumper) Reset()              {}
func (d *dumper) HandlePacket(pkt *jp.WirePacket) {
	if d.c.State() != jp.StateConfiguration {
		return
	}
	d.n++
	_ = os.WriteFile(fmt.Sprintf("%s/config-%03d-id%02x.bin", d.dir, d.n, int(pkt.PacketID)), pkt.Data, 0o644)
}

// memoryWatchdog dumps every goroutine's stack and exits when the heap passes
// limit, so a runaway allocation is caught before the OOM killer.
func memoryWatchdog(limit uint64) {
	var ms runtime.MemStats
	for range time.Tick(20 * time.Millisecond) {
		runtime.ReadMemStats(&ms)
		if ms.HeapAlloc > limit {
			buf := make([]byte, 1<<20)
			n := runtime.Stack(buf, true)
			fmt.Fprintf(os.Stderr, "heap %d MiB over limit; goroutines:\n%s\n", ms.HeapAlloc>>20, buf[:n])
			os.Exit(3)
		}
	}
}

func blockName(state int32) string {
	id, _ := blocks.StateProperties(int(state))
	return blocks.BlockName(id)
}

func drain[T any](ch chan T) {
	for {
		select {
		case <-ch:
		default:
			return
		}
	}
}

func dist(ax, ay, az, bx, by, bz float64) float64 {
	return math.Sqrt((ax-bx)*(ax-bx) + (ay-by)*(ay-by) + (az-bz)*(az-bz))
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func secs(d time.Duration) float64 { return math.Round(d.Seconds()*100) / 100 }
