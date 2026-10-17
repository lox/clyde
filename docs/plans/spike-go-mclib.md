# Spike: a Go body on go-mclib

Investigated 2026-10-17, before slice 5. **Recommendation: keep the TypeScript
mineflayer body and JavaScript skills for now.** The recovered go-mclib stack
is not a practical drop-in replacement: migrating on that stack means owning
client repairs, packet/data updates, physics fixes, and a new skill API, not
translating Clyde's small HTTP wrapper into Go. This is **not a finding that
all Go clients require the same work**. Supplemental research identifies
**mc26-kit as the strongest future candidate**. The follow-on Paper experiment
below now verifies useful gameplay, but also default-width movement failures
and an example stop-command gap; it does not justify changing slices 5/6.

The hands-on spike did join Paper 26.2, walk, dig, collect a log and chat.
That required dependency repair and a registry-decoding patch. Its physics
also needed the same wall-width workaround as mineflayer; race detection
found additional concurrency bugs. This is enough to demonstrate feasibility,
not enough to justify delaying skills by an estimated 4–8 engineer-weeks
to reach parity on the **recovered go-mclib stack**. That estimate does not
apply to mc26-kit, MineGo, or other Go clients without their own evaluation.

Prototype and transcripts: [spike/go-mclib](https://github.com/lox/clyde/tree/spike/go-mclib/spike/gomclib).
Only this document lands on main. No body, harness, gym, or production changes
are part of this spike. No PR or Auto-review is required for this research doc.

## What was actually available

The spec thread's report is historical, not a current dependency guarantee.
On the investigation date, all three GitHub repository pages, GitHub API
lookups and a client clone returned **404 / repository not found**:

- `github.com/go-mclib/client`
- `github.com/go-mclib/protocol`
- `github.com/go-mclib/data`

The public organization API listed only `nbt` and `go-schematic`. A 404 does
not establish whether the repositories were deleted, renamed, or made
private. It does establish that we cannot currently rely on their public
history, issues, reviews, tags, or future releases. No conclusion about the
maintainer's intent follows from it.

Fortunately, `proxy.golang.org` still serves immutable module source archives:

| Module | Recoverable revision / timestamp (UTC) | Source Go files / lines | Test files / lines |
|--------|-----------------------------------------|-------------------------|--------------------|
| client | `v0.0.0-20260626124012-de01d7956dc3`, June 26 | 45 / 10,341 | 1 / 94 |
| protocol | `v0.0.0-20260627053125-6905af045007`, June 27 | 42 / 9,043 | 24 / 3,838 |
| data | `v0.0.0-20260626135928-f4a527b8705b`, June 26 | 84 / 77,817 | 11 / 1,332 |

Counts include examples and generators, exclude `_test.go` from source counts,
and are not a claim that 97,201 lines were handwritten. Most data is generated.
The client has 9,172 lines under `pkg/`, including its test. All three modules
declare Go 1.25; the prototype uses Go 1.27 because it reuses Clyde's RCON helper.

All three archived modules include **MIT licenses**. Client/protocol copyright
2025 Kevin Svitač; data copyright 2026 go-mclib contributors. Depending on,
copying, modifying and redistributing them is permitted with the copyright
and permission notices retained. This does not license Mojang's server or
decompiled source; do not redistribute those as part of a fork.

The archived protocol README explicitly names `@SKevo18` as its sole maintainer,
says spare-time maintenance is not guaranteed, and says it is unstable and
not production-ready. Thus **single-maintainer and WIP are verified as the
project's own published statements**, not a fresh contributor-count audit.
With history unavailable, total commits, recent commit frequency, issue
response times and present bus factor cannot be independently measured.
The latest proxy-cached revisions above are about four months old, not proof
that private development stopped then. The public `nbt` repository has a
September 17 commit by SKevo18 titled “26.3 update”; that is evidence of activity
on NBT, **not** evidence that the client supports 26.3.

### Tests and build quality

Executed on Go 1.27.2, not inferred from test-file counts:

- Client with its own go.mod: **does not build**. It pins February data and
  protocol modules while its current `self.go` expects the newer
  `S2CSetTime.ClockUpdates`. Merely `go get client@latest` is insufficient.
- Explicitly pinning the three June revisions together builds, and the
  client's tests pass. `go test -coverprofile=… ./...` reports **0.5% total
  statement coverage**, with world 10.0% and physics, pathfinding, inventory
  and client orchestration 0%. Its only test file covers chunk keys/cache.
- Protocol's tests pass: **45.7% total statement coverage**; network structures
  61.0%, NBT 52.8%, crypto 33.6%. Auth and the TCP Java protocol package have
  0% coverage. Tests do not establish working Microsoft auth or Paper physics.
- Data's own tests fail: block tests require absent `../generate/blocks.json`,
  and language tests use `TextComponent.Render` absent from the pinned older
  protocol. Updating the protocol fixes the language build, not the missing
  fixture. Its partial coverage profile is 8.9%, **not a successful full-suite
  coverage result**. Packets have 0% coverage. Chunk tests pass (73.2%).
- A real `-race` run of the prototype exits **66**, “Found 15 data race(s)”.
  Examples: `physics.tick` writes `onGround` without the mutex used by
  `IsOnGround`; `tick` reads impulses concurrently with `SetInput`; teleport
  handling and physics share velocities; disconnect shares queue state.
  These include upstream methods called in the ordinary way, not just an
  unsynchronized counter added by the spike.

The README really is stale: it says world knowledge, inventory, entities,
physics and A* are unchecked TODOs, although those modules exist in source.
The source is more capable than the README, but existence is not maturity.

## Protocol and server upgrades

The recovered client has `ProtocolVersion = 776 // 26.2` in
`pkg/client/modules/protocol/protocol.go`. Data's generated `version_gen.go`
independently says Minecraft **26.2 / 776**, and the live Paper 26.2 join
succeeds after the decoding patch. The spec thread's exact-match claim is
therefore verified for these pins. Auth/encryption, session-server and chat
certificate implementations also exist; this spike tested offline login only.

The protocol README still gives a 26.1 / 775 installation example, even linking
an old 772 packet directory. Client CLAUDE.md says “Only the latest Minecraft
protocol version is supported.” Protocol README calls this its **“live in the
moment”** policy: no concurrent old-version support, and incidental wire
compatibility is not guaranteed. This is a policy, not proof that the current
public artifacts actually follow the latest release. Mojang's manifest now
lists 26.3 as release and 26.4 snapshots; the recovered client remains 26.2.
The spec thread's `774.2` tag/release-date claim could not be reverified while
the repositories were inaccessible; do not use it as a deployment pin.

Consequences for Clyde:

- **Gym Paper 26.1.2 is protocol 775; this client is 776.** It is not the same
  match. Switching implementations also requires a separately versioned gym
  image/server, or a tested translator supporting newer-client → older-server
  connections (normally ViaVersion), or a maintained 775 client snapshot.
  Do not silently bump the gym on main as part of the body change.
- **Production Paper 26.2 is 776.** Direct offline Paper worked in the spike;
  Bonnie's Velocity/proxy route, forwarding and plugins remain untested.
  A matching number does not settle those integrations.
- If Bonnie or the gym bumps versions, first check **protocol number**, not
  just marketing version. Same-protocol patch updates still need smoke/gym
  testing. A new protocol requires new mappings, codecs, registries, block
  states/shapes, metadata/components and possibly movement changes. Updating
  one handshake constant is not an upgrade.
- Pin client, data and protocol **as a tested set**. Retaining a pin preserves
  old behavior; it does not supply backported fixes. A new upstream “latest”
  would not automatically keep a 775 gym and a 776 production server working.
  A fork must own both branches or arrange version translation.
- Current mineflayer 4.39.0's tested ceiling is 26.1, verified in its published
  `lib/version.js`. Clyde's existing production plan still needs ViaBackwards
  for 26.2 (older client → newer server). Staying with mineflayer does not
  magically remove that dependency; it avoids replacing the rest of the body.

Data's documented upgrade process downloads vanilla server reports, regenerates
data using reports **and decompiled Minecraft source** in `../mcsrc/current`,
then manually updates packet bindings and component decoders. It is not a
self-contained one-command upgrade in the recovered archives. Budget an owner
for this if forking; assume days per routine bump and potentially weeks for
substantial wire/physics changes, not a promised upstream turnaround.

## Coverage of Clyde's contract

Compared against current `body/src`, `api/action.schema.json` and
`api/state.schema.json`, not the roadmap's shorthand. **Present** means an
upstream facility exists, not that it passed production acceptance. **Partial**
means useful lower-level pieces but missing semantics or reliability.
Effort below is estimated engineer-days to match Clyde, after a working
connection on the recovered go-mclib stack, including focused tests; rows
overlap and must not be summed. These are not estimates for other Go clients.

| Contract/capability | Coverage and evidence | Remaining work / effort |
|---------------------|-----------------------|-------------------------|
| Bot name, dimension, position, yaw/pitch, health, food, XP, game mode, on-ground | **Present**, self getters + physics; position/health packets handled | Map names/enums, degrees to Clyde's radians/sign convention, clamp/round values, consistent snapshot: 0.5–1 d |
| Oxygen, in-water/in-lava | **Partial**: own air metadata is not tracked by self; physics has fluid/block helpers but no full card adapter | Handle own metadata and scale air ticks to 0–20; derive/expose fluid state, do not repeat mineflayer's “any entity's air” bug: 0.5–1.5 d |
| Time, phase, rain/thunder | **Partial**: self TimeOfDay and time packets; game-event callbacks; no maintained rain/thunder getters | Wire weather events and initial values, modulo/phase conversion: 0.5–1 d. Biome is mentioned in roadmap prose but is **not a current schema field**, so not a parity blocker |
| Visible notable blocks | **Partial**, world chunk cache/GetBlock/FindBlocks + generated shapes | Radius, nearest/count ordering, exposed-face/no-x-ray rule, unloaded-neighbor handling; avoid treating unloaded chunks as air: 1–2 d |
| Entities/card classification | **Partial**, tracked spawn/move/velocity/metadata and UUID lookup; generated EntityCategory (`monster`, `creature`, aquatic/ambient/misc) | Match Clyde's hostile/passive semantics rather than treating every non-monster as passive; join player-list names; health/item names from metadata, distances/compass/top eight: 1–2 d |
| Inventory totals/held/used slots | **Present**, slot decoding, held slot/item, search, hotbar/equip helpers | Aggregate only current inventory range, not armor/crafting; snapshot safely: 0.5–1 d |
| `goto` | **Partial**, A* FindPath/NavigateTo/Stop, default 10,000 nodes; walks/jumps/falls/doors/parkour in source | GoalNear/range semantics, unloaded boundaries, progress fields, movement fixes/races, reliable cancellation. No mining/scaffolding routes like mineflayer-pathfinder: 2–4 d plus hardening |
| `follow` | **Missing high-level action**, entity/player tracking and NavigateTo available | Moving goal, repath cadence, distance/lastMovedAt, stop/preemption/lost-player behavior: 1–2 d |
| `dig` | **Partial**, BreakBlock(start/finish), CancelBreakBlock, SwingArm; hardness/tool data | Approach/LOS, best tool, harvestability, break-time modifiers/effects/water, server confirmation/timeout/abort: 2–4 d. Spike only implements bare hand on dirt/grass/log |
| `place` | **Partial**, PlaceBlock/InteractBlock and inventory HoldItem | Empty/replaceable target, neighbor-face choice, approach/reach, equip, confirmed placement, abort: 1–2 d |
| `collect` | **Missing high-level action**, FindBlocks + navigation/dig/entity/inventory pieces | Target selection, exposure/regions, harvest loop, dropped-item identification/pickup, inventory-full/retry/progress and preemption: 2–4 d after dig/goto. Spike gathers one fixture log, not arbitrary trees |
| Chat/whispers | **Present**, chat callbacks, SendMessage/SendCommand, signing implementation | Events, op stop command, long-message/error behavior: 0.5–1 d. Offline chat proven; signed/online chat not exercised |
| Containers | **Partial**, open state, slot views, left/right/shift-click, close and click state IDs; item-sorter example | Reach/open/await close, reliable transfer by name/count, click reconciliation, query views; exercise real chest/furnace workflows: 1–3 d |
| Crafting and recipe lookup | **Missing high-level API**, packet/registry names are not a recipe database or `recipesAll`/`craft` implementation | Recipe acquisition/data, ingredient matching/alternatives, 2×2 vs table grid, transactions/output/space checking; current executor calls recipesAll via /query: 3–6 d |
| Safety limits | **Missing Clyde policy**, combat can attack players | Region break/place refusal, rolling successful-break cap, protected-path exclusions, no player attacks, op stop and central mutation gate: 2–3 d |
| HTTP/state/actions/events/stop | **Missing Clyde layer** | Six endpoints, JSON Schema validation, text card, readiness/errors/1 MiB request cap, action/lastAction lifecycle and progress, ring/cursor/missed/reset semantics: 2–4 d |
| `/eval`, `/query`, skill runtime | **Missing** | A new execution surface, or a substantial compatibility shim; alternatives below: 3–7 d for a narrow API, much more for mineflayer compatibility |

Safety and cancellation are not incidental HTTP concerns. A new action must
mark the previous one aborted **before** starting, stop movement/digging, and
prevent late work from stealing the next action. Go contexts alone do not
stop packet writes: every mutation must check the active action generation /
context at a central boundary. Do not expose raw `Client.WritePacket` to model
code. Today's mineflayer wrappers are explicitly not a hostile-code sandbox;
`/query` is read-only by caller convention, not enforcement. A Go replacement
should specify whether that remains the contract or becomes a restricted
snapshot-only query runtime; it must not accidentally start replacing actions.

## Live Paper experiment

Own orb, **separate** `~/paper-spike` directory, Paper **26.2 build 134**,
Java 25.0.4.1, Go 1.27.2; offline mode, loopback ports 25566/25576, survival,
no third-party anti-cheat or protocol translator. Supervised using
`amp orb service start paper-spike`; stopped after the spike. Gym's 26.1.2
directory and runner were not modified. Fresh Paper first boot reached
`Done (20.054s)`, excluding download. Timings below exclude Go compilation
and the five-second fixture preparation.

### Failures before success

1. Plain client pin fails compilation as described above. Pinning June data
   and protocol fixes that build failure, without changing client code.
2. First real connection reaches login/configuration, then the process is
   killed with exit 137. RSS grows past 3.8 GiB within seconds. A diagnostic
   watchdog dump catches approximately 4,964 MiB allocated in
   `nbt.Reader.readList`, called by `S2CRegistryData.Read`.
3. **Verified cause:** that packet reader uses `ReadByteArray(1048576)` before
   `DecodeNetwork`, but registry NBT is an inline self-delimiting tag, not a
   VarInt-length-prefixed byte array. The compound tag byte `0x0a` is consumed
   as length ten; the resulting misaligned list header can request an enormous
   allocation. A captured real registry decodes correctly when read from the
   stream. The patch uses `nbt.NewReaderFrom(buf.Reader()).ReadTag(true)`.
   A two-entry public packet-decoding regression test fails before the patch
   (`unexpected EOF`) and passes after it. Six analogous NBT readers were
   patched in the spike; only registry decoding is covered by this test.
   Packet writers and other codecs are not thereby certified.
4. A first uncontrolled natural-world attempt joins in 1.20 s and chats,
   but goto times out after 60 s, records 1,377 position packets over 88.45 s,
   dies to zombies and gathers no log. Spawn/respawn teleports are included
   in that count. It cannot isolate physics from terrain, death and targeting.
   A subsequent invalid fixture run tried filling unloaded chunks; that was
   fixed with explicit forceload/wait and is not evidence against go-mclib.
5. Pickup iterations exposed **spike bugs**: choosing an arbitrary nearby dirt
   drop, or navigating onto the top of a floating upper log before the item
   settled. The final controlled fixture has one log, no stale dropped items,
   filters metadata for `_log`, waits 800 ms, and checks inventory gain. These
   fixes are spike collection logic, not upstream collection capabilities.

### Successful controlled run and wall A/B

Prepared a flat grass platform at Y=160, one oak log at (17,161,8), peaceful/day,
and teleported the bot to (0.5,161,0.5) **only for setup**. Client packets then
performed all walking, digging and gathering. The normal-width client uses
the NBT patch but otherwise unmodified client source:

| Operation | Normal width 0.6 | Optional width 0.6002 |
|-----------|------------------|-----------------------|
| Login to spawn | 0.18 s | 0.18 s |
| Goto (12.5,161,8.5), 14.5 m displacement | 3.65 s, 0.03 m from goal | 3.65 s, 0.03 m from goal |
| Dig grass block (13,160,8) | 1.15 s, server sent air | 1.15 s, server sent air |
| Navigate/dig/pick up one oak log | 7.25 s | 7.50 s |
| Four ~20 m waypoint walks | 4.70 s each | 4.70 s each |
| Chat | received by Paper immediately (reported 0.00 s) | same |
| RCON inventory check `clear Clyde #minecraft:logs 0` | Found 1 matching item | Found 1 matching item |
| Total including setup wait and wall probe | 39.04 s | 39.28 s |
| Five-second wall-contact/slide probe | **26 corrections** | **0 corrections** |

The log loop uses upstream `FindBlocks`, `NavigateTo`, `BreakBlock`, metadata
and inventory APIs, plus bare-hand timing written for the spike. Walking
success is checked by navigation completion and distance; digging by the
server block update; collection independently by RCON. Timings are individual
samples, not a benchmark distribution. Repeated experimental runs reached
the same wall contrast: a race-enabled normal-width run also had 26 corrections,
and an earlier width-patched run had zero.

For the wall probe, fill a stone wall at X=20, teleport to (17.5,161,0.5),
wait a second, reset the correction counter, push east for two seconds then
walk south for three. Default-width contact produces repeated server
positions. With **only physics.PlayerWidth changed from 0.6 to 0.6002**,
contact is X=19.69990 and the bot slides to Z=13.19 with zero corrections.
This is the same 0.3001 half-width workaround recorded in `01-body.md`.
It is evidence for this boundary case, not a general proof of vanilla physics.

There were **no server kicks in the completed patched offline runs**. The
unpatched join OOM/crash is a client failure, and the race-enabled run fails
locally with exit 66. None of this proves freedom from anti-cheat kicks or
rubber-banding on Bonnie's plugins. Slabs, stairs, fluids, ladders, knockback,
unloaded chunk transitions, latency, long survival runs and Velocity still
need acceptance tests. Go does not make those obligations disappear.

Prototype `./prepare`, `go test ./...`, `go vet ./...` and `go build ./...`
pass after the NBT patch. The live race run does **not** pass. Existing Clyde
body tests (20/20) and TypeScript build pass; harness build/tests pass. Those
checks demonstrate no main-body changes, not parity of the prototype.

## Supplemental survey: other Go clients

Source inspection and local tests on 2026-10-17 broaden the candidate list.
They do not supersede the measured go-mclib failures above, but they do limit
the conclusion: **do not generalize go-mclib's repair/parity estimate to the
Go ecosystem**. The initial survey used source and offline tests; the separate
follow-on Paper experiment is recorded after it below.

### mc26-kit: strongest next candidate

[mj41/go-mc26-kit](https://github.com/mj41/go-mc26-kit/tree/f742fe252fe301624946de5b5a562ee000c8c9a0)
is an MIT-licensed derivative of Tnze/go-mc, with retained upstream copyright
and a per-file [provenance ledger](https://github.com/mj41/go-mc26-kit/blob/f742fe252fe301624946de5b5a562ee000c8c9a0/COPIED).
Inspected source includes actual physics/collision, A* navigation, goto/follow,
entity/world tracking, digging/placing, recipe lookup/crafting and containers,
not just README promises. Its generated protocol/data dependency is
[go-mc26](https://github.com/mj41/go-mc26); the selected dependency version
chooses the server protocol at **build time**, not per connection. A gym 26.1
and production 26.2 deployment would need separately pinned builds or a
separately tested translation arrangement.

Evidence, with distinct limits:

- Locally, `go test -race ./...` **passed** at kit revision
  `f742fe252fe301624946de5b5a562ee000c8c9a0`, using its default
  `go-mc26 v0.261.2` dependency. This includes physics, pathfinding, simulated
  worlds and example-robot tests; it is not a live Paper run or an exhaustive
  concurrency audit.
- The [kit CI run](https://github.com/mj41/go-mc26-kit/actions/runs/38288719650)
  passed its [26.1/26.2/26.3 matrix](https://github.com/mj41/go-mc26-kit/blob/f742fe252fe301624946de5b5a562ee000c8c9a0/.github/workflows/ci.yml):
  dependencies `v0.261.2`, `v0.262.2`, `v0.263.2`, with build, vet and tests.
  Those upstream matrix tests do not use `-race`.
- The [mc26 release run](https://github.com/mj41/mc26/actions/runs/38293848810)
  succeeded; its [release workflow](https://github.com/mj41/mc26/blob/11f103787488f5a74cf3b2febcaafd4716c196a9/.github/workflows/release.yml)
  invokes the pipeline with `--e2e`. The
  [robot scenarios](https://github.com/mj41/go-mc26-kit/blob/f742fe252fe301624946de5b5a562ee000c8c9a0/examples/robot/README.md#how-it-is-tested)
  describe live **vanilla** server checks of positions, inventory and blocks,
  and rejection of movement complaints/kicks. This is upstream evidence,
  not a reproduction here and not Paper acceptance.
- At survey time **Paper movement was unverified**. The follow-on experiment
  below found wall/stair corrections with upstream's default width and success
  in those fixtures with a width-only patch, not general Paper acceptance. The
  [physics package](https://github.com/mj41/go-mc26-kit/blob/f742fe252fe301624946de5b5a562ee000c8c9a0/bot/physics/player.go)
  documents gaps including swimming pose, cobwebs/berry bushes, powder snow,
  bubble columns and some effects. The robot docs also acknowledge stuck
  cases and survival deaths.

The kit repository was created October 3 and had one listed contributor,
`mj41`, with activity October 12. It is very young, declares no API compatibility
promise, and has no independently established long-term maintenance record.
Nevertheless, its existing high-level client code makes it a stronger next
candidate than rebuilding the recovered go-mclib stack. The follow-on experiment
below covers Paper walk/wall/dig/log/chat, crafting, containers and limited
stop/preemption. A new skill runtime and Clyde's safety/HTTP contract would
still be required; no mc26-kit migration effort is estimated here.

### Other candidates and limitations

| Candidate / inspected revision | Verified evidence | Consequence for Clyde |
|-------------------------------|-------------------|-----------------------|
| [Tnze/go-mc](https://github.com/Tnze/go-mc/tree/539b4a3a7f030332eb58b8a946116ae7907630d2) | MIT; established history, but current master's [protocol constant](https://github.com/Tnze/go-mc/blob/539b4a3a7f030332eb58b8a946116ae7907630d2/bot/mcbot.go) is **767 (1.21/1.21.1)**, not 26.x. Last master commit December 24, 2024; a later repository `pushed_at` is not a newer master implementation. Login/chat, chunk load/unload and inventory/container-click primitives exist; no current movement/physics/pathfinding engine or high-level crafting automation found. | A foundation rather than a mineflayer substitute. `go test ./bot/... ./net/... ./level/...` passed locally, but bot itself had no tests to run and its subpackages had no test files. [Issue 299](https://github.com/Tnze/go-mc/issues/299) requests 26.2 and points to mc26; that derivative is more relevant than porting upstream master ourselves. |
| [zeozeozeo/minego](https://github.com/zeozeozeo/minego/tree/9c0afa678fadf85a13c88b3e2772601fc59bb758) | Real version packs for 26.1/26.2, navigation/physics, mining, crafting and containers; local `go test -race ./...` passed. Its [parity matrix](https://github.com/zeozeozeo/minego/blob/9c0afa678fadf85a13c88b3e2772601fc59bb758/docs/mineflayer-parity.md) still records partial semantics and pending server coverage. The opt-in live 26.2 integration test was **not run** here. | Technically promising, but **licensing is unresolved**: no project license in the inspected tree; [protocol NOTICE](https://github.com/zeozeozeo/minego/blob/9c0afa678fadf85a13c88b3e2772601fc59bb758/internal/protocol/NOTICE.md) identifies copied go-mclib revision `6905af045007` and references an absent `third_party/go-mclib-protocol/LICENSE.md`. No module dependency does not mean no inherited code. Resolve the license grant and retained notices before adoption; do not infer permission from public visibility. |
| [Understudy client](https://github.com/blocktopiaworld/understudy-client/tree/7cd731ddb193a20c1229b0e87f5e5804183c7f6c) | Apache-2.0; current version tables including 26.1/26.2; library and HTTP controls, world/entity/inventory state, digging, placement and crafting. Local `go test -race ./...` passed. Its [movement implementation](https://github.com/blocktopiaworld/understudy-client/blob/7cd731ddb193a20c1229b0e87f5e5804183c7f6c/understudy/move.go) explicitly uses straight-line dead reckoning, **not pathfinding**. Offline-mode only. | Useful for controlled server tests, not a ready autonomous-survival body. Navigation/terrain behavior remains substantial work; no Paper run was performed here. |

Keep mineflayer for now, but distinguish that immediate delivery choice from
rejecting every Go migration. **mc26-kit remains a credible future option**;
its migration effort is not yet estimated. None of these clients automatically
preserves `/eval`, `/query`, or JavaScript skills against mineflayer's `bot`.

## Follow-on: mc26-kit verified on Paper

Lachlan chose mineflayer for slices 5/6; the coordinator recorded that decision
in the roadmap. This independent experiment does not change runtime contracts
or block those slices. **Recommendation unchanged: keep mineflayer now, retain
mc26-kit as a real future option rather than treating go-mclib's repair estimate
as a verdict on Go.** No further experiments or migration work are planned.

### Pins and retained reproduction

- Kit [`f742fe252fe301624946de5b5a562ee000c8c9a0`](https://github.com/mj41/go-mc26-kit/tree/f742fe252fe301624946de5b5a562ee000c8c9a0),
  supplied `examples/robot`, built with `-race`.
- Protocol/data module `github.com/mj41/go-mc26 v0.262.2`. Selecting this
  published 26.2 dependency was sufficient: **no codec or dependency repair**
  like the recovered go-mclib stack needed. This is a build-time version pin.
- Paper **26.2 build 134**, Java **25.0.4.1**, Go **1.27.2**, Linux amd64;
  offline, survival/peaceful, no plugins or translator, separate disposable
  `~/paper-spike` on loopback 25566 / RCON 25576. Paper jar SHA-256:
  `3cce0a4922baf79bd8e1275ee5ff9f073f24675770e09b0d4be9442d513e7a6b`.
- Two builds: upstream-default **width 0.6**, and a **one-line width 0.6002**
  patch in `bot/physics/player.go`. Do not attribute patched results to default.

The existing [spike branch's mc26-kit directory](https://github.com/lox/clyde/tree/spike/go-mclib/spike/mc26-kit)
retains the portable fixture driver, exact build script, MIT notices,
[width patch](https://github.com/lox/clyde/blob/spike/go-mclib/spike/mc26-kit/width.patch),
and [sanitized text/JSON evidence](https://github.com/lox/clyde/tree/spike/go-mclib/spike/mc26-kit/results)
from five actual runs. [README setup and replay](https://github.com/lox/clyde/blob/spike/go-mclib/spike/mc26-kit/README.md)
includes the pinned Paper download, checksum, private generated RCON password,
and supervised-service lifecycle. No jar, world or credential is published.

```sh
git clone --branch spike/go-mclib https://github.com/lox/clyde.git clyde-go-spike
cd clyde-go-spike/spike/mc26-kit
./prepare  # builds default and width-patched race-enabled examples
# Create/start a NEW disposable Paper server using README.md; never the gym.
python3 run.py baseline-repeat --server-dir "$HOME/paper-mc26-repro" --confirm-disposable
python3 run.py wide --server-dir "$HOME/paper-mc26-repro" --confirm-disposable
python3 run.py wide-contained --server-dir "$HOME/paper-mc26-repro" --confirm-disposable
```

Run those separately: baseline's staircase is expected to fail. The driver
resets terrain/items/inventory; never use a shared server. Its `ok` means no
command error, **not** full behavioral acceptance. The portable CLI/path
packaging was checked and both binaries rebuilt offline; the retained live
runs used the same scenario commands with fixed orb paths. No new Paper
replay was run during preservation. All experimental services are stopped.

### Observations, not a parity claim

| Scenario | Upstream default width 0.6 | Width-only patch 0.6002 |
|----------|----------------------------|------------------------|
| Join, flat goto, grass/log dig, pickup, chat | Passed twice; RCON confirmed position, removed blocks and one log | Passed in three runs |
| Five-second wall contact/slide | **3 corrections** in each of two runs | **0 corrections** in each of three runs |
| Three one-block rises then descent | **71 corrections** on ascent; example's relog watchdog self-exited **3**, not a server kick | Ascent/descent passed three times without additional corrections |
| Detour around a three-high wall | Passed, no additional corrections | Passed |
| Recipes/crafting, table placement, chest transfer | Passed: log → 4 planks → table → placed table; seeded planks/sticks → table-crafted pickaxe; chest store/take | Repeated successfully |
| Ten-block, one-deep contained water trench | Not tested | Reached far bank in **5.24 s**, no additional corrections |
| Stop walking, then new goto | Passed | Repeated successfully |
| Stop digging, then new goto | Not tested | **Example `cmdStop` path failed twice**, details below |

Independent RCON checks include position, inventory counts, chest NBT and block
markers; setup teleports are excluded from correction counts and never satisfy
a walk. Crafting ingredients for the 3×3 pickaxe were seeded, not gathered in
survival. An earlier two-deep water fixture leaked and triggered escape
reflexes: its failure is **inconclusive**, not evidence against water physics.
Sample timings: login/configuration 1.54–2.53 s; ~14.5 m flat goto 3.75 s;
grass dig 1.00 s; oak-log dig 3.15 s; nearby pickup 0.55 s; four planks 0.24 s;
table pickaxe 0.25 s; chest deposit 0.30 s. These exclude compile/fixture time
and are individual samples, not benchmark distributions.

**Stop-dig is an example `cmdStop` integration failure.** Twice, `stop` answered
“stopped”, yet the log broke about three seconds after digging began, before
the queued goto ran. Client air state and Paper's `DIG_CONTINUED_AFTER_STOP`
marker agree. [`cmdStop`](https://github.com/mj41/go-mc26-kit/blob/f742fe252fe301624946de5b5a562ee000c8c9a0/examples/robot/console.go#L526-L531)
stops Walker and flags order cancellation, but does not cancel Hands.
**[`Hands.CancelDig`](https://github.com/mj41/go-mc26-kit/blob/f742fe252fe301624946de5b5a562ee000c8c9a0/bot/act/dig.go#L44-L54)
exists, but was not integrated or tested in this path**; this is not evidence
that the library cancellation API fails.
A Clyde adapter must still enforce full cancellation and mutation-generation
gates across movement, dig, inventory and model code.

Default-width 26.2 `go test -race ./...` and `go vet ./...` passed; patched
physics/control/act race tests and vet passed. No Go race warnings appeared
in five live runs, and no server kicks were observed. Three runs exited 0;
the default staircase run self-exited 3; the last patched run exited **1** on
explicit quit with a closed-network-connection error after its operations.
That shutdown error is neither a kick nor a race report. Paper logged no
`moved wrongly`/`moved too quickly` warnings despite client-visible corrections;
absence of server warnings alone is not movement acceptance.

Remaining gaps: Paper **26.1.2** gym version; following; online auth and
Velocity/proxy/plugins; prolonged natural survival and adverse terrain/effects/
latency; Clyde's six HTTP endpoints, state/schema and safety limits; complete
preemption; `/eval`, `/query` and JS skills. No migration estimate follows from
these fixture successes, and no runtime or gym change belongs on main here.

## Code-as-action and the slice 5 decision

Preserving the URL and `{code, timeoutMs}` shape does **not** preserve the
contract. Current eval code is an async function body with live mineflayer
`bot`, `mc`, `Vec3`, `goals`, `Movements`, `goto`, timers, logs and abort signal.
`recipes_for` and `lookup` are already JavaScript query programs in the
executor, not merely future skills. Container and crafting operations are
available through that live API even though they have no primitive action.

| Choice for a Go body | Model-authoring consequences | Engineering consequences |
|----------------------|-----------------------------|--------------------------|
| Goja with a mineflayer-compatible `bot` | Familiar JS; existing examples may work only if their entire object/async surface is emulated. Models will reach for mineflayer APIs beyond the implemented subset | Goja is an ECMAScript engine, not Node or mineflayer. Need bindings for registry/block/item/Vec3 types, promises, timers/event loop, callbacks, error/return serialization and cancellation. Runtime is not concurrently goroutine-safe. Full compatibility negates the “minimal Go body” premise |
| Goja with a **new narrow Clyde API** | Keep loops, conditionals, sequencing and JSON; give models a short precise reference and examples. Existing mineflayer skills and query programs must be ported | Most plausible Go design: expose only context-bound operations, snapshot/read APIs, recipes, inventory/container/craft operations. One VM owner; interrupt JS on timeout, and separately cancel host operations. No raw client or Go pointers. Approximately 3–7 d for runtime/bindings/tests beyond the body operations |
| Starlark | Familiar Python-like syntax, but not Python: no imports of Python libraries, no async/await, no ordinary while loop; models need bounded `for` loops and supplied builtins. Small API is easy to document, unfamiliar Minecraft ecosystem | Builtins can synchronously await context-aware body operations. `Thread.Cancel` and execution-step limits help; they do not bound Go builtin duration or memory. Change skill extension, eval tool/prompt semantics and all existing JS queries; not a compatible `/eval` |
| Compiled Go-defined skills | Strong typing and normal Go tooling, but a model must author valid code and pass build/test review before admission. Cannot submit arbitrary new code and immediately execute it as today's eval tool does | Compile/register skills as trusted code, or build/run a separate constrained worker. Go plugins are not a portable sandbox or an arbitrary eval mechanism. Precompiled skills alone shrink the executor's open-ended action space; retain a script escape hatch if that capability matters |
| Node JS skill worker calling a narrow Go HTTP API | Real JS runtime; no need to imitate Node in Go, but no live mineflayer `bot` unless retained | A reasonable hybrid **if a mature Go body already existed**. Adds another process/RPC/cancellation boundary and still needs the new domain API. Retaining Node + mineflayer for movement while Go wraps HTTP is today's architecture with another layer, not a Go-body migration |

Goja supports Go values/functions and promises, but its README explicitly
requires the host to provide timers/event-loop behavior. Do not promise that
adding an engine gives compatibility with Node plugins. Its `Interrupt` can
stop interpreted JavaScript; a blocking native operation still needs context
cancellation. Starlark's own docs recommend a separate OS-limited process for
untrusted code. Neither engine makes in-process model code a safe resource
sandbox by default.

Concretely, a narrow Goja design could use synchronous, context-aware host
calls so the first version needs no promise/event-loop emulation:

```js
// Illustrative NEW API, not implemented by this spike or compatible with bot.
const logs = clyde.findBlocks({ name: "oak_log", radius: 32, exposed: true });
for (const block of logs.slice(0, 3)) {
  clyde.dig(block.position); // approach, dig, confirm; throws on abort/failure
  clyde.pickupNear(block.position);
}
return clyde.inventory();
```

These calls operate **inside the current eval action's context**, not by
recursively posting `/action` and aborting their own caller. Return JSON and
logs in today's result shape. A separate `/query` VM would expose only snapshot,
lookup and recipe functions, not dig/craft/move. Models retain code-as-action
but must learn this API instead of assuming mineflayer; crafting primitives
still need implementing in Go. This is the sensible scripting direction if
a migration is mandated, not a recommendation to implement it for slice 5.

For slice 5 **accept the mineflayer dependency deliberately**. Keep the planned
JS format, supply the exact runtime/API in skill descriptions, prefer public
methods over private plugin internals, and gym-test every admitted skill.
Do not build a comprehensive portability shim for an unproven migration.
Five small JS skills are cheaper to rewrite later than a clone of mineflayer's
API is to build now. If future portability is valuable, record the required
operations per skill; this is an inventory of actual needs, not a speculative
abstraction. The roadmap now records mineflayer for slices 5/6; mc26-kit's
separate Paper evidence above does not change that choice. This spike
intentionally changes only its report on main.

## Depend, vendor, fork, or reimplement?

These are estimates **specifically for the recovered go-mclib client/data/
protocol stack**, for one engineer familiar with Clyde/Go; not measured
delivery promises or estimates for mc26-kit or other Go clients. “Minimal
client” below still has to receive/configure/play, decode world and entities,
move like Minecraft, manipulate inventory and preserve cancellation/safety.
A packet transport library does not do that.

| Approach | Initial work to Clyde parity | Risk and ongoing ownership |
|----------|-----------------------------|-----------------------------|
| Depend on cached pins | Hours to build a demo; **not viable unpatched** for normal Paper registry configuration | No currently accessible upstream client release channel; broken dependency pins, unsafe parsing and races. Checksums preserve identity, not correctness. Would still need local replacements for required fixes |
| Vendor pinned sources | Roughly 1–2 d to capture/license/document, then **4–8 weeks** total parity/hardening work | Prevents source disappearance and enables patches; no automatic update support. Once modified it is a private fork in substance, with less convenient upstream comparison |
| Maintain a fork of client + data (+ protocol as needed) | Same **4–8 weeks** broad estimate; most practical route **within this recovered stack**, not the best of all Go candidates | Explicit owner for three coupled layers, codec/security fixes, race-safe snapshots/tick ownership, version branches and Paper acceptance matrix. GitHub Fork UI cannot fork inaccessible repos; seed a new repository from MIT proxy archives, preserving notices/provenance |
| New minimal client on protocol (+ data or replacement codecs) | Approximately **8–16+ weeks**, with greater uncertainty | Rebuild configuration/session orchestration, chunk/world/metadata state, physics/collision, A*, transactions, crafting and safety; shared packet/data faults still apply if reused. Dropping data also means replacing its mappings/codecs. Less inherited architecture, much more Minecraft expertise required |
| Keep mineflayer + existing Go harness | No migration delay; **0.5–1 d** to make the slice-5 API decision/documentation explicit | Keep known width/oxygen/preemption workarounds and existing version translator plan; retain mature plugin/data ecosystem. Best fit for model-authored skills and current gym evidence |

The recovered go-mclib stack's 4–8 week fork estimate covers roughly: upstream
codec/concurrency/physics hardening (1–2 weeks), body/state/action/safety parity
(1–2 weeks), containers, recipes/crafting and scripting (1–2 weeks), then gym/proxy/soak acceptance
(about a week), with overlap. It excludes production rollout and unspecified
future gameplay, and the upper bound is not a guarantee. There is no measured
CPU/memory/deployment benefit here to offset that work; Go performance is not
assumed from the language alone.

No immediate move is recommended. The mc26-kit Paper evidence above supports
keeping that option open, not assuming it needs go-mclib's repairs or missing
features. If the recovered go-mclib stack is nevertheless chosen,
use these **gated** slices (not a migration plan or estimate for mc26-kit):

1. Recover/license a pinned stack; fix registry/codecs and race ownership;
   establish repeatable Paper join/wall/physics tests. Stop if this cannot be
   made reliable before rewriting Clyde.
2. Implement schema-compatible HTTP state/events/stop and action cancellation
   plus central safety gates. Keep existing TS body as the comparison oracle.
3. Port navigation, digging/placement, collection and follow; run old gym tasks
   and a preemption soak against an explicitly versioned separate gym.
4. Add real recipes/crafting/containers and a narrow Goja skill API; port
   executor queries and five skills. Test logs → planks → table → placement
   and chest deposit, not only individual packet sends.
5. Run side-by-side gym/latency/survival trials, then test the production proxy
   path in isolation. Only replace the body after behavioral parity; deploy
   changes still belong in `lox/selfhosted-ovh`, never this spike.

## Reproduction and sources

The prototype branch README has exact Paper setup, `./prepare`, baseline,
width-patch and race commands. `results/` holds timing JSON and client logs,
the pre-fix NBT test failure, OOM stack and dependency test outputs. The local
patched copies are ignored; `prepare` reconstructs them from immutable modules
and the retained patch without editing the module cache. No server jars or
world data are committed. Do not run its destructive fixture flags anywhere
except a throwaway local server.

Primary sources inspected:

- [client module metadata](https://proxy.golang.org/github.com/go-mclib/client/@latest),
  [June client source archive](https://proxy.golang.org/github.com/go-mclib/client/@v/v0.0.0-20260626124012-de01d7956dc3.zip):
  README/CLAUDE/LICENSE/go.mod; `pkg/client/actions.go`; modules self, world,
  entities, physics, collisions, pathfinding, inventory, chat; example item sorter.
- [protocol metadata](https://proxy.golang.org/github.com/go-mclib/protocol/@latest),
  [June protocol source archive](https://proxy.golang.org/github.com/go-mclib/protocol/@v/v0.0.0-20260627053125-6905af045007.zip):
  README/LICENSE/auth, Java TCP/net structures, NBT reader.
- [data metadata](https://proxy.golang.org/github.com/go-mclib/data/@latest),
  [June data source archive](https://proxy.golang.org/github.com/go-mclib/data/@v/v0.0.0-20260626135928-f4a527b8705b.zip):
  version, registries, entity categories/metadata, block hardness, chunk types,
  packet readers, generation documentation and test fixtures.
- [Public organization repository list](https://api.github.com/orgs/go-mclib/repos),
  [NBT history](https://github.com/go-mclib/nbt/commits/main/).
- [Paper 26.2 build metadata](https://fill.papermc.io/v3/projects/paper/versions/26.2/builds/134),
  [Mojang release manifest](https://piston-meta.mojang.com/mc/game/version_manifest_v2.json).
- [Goja README](https://github.com/dop251/goja),
  [Starlark Go README and resource limits](https://github.com/google/starlark-go),
  [Starlark language specification](https://github.com/google/starlark-go/blob/master/doc/spec.md).
- Clyde's `body/src/{actions,eval,safety,state,server,index}.ts`,
  `api/{state,action}.schema.json`, `harness/executor/tools.go`,
  `gym/bin/paper`, `01-body.md`, `04-executor.md`, and `roadmap.md`.

The spec thread was read for its exact earlier assertions, then checked against
the recovered sources and live server rather than accepted as evidence by itself.
