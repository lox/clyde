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
  (`observe`, `act`, `eval_js`, `recipes_for`, `lookup`, `find_skill`),
  the `until` check, and the judge beside it. The prompt lives in
  `prompts/executor.md`.
- `mcp/`: the same tools plus `run_goal` and `stop` over MCP.
- `cmd/clyde/`: `clyde run` and `clyde mcp`.

## Executor and MCP

Both need a running body (`cd body && npm run dev`) and `OPENAI_API_KEY`.

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

To use it from Amp, add an MCP server whose command is
`go run ./cmd/clyde mcp` with `harness/` as its working directory.

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
