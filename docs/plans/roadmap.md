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

## Architecture: one persistent player

Lachlan approved replacing job-centred orchestration with one durable agent
session in [the architecture discussion](https://ampcode.com/threads/T-01a1940d-be92-7625-9329-7da018f30d0d).
The intended product is a long-time-horizon player that remembers commitments,
investigates failures and continues useful work across interruptions, context
compaction and process restarts. Slice 14 implements this in source through
`4fd3d1e`, accepted with exact-head CI green. The deployed service still uses
the earlier planner/executor architecture; no rollout is authorized here.

- One continuing model conversation owns dialogue, plans and tool use. Bounded
  inference bursts and actions do not reset its identity or project context.
- A durable inbox, fresh observations before inference, archived searchable
  history and model-written working memory maintain continuity. Restart
  reconciles unknown side effects instead of replaying them.
- The body retains hard safety and exclusive action ownership. Judge evidence
  attracts attention or stops the applicable action; it does not start a new
  recovery lifecycle. Operator stop is latched; finite durable inference
  allowances pause execution without erasing context.
- Independent predicates verify milestones without ending the conversation.
  Keep the gym, skill admission, Mineflayer body, Go provider boundary and
  single-goal development tools. Persistent play has no planner/executor
  handoff, completed-goal cap or two-recovery closure.
- Matched hour-long attended trials verified requirements and construction
  through native compaction/restart. They did not demonstrate better general
  gameplay, latency or cost; autonomous days-long play remains unproven.

See `14-persistent-player.md` for contracts, migration and acceptance evidence.

### Persistent session (`harness/session`)

`clyde serve` owns a durable inbox, provider-compatible conversation, original
searchable history and model-written working notes. Each model call receives
fresh observations. Action tools return promptly with identity/status; chat
and read-only tools do not replace useful movement. The same session handles
results, player revisions and failure investigation. Explicit `wait` yields
until an event or model-selected deadline without frontier polling.

Compaction retains complete recent exchanges and archived originals, then
resets provider continuation onto notes plus retained native history. Incoming
chat supersedes compaction without latching an operator-only pause. Genuine
compactor failures retain history and pause rather than retrying indefinitely.
Restart stops stale work, observes live state and records ambiguous outcomes
without replaying mutations/chat. At-most-once attempts are not delivery proof.

Default limits are 240 durable reservations shared by inference, compaction
and event-driven judge ticks, 45s frontier deadlines, 4096 output tokens and
128KiB active-history compaction pressure. These are not hard dollar limits.
Reservations survive restart and cancellation; only local operator controls
extend them. `control stop` latches across restart, `resume` grants no requests,
and `extend` does not clear stop. Local advisory leases exclude other local
controllers; remote/aliased body controllers remain an operator responsibility.

### Body (`body/`)

mineflayer bot wrapped in a small HTTP server. The body owns everything that
must be fast and deterministic: pathfinding, block interaction, inventory,
and hard safety limits.

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
  body action stop (`POST /stop` plus a chat command for ops). Trusted eval
  runs in-process, not in a malicious-code security sandbox. The persistent
  session's operator stop additionally prevents automatic attention/actions.

### Attention and judge (`harness/judge`)

The judge supplies evidence, never Minecraft tactics. Persistent play polls
deterministic danger/stall checks every 500ms and runs Jev before frontier
attention, not as paid idle polling. A first danger transition stops the
observed action once; persistent danger does not repeatedly veto its own
response. Stuck cancellation is action-incarnation-specific.

- Contract: `choice`, `score`, `noul` questions (Jev's native shapes).
- Implemented backends: `jev` (typesafe.ai `POST /v1/systemone`) and `replay`
  (fixture-driven tests). Classifier distillation remains deferred.
- Cheap code pre-checks run first every tick (health below threshold, hostile
  within N blocks, no progress for M seconds); the model is asked only when
  code cannot decide.
- Every tick is logged as JSONL (state card, questions, answers, latency,
  cost). Each persistent-session tick conservatively reserves a request even
  when prechecks answer without calling a provider.

### Bounded development tools (`harness/executor`, `harness/mcp`)

`clyde run` retains the single-goal executor for gym comparisons. Its body and
skill tools are reused by the persistent session; no recipe prerequisites are
hand-coded. MCP exposes development tools and `run_goal`. These interfaces
must not compete with a running persistent player for the body.

The legacy `harness/planner` scheduler remains in source but is not on the
`serve` path; shared candidate-staging utilities are still used. Historical
slices document the prior hierarchy, priority lanes and recovery lifecycle.

## Contracts (`api/`)

- One JSON Schema for the state card and for actions in `api/`, generating TS
  types for the body and Go types for the harness. Jev receives structured
  JSON; LLMs receive a rendered text card from the same data.
- Typed goals with machine-checkable `until` predicates: `items`, `delivered`,
  `arrived`, `built`; `judged` is reserved and unsupported. The same predicates
  are gym checkers and the persistent player's `verify` evidence tool.

## Skills (`skills/`)

`skills/<name>/{skill.md,index.js,test.yaml}`. `skill.md` is the
model-facing description (when to use, preconditions, failure modes);
`index.js` runs via `POST /eval`; `test.yaml` is a gym task that must pass
before the skill is admitted. The player can stage model-authored proposals;
operators explicitly gate and promote them. Play never admits its own code.

## Memory (`memory/`, runtime data)

- Current session: versioned SQLite `player_*` tables for native messages,
  original history, inbox, action intents/results, waits and accounting.
- Working memory: model-written compaction notes plus bounded history search;
  intent and summaries never certify verified world progress.
- Legacy episodes, code-observed places and semantic notes remain readable
  through `memory`; legacy project/dialogue/jobs import once as unverified
  commitments. Old recovery closures do not govern the new session.
- Migration leaves old records intact. Rollback requires the matching stopped
  whole-memory backup and binary; do not downgrade against upgraded memory.

## Gym (`gym/`)

Throwaway Paper server (offline mode, fixed seed, pre-generated world
templates) plus `gym/tasks/*.yaml` with checkers. Produces a scoreboard per
run so slices and PRs can show numbers. Runs in CI and in orbs.

## Deployment (`deploy/`)

Initial live target is a separate persistent Clyde play world on media-ovh,
configured in `lox/selfhosted-ovh` (PR 11). Lachlan chose to interact with
Clyde there before considering Bonnie. Java players enter through authenticated
Clyde-only Velocity at `minecraft.loxflix.xyz:25567`; Bedrock uses its
Geyser/Floodgate native HTTPS/NetherNet route on port 19134. Full Clyde uses
an unpublished offline bot proxy to private Paper with ViaBackwards. World,
memory and stable private RCON credentials are independent of Bonnie.
Bonnie and its public proxies are not part of this rollout.

Clyde source `7700696` is deployed there through `lox/selfhosted-ovh` PR 12.
The host verified the image, memory continuity and one live model reply;
Lachlan subsequently confirmed actual Java login and conversation with Clyde.
Slice 12's proxies are deployed through infrastructure PR 13; the same world,
player identities and bot memory are preserved. Real Bedrock client gameplay
remains unverified. See slices 7/12 and the host stop/rollback runbook.

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
| 7 | Simple deployment: separate play world live; owner Java joined | `07-production.md` | 4 |
| 8 | Judge distillation: classifier backend from JSONL logs | `08-distill.md` | 3, 6 |
| 9 | Harder gym curriculum and evidence-led capability improvements (done, `0787b89`) | `09-capability-curriculum.md` | 5, 6 |
| 10 | Model-led recovery and judge evidence (done, `b2755c1`) | `10-model-led-recovery.md` | 6, 9 |
| 11 | Multi-turn conversation and remembered projects (done, `c2d7442`) | `11-conversation-projects.md` | 6, 10 |
| 12 | Authenticated Java access (deployed; owner Java login confirmed) | `12-java-access.md` | 7 |
| 13 | Bounded model-led exploration (accepted, `52f135c`; not deployed) | `13-bounded-exploration.md` | 10, 11 |
| 14 | Persistent player: durable session, continuity, bounded attention (accepted, `4fd3d1e`; not deployed) | `14-persistent-player.md` | 5, 6, 13 |
| 15 | Local survival during frontier latency (narrow experiment `2b21099`; runtime admission still blocked) | `15-local-survival.md` | 5, 14 |

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
- Slice 12 deployed through infrastructure PR 13, merged at `264158f3`, all
  checks green. Isolated auth/UUID/inventory/rollback matrix and live negative
  auth tests passed; independent external Java status and trusted Bedrock HTTPS
  passed. Stopped whole-data/memory snapshots preceded cutover. The host
  verified bot identity/playerdata, all 1,611 historical episodes, memory files
  and places retained; only the bot Minecraft host changed, not image/settings.
  All 25 untargeted containers stayed unchanged, including Bonnie. The owner
  subsequently confirmed actual Java login/chat; host observed client26.2.
  Real Bedrock gameplay remains unverified.
- Slice 13 approved by Lachlan following the wood-task failure and Oracle review:
  keep failures inside the executor's existing 40-turn/300-second conversation;
  make stuck action-local with incarnation-safe conditional cancellation, add
  compact collision evidence and expose truthful admission/closure provenance.
  The incident charged zero recoveries; an old voluntary closure is not
  numerical exhaustion. Preserve danger/body safety and the outer two-recovery
  policy for this first slice. A durable exploration allowance is deferred
  until matched trials show that boundary blocks useful work. Implementation
  and isolated validation are approved; no live changes or rollout authorized.
- Slice 13 accepted at `52f135c`, exact-head CI green and final Oracle scoped
  approval. Matched obstruction results were 1/4 before versus 4/4 after;
  unchanged controls were 11/12 versus 12/12 and skill gates 5/5 both arms.
  Legacy closed-zero follow-up completed eight birch logs without a refill.
  Oracle found a late-stuck terminal-acceptance race and then unmatched partial
  tool-call history on resumption; both were fixed with deterministic before/
  after regressions. The coordinator reran all 21 terminal-history cases ten
  times under race detection, changed-package race tests, build and vet.
  Earlier body tests passed 26/26. Final fixes preserve cumulative history,
  usage, turns and deadline; paid-model arms were not rerun on those fixes.
  The retained moving-status missed return target limits capability claims,
  not ownership acceptance. Returned usage was $1.342987860, unknown unreturned
  billing excluded from any invoice claim. The outer recovery closure remains;
  no general reliability/latency gain or durable allowance extension claimed.
  Source acceptance is not rollout approval: live image/world remain unchanged.
- Lachlan approved the persistent-player pivot after comparison with
  `yc-software/minecraft-agent-harness`. Slice 14 replaces planner/executor
  handoffs with one durable conversation and explicit waiting, recoverable
  history and compaction. Goal predicates become milestone evidence, not
  session termination. Preserve hard safety, exclusive ownership, independent
  verification and explicit skill admission; replace the old goal-count and
  recovery-closure limits with durable inference allowances and operator stop.
  Existing bounded `run`/MCP development interfaces remain. Validate matched
  isolated continuity trials with the current model before claiming capability;
  implementation approval does not authorize production rollout.
- Persistent-player source accepted at `4fd3d1e`, exact-head body/harness/gym
  and security checks green. One attended hour per matched arm verified revised
  destination, table placement, eight-plank delivery and the three-block birch
  row. Candidate native compaction and restart preserved requirements; both
  four-minute quiet-wait probes made zero frontier calls. Candidate initially
  missed supplies, later recovered, and gathered only three of six requested
  new logs after earlier work consumed part of the trunk. Status/revision
  latency rose from 2.746/4.342s to 3.693/7.327s; returned full-arm usage cost
  rose from $0.374 to $2.441. This accepts the continuity architecture, not a
  general capability, latency, cost or days-long-autonomy improvement.
  Unchanged executor controls were 12/12 both arms; shared skill gates 5/5.
  Coordinator review found chat-during-compaction could latch an operator-only
  pause; the fix preserves history and charged usage while handling the new
  message without resume. Requested/automatic cases failed before and passed
  after; true provider/deadline/empty-summary controls still pause. Coordinator
  reran those tests ten times under race detection, changed-package race tests
  three times, full build/vet/tests, body 26/26/build and generation checks.
  Last-reservation withholding, history paging and compaction supersession
  have deterministic regression evidence, not rerun hour-long paid trials.
  Returned usage including smokes/controls was $3.976797222; unreturned billing
  remains unknown. Upstream native-history fix `582660e` is pinned from the
  published agent-harness PR 13, merged after the review check recovered;
  its checks and Clyde's real provider reset/continuation passed. No live
  image, memory or world changes; rollout remains separately authorized.
- Lachlan approved one local survival slice before any general reaction
  framework. Audit existing Mineflayer controllers/plugins, reproduce a
  hostile interrupting collection under delayed frontier inference, then
  implement one admitted model-authored response to a model-selected refuge.
  Reuse local pathfinding; keep tactical trigger/response decisions in skills
  and exclusive ownership, cancellation, deadlines and hard safety in code.
  Measure matched survival, damage and actual movement latency under 5/15/30s
  delays. Preserve failures and test stale controllers, operator stop,
  blocked routes and false activation. No new model tier, automatic admission
  or production changes. See `15-local-survival.md` for the bounded contract.
- Slice 15 audit/baseline evidence is published at `b0dbaf9`, exact-head CI
  green. Six unchanged-source trials survived and reached the fixture-selected
  refuge in 2/2 cases at 5s delay (health 2), but 0/4 at 15/30s. These small
  trials are not a general survival-rate estimate. Real-plugin probes and
  separate rechecks confirmed late digging/placement after replacement,
  operator stop and eval timeout; exported idle flags cannot prove quiescence.
  Installed pathfinder 2.4.5 has no public cancellation-and-drain boundary for
  private asynchronous controller chains. Evidence-only source is accepted;
  no runtime fix, arming, dependency change, candidate trial or paid call has
  occurred. The initial upstream-first recommendation is superseded below.
- Oracle review supported local reactions but found that pathfinder-only
  cancellation is insufficient: real direct Mineflayer digging can issue a
  start packet after stop while an internal look promise settles. The user
  approved a narrower model-free experiment before an upstream project:
  assertion-bearing controller/primitive cancellation probes, movement-only
  navigation for both collection and retreat, and bounded collect-to-retreat
  handoffs in the frozen arena. No residual conflicting command authority or
  stale cleanup may survive handoff; already-sent world effects are reconciled,
  not promised away. Establish the responsible layer and the smallest fix
  from failing tests. No general arming API, paid trials, new dependency or
  production change in this experiment; see `15-local-survival.md` for gates.
- The narrower experiment is published at `2b21099`, exact-head CI green.
  Movement-only navigation passed wall/gap controls; five no-threat local
  phase handoffs reached the refuge with 1–236ms handoff and 6–319ms first
  command latency. These are mechanics results, not survival trials. The
  heterogeneous matrix has 12/30 passing cases; its 18 no-go cases include
  14 deliberately unrestricted controller/primitive negative controls, not
  18 failures of the restricted candidate. Candidate blockers remain: body
  replacement admits a new owner before entered equip settles, the ordinary
  token-conditional danger stop cancels a same-owner retreat, and native
  collectblock cannot harvest under `canDig=false`. Its shared-target cleanup
  also interferes with replacement work. An uncooperative local operation
  fails closed but misses latency/arrival gates. Prefer a narrow opt-in body
  operation/ownership boundary using explicit collection primitives; no
  upstream project has been shown necessary. No runtime source changed, no
  paid calls or model admission, and the 18 survival trials remain unrun.
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
- The body refuses late pathfinder goals/digs attributed to an aborted
  action's async context. Slice 15 demonstrates that pathfinder physics
  callbacks and their delayed continuations can escape that attribution;
  these guards are not a complete ownership guarantee. Already-entered
  Mineflayer primitives and collector cleanup also need a supported boundary;
  that prerequisite remains unresolved, not assigned to pathfinder alone.
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
- Replies use a durable send intent: at-most-once send attempts, not
  exactly-once delivery. A crash after intent can lose a reply. Persistent
  play reconciles uncertain outcomes with fresh state, never blind replay.
- Model-authored skills remain staged candidates. Operators explicitly
  gate them on an isolated gym and promote through the shared skill Store;
  ordinary play never resets a world or automatically promotes code.
- No x-ray: the state card lists only blocks exposed to air, so Clyde knows
  what a player could see (default).
- The chat kill switch stops only the body action. Source `serve` has a
  latched local `control stop/resume`; untrusted chat cannot grant resume.
  The older deployed service still requires stopping the whole service when
  Clyde must stay stopped. Chat operator authentication remains deferred.
- The body exits on disconnect and its supervisor restarts it; no in-process
  reconnect (default).
- Gym observes the server over RCON, not through a second bot, and agents
  plug in as an executable plus environment variables (default).
- CI regenerates the gym world template each run (about 25 s); no
  actions/cache until gym runs get slow enough to matter (default).

## Open questions

- Whether to run a GPU box for a distilled judge (OVH has none in Sydney; AWS
  g6 in ap-southeast-2 or a home GPU on the Tailnet are the candidates).
