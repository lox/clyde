# harness

Go module `github.com/lox/clyde/harness`. See `docs/plans/roadmap.md` for
what belongs here.

- `jev/`: client for TypeSafe's `POST /v1/systemone` (Jev). Reads
  `TYPESAFE_API_KEY`; never logs it.
- `judge/`: tier-1 judge. Pre-checks, backends (`jev`, `replay`), the tick
  loop, JSONL records, and the fixture eval. Questions live in
  `prompts/judge.yaml`.
- `cmd/judge/`: the judge CLI.
- `goal/`: goal types generated from `api/goal.schema.json` (`go generate
  ./goal`) and the `until` predicate evaluator, with an RCON observer.
- `rcon/`: minimal Minecraft RCON client.
- `gym/`, `cmd/gym/`: the gym task runner behind `gym/bin/run`; see
  `gym/tasks/README.md`.
- `body/`: client for the body's HTTP API, with state card and action types
  generated from `api/` (`go generate ./body`), and a goal observer that
  answers `items` and `arrived` from the bot's own card.
- `llm/`: the only package that constructs model providers
  (`github.com/lox/agent-harness`: OpenAI Responses by default, Anthropic
  as an alternative), plus per-model prices.
- `executor/`: tier-2 loop. One goal, one agent-harness `Run`, body tools
  (`observe`, `act`, `eval_js`, `recipes_for`, `lookup`, `find_skill`, `run_skill`),
  the `until` check, and the judge beside it. The prompt lives in
  `prompts/executor.md`.
- `mcp/`: the same tools plus `run_goal` and `stop` over MCP.
- `cmd/clyde/`: `clyde run`, `clyde mcp`, `clyde serve`, and skill gate/promotion.
- `planner/`, `memory/`: event-triggered planning, player-first scheduling,
  SQLite episodes/checkpoints/observations and bounded Markdown notes.
- `skills/`: shared content-bound skill loading/admission; see
  [skill format, gate, promotion, and isolation](../skills/README.md).

## Executor and MCP

Both need a running body (`cd body && npm run dev`). Executor goals need
`OPENAI_API_KEY`; direct MCP body/skill tools do not.

```sh
cd harness
go run ./cmd/clyde run --goal goal.json --rcon 127.0.0.1:25575   # outcome JSON on stdout
go run ./cmd/clyde mcp                                           # MCP over stdio
go run ./cmd/clyde mcp --http 127.0.0.1:7778                     # streamable HTTP
```

The model is `CLYDE_EXECUTOR_PROVIDER` (`openai` or `anthropic`),
`CLYDE_EXECUTOR_MODEL`, and `CLYDE_EXECUTOR_EFFORT`; defaults are in
`llm/llm.go`. `--rcon` checks the goal through the server console
(password from `GYM_RCON_PASSWORD`); without it only `items` and `arrived`
goals can be checked. The judge runs beside the loop when
`TYPESAFE_API_KEY` is set (`--judge off` disables it). `clyde run` exits 0
when the goal was reached and 2 otherwise.

Skill tools expose only locally admitted versions. `--skills-dir` selects
the library (default `skills/`); `--skills-dir ''` disables both tools for
controlled comparisons. The gym agent reads the equivalent `CLYDE_SKILLS_DIR`
(including an explicitly empty value). Outcomes include `skill_calls`, a
per-name count of actual attempted invocations, including failures.

To use it from Amp, add an MCP server whose command is
`go run ./cmd/clyde mcp` with `harness/` as its working directory.

## Local planner runbook

Use only a throwaway local server for this slice. From the repository root,
build once and attach to an already running body (the example uses the
separate live instance described in `skills/README.md`):

```sh
(cd harness && go build -o ../gym/.bin/clyde ./cmd/clyde)
amp orb service start clyde-planner --command 'gym/.bin/clyde serve --body http://127.0.0.1:7788 --rcon 127.0.0.1:25576 --max-autonomous 3 --max-steps 12 --max-time 90s --planner-time 30s'
amp orb service logs clyde-planner --lines 100
amp orb service stop clyde-planner
amp orb service restart clyde-planner
```

SIGINT/SIGTERM cancels and joins the worker, stops the body and retains its
queue. Stop the planner **before** stopping/restarting its body or Paper.
Stopping only a body action is not a latched kill switch: the scheduler can
start another action. The initial private deployment stops the whole service
as its reliable kill switch; chat latching is deferred. See `deploy/README.md`.

`CLYDE_PLANNER_PROVIDER`, `CLYDE_PLANNER_MODEL`, `CLYDE_PLANNER_EFFORT` are
independent of the executor variables above; both default to Sol 6.1, low.
The planner has at most two schema-validation attempts within
`--planner-time` per event. `--max-steps`/`--max-time` bound each fresh
executor. `--max-autonomous` is a durable completed-goal cap (default ten),
not reset by restart; explicitly raise it to continue. Player requests are
not charged to that cap. These are work/time budgets, not a hard dollar cap;
new player requests and post-mortems can incur further calls. No repeated
idle observation invokes a model. Usage/cost is stored in SQLite, including
returned usage for cancelled calls; unreturned provider billing is unknown.

Whispers and public chat starting with `Clyde`, `Clyde:` or `Clyde,` enqueue
player work. A player request cancels autonomous planning/execution, waits
for cleanup and a confirmed body stop, then runs first. Suspended goals keep
their original predicate and resume with a fresh executor, not the old
conversation. Unsupported requests receive a bounded idle/refusal reply.
Death/failure invokes a post-mortem that may choose a changed goal or idle.
The harness permits at most **two recovery executions** per durable episode,
charged before dispatch. Each retains the existing step/time limits. Idle,
refusal or exhaustion closes the episode; no timer polls the model again.
Restart, death notifications, new player requests and danger clearing/returning
do not replenish it. Only successful execution followed by a fresh observation
with no fired danger prechecks resets recovery and permits ordinary autonomy.
Safe player work remains eligible while recovery is closed. A successful
inventory milestone during danger is not proof of safety and does not reset it.

Code-detected danger still preempts ordinary work. Recovery is not cancelled by
the scheduler or judge merely because a danger check persists, appears or its
number falls. Instead the active judge decides whether **this recovery** should
stop/replan, using its goal/action and factual observations. It can immediately
interrupt for a new threat; unchanged confidence handling, the deterministic
stall guard, deadlines, player preemption and body safety remain in force.
There are no Go-selected recovery tactics or grace-period thresholds.

Defaults: `--memory memory/episodes.db`, `--semantic-dir memory/semantic`,
`--candidates-dir memory/skill-candidates`, and `--planner-prompt
prompts/planner.md`. Keep custom runtime paths under gitignored `memory/`.
SQLite records the queue, accepted event identities, outcomes and observed
places; semantic notes are limited to 4 KiB each/16 KiB total and require
supporting episode IDs. Requests and retrieved notes remain untrusted data.
One local CLI controller owns each body URL and database via advisory locks;
remote controllers/host aliases are still the operator's responsibility.

Restart stops stale actions and checks running predicates against live state.
Satisfied work is recorded; unsatisfied ordinary work is requeued. An unfinished
running recovery has already spent its attempt: restart does not replay it,
but may plan another response within the remaining allowance. Player-preempted
recovery retains its context and spends another attempt if executed again.
Existing checkpoints without recovery fields decode as ordinary work; their
existing pause and queue are retained, not silently reset on upgrade. This
cannot make arbitrary side effects idempotent. Reply intent is committed before
sending: restart does not resend, but a crash before send can lose the reply.
This is at-most-once send attempt, **not exactly-once delivery**. The bounded
body event ring can lose events evicted during downtime; gaps are recorded.

Candidate proposals are immutable separate roots:
`memory/skill-candidates/proposal-*/<name>/{skill.md,index.js,test.yaml}`.
Play never executes, gates, admits or promotes them. Inspect the files, then
explicitly gate that proposal root on the **separate candidate world** using
the full isolation environment in `skills/README.md`; explicitly promote
only after passing. Never gate/reset the planner's live world. A failed
revision cannot replace the installed skill or its receipt. Local receipts
are intentionally untracked; rerun the existing gates after a fresh checkout.

## Judge

```sh
cd harness
go run ./cmd/judge --body http://127.0.0.1:7777          # live body, Jev backend
go run ./cmd/judge --fixtures judge/testdata/cards --backend replay
go run ./cmd/judge eval --backend jev                    # live accuracy and latency
```

Interrupts (reasons to stop and replan, never actions) print to stdout as
JSON lines; every tick appends a record to `memory/judge.jsonl`. Any judge
log, including `judge eval --log`, works as a `--replay` file: answers are
matched by the card's `ts`.

During recovery only, the judge adds `recovery` evidence: the typed goal,
preceding/current fired checks, appeared/cleared checks and the preceding state
card. The current card still supplies action/events. Damage cause is explicitly
unknown; numeric change is evidence, not a deterministic cancellation rule.
JSONL retains the original `card`, exact backend `observation`, answers and
interrupt, plus `state_ms` timing at the existing observation boundary. Ordinary
work receives the original raw card, without a global temporal history feature.
State/backend failures remain logged gaps, not a survival guarantee; a judge
outage does not substitute an any-damage cancellation policy for recovery.

Thresholds, rate, and deadline default to the values in
`judge/config.go`; override any of them with `--config file.yaml`, e.g.

```yaml
rate: 4
deadline: 400ms
prechecks:
  hostile_range: 6
  stuck_after: 8s
decision:
  min_confidence: 0.6
```

`judge eval --backend jev` is skipped when `TYPESAFE_API_KEY` is unset;
`go test ./...` never touches the network.
