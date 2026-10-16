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
  the PR states the measured time.
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
  in the PR.
- CI: a `gym` job in `.github/workflows/ci.yml` installs Java, downloads
  Paper, and runs `gym/bin/run --all --agent cheat`. It must pass on the PR.

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

## Not in this slice

The body, the judge, `judged` predicates, an executor-driven agent, world
templates committed to git, and Bedrock.
