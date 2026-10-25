# Full Clyde, simple private deployment

One container runs the compiled Node body and existing `clyde serve`: Sol 6.1
low planner **and** executor, Jev judge, SQLite/semantic/spatial memory, and
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
- Models remain configurable independently through
  `CLYDE_PLANNER_{PROVIDER,MODEL,EFFORT}` and
  `CLYDE_EXECUTOR_{PROVIDER,MODEL,EFFORT}`. Both default to OpenAI
  `gpt-6.1-sol`, `low`. No packaging-specific cap or fallback model.
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

Wait for `body ready; starting planner`, then the `clyde serve:` model/memory
line. Body readiness requires a successful `/state` after Minecraft spawn,
not merely an open TCP listener. Unexpected exit of either child fails the
service and stops the other. SIGTERM cancels/joins the planner first while
the body is still available for stop cleanup, then terminates the body.
The entrypoint allows 35 seconds for planner shutdown and 5 for body exit;
Compose's 50-second grace plus `init: true` bounds cleanup and reaps children.

Existing default budgets remain: 40 executor turns, 5 minutes per goal,
30 seconds per planning event and 10 durable autonomous completed goals.
Use the Compose `command` array to override existing serve flags, e.g.
`[--max-autonomous, "20", --max-time, 10m]`. A restart does not reset the
autonomous cap. This is not a hard dollar cap; player events/post-mortems
can make more calls. `Clyde stop` chat/`POST /stop` stops an action only:
the scheduler can act again. **Stop the entire service to keep Clyde
stopped.** Chat latching is deferred. Do not delete the memory volume to
restart, and do not use Compose `down -v`.

Slice 13 keeps action failures and applicable judge `stuck` stops inside that
same executor conversation and 40-turn/300-second allowance. Danger still
interrupts the execution. A stuck stop uses the observed action's opaque
`token`, not its restartable numeric `id`: `POST /action` with
`{"type":"stop","token":"<observed owner>"}` returns `stopped:null` without
touching controls on a mismatch. Unconditional `POST /stop` is unchanged.

Upgrade the body and harness together: new action statuses require tokens;
older bodies cannot provide incarnation-safe ownership. The twelve local
collision cells report actual contact shapes/unknown cells, not a resource or
reachability scan. Recovery checkpoint additions are additive: old closed
episodes retain unknown closure provenance, not retroactively labelled
exhaustion. Safe ordinary player work remains admissible while autonomy is
paused; merely accepting a request never refills recovery or unpauses autonomy.
Only successful execution plus a fresh safe observation resets the episode.
These source changes do not authorize a production image update. Any later
rollout still requires the host's stopped whole-memory backup and separately
approved image/lifecycle verification; keep a matching old image and memory
snapshot for rollback.

## Verification before a real-world launch

Use a separate disposable Paper instance/network/world with its own private
RCON; never production as an image test. No published body/MCP ports are
needed: use `docker exec` loopback requests/stdio tools. Preserve a transcript
of the image ID, model slots, gate scoreboard and these checks:

1. Normal image startup reaches spawned `/state` before `serve`. Confirm
   Sol 6.1 low for both slots, Jev, no host port bindings and writable memory.
2. In a **separate temporary container with entrypoint overridden**, start
   only the body, supply three oak logs on the disposable server, and use
   `clyde mcp` over stdio to call `find_skill` then
   `run_skill({"name":"craft-item","args":{"item":"oak_planks","count":9}})`.
   Independently check at least nine planks over RCON. Stop this container
   before the full service; do not compete with its planner.
3. Run full `serve`, give a modest player request (e.g. craft a table from
   supplied logs), check the outcome over RCON and the persisted episode.
   Stop/start with the **same memory volume**. Check old episode IDs/content,
   semantic/home data and usage remain, with no duplicate player reply.
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
