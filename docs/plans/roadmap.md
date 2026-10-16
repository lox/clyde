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
   judgement and Anthropic/OpenAI-compatible models for reasoning; swap any of
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
  under three agents. Good enough for a 2–4 Hz judge.
- Action timeouts dominate wall time (mine p95 90 s, goto p95 45 s). Hence:
  progress deltas from the body and a judge that calls "stuck" early.
- Planner latency 5–11 s p50 and a FIFO queue delayed chat replies. Hence:
  priority lanes and event-triggered planning.
- Jev as an action picker had low confidence. Hence: Jev judges, LLMs act.
- Machine-checkable goal predicates shared with the benchmark harness worked
  well. Adopted as `until` predicates.

## Delivery slices

Each slice is one PR-sized unit with its own plan file and gym evidence.

| # | Slice | Plan | Depends on |
|---|-------|------|------------|
| 0 | Bootstrap: repo, docs, setup, CI skeleton | this file | – |
| 1 | Body v0: HTTP API, actions, eval, events, safety | `01-body.md` | 0 |
| 2 | Gym v0: Paper runner, world templates, tasks, scoreboard | `02-gym.md` | 0 |
| 3 | Judge v0: contract, jev + replay backends, pre-checks, JSONL | `03-judge.md` | 0 |
| 4 | Executor + MCP server | `04-executor.md` | 1, 2, 3 |
| 5 | Skills: format, loader, gym gate, first five skills | `05-skills.md` | 4 |
| 6 | Planner + memory | `06-planner.md` | 4, 5 |
| 7 | Production: proxy join, regions, kill switch, compose profile | `07-production.md` | 4 |
| 8 | Judge distillation: classifier backend from JSONL logs | `08-distill.md` | 3, 6 |

Slices 1, 2, and 3 are independent and run as parallel threads.

## Decisions log

Settled with the user unless marked default. Defaults are the coordinator's
choice and can be reversed by saying so.

- Name: Clyde. Repo: `lox/clyde`, private (default).
- Body in TypeScript on mineflayer; harness in Go.
- Judge: Jev first, behind a backend interface.
- LLM providers: Anthropic and OpenAI-compatible first, behind one interface
  (default).
- Issue tracking: GitHub issues on this repo (default).
- Review: Amp auto-review on every PR; no Codex (default).
- Minecraft version: body pinned to 26.1 (mineflayer 4.39.0 ceiling); Paper
  26.2 servers need ViaBackwards.
- No Microsoft account; Velocity-based join (see Deployment).

## Open questions

- Which mid-tier model for the executor loop once gym numbers exist.
- Whether to run a GPU box for a distilled judge (OVH has none in Sydney; AWS
  g6 in ap-southeast-2 or a home GPU on the Tailnet are the candidates).
