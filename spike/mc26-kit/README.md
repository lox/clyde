# mc26-kit on Paper (research only)

Durable reproduction of the 2026-10-17 experiment. This is **not** a Clyde body,
a replacement HTTP/skill contract, or a production-ready client. The report is
[`docs/plans/spike-go-mclib.md` on main](https://github.com/lox/clyde/blob/main/docs/plans/spike-go-mclib.md).
Keep mineflayer for slices 5/6; mc26-kit remains a future option.

## Exact inputs

- [mc26-kit source](https://github.com/mj41/go-mc26-kit/tree/f742fe252fe301624946de5b5a562ee000c8c9a0),
  revision `f742fe252fe301624946de5b5a562ee000c8c9a0`, supplied `examples/robot`.
- `github.com/mj41/go-mc26 v0.262.2`, selected using its documented dependency
  mechanism. This is a 26.2 build, not automatic runtime version negotiation.
- Paper **26.2 build 134**, offline, no plugins, survival; Java **25.0.4.1**,
  Go **1.27.2**, Linux amd64. Both robot binaries compiled with `-race`.
- `width.patch` changes **only** `bot/physics/player.go`'s `playerWidth` from
  **0.6 to 0.6002**. Default results must not be described as patched results.
  MIT notices and upstream provenance are retained (see `NOTICE`).

## Build without running Minecraft

Requires Go (tested 1.27.2), Python 3, curl, tar, patch and a C compiler for
Go's race detector; the Paper setup also uses openssl and sha256sum. In an orb,
add `~/tools/go/bin` to PATH if needed.

```sh
git clone --branch spike/go-mclib https://github.com/lox/clyde.git clyde-go-spike
cd clyde-go-spike/spike/mc26-kit
./prepare
# Emits bin/robot-default and bin/robot-wide; source remains in ignored .work/kit.
# prepare intentionally refuses to overwrite an existing .work directory.
```

The default executable is built **before** the patch. No client behavior,
codec or dependency fixes are applied to it. Selecting the published 26.2
module is the only dependency change. `prepare` downloads the pinned source,
keeps its license/provenance, and does not edit the shared Go module cache.

## Separate disposable Paper server

**Never point the runner at a shared/production server.** It resets a platform,
kills dropped items, clears the bot inventory and teleports players for setup.
The fixed target is loopback **25566**, RCON **25576**. Do not change the gym.
The server directory must be new; no jars, worlds or generated credentials go
in Git. Accept Mojang's EULA only if you agree to it.

```sh
PAPER_SPIKE_DIR="$HOME/paper-mc26-repro"
mkdir "$PAPER_SPIKE_DIR"  # fails rather than reusing someone else's server
curl -fL -o "$PAPER_SPIKE_DIR/paper.jar" \
  https://fill-data.papermc.io/v1/objects/3cce0a4922baf79bd8e1275ee5ff9f073f24675770e09b0d4be9442d513e7a6b/paper-26.2-134.jar
# That object name is the expected SHA-256; verify it before starting Java.
printf '3cce0a4922baf79bd8e1275ee5ff9f073f24675770e09b0d4be9442d513e7a6b  %s/paper.jar\n' \
  "$PAPER_SPIKE_DIR" | sha256sum -c -
printf 'eula=true\n' > "$PAPER_SPIKE_DIR/eula.txt"
umask 077
cat > "$PAPER_SPIKE_DIR/server.properties" <<'PROPERTIES'
server-ip=127.0.0.1
server-port=25566
online-mode=false
level-seed=mcbot
level-name=world
difficulty=peaceful
gamemode=survival
max-players=5
spawn-protection=0
view-distance=6
simulation-distance=4
enable-rcon=true
rcon.port=25576
broadcast-rcon-to-ops=false
PROPERTIES
printf 'rcon.password=%s\n' "$(openssl rand -hex 24)" >> "$PAPER_SPIKE_DIR/server.properties"
# In an Amp orb (use the installed Java 25 location):
amp orb service start paper-mc26-repro --command \
  "cd '$PAPER_SPIKE_DIR' && '$HOME/tools/jdk/bin/java' -Xms1G -Xmx2G -jar paper.jar --nogui"
amp orb service logs paper-mc26-repro
# Wait for Done before running the driver.
```

Outside an orb, run the same Java command in the server directory in a
foreground terminal instead of the `amp orb service` commands. The runner
reads the password from `server.properties` and never logs/passes it on argv.

## Replay

From `spike/mc26-kit` after Paper is ready:

```sh
python3 run.py baseline-repeat --server-dir "$HOME/paper-mc26-repro" --confirm-disposable
python3 run.py wide --server-dir "$HOME/paper-mc26-repro" --confirm-disposable
python3 run.py wide-contained --server-dir "$HOME/paper-mc26-repro" --confirm-disposable
# New output goes to ignored runs/<label>/{transcript.log,results.json}.
amp orb service stop paper-mc26-repro
```

Run commands separately: the default staircase replay is expected to fail.
The runner returns nonzero on a client nonzero exit or a command error, but
**it is not a pass/fail acceptance suite**. `steps[].ok` means the example
answered without a command error, not that Clyde's contract was satisfied.
In particular, a successful `stop` response does not certify dig cancellation.
Review server observations and `DIG_AFTER_STOP`/`WALL_CORRECTIONS` markers.
`--output` and `--binary` can override the generated-output and binary paths.
The portable CLI/path plumbing was checked without rerunning Paper; the
recorded experiments used the same command sequence with fixed orb paths.

## What the retained evidence shows

`results/` contains sanitized text/JSON from **five actual runs**, not output
from the later portable wrapper. No failures have been rewritten as passes.
Local filesystem roots and terminal colors are removed; timestamps, command
answers, loopback addresses, timings and client exit codes remain.

| Evidence | Interpretation |
|----------|----------------|
| `initial.{log,json}` | Upstream-default smoke: join, flat walks, grass/log dig, pickup, chat, crafting, placement, containers, walk stop/new goal. Three wall corrections. Exit 0. This early script did not yet have the staircase. |
| `baseline-repeat.{log,json}` | Default width repeated three wall corrections. Obstacle detour passed; staircase generated **71 corrections**, then the example's relog watchdog self-exited **3**, not a server kick. |
| `wide.{log,json}` | Width-only patch: **zero** wall corrections, staircase ascent/descent and other smoke steps passed. Exit 0. |
| `wide-extended.{log,json}` | Patched movement repeated successfully. A two-deep water fixture leaked and triggered escape reflexes, so its water failure is inconclusive. Stop-dig failure was independently observed. Exit 0 is not a claim that every scenario passed. |
| `wide-contained.{log,json}` | Corrected one-deep contained water crossed in **5.24 s**, no corrections; repeated stop-dig failure. On explicit quit, the example logged a closed-connection error and exited **1** after the operations, not a race report or server kick. |
| `server.log` | Independent Paper observations: fixture confirmation markers, chat and `DIG_CONTINUED_AFTER_STOP` twice. No `moved wrongly`/`moved too quickly` log lines, despite corrective teleports being visible to the client. |
| `summary.json` | Consolidated results, sample timings and untested acceptance gaps. |

The **example's `cmdStop` path fails to abort an ongoing dig**: stop answered
“stopped”, but a log was broken about three seconds after digging began,
before the queued goto ran. Source confirms `cmdStop` stops `Walker` and flags
order cancellation, but does not call `Hands.CancelDig`.
**`Hands.CancelDig` is available; integrating and
testing it was not done.** This is not evidence that the cancellation API is
broken. A Clyde adapter still needs complete action-generation/safety gates.

Independent checks used RCON for positions, air/table markers, one collected
log, crafted item counts and chest NBT. Planks/sticks for the 3×3 pickaxe were
seeded explicitly; this is not an unseeded survival resource-acquisition test.
Teleportation was fixture setup only, never used to satisfy a walk. Counts
exclude commanded setup teleports. Timings are individual samples, not a
benchmark distribution.

During the experiment, default-width **26.2** `go test -race ./...` and
`go vet ./...` passed. After the width change, physics/control/act race tests
and vet passed. No Go race warnings appeared in the five live sessions;
this is not proof about unexercised concurrency paths. Re-run offline checks
from the generated source if desired (it is width-patched after prepare):

```sh
(cd .work/kit && patch -R -p1 < ../../width.patch)
(cd .work/kit && go test -race ./... && go vet ./...)
(cd .work/kit && patch -p1 < ../../width.patch)
(cd .work/kit && go test -race ./bot/physics ./bot/control ./bot/act && go vet ./...)
```

Remaining gaps: Paper 26.1.2, player following, auth/proxy/plugins, long natural
survival, adverse terrain/effects/latency, Clyde's HTTP/schema/state/safety and
full preemption contract, and eval/query/JavaScript skills. No migration effort
estimate is inferred from these runs. No further experiments are required to
preserve this evidence.
