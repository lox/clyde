# Full Clyde, simple private deployment

One container runs the compiled Node body and `clyde serve`: one durable Sol 6.1
low player conversation, Jev judge, SQLite/semantic/spatial memory, and
all five admitted bundled skills. No new controller, public API/MCP port,
dashboard, automatic image updater or production-world skill gate.
Host integration/startup belongs to `lox/selfhosted-ovh`, not this repository.

## 1. Gate exact sources on a disposable gym

Check out the intended Clyde commit. On a development machine/orb with the
repository's Go 1.27.2, JDK 25 and Node installed, run the existing gates.
These commands **reset a throwaway world**. Never point them at Bonnie,
its proxy, production RCON, or a world you want to keep. Use a shell without
production MC/RCON settings; the overrides below deliberately isolate all
endpoints and directories from other local servers:

```sh
export PAPER_DIR="$HOME/paper-package-gates"
export GYM_TEMPLATES="$HOME/paper-package-templates"
export GYM_PAPER_NAME=paper-package-gates
export GYM_MC_PORT=25567 GYM_RCON_PORT=25577 BODY_LISTEN_PORT=7789
export MC_HOST=127.0.0.1 MC_PORT=25567
unset GYM_RCON_PASSWORD # use only the gym's disposable default credential
gym/bin/paper install
gym/bin/gate-skills
gym/bin/paper stop
deploy/export-admissions
tar -czf /tmp/clyde-admissions.tgz -C deploy admissions
```

The model-free gates invoke captured source snapshots through the real body;
independent RCON predicates must pass before `skills.Store` writes receipts.
`deploy/export-admissions` refuses missing/stale admissions. Its generated,
gitignored `deploy/admissions/` contains only `revision.txt` and five
`<name>/.admission.json` files: collect-logs, craft-item, craft-place-table,
deposit-items, eat-food. Receipts contain source hashes/time/evidence paths,
not secrets or skill code. Retain the printed scoreboard/run results as
evidence beside the tar. Receipts are local provenance, not signatures or a
hostile-code security boundary; accept them only from a trusted gate run.

Do not edit sources between gating, export and build. After a source update,
rerun gates and export. A trusted gate machine may transfer just the tar to
a build machine checked out at the **same revision**; no Java/Paper, model
keys, world data or development dependencies are needed on the build machine.
Do not manually write receipts or run gates during ordinary startup.

## 2. Build once, identify the image

From the exact checkout root on the Docker machine:

```sh
# Only when importing a trusted export; inspect its file list before extraction.
tar -tzf /tmp/clyde-admissions.tgz
tar -xzf /tmp/clyde-admissions.tgz -C deploy
revision=$(git rev-parse HEAD)
test "$(cat deploy/admissions/revision.txt)" = "$revision"
docker build -f deploy/Dockerfile --build-arg CLYDE_REVISION="$revision" \
  -t "clyde:$revision" .
docker image inspect "clyde:$revision" --format '{{.Id}} {{json .Config.Labels}}'
```

The build uses versioned Go/Node images, `go.sum` and `npm ci` lockfiles,
compiles both runtimes, and checks every receipt against the image's actual
skill contents via `clyde skill verify`. Missing export, revision mismatch,
missing skill or changed contents fails the build. Startup rechecks all five
admissions. This is a reproducible source/build procedure, not a claim of
bit-identical images (OS packages and base-image tags can be refreshed).
Record the resulting image ID and commit; use that revision tag locally,
not `latest`. No registry is required. `.dockerignore` excludes runtime
memory, gym worlds/logs, Git, keys and environment files from build context.
Never pass keys as build arguments.

## 3. Private runtime configuration

`compose.example.yaml` is an **opt-in example**, not permission to launch.
Host owners adapt it in `lox/selfhosted-ovh` through that repo's PR flow.
The service must share only the private `minecraft_bonnie_net` network:

- Minecraft: `MC_HOST=minecraft-bonnie-bot-proxy`, `MC_PORT=25565`,
  `MC_USERNAME=Clyde`, `MC_AUTH=offline`, protocol 26.1 unchanged.
  Host owners provision the private bot proxy and whitelist Clyde's offline
  UUID. Clyde needs no Velocity forwarding secret.
- RCON: host owner verified authenticated `minecraft-bonnie:25575` on this
  private network, with no published host port. Set `CLYDE_RCON_ADDR` and
  supply its existing password as `GYM_RCON_PASSWORD` in the private env
  file. Securely extract it on the host; **never mount all of
  server.properties**, print it, or invent credentials.
- Keys: `OPENAI_API_KEY` and `TYPESAFE_API_KEY` in ignored mode-0600
  `clyde.private.env`, outside Git and the Docker context. Jev is explicit
  (`--judge jev`); a missing key fails startup, not a silent judge-off mode.
  Alternative providers use the existing `ANTHROPIC_API_KEY` contract.
- The single player uses `CLYDE_EXECUTOR_{PROVIDER,MODEL,EFFORT}`, defaulting
  to OpenAI `gpt-6.1-sol`, `low`. Remove `CLYDE_PLANNER_*` overrides: `serve`
  rejects them rather than silently changing models. Model/effort and runtime
  limits are recorded with the session; incompatible startup settings fail.
- Safety uses existing `BODY_MAX_BREAKS_PER_MIN`,
  `BODY_PROTECTED_REGIONS` (JSON array of inclusive min/max coordinates and
  optional dimension), `BODY_OPS` and `BODY_SCAN_RADIUS`. Choose real
  protected regions with the world owner. Safety is enforced in the body,
  not by a prompt. See `body/src/config.ts` and `.env.example`.
- Persistent volume: `/opt/clyde/memory`, owned by UID/GID **1000:1000**.
  A new named volume inherits image ownership. A bind mount must be writable
  by that identity. Keep the entire directory, including SQLite WAL/SHM,
  semantic notes, candidates and judge logs. Back up while stopped.

Without RCON, unset `CLYDE_RCON_ADDR`: inventory/arrival checks work from
the body card, but build/delivery goals cannot be verified. Do not present
that configuration as full predicate support. The existing `judged`
predicate is not implemented even with RCON; Jev is the interrupt judge,
not a substitute for that checker.

The body API is forced to container loopback port 7777. Publish **no** ports,
do not use host networking, and do not start HTTP MCP. There must be exactly
one service replica/controller for this identity/world/memory. CLI advisory
locks are local protections, not a distributed lease. Do not run another
`clyde run/mcp/serve` against its body while `serve` owns it.

## 4. Start, stop, restart

Only after isolated image verification and host authorization, in the host
Compose project (replace `-f` with the host's integrated Compose file):

```sh
export CLYDE_IMAGE="clyde:$revision"
docker compose -f deploy/compose.example.yaml --profile clyde up -d --no-deps clyde
docker compose -f deploy/compose.example.yaml logs --tail 80 clyde
# Reliable kill switch; leaves memory intact, does not restart Minecraft/proxies.
docker compose -f deploy/compose.example.yaml stop clyde
# Explicit attended restart; no auto-restart policy.
docker compose -f deploy/compose.example.yaml --profile clyde up -d --no-deps clyde
```

Wait for `body ready; starting persistent player`, then the `clyde serve:` model/memory
line. Body readiness requires a successful `/state` after Minecraft spawn,
not merely an open TCP listener. Unexpected exit of either child fails the
service and stops the other. SIGTERM cancels/joins the player first while
the body is still available for stop cleanup, then terminates the body.
The entrypoint allows 35 seconds for player shutdown and 5 for body exit;
Compose's 50-second grace plus `init: true` bounds cleanup and reaps children.

The initial allowance defaults to **240 durable reservations**, shared by
frontier inference, compaction and Jev attention ticks. Each Jev tick reserves
one conservatively even when deterministic prechecks avoid a model request.
Jev runs before frontier attention, not as paid idle polling; local hard
prechecks continue every 500ms. Frontier calls have a 45-second deadline and
4096 output-token cap; SDK retries are disabled for persistent play. Tool
dispatch has a 12-second deadline and body actions retain their existing
timeouts. Returned usage/cost and unknown billing are reported separately;
these are request/output limits, **not a hard dollar cap**. Exhaustion stops
owned work and pauses the same session. Restart/chat/compaction never refill it.

Use `command: [--request-limit, "240", --call-timeout, 45s,
--max-output-tokens, "4096"]` for initial configuration. The stored allowance
wins on restart. `--context-bytes` defaults to 131072 serialized active-history
bytes, not a token cap. Compaction retains the complete current exchange and
archives originals. New attention supersedes cancelled compaction without an
operator pause or refund; real provider/invalid-summary failure pauses with an
explicit reason, without retrying forever. Read status/history before deciding
to resume. Legacy serve flags
(`--max-autonomous`, `--planner-time`, `--planner-prompt`, `--max-steps`,
`--max-time`) are rejected; `run`/MCP retain their bounded development contracts.

Local operator controls require filesystem/container access. They are never
model tools or chat commands. On an authorized running host:

```sh
docker compose exec clyde clyde control status
docker compose exec clyde clyde control stop
docker compose exec clyde clyde control extend --requests 100
docker compose exec clyde clyde control resume
docker compose exec clyde clyde control compact
```

`stop` survives restart; `extend` does not clear it. `resume` clears the stop
and error pause but grants no requests. An in-flight request remains charged;
tools with no committed receipt remain uncertain after a crash and are not
replayed. Ordinary chat cannot resume or grant allowance. Body `Clyde stop`
chat/`POST /stop` still stops only an action; use operator control or stop the
whole service for a durable stop. Do not delete memory or use `down -v`.

### Upgrade and rollback require stopped whole-memory copies

Stop the old service and back up the entire memory directory, including
SQLite/WAL/SHM, semantic files, places, skill candidates and admission evidence.
Verify the backup before starting the new binary. New versioned `player_*`
tables preserve old episodes/places/checkpoints. Legacy project, dialogue and
pending jobs import once as **unverified context**, never commands to replay.
Historical executor transcripts do not exist; old recovery closures have no
authority over the new player. The native session journal must not be flattened
or edited by hand. Incompatible journal versions fail startup.

Rollback requires the **old image plus its matching stopped whole-memory
snapshot**; never run the old binary against upgraded memory alone. Keep the
new stopped snapshot separately for investigation. Upgrade body and harness
together for incarnation-safe action tokens. These source/runbook changes are
not rollout approval; host changes still require separate authorization through
`lox/selfhosted-ovh`. Production memory and worlds are not validation fixtures.

## Verification before a real-world launch

Use a separate disposable Paper instance/network/world with its own private
RCON; never production as an image test. No published body/MCP ports are
needed: use `docker exec` loopback requests/stdio tools. Preserve a transcript
of the image ID, model slots, gate scoreboard and these checks:

1. Normal image startup reaches spawned `/state` before `serve`. Confirm
   Sol 6.1 low for the single player, Jev, no host port bindings and writable memory.
2. In a **separate temporary container with entrypoint overridden**, start
   only the body, supply three oak logs on the disposable server, and use
   `clyde mcp` over stdio to call `find_skill` then
   `run_skill({"name":"craft-item","args":{"item":"oak_planks","count":9}})`.
   Independently check at least nine planks over RCON. Stop this container
   before the full service; do not compete with its persistent player.
3. Run full `serve`, give a modest player request (e.g. craft a table from
   supplied logs), check the outcome over RCON and the persisted milestone history.
   Stop/start with the **same memory volume**. Check old episode IDs/content,
   semantic/home data, native session history and usage remain, with no duplicate
   player reply. Exercise `control stop`, restart, ordinary chat (must not resume),
   then explicit `control resume`; confirm allowance is unchanged.
4. `docker stop --time 50` exits without a forced runtime kill; Minecraft
   `list` shows Clyde offline and no body remains. In disposable containers,
   also kill each child in turn and confirm the unit fails and its sibling
   disappears. Test TERM during readiness and unreachable Minecraft.
5. Missing Jev or a missing/stale receipt must fail clearly. Never bypass
   admission just to make a smoke pass. No gate/reset command runs at startup.

The source orb has Docker CLI but **no Docker daemon/socket** (and no
Compose plugin). Local packed-filesystem tests are not container evidence.
The host deployment owner must close actual image/build/Compose/lifecycle
checks against a disposable server before any Bonnie startup. Read the
slice 7 As built for the local evidence and remaining launch acceptance.
