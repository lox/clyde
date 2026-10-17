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
