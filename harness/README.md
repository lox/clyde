# harness

Go module `github.com/lox/clyde/harness`. See `docs/plans/roadmap.md` for
architecture and slice acceptance, and `docs/plans/14-persistent-player.md`
for the persistent-player implementation and isolated evidence.

- `jev/`: client for TypeSafe's `POST /v1/systemone` (Jev). Reads
  `TYPESAFE_API_KEY`; never logs it.
- `judge/`: prechecks, backends (`jev`, `replay`), the standalone tick loop,
  JSONL records and fixture evaluation. Questions live in `prompts/judge.yaml`.
- `cmd/judge/`: the judge CLI.
- `goal/`: generated goal types (`go generate ./goal`) and independent
  `until` predicate evaluation, including RCON observations.
- `rcon/`: minimal Minecraft RCON client.
- `gym/`, `cmd/gym/`: task runner behind `gym/bin/run`; see `gym/tasks/README.md`.
- `body/`: HTTP client and generated state/action types (`go generate ./body`),
  plus an observer for inventory and arrival predicates.
- `llm/`: the only package constructing model providers
  (`github.com/lox/agent-harness`: OpenAI Responses by default, Anthropic
  as an alternative), plus per-model prices.
- `executor/`: bounded single-goal development loop, shared body/skill tools,
  independent checks and judge integration. Prompt: `prompts/executor.md`.
- `mcp/`: the same tools plus `run_goal` and `stop` over MCP.
- `session/`: persistent conversation, inbox, accounting, asynchronous tools,
  explicit waits, history and compaction. Prompt: `prompts/player.md`.
- `cmd/clyde/`: `run`, `mcp`, persistent `serve`, local operator `control`,
  and skill gate/promotion.
- `memory/`: existing SQLite episodes/checkpoints/observations and Markdown notes.
- `planner/`: legacy scheduler/types and shared candidate-staging utilities.
  `serve` reuses the candidate utility, not the planner/executor scheduler.
- `skills/`: content-bound loading/admission; see
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

## Local persistent-player runbook

Use only a disposable local server for validation. From the repository root,
build once and attach to an already running body (the example uses the separate
attended instance described in `skills/README.md`):

```sh
(cd harness && go build -o ../gym/.bin/clyde ./cmd/clyde)
amp orb service start clyde-player --command 'gym/.bin/clyde serve --body http://127.0.0.1:7788 --rcon 127.0.0.1:25576 --judge jev --request-limit 240 --call-timeout 45s --max-output-tokens 4096'
amp orb service logs clyde-player --lines 100
amp orb service stop clyde-player
amp orb service restart clyde-player
```

SIGINT/SIGTERM cancels and joins inference, stops owned body work and retains
the conversation. Stop the player **before** stopping/restarting its body or
Paper. One durable session owns dialogue, projects and tools. There are no
planner/executor handoffs, completed-goal cap or two-recovery closure in `serve`.

The player uses `CLYDE_EXECUTOR_{PROVIDER,MODEL,EFFORT}`, defaulting to OpenAI
Sol 6.1 low. Remove `CLYDE_PLANNER_*` overrides: they are rejected. Legacy
serve flags `--max-autonomous`, `--planner-time`, `--planner-prompt`,
`--max-steps` and `--max-time` are rejected too. `run`/MCP keep their bounded
development contracts. Explicit `--judge jev` requires `TYPESAFE_API_KEY`.

The initial 240-request allowance is shared by inference, compaction and
event-driven Jev ticks, conservatively including precheck-only ticks. Requests
are charged before dispatch; cancellation, failure, chat and restart never
refund or refill them. The stored allowance wins over startup configuration.
Frontier calls default to 45 seconds and 4096 output tokens, with SDK retries
disabled. Tool dispatch has a 12-second deadline; body action limits remain.
Exhaustion stops owned work and pauses the same session. Returned usage/cost
and unknown billing are separate; these are not hard dollar limits.

Local filesystem access is operator authority. From the repository root:

```sh
gym/.bin/clyde control status
gym/.bin/clyde control stop
gym/.bin/clyde control extend --requests 100
gym/.bin/clyde control resume
gym/.bin/clyde control compact
```

Use `--memory PATH` on each command for a nondefault database. Stop is latched
across restart; extension does not clear it. Resume clears stop/error pause but
grants no requests. Ordinary chat cannot resume or extend allowance. Body
stop/chat stop halts only an action, not durable player attention.

Whispers and public chat starting with `Clyde`, `Clyde:` or `Clyde,` enter the
durable inbox. Incoming chat cancels stale inference, not useful movement.
Chat/read-only tools do not replace actions; model-selected replacement joins
prior body work. Action tools return promptly with tokens/status. Outcomes and
fresh observations feed the same conversation. Explicit `wait` persists across
restart, wakes on events/deadlines and makes no frontier polling calls.

Independent `verify` checks enter history without ending the session. Inventory
and arrival can use body observations; construction/delivery require RCON;
`judged` is unsupported. First danger stops its action once, not every response
to persistent danger. Stuck stops target only the observed action token. Jev
supplies evidence before attention; local prechecks poll every 500ms. Models
choose tactics; code does not select an escape/eat/flee plan.

Native messages and original tool exchanges are archived in versioned SQLite
`player_*` tables. `history` provides bounded search and byte-offset paging;
`memory` reads legacy notes/places/episodes. `--context-bytes` defaults to
131072 serialized bytes. Compaction preserves the complete latest exchange and
publishes model-written notes atomically; notes are not verified progress.
New attention supersedes cancelled compaction without an operator pause or
refund. Context pressure may require a fresh compaction including the new
events. Real provider/invalid-summary failures pause rather than retry forever.

Defaults: `--memory memory/episodes.db`, `--semantic-dir memory/semantic`,
`--candidates-dir memory/skill-candidates`, and `--prompt prompts/player.md`.
Keep runtime paths under gitignored `memory/`. Local advisory body/database
leases exclude other local Clyde controllers; remote/aliased HTTP controllers
remain an operator responsibility. Requests and retrieved notes are untrusted.

Restart stops stale work and observes live state; ambiguous mutations/chat
are never replayed. This is at-most-once send attempt, **not exactly-once
delivery**. Quiet waits and resource accounting survive. Event-ring gaps are
recorded, not reconstructed. Legacy project/dialogue/jobs import once as
unverified context, not executable work; old recovery closures have no authority.
Episodes, notes, places and the legacy checkpoint remain intact. Before upgrade,
take a stopped whole-memory backup. Rollback requires the matching old image
and backup, not the old image against upgraded memory. See
[deployment and rollback](../deploy/README.md); source validation is not rollout
approval.

Candidate proposals remain immutable separate roots:
`memory/skill-candidates/proposal-*/<name>/{skill.md,index.js,test.yaml}`.
Play never executes, gates, admits or promotes them. Inspect the files, gate
that proposal root on the **separate candidate world** using the full isolation
environment in `skills/README.md`, then explicitly promote only after passing.
Never gate/reset the player's world. A failed revision cannot replace the
installed skill or receipt. Receipts are untracked; rerun gates after checkout.

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

Bounded executor callers can still supply explicit `recovery` evidence; the
persistent player does not use that legacy episode policy. This adds the typed
goal, preceding/current fired checks, appeared/cleared checks and the preceding
card. The current card supplies action/events. Damage cause is explicitly
unknown; numeric change is evidence, not a deterministic cancellation rule.
JSONL retains the original `card`, exact backend `observation`, answers and
interrupt, plus `state_ms`. Ordinary work receives the raw card without global
temporal history. State/backend failures remain logged gaps, not a survival
guarantee.

Standalone judge thresholds, rate and deadline default to `judge/config.go`;
override them with `--config file.yaml`, e.g.:

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
`go test ./...` never calls paid providers (HTTP fixtures use loopback).
