# Clyde roadmap

Clyde is an AI-driven Minecraft player. Frontier models supply the intelligence;
the code supplies a body, a cheap reflex layer, a skill library, memory, and a
gym to measure progress. Capability should come from swapping in better models
and accumulating skills and memory, not from hand-coded strategy.

This file is the source of truth for scope, architecture, and sequencing. Slice
plans live beside it as `docs/plans/NN-name.md`. Update this file when a
decision changes; do not let it drift from the code.

## Goals

1. Play as a real player on Bonnie's Paper server: survive, gather, build, trade,
   follow instructions in chat, and get noticeably better over time.
2. Everything the agent does must be measurable in a throwaway gym server so
   that model, prompt, and skill changes can be compared.
3. Every model slot is pluggable. Start with Jev (typesafe.ai) for fast
   judgement and OpenAI models for reasoning; swap any of
   them by config.
4. Operate safely on a shared world: hard limits in the body, a kill switch, and
   no griefing.

## Non-goals (for now)

- Microsoft-account authentication. Clyde joins as an offline-mode player
  through a proxy we control (see Deployment).
- Training our own vision or policy models. Observations are symbolic (blocks,
  entities, inventory), not pixels.
- Multi-agent play. One Clyde at a time until the single-agent loop is good.

## Architecture: four tiers

```
┌──────────────────────────────────────────────────────────────────────┐
│ Tier 3  Planner (frontier model, slow, event-triggered)              │
│   goals, curriculum, post-mortems, skill authoring, memory curation   │
├──────────────────────────────────────────────────────────────────────┤
│ Tier 2  Executor (Go, mid model, stateless per goal, tool loop)      │
│   tools: observe, act, eval_js, find_skill, recipes_for, lookup       │
│   also an MCP server so Amp threads can drive the bot directly        │
├──────────────────────────────────────────────────────────────────────┤
│ Tier 1  Judge (Go, Jev or other System-1 backend, 2–4 Hz)            │
│   danger? stuck? progressing? interrupt? — on a structured state card │
├──────────────────────────────────────────────────────────────────────┤
│ Tier 0  Body (TypeScript, mineflayer, HTTP API)                      │
│   GET /state, POST /action, POST /eval, GET /events; hard safety      │
└──────────────────────────────────────────────────────────────────────┘
```

### Tier 0: body (`body/`)

mineflayer bot wrapped in a small HTTP server. The body owns everything that
must be fast and deterministic: pathfinding, block interaction, inventory,
combat reflexes, and hard safety limits.

- `GET /state`: the state card as JSON (position, health, hunger, time, biome,
  nearby notable blocks, entities classified hostile/passive/player, inventory,
  current action and its progress, recent events).
- `POST /action`: one of `goto`, `follow`, `collect`, `dig`, `place`, `chat`,
  `eval`, `stop`. A new action aborts the current one. Actions report progress
  deltas (distance remaining, items collected) rather than deciding they are
  stuck; the judge decides that.
- `POST /eval`: run JavaScript against the bot with a timeout. This is
  code-as-action for anything without a primitive, and the mechanism skills use.
- `POST /query`: the same, but beside the running action instead of
  replacing it, for read-only lookups such as recipes.
- `GET /events`: ring buffer of chat, damage, deaths, item pickups, action
  start/finish, with `since` cursor.
- Hard safety lives here, not in prompts: never break blocks inside protected
  regions, never attack players, hard cap on blocks broken per minute, and a
  kill switch (`POST /stop` plus a chat command for ops).

### Tier 1: judge (`harness/judge`)

A ticking loop that asks a cheap, fast model a fixed set of questions about
the structured state card and emits interrupts. It never chooses actions; the
herobrine benchmarks show action-menu confidence is poor (0.48 over 28
options) while yes/no and scored judgements are cheap and reliable.

- Contract: `choice`, `score`, `noul` questions (Jev's native shapes).
- Backends: `jev` (typesafe.ai `POST /v1/systemone`), `logprob` (any
  OpenAI-compatible model with logprobs), `classifier` (fine-tuned local model,
  later), `replay` (fixture-driven, for tests).
- Cheap code pre-checks run first every tick (health below threshold, hostile
  within N blocks, no progress for M seconds); the model is asked only when
  code cannot decide.
- Every tick is logged as JSONL (state card, questions, answers, latency,
  cost) so a classifier can later be distilled from Jev's labels.

### Tier 2: executor (`harness/executor`)

Stateless per goal. Receives a typed goal, runs a tool loop with a
mid-tier model, and returns when the goal's `until` predicate holds or the
judge interrupts. Tools: `observe`, `act`, `eval_js`, `find_skill`,
`recipes_for`, `lookup` (minecraft-data queries). No recipe prerequisites
are hand-coded; the model asks `recipes_for` and reasons.

The executor is also exposed as an MCP server so an Amp thread can drive
Clyde interactively during development.

### Tier 3: planner (`harness/planner`)

A frontier model invoked on events (goal finished, goal failed, player
request, death, long idle), not on a cadence. It writes goals, maintains the
curriculum, runs post-mortems on failures, authors and revises skills, and
curates `memory/*.md`.

### Priority lanes

Two lanes feed the executor: `player` (chat instructions, danger) preempts
`autonomous` (planner goals). A preempted autonomous goal is re-queued, not
lost.

## Contracts (`api/`)

- One JSON Schema for the state card and for actions in `api/`, generating TS
  types for the body and Go types for the harness. Jev receives structured
  JSON; LLMs receive a rendered text card from the same data.
- Typed goals with machine-checkable `until` predicates: `items`, `delivered`,
  `arrived`, `built`, `judged`. The same predicates are the gym checkers.

## Skills (`skills/`)

`skills/<name>/{skill.md,index.js,test.yaml}`. `skill.md` is the
model-facing description (when to use, preconditions, failure modes);
`index.js` runs via `POST /eval`; `test.yaml` is a gym task that must pass
before the skill is admitted. The planner authors skills; the gym gates them.

## Memory (`memory/`, runtime data)

- Episodic: SQLite, written by the harness (goals, outcomes, deaths, chat).
- Spatial: maintained by code, not the model (POIs, last-seen players, home,
  resource locations).
- Semantic: `memory/*.md` curated by the planner and injected into prompts.

## Gym (`gym/`)

Throwaway Paper server (offline mode, fixed seed, pre-generated world
templates) plus `gym/tasks/*.yaml` with checkers. Produces a scoreboard per
run so slices and PRs can show numbers. Runs in CI and in orbs.

## Deployment (`deploy/`)

Production target is Bonnie's server in `lox/selfhosted-ovh`. That repo only
gains a compose service for Clyde and ViaBackwards on Paper; everything else
is here.

Clyde joins without a Microsoft account. Design (accepted, implemented in a
separate `selfhosted-ovh` thread):

- Velocity proxy in front of Paper, `online-mode=true`, with Geyser-Velocity
  and Floodgate-Velocity for Bedrock players. Paper runs `online-mode=false`
  with modern Velocity forwarding and no published ports.
- Because Velocity forwarding rejects direct connections, Clyde joins through
  a second, internal, offline-mode Velocity that shares the forwarding secret.
  Alternative: the body implements the Velocity handshake itself.
- Floodgate is required only on the proxy; backend Floodgate is optional.

Clyde is an opt-in compose profile with its own kill switch.

## Learnings adopted from herobrine (xatuke/herobrine)

- Jev: p50 398 ms, p95 580 ms, about 2,350 tokens and $0.0001 per call; flat
  under three agents. Good enough for a 2–4 Hz judge. Our slice 3 measurement
  (30 calls on state-card fixtures, 2026-10-17): p50 76 ms, p95 126 ms,
  1,431 input tokens, $0.00006 per call.
- Action timeouts dominate wall time (mine p95 90 s, goto p95 45 s). Hence:
  progress deltas from the body and a judge that calls "stuck" early.
- Planner latency 5–11 s p50 and a FIFO queue delayed chat replies. Hence:
  priority lanes and event-triggered planning.
- Jev as an action picker had low confidence. Hence: Jev judges, LLMs act.
- Machine-checkable goal predicates shared with the benchmark harness worked
  well. Adopted as `until` predicates.

## Delivery slices

Each slice is one reviewable unit with its own plan file and gym evidence.

| # | Slice | Plan | Depends on |
|---|-------|------|------------|
| 0 | Bootstrap: repo, docs, setup, CI skeleton | this file | – |
| 1 | Body v0: HTTP API, actions, eval, events, safety (done, `4f50deb`) | `01-body.md` | 0 |
| 2 | Gym v0: Paper runner, world templates, tasks, scoreboard (done, `1a33297`) | `02-gym.md` | 0 |
| 3 | Judge v0: contract, jev + replay backends, pre-checks, JSONL (done, `53778fc`) | `03-judge.md` | 0 |
| 4 | Executor + MCP server (done) | `04-executor.md` | 1, 2, 3 |
| 5 | Skills: loader, admission, five skills, multi-step gym tasks (done, `4c94e9a`) | `05-skills.md` | 4 |
| 6 | Planner + memory (foundations shipped; integration/live demo in progress) | `06-planner.md` | 4, 5 |
| 7 | Production: proxy join, regions, kill switch, compose profile | `07-production.md` | 4 |
| 8 | Judge distillation: classifier backend from JSONL logs | `08-distill.md` | 3, 6 |

Slices 1, 2, and 3 are independent and run as parallel threads.

## Decisions log

Settled with the user unless marked default. Defaults are the coordinator's
choice and can be reversed by saying so.

- Name: Clyde. Repo: `lox/clyde`, public, MIT.
- Harness in Go.
- Body stays TypeScript on mineflayer for slices 5 and 6 (chosen by Lachlan).
  JavaScript skills use its bot API. The mc26-kit Paper spike continues
  separately; it does not block skills or planner/memory delivery.
- Slices 5 and 6 start together: slice 6 prepares planner/memory in separate
  files, then integrates after slice 5 ships its skill contract. No competing
  skill loader or concurrent edits to executor/CLI wiring (default).
- Judge: Jev first, behind a backend interface.
- LLM providers: OpenAI first (Responses API), Anthropic second, behind one
  interface in `harness/llm`. Lachlan prefers OpenAI models.
- Executor tool loop: `github.com/lox/agent-harness` (ours, MIT), behind
  `harness/llm` and `harness/executor`. Changes to it are delegated to a
  thread in that repo and pinned here; no fork or vendoring.
- Executor model: `gpt-6.1-sol` at low reasoning effort, the default in
  `harness/llm` (chosen by Lachlan, 2026-10-17). On the four gym tasks,
  three runs each, `gpt-6.1-sol` and `gpt-6-astra` both passed 12/12 with
  one tool call per task and about the same median times (collect-logs
  21 s and 22 s, goto-marker 8 s, place-crafting-table 3 s and 4 s,
  stock-chest 3 s and 4 s); Sol cost $0.0009–0.0013 per task, Astra
  $0.0055–0.0101. These tasks no longer separate models, so harder
  multi-step tasks come before any change of model. Slice 5 adds them
  (for example logs → planks → crafting table → place), since skills need
  tasks that one tool call cannot solve to show they help (default).
- Executor outcomes are `succeeded`, `failed`, `interrupted`,
  `budget_exhausted`, and `cancelled` (the caller cancelled the run), and
  every run ends with `POST /stop` (default).
- The executor checks goals over RCON when it has it (the gym, a dev
  server); without RCON the bot's own card answers `items` and `arrived`
  only (default).
- Code an aborted action started (eval code, plugin loops) cannot set
  pathfinder goals or dig; the body tracks each action's async context to
  refuse it (default).
- Issue tracking: GitHub issues on this repo (default).
- Delivery: commit to `main` (Amp merge-to-main), no pull requests; Auto-review
  runs locally before every ship; no Codex.
- Minecraft version: body pinned to 26.1 (mineflayer 4.39.0 ceiling); Paper
  26.2 servers need ViaBackwards.
- No Microsoft account; Velocity-based join (see Deployment).
- Contract ownership for the parallel slices: slice 1 owns
  `api/state.schema.json`, slice 2 owns `api/goal.schema.json`. Harness Go
  module is `github.com/lox/clyde/harness` (default).
- CI: one GitHub Actions workflow, `.github/workflows/ci.yml`, with a job per
  component; slices extend it rather than adding workflows (default).
- Go YAML library is `go.yaml.in/yaml/v3`; use it everywhere in `harness/`
  (default).
- Body event types the judge treats as needing attention: `chat`,
  `whisper`, `damage`. The body emits those names (default).
- The judge flags new chat only on the first tick that sees it; the executor
  reads chat it must answer from `GET /events`, not from judge interrupts
  (default).
- No x-ray: the state card lists only blocks exposed to air, so Clyde knows
  what a player could see (default).
- The chat kill switch latches until an op resumes it; slice 7 adds the
  latch and a resume command. Until then a stop lasts only until the next
  action (default).
- The body exits on disconnect and its supervisor restarts it; no in-process
  reconnect (default).
- Gym observes the server over RCON, not through a second bot, and agents
  plug in as an executable plus environment variables (default).
- CI regenerates the gym world template each run (about 25 s); no
  actions/cache until gym runs get slow enough to matter (default).

## Open questions

- Whether to run a GPU box for a distilled judge (OVH has none in Sydney; AWS
  g6 in ap-southeast-2 or a home GPU on the Tailnet are the candidates).
