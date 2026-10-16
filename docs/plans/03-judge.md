# Slice 3: judge v0

Goal: a ticking System-1 loop that reads the state card, runs cheap code
pre-checks, asks Jev a fixed set of typed questions only when code cannot
decide, emits interrupts, and logs every tick as JSONL. It never chooses
actions.

## Done when

- `harness/jev` is a small client for `POST https://api.typesafe.ai/v1/systemone`
  with `Authorization: Bearer $TYPESAFE_API_KEY`, `model: jev-latest`, a
  `state` (the state card JSON), and a `questions` map of `noul`, `choice`,
  and `score` questions. It decodes `answers` (`noul`; `choice` with
  `probabilities` and `confidence`; `score` with `confidence`) and `usage`.
  `httptest` tests cover the request shape and auth header, each answer type,
  non-2xx errors, and context timeouts. The key is never logged or included
  in errors.
- `harness/judge` defines a backend interface (state card plus questions in,
  typed answers plus usage and latency out) with two backends: `jev` and
  `replay` (answers read from fixture files, for tests). Adding `logprob` or
  `classifier` later must not change the interface.
- The question set lives in `prompts/judge.yaml`, not in Go code: at least
  `danger` (noul), `stuck` (noul: is the current action failing to make
  progress), `progress` (score), and `interrupt` (choice of reasons such as
  `none`, `danger`, `stuck`, `player_request`). Interrupts are reasons for
  the executor to stop and replan, never actions.
- Pre-checks run first every tick and decide without the model when they
  can: health at or below a threshold, a hostile within N blocks, no
  position or progress change for M seconds while an action is running,
  standing in lava, oxygen at or below a threshold, and nothing happening at
  all (skip the model). "Nothing happening" means no action, no hostiles,
  healthy, not in lava or water, and no new chat or damage events since the
  last tick; a quiet bot with a fresh chat message still goes to the model.
  Thresholds are config. Table tests cover both sides of each threshold,
  and the lava, drowning, and player-chat fixtures must not hit the skip.
- The loop ticks at a configurable rate (default 2 Hz) with a per-tick
  deadline (default 800 ms). A slow or failed backend call is logged as a
  timeout or error and the tick falls back to pre-checks; ticks never
  overlap and never queue up.
- Every tick appends one JSONL record: timestamp, state card, pre-check
  results, questions sent, answers, interrupt emitted, backend, latency,
  token usage, and estimated cost. Tests assert the record shape from a
  replay run.
- `harness/cmd/judge` runs the loop against a body (`--body
  http://127.0.0.1:7777`, polling `GET /state`) or against a directory of
  fixture state cards (`--fixtures`), and prints interrupts as JSON lines.
- `harness/judge/testdata/` holds at least eight hand-written state cards
  with expected labels: calm day, zombie adjacent at night, low health,
  standing in lava, drowning, a `goto` with no progress for 20 s, a
  `collect` that is progressing, and a player asking for help in chat.
  `judge eval --backend jev` runs them against live Jev and prints accuracy
  plus latency p50/p95 over at least 20 calls. That output is in the commit message.
- `go build ./...`, `go vet ./...`, and `go test ./...` pass in `harness/`
  without network access; the live Jev check is a separate command, skipped
  when `TYPESAFE_API_KEY` is unset.

## Starting point

Nothing in `harness/` yet beyond a README. Create `harness/go.mod` as module
`github.com/lox/clyde/harness`. Slice 2 (gym) also creates it in parallel, so
rebase onto whichever lands first. CI already has a `harness` job that runs
`go build`, `go vet`, and `go test` once `harness/go.mod` exists.

The state card schema (`api/state.schema.json`) is being written in slice 1
in parallel. Until it lands, decode the fields pre-checks need into a small
local struct, using the field names in the draft `body/src/state.ts`
(`bot.health`, `bot.food`, `bot.oxygen`, `bot.position`, `bot.inLava`,
`entities.hostiles[].distance`, `action`, `events`), and pass the full card
to Jev as raw JSON. If slice 1 merges first, use its generated Go types.

## Notes

- Jev facts from herobrine: p50 398 ms, p95 580 ms, about 2,350 tokens and
  $0.0001 per call. Report how ours compare.
- Jev accepts structured JSON as `state`; send the card JSON, not the
  rendered text card.
- Use `confidence` as a second axis: a `choice` answer with low confidence
  should not interrupt on its own. Make the cut-off config and log it.
- Ask all questions in one request per tick; Jev answers a whole map at
  once.
- Provider calls live only in `harness/jev` (and later `harness/llm`); the
  judge depends on the backend interface.
- JSONL logs go under `memory/` by default (`memory/*.jsonl` is
  gitignored). They are the training data for slice 8, so keep the full
  state card and full answers, not summaries.

## As built

- Layout: `harness/jev` (client), `harness/judge` (pre-checks, backends,
  loop, records, eval), `harness/cmd/judge` (`judge [run]` and `judge
  eval`), `prompts/judge.yaml`. YAML is `go.yaml.in/yaml/v3`, the
  maintained fork of `gopkg.in/yaml.v3`.
- Thresholds default in `harness/judge/config.go` and can be overridden
  with `--config file.yaml`: low health ≤ 6, low oxygen ≤ 6, hostile ≤ 4
  blocks, stuck after 10 s without ≥ 0.5 blocks of movement or a progress
  text change, healthy (for the idle skip) ≥ 14, choice confidence cut-off
  0.5. A low-confidence choice still interrupts when the noul of the same
  name (`danger`, `stuck`) is ≥ 0.7.
- "New since the last tick" uses card time: an event is new when `ts −
  ageMs` is after the previous card's `ts`, and every event on the first
  card is new. A card older than the previous one resets pre-check state,
  so a fixture directory read in name order behaves like separate streams.
  Attention event types default to `chat`, `whisper`, `damage`; align them
  with slice 1's event names when its schema lands. If the backend fails
  on the tick that first sees a chat message, that message is not new on
  later ticks and does not reach the model again; the executor should not
  rely on the judge alone for chat it must answer.
- A single card cannot show "no progress for 20 s", so `goto_no_progress`
  goes to the model; the code `stalled` check fires only across ticks.
- `replay` reads any judge JSONL log (including `judge eval --log`) and
  matches answers by card `ts`. `testdata/replay.jsonl` is a recorded live
  Jev run over the fixtures.
- `judge eval` asks the model on every fixture, even ones pre-checks decide,
  so every call measures Jev. It reports per-question model accuracy and a
  separate `judge` line: the full verdict (pre-checks, then model through
  the confidence gate) against the interrupt label. Ten fixtures: the eight
  required plus `creeper_approaching` (hostile beyond the code range) and
  `player_chat_casual` (chat not addressed to the bot).
- Live result, 30 calls (10 fixtures × 3) from an orb on 2026-10-17:
  100% on all questions and on the judge verdict; latency p50 76 ms, p95
  126 ms, max 336 ms; 1,431 input tokens and $0.00006 per call at $0.042
  per Mtok. That is about 5× faster, 40% fewer tokens, and 40% cheaper
  than the herobrine numbers. The fixtures are clear-cut, so accuracy on
  them says little about ambiguous situations; JSONL logs from real play
  are the better benchmark.

## Not in this slice

`logprob` and `classifier` backends, acting on interrupts (the executor does
that in slice 4), changes to the body, and the gym.
