# Gym tasks

Each `*.yaml` file here is one task: a scenario set up over RCON, a goal with
a machine-checkable `until` predicate, and a timeout.

```sh
gym/bin/run collect-logs                 # one task, default agent (idle)
gym/bin/run --all --agent cheat          # every task
gym/bin/run goto-marker --agent ./my-agent --timeout 30s
gym/bin/run --all --agent executor --runs 3   # the executor; needs OPENAI_API_KEY
```

The runner prints a Markdown scoreboard (per task: pass rate, median time
to pass, and tokens and cost per run when the agent reports them; then one
row per run) and writes `results.json`, `scoreboard.md`, and per-task
`goal.json` and `agent.log` under `gym/runs/<timestamp>/` (gitignored;
`<task>.<n>/` with `--runs`). It exits non-zero unless every task passes.

## Format

```yaml
name: stock-chest          # must match the file name
template: mcbot            # world snapshot restored before the task
timeout: 120s              # Go duration; --timeout overrides it
setup:                     # console commands, run in order after the agent joins
  - forceload add -330 140 -290 165
  - time set day
  - setblock -307 79 151 minecraft:chest[facing=west]
  - give {player} minecraft:oak_log 8   # {player} is MC_USERNAME
goal:                      # api/goal.schema.json
  description: Put all 8 of your oak logs into the chest in front of you.
  until:
    delivered:
      container: {x: -307, y: 79, z: 151}
      item: minecraft:oak_log
      count: 8
```

- `template`: only `mcbot` exists (seed `mcbot`, generated on first use and
  cached in `~/paper-templates/mcbot`).
- `setup`: commands are written without a leading slash. A reply that looks
  like an error (parse error, unloaded position, no such player) aborts the
  task with result `error`. Chunks are only loaded near players, so start
  with `forceload add` for any area you touch. Setup runs after the player
  joins because `tp`, `give`, and `clear` need them online.
- `goal.until` has exactly one predicate:
  - `items: {item, count}`: the player holds at least `count` (`item` may be
    a `#tag`).
  - `arrived: {at: {x, y, z}, radius}`: the player's feet are within
    `radius` blocks (inclusive, 3D).
  - `built: {blocks: [{pos: {x, y, z}, block}]}`: every block matches
    (`block` may carry states such as `[facing=north]`, or be a `#tag`).
  - `delivered: {container: {x, y, z}, item, count}`: the container holds at
    least `count`.
  - `judged` is reserved and rejected until the judge is wired in.
- A task whose goal already holds after setup is reported as `error`.

## Agent contract

An agent is any executable. `--agent idle` and `--agent cheat` run the
built-ins in `gym/agents/`; anything else runs with `bash -c` from the repo
root. The runner starts it in its own process group with:

| variable | meaning |
|----------|---------|
| `GYM_TASK` | path to the task's goal as JSON |
| `GYM_READY` | path that exists once setup has finished; start acting then |
| `MC_HOST`, `MC_PORT` | the gym server, `127.0.0.1:25565` |
| `MC_USERNAME` | the player to join as (`clyde`) |
| `GYM_RCON_ADDR`, `GYM_RCON_PASSWORD` | the console, for test agents such as `cheat` and for checking goals |
| `GYM_OUTCOME` | optional: write `{"model", "tokens": {"total"}, "cost_usd"}` JSON here for the scoreboard |

The agent must join as `MC_USERNAME` within 60 s. The runner polls the goal
about once a second; when it holds, the task times out, or the agent exits,
the runner sends SIGTERM to the group (SIGKILL after 5 s).

Built-in agents prove the checkers without the body: `idle` joins and stands
still, so every task fails at its timeout; `cheat` joins and satisfies the
goal over RCON once `GYM_READY` exists, so every task passes. `executor`
starts the body, waits for `GYM_READY`, and runs `clyde run` on the goal
with the model from `CLYDE_EXECUTOR_MODEL` (see `harness/README.md`); its
judge log and outcome land in the task's run directory.
