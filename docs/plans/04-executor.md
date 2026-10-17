# Slice 4: executor + MCP server

Goal: give Clyde a typed goal and let an OpenAI model reach it by calling
body tools in a loop. The loop stops when the goal's `until` predicate
holds, the judge interrupts, or the step budget runs out. The same tools are
exposed over MCP so an Amp thread can drive Clyde directly, and an executor
agent plugs into the gym so every later change has a scoreboard.

## Done when

- `harness/llm` is the only package that constructs model providers. It
  uses `github.com/lox/agent-harness` (MIT, pinned in `go.mod`) and its
  OpenAI Responses provider by default, plus its Anthropic provider as an
  alternative. Provider, model, and reasoning effort are config
  (`CLYDE_EXECUTOR_PROVIDER`, `CLYDE_EXECUTOR_MODEL`,
  `CLYDE_EXECUTOR_EFFORT`).
- `harness/body` is a Go client for the body API, using Go types generated
  from `api/state.schema.json` and `api/action.schema.json` with the same
  generator slice 2 uses for `api/goal.schema.json`.
- `harness/executor` runs one goal with `agent-harness`'s `Run`, with no
  state carried between goals, and returns an outcome: `succeeded`,
  `failed` (the model gave up or refused), `interrupted` (the judge's
  reason attached), or `budget_exhausted` (max steps or wall time). The
  outcome also records steps, tokens, cost, and duration. Tools:
  - `observe`: the rendered text card (`GET /state?format=text`).
  - `act`: one `POST /action`, validated against the action schema. It
    waits until the action finishes or a few seconds pass, then returns
    the result or the progress so far.
  - `eval_js`: `POST /eval`.
  - `recipes_for` and `lookup`: minecraft-data queries answered through
    `POST /eval` against `bot.registry`, so there is no second copy of
    minecraft-data in Go.
  - `find_skill`: searches `skills/*/skill.md`. The directory is empty
    until slice 5, so it returns nothing yet, but the tool and its
    contract exist.
  Bad tool arguments come back to the model as tool errors and never crash
  the loop.
- The goal's `until` predicate (from `harness/goal`, slice 2) is checked
  after every tool call and about once a second. When it holds, the run is
  cancelled and the outcome is `succeeded`.
- The judge runs beside the executor against the same body. A `danger` or
  `stuck` interrupt cancels the run's context, the executor sends the body
  `POST /stop`, and the outcome is `interrupted`. The judge's stall
  pre-check uses `distanceRemaining`, `lastMovedAt`, and `collected`, and
  the generated state card types instead of its local struct (issue #3).
- Chat Clyde must answer is read from `GET /events`, not from judge
  interrupts. For this slice the executor just includes new chat in its
  observations; acting on player requests waits for slice 6's lanes.
- `harness/cmd/clyde` provides `clyde run --goal goal.json` (one goal
  against a running body; the outcome is printed as JSON) and `clyde mcp`.
- `clyde mcp` is an MCP server (stdio; streamable HTTP optional) built on
  the official Go MCP SDK. It exposes `observe`, `act`, `eval_js`,
  `recipes_for`, `lookup`, `run_goal`, and `stop`. Each tool is defined
  once and adapted for both the executor and MCP. The commit message includes
  a transcript of an MCP client driving the bot on the gym server.
- A `gym/agents/executor` agent starts the body, waits for `GYM_READY`,
  and runs `clyde run` on `GYM_TASK`. `gym/bin/run --all --agent executor` runs all four gym tasks
  three times each with two OpenAI models, a mid tier and a frontier tier.
  The scoreboard (pass rate, median time to pass, tokens and cost per task)
  is in the commit message and recorded in the roadmap so we can choose
  the default model.
- Offline tests use a scripted fake `agent-harness` provider and an
  `httptest` body. They cover: `until` holding mid-run stops early with
  `succeeded`; a judge interrupt cancels and sends `/stop`; max steps gives
  `budget_exhausted`; bad tool arguments return a tool error. They run in
  CI's `harness` job with no network.
- Issue #2: a soak script interrupts a running `collect` with a `goto` at
  least 50 times on the gym server. Either it never reproduces "goal was
  changed" (say so on the issue and close it) or it does and the body gets
  a fix.

## Starting point

On `main`: the body (slice 1) with `api/state.schema.json` and
`api/action.schema.json`; the judge (slice 3) in `harness/judge`,
`harness/jev`, `harness/cmd/judge`; the gym (slice 2) with
`harness/goal`, `api/goal.schema.json`, `gym/bin/run`, and the agent
contract (an executable plus `GYM_TASK`, `MC_HOST`, `MC_PORT`,
`MC_USERNAME`, and `GYM_READY`: a file the runner creates once task setup
has run, which agents wait for before acting). Goal predicates are keyed
by kind, for example `until: {items: {item, count}}`. Read the "As built" sections of 01, 02, and 03 first.

`lox/agent-harness` is ours (MIT since 8117cc9). If the executor needs a
change there (a provider option, runner behaviour, a bug), do not fork or
vendor it. Tell the coordinator what you need. The coordinator delegates it
to a thread in `lox/agent-harness`, and you pin the new version when it
lands.

## Notes

- `agent-harness` notes from review: `Run` reports cancellation as
  `StopCancelled` with a nil error, so map the `Result` to our outcome
  rather than expecting `context.Canceled`. Tools run one after another.
  There is no MCP in it, and its OpenAI provider is Responses-only.
- Within one goal the OpenAI provider continues with `previous_response_id`.
  Never reuse a response ID or thread across goals.
- Cancelling an HTTP request does not stop a running Minecraft action.
  Always send `POST /stop` on interrupt or cancel.
- Keep prompts in `prompts/executor.md`, not in Go.
- Live runs need `OPENAI_API_KEY` in the environment. If it is missing,
  stop and tell the coordinator instead of switching provider.

## Not in this slice

The planner, priority lanes, memory, skill authoring and the skills gym
gate (slice 5), production join, and a Chat Completions provider.
