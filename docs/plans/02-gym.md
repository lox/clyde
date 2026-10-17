# Slice 2: gym v0

Goal: a throwaway Paper server plus a task runner that sets up a scenario,
lets an agent play, checks a machine-checkable goal predicate, and prints a
scoreboard, so every later slice can show numbers instead of anecdotes.

## Done when

- `gym/bin/paper start` runs Paper (offline mode, seed `mcbot`, loopback
  only) with RCON enabled on 127.0.0.1:25575. The RCON password comes from
  `GYM_RCON_PASSWORD` with a fixed local default, and `.agents/setup` keeps
  `~/paper/server.properties` in sync with `gym/server.properties` instead
  of copying it only once.
- World templates: `gym/bin/paper reset` restores a pristine snapshot of the
  `mcbot` world, generated on first use and cached outside git (for example
  `~/paper-templates/mcbot`). Reset plus start is fast enough to run before every task;
  the commit message states the measured time.
- `api/goal.schema.json` defines a typed goal with an `until` predicate of
  kind `items`, `arrived`, `built`, or `delivered`. `judged` is reserved in
  the schema and rejected by the runner until slice 3's judge is wired in.
  Go types are generated from the schema (`go generate` in `harness/`);
  generated files are never hand-edited.
- `harness/goal` evaluates predicates against the live server through a
  small observer interface with an RCON implementation:
  - `items`: player has at least N of an item (e.g. via `clear <p> <item> 0`,
    which counts without removing).
  - `arrived`: player within R blocks of a point (`data get entity <p> Pos`).
  - `built`: every listed block matches (`execute if block`).
  - `delivered`: a container at a given position holds at least N of an item.
  Unit tests use a fake observer and cover both sides of each boundary
  (N-1 vs N items, R vs R+0.01 distance, one wrong block in a structure).
- `gym/tasks/*.yaml`: name, template, setup (RCON commands such as `tp`,
  `give`, `fill`, `time set`, `gamerule`), goal, timeout. At least four
  tasks, one per predicate, so every RCON path runs live: `collect-logs`
  (items), `goto-marker` (arrived), `place-crafting-table` (built), and
  `stock-chest` (delivered). The task file format is documented in
  `gym/tasks/README.md`.
- `gym/bin/run <task>|--all [--agent <cmd>]` resets the world, starts Paper
  if needed, runs setup, launches the agent with `GYM_TASK` (path to the
  task's goal as JSON), `MC_HOST`, `MC_PORT`, and `MC_USERNAME` in its
  environment, polls the predicate about once a second until it passes or
  times out, stops the agent, and prints a Markdown scoreboard (task, result,
  time to pass, timeout, agent exit status). Each run also writes JSON to
  `gym/runs/<timestamp>/` (gitignored).
- Two built-in agents prove the checkers in both directions without the
  body: `idle` joins as `MC_USERNAME` and stands still (every task fails at
  timeout), and `cheat` joins and then satisfies the goal over RCON (every
  task passes). Both scoreboards, run twice to show they are repeatable, are
  in the commit message.
- CI: a `gym` job in `.github/workflows/ci.yml` installs Java, downloads
  Paper, and runs `gym/bin/run --all --agent cheat`. It must pass on `main`.

## Starting point

`gym/bin/paper` (start, stop, status, logs, reset as an orb service) and
`gym/server.properties` exist from the spec thread. `.agents/setup` installs
Go 1.27, Temurin 25, and Paper 26.1.2 under `~/tools` and `~/paper`. There is
no `harness/go.mod` yet; create it as module `github.com/lox/clyde/harness`.
Slice 3 (judge) also creates it in parallel, so rebase onto whichever lands
first. CI already has a `harness` job that runs `go build`, `go vet`, and
`go test` once `harness/go.mod` exists.

## Notes

- `gym/bin/paper` uses `amp orb service`, which does not exist in CI. Let
  the runner fall back to a plain child process when not in an orb; keep one
  code path for everything else.
- The predicates in `harness/goal` are the same ones the executor will use
  as `until` conditions in slice 4. Keep them independent of the gym runner.
- Prefer RCON over a second observer bot: it needs no extra player slot and
  sees the server's truth, not a client's view.
- The `idle` and `cheat` agents need a minimal mineflayer client. Keep it to
  a few lines and pin it to protocol `26.1` like the body; do not duplicate
  the body's HTTP API.
- The agent contract (an executable plus environment variables) is what
  slice 4 plugs the body and executor into. Keep it that simple.
- Paper 26.1.2 speaks the 26.1 protocol the body is pinned to; no
  ViaBackwards in the gym.

## As built

- Layout: `harness/rcon` (minimal RCON client), `harness/goal` (generated
  types, `Evaluate`, `Observer`, and the `RCON` observer), `harness/gym`
  (task loading, runner, scoreboard, cheat commands), `harness/cmd/gym`
  (built by `gym/bin/run` into `gym/.bin/gym`), `gym/agents/` (`join.js`
  on mineflayer 4.39.0, `idle`, `cheat`). Go types are generated with
  `go-jsonschema`, pinned as a `tool` in `harness/go.mod`; CI checks that
  `go generate ./...` leaves no diff.
- Predicates are keyed by kind (`until: {items: {item, count}}`) rather
  than tagged with a `kind` field, because the generator turns that shape
  into one struct with a pointer per kind. The generator does not enforce
  "exactly one", so `goal.Kind` does. Item and block ids are pattern-checked
  in the schema and the RCON observer refuses any word containing
  whitespace, so a goal file cannot inject console commands.
- RCON replies (Paper 26.1.2): `clear <p> <item> 0` → `Found N matching
  item(s)` / `No items were found` / `No player was found`; `data get
  entity <p> Pos` → `[x d, y d, z d]` / `No entity was found`; `execute if
  block` → `Test passed` / `Test failed`; `delivered` uses `execute if items
  block <pos> container.* <item>` → `Test passed. Count: N` / `Test failed`
  / `… is not a container`. Errors come back as ordinary reply text, so
  setup treats parse errors, unloaded positions, and missing players as
  failures; `No blocks were filled` and `Could not set the block` mean
  "unchanged" and are not.
- Setup runs after the agent's player joins, because `tp`, `give`, and
  `clear` need them online and chunks are only loaded near players (tasks
  start with `forceload add`). The runner then creates the file named by a
  fifth variable, `GYM_READY`, and starts the clock; agents should act once
  it exists. The cheat agent also gets `GYM_RCON_ADDR` and
  `GYM_RCON_PASSWORD`. A task whose goal already holds after setup is an
  `error`, so a broken task cannot pass under `cheat` in CI unnoticed.
- `gym/bin/paper install` downloads Paper and renders
  `~/paper/server.properties` from `gym/server.properties` (password from
  `GYM_RCON_PASSWORD`, default `clyde-gym`); `.agents/setup` and `start`
  both call it. `start` blocks until RCON is up. Outside an orb (no `amp
  orb service`) it runs Paper as a background process with a pidfile;
  everything else is the same code path.
- Paper 26.1 keeps all dimensions in one `world/` directory, so the
  template is a copy of it. Measured in an orb (4 vCPU): generating the
  template 26 s once; reset plus start 13.4–18.3 s per task over 16 runs (about 3 s
  graceful stop, 12 s boot).
- Seed `mcbot` spawns on a meadow sloping east to a lake with no trees
  within 48 blocks, so `collect-logs` places two 3-log oak trunks, and the
  arena tasks flatten an 11×11 area on the plateau west of spawn.
  `goto-marker` crosses natural terrain, about 22 blocks uphill.
- Runner extras: `--timeout` overrides every task's timeout, the default
  agent is `idle`, and the exit status is non-zero unless every task
  passes. Task YAML is decoded as YAML 1.2 (`go.yaml.in/yaml/v3`) and then
  as JSON so the generated validation runs; YAML 1.1 decoders read the key
  `y` as `true`.
- The cheat agent fills containers in stacks of 64 and cannot satisfy
  `#tag` items or blocks; no shipped task needs either.

## Not in this slice

The body, the judge, `judged` predicates, an executor-driven agent, world
templates committed to git, and Bedrock.
