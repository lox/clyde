# Reusable JavaScript skills

Each `<name>/` contains exactly three source files. Names match
`^[a-z][a-z0-9]*(-[a-z0-9]+)*$`; paths and symlinked source files are refused.
The five initial bodies and descriptions were authored/refined by
`gpt-6.1-sol`, low effort, through `harness/llm`, using live gym feedback.

## Format and invocation

- `skill.md`: model-facing Markdown describing purpose/search terms, JSON
  inputs, preconditions, runtime bindings, returned result, and failure modes.
  `find_skill({query})` ranks admitted matches and returns their full descriptions.
- `index.js`: the **body of an async function**, not a module. `run_skill`
  prepends `const args = <JSON object>;` and sends the captured code to the
  body's existing `/eval` action. No import/require or module loader.
- `test.yaml`: a normal [gym task](../gym/tasks/README.md), named `test`,
  with the additional invocation below. The goal is checked independently
  over RCON, not against the skill's return value or an LLM's claim.

```yaml
name: test
template: mcbot
timeout: 60s
setup: # RCON prerequisites, never part of skill execution
  - clear {player}
  - give {player} minecraft:oak_log 3
skill:
  args: {item: oak_planks, count: 9}
  timeoutMs: 30000 # optional; body default 60000, max 600000
goal:
  description: Craft at least nine oak planks.
  until:
    items: {item: minecraft:oak_planks, count: 9}
```

Executor and MCP share these tools and the `harness/skills.Store` loader:

```json
{"name":"craft-item","args":{"item":"oak_planks","count":9},"timeoutMs":30000}
```

Runtime bindings are `bot` (mineflayer, pathfinder and collectblock), `mc`
(bot registry), `Vec3`, `goals`, `Movements`, `goto(goal)`, `sleep(ms)`,
`log(...)`, and `signal`. Return a JSON-serializable value; throw on failure.
Use `signal.throwIfAborted()` in loops and abort-aware `goto`/`sleep`.
One invocation replaces the current body action. Body safety wrappers still
guard digging, placement, player attacks, and the break cap. Request
cancellation stops the body action; eval completion/timeout expires its
movement/dig ownership too. Skills must not leave plugin work running.
This is **trusted model code, not a security sandbox**: raw packets or
arbitrary bot access can bypass conventions. `/query` is read-only by
convention, not enforcement. Never overwrite the guarded Movements.

## Admission on a fresh checkout

```sh
gym/bin/gate-skills                      # build clyde; gate all five, no model/key
gym/bin/gate-skills craft-item           # reset and retest this installed version
```

Paper must be installed (`gym/bin/paper install`); body dependencies are
installed by the gym agent if absent. Each gate resets the throwaway world,
starts the body, waits for `GYM_READY`, and invokes a frozen candidate snapshot.
Only the independent goal passing admits it, and only if its current three
files still match that snapshot. The gate does not call any model/provider.
It returns nonzero if any gate fails and writes results under `gym/runs/`.

The local `<name>/.admission.json` receipt contains `sha256`, `passed_at`,
and an `evidence` run path/detail. SHA256 covers each filename, NUL, decimal
byte length, NUL, and contents, in order `skill.md`, `index.js`, `test.yaml`.
Any edit invalidates admission immediately. Receipts are gitignored at
**any root** and are not shipped; rerun gates after checkout or updates.
Discovery silently omits unadmitted/incomplete skills; execution returns a
tool error for missing, invalid, or unadmitted names. Retesting an installed
version revokes its old receipt even if the new run fails.

## Stage, gate, then explicitly promote

Never edit a working installed skill in place to test a revision. Stage the
same three-file layout under a **separate root**, for example
`memory/skill-candidates/craft-item/`, then:

```sh
gym/bin/gate-skills --dir memory/skill-candidates craft-item
gym/.bin/clyde skill promote --from memory/skill-candidates --dir skills craft-item
```

A failed gate only revokes the candidate's receipt. Promotion requires an
admitted candidate with unchanged files and copies the tested snapshot and
receipt through a staged replacement; an invalid candidate never touches the
live installation. Serialize gates/promotions for the same name and gym
instance. This is a local development gate, not a concurrent/hostile registry.

Package entry points for other harness callers:

- `skills.Store{Dir}.LoadCandidate(name)` returns a captured `Skill` with
  `Name`, `Description`, `Code`, `Test`, and `Digest` (gate-only bypass).
- `Store.Load`, `Find`, and `Run(ctx, bodyClient, name, jsonArgs, timeoutMs)`
  require admission. `Skill.Run` executes a captured candidate snapshot.
- `gym.Gate(ctx, store, name, gym.Runner)` returns `(gym.Result, error)`;
  use the repository's `gym/agents/executor` as Runner.Agent. A failed world
  goal is a non-pass Result, not necessarily a Go error: check **both**.
- `liveStore.Promote(candidateStore, name)` publishes a passing revision.
- `Store.RecordGate` is receipt plumbing for the gym, not a manual promotion
  API. Only feed it independent checker results.

## Isolate candidate worlds from the live planner

`PAPER_DIR` alone is **not isolation**. Use distinct service names, Minecraft
and RCON ports, world/template directories, and body ports. The same existing
Paper lifecycle supports all of them; no second server runner is needed.

For example, if the live world uses `paper-live`, ports 25566/25576, body
7788, and `$HOME/paper-live{,-templates}`, run candidate gates in a separate
process with:

```sh
PAPER_DIR="$HOME/paper-candidates" \
GYM_TEMPLATES="$HOME/paper-candidate-templates" \
GYM_PAPER_NAME=paper-candidates \
GYM_MC_PORT=25567 GYM_RCON_PORT=25577 BODY_LISTEN_PORT=7789 \
gym/bin/gate-skills --dir memory/skill-candidates craft-item
```

All five overrides are essential when running beside a live instance (plus
the body port). `GYM_PAPER_MODE` selects `orb` or `process`; it defaults to a
supervised orb service when available. In-process callers can put these
`KEY=value` settings in `gym.Runner.Env`, which overrides inherited values
for Paper, RCON dialing, and the agent without changing process-global env.
Defaults remain the disposable `paper` gym at 25565/25575, body 7787,
`$HOME/paper` and `$HOME/paper-templates`. Never point a gate at a world
whose contents/player state you need to keep.
