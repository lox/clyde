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

- Giving Clyde a Microsoft account. Clyde joins as an offline-mode player
  through a proxy we control; human Java players authenticate normally
  (see Deployment).
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
  `eval`, `stop`. Chat sends beside current work and preserves its status;
  other new actions abort the current one. Actions report progress
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
curates `memory/*.md`. Bounded addressed dialogue and one durable model-owned
project intent accompany goal context. A read-only planner call may answer
beside the exclusive executor; the scheduler serializes accepted decisions.
Models choose whether to preserve or replace work. Replacement waits for joined
cleanup; stale decisions are discarded with bounded replanning. Project intent
is not verified progress, and reply intents guarantee at-most-once attempts,
not delivery.

### Priority lanes

Two lanes feed the executor: `player` work has priority over `autonomous`
(planner goals). An addressed chat turn first goes to the model: conversation
can preserve running work, a new goal can suspend it, and explicit replacement
discards superseded work. Suspended goals are re-queued, not lost. Danger still
preempts ordinary execution; bounded recovery uses its own judge context.

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

Initial live target is a separate persistent Clyde play world on media-ovh,
configured in `lox/selfhosted-ovh` (PR 11). Lachlan chose to interact with
Clyde there before considering Bonnie. Full Clyde connects directly to
private offline Paper with ViaBackwards; Bedrock/iPad joins through that
world's Geyser/Floodgate native HTTPS/NetherNet route on port 19134. World,
memory and stable private RCON credentials are independent of Bonnie.
Bonnie and its public proxies are not part of this rollout.

Clyde source `7700696` is deployed there through `lox/selfhosted-ovh` PR 12.
The host verified the image, memory continuity and one live model reply;
real owner client gameplay remains unverified. Lachlan uses Java Edition,
not Bedrock. Slice 12 adds Clyde-only authenticated public and unpublished
offline-bot Velocity proxies to the same world; until cutover, human access
remains Bedrock-only. See slices 7/12 and the host stop/rollback runbook.

The previously implemented Bonnie route remains available for a later,
separately approved deployment, without a Microsoft account:

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
| 6 | Planner + memory (done, `a567305`) | `06-planner.md` | 4, 5 |
| 7 | Simple deployment: separate play world live; real iPad join pending | `07-production.md` | 4 |
| 8 | Judge distillation: classifier backend from JSONL logs | `08-distill.md` | 3, 6 |
| 9 | Harder gym curriculum and evidence-led capability improvements (done, `0787b89`) | `09-capability-curriculum.md` | 5, 6 |
| 10 | Model-led recovery and judge evidence (done, `b2755c1`) | `10-model-led-recovery.md` | 6, 9 |
| 11 | Multi-turn conversation and remembered projects (done, `c2d7442`) | `11-conversation-projects.md` | 6, 10 |
| 12 | Authenticated Java access to the existing Clyde world (in progress) | `12-java-access.md` | 7 |

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
- Initial deployment: full planner/executor/judge/memory/skills on media-ovh,
  one private opt-in service. Simplify operations, not bot intelligence
  (chosen by Lachlan). Stop the whole service for a reliable initial kill
  switch; chat-stop latching is deferred.
- Initial play takes place in a fresh separate persistent world, not Bonnie
  (chosen by Lachlan). No protected regions are invented for this gathering/
  building world. Host rollout and a console-requested crafting goal passed;
  real owner iPad authentication/gameplay remains unverified. Improvements
  now target harder isolated gym tasks, not more deployment hardening or
  experiments on the live world. Judge distillation is deferred behind this
  capability work; slice numbers do not require doing slice 8 first.
- First harder-gym iteration accepted at `0787b89`, CI green: revised
  model-authored table crafting and body-owned confirmed-placement progress.
  Base tasks remain 9/9, variants 8/9; pickaxe median 137s to 78s, shelter
  76s to 89s. No general pass-rate or latency gain. The next bounded
  investigation is stone targeting/navigation versus early judge judgement;
  intermittent state-read deadlines and the historical live gather stall
  remain unresolved. Live image/world were not updated by this slice.
- Oracle review confirmed recent skill/body changes are correctly placed,
  but identified scheduler policy that discards recovery goals and blocks
  responses to persistent danger. Lachlan requested correction. Slice 10
  moves choice of recovery/idle to the models while retaining durable bounded
  attempts, body safety and exclusive ownership in code. It also investigates
  factual temporal judge context before changing calibration. No hard-coded
  Minecraft recovery tactics or live deployment are part of this iteration.
- Recovery iteration accepted at `b2755c1`, exact-head CI green and Oracle
  recheck passed. Models can choose recovery/idle; recovery-specific danger
  evidence goes to active Jev, not a deterministic persistent/worsening-danger
  veto. Ordinary preemption, stall guard and body safety remain. Two durable
  executions are charged before dispatch; restart/preemption/notifications do
  not refill them. Idle/exhaustion latch; reset requires success and a fresh
  code-safe observation, not proof of full recovery. Useful starvation action
  passed but ended at health 1. Matched tasks were 18/18 before and 17/18
  after; the late stone stall remains. Global temporal history was rejected
  after regressions. No general capability/survival gain or live update claimed.
- Defer semantic decisions to models wherever possible (explicitly requested
  by Lachlan; recorded in `AGENTS.md`). Slice 11 makes multi-turn dialogue,
  clarification and one remembered ongoing project first-class. Models choose
  intent changes, intermediate goals, replies, memory and reusable procedures;
  code persists context and enforces safety/ownership/limits/verification.
  No keyword intent tree, scripted Minecraft plan, new agent service or
  automatic skill promotion. Validate in isolated gyms before any live rollout.
- Conversation iteration accepted at `c2d7442`, exact-head CI green and Oracle
  recheck passed. A bounded read-only planner answers beside one exclusive executor; models preserve,
  suspend or replace work and curate one durable project intent. Existing
  clarification, goal chaining and semantic recall already worked. Matched
  status latency was 32.838s to 3.101s; revision 43.464s to 9.542s. A separate
  observed-motion probe replied in 2.553s without replacing its running action.
  Both full arms verified supplies and preference recall through restart;
  executor controls were 12/12 and skill gates 5/5 before and after. These are
  bounded trials, not general latency or recall guarantees. The later recovery
  admission/stop fixes have regression evidence, not rerun model measurements.
  Reported usage including failures/exclusions was $2.112174484; unreturned
  billing is unknown. Project intent is not a verified progress ledger; model/
  context reliability remains the next bottleneck. No new skill was necessary
  or promoted. Slice 11 itself left the live image/world unchanged.
- Lachlan subsequently approved the separate-world rollout. Source `7700696`
  is live through `lox/selfhosted-ovh` PR 12, merged at `aec9fd74`. The host
  took and verified a fresh stopped whole-memory snapshot before recreation;
  all 1,334 historical episodes, 13 semantic files and 7 places/home survived.
  Only Clyde was recreated; all 24 other production containers were unchanged.
  A live admin-console whisper received one factually checked model reply in
  5.436s: queue 1ms, planning 5.367s, dispatch 69ms. Planning includes context/
  client/provider work, not pure provider latency. Latency remains unresolved;
  measure that path before selecting an optimisation, without scripted replies
  or silently changing models/budgets. Real owner iPad gameplay is still pending.
  Downgrade requires the old image and matching stopped whole-memory snapshot;
  do not run it against upgraded memory alone. Bonnie remains untouched.
- Lachlan clarified he uses Java Edition and requested proxies like Bonnie's.
  The existing world is already Java Paper; its human entrance was Bedrock-only,
  not a demonstrated server failure. Slice 12 reuses the tested dual-Velocity
  pattern with new Clyde-only services: authenticated Java TCP25567, existing
  Bedrock TCP+UDP19134 on the public proxy, and an unpublished offline bot
  proxy with modern forwarding. Preserve world/player UUIDs, memory, whitelist,
  Bedrock identity/linking/TLS and bot image/settings. No Bonnie changes or
  direct public offline backend. Owner Java username/version and real client
  acceptance remain pending; server-side tests must not be called owner login.
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
- Planner replies use a durable send intent: at-most-once send attempts,
  not exactly-once delivery. A crash after intent can lose a reply. Restart
  reconciles in-flight goals by live predicates, not by replaying side effects.
- Planner-authored skills remain staged candidates. Operators explicitly
  gate them on an isolated gym and promote through the shared skill Store;
  ordinary play never resets a world or automatically promotes code.
- No x-ray: the state card lists only blocks exposed to air, so Clyde knows
  what a player could see (default).
- The chat kill switch currently stops only the action, not the scheduler.
  A latched chat stop/resume is deferred beyond the initial attended launch;
  stop the whole service when Clyde must stay stopped.
- The body exits on disconnect and its supervisor restarts it; no in-process
  reconnect (default).
- Gym observes the server over RCON, not through a second bot, and agents
  plug in as an executable plus environment variables (default).
- CI regenerates the gym world template each run (about 25 s); no
  actions/cache until gym runs get slow enough to matter (default).

## Open questions

- Whether to run a GPU box for a distilled judge (OVH has none in Sydney; AWS
  g6 in ap-southeast-2 or a home GPU on the Tailnet are the candidates).
