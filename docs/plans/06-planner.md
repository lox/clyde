# Slice 6: planner and memory

Goal: Clyde can choose a useful next goal, remember outcomes and places,
respond to player requests ahead of autonomous work, and learn from failures
without running a frontier-model planning loop on a timer.

## Done when

- `harness/planner` invokes a configurable OpenAI model through
  `harness/llm`/agent-harness on events: goal finished/failed, judge
  interrupt, player request, death, or becoming idle. Default to
  `gpt-6.1-sol`; planner model/effort can be configured independently from
  the executor. Prompts live in `prompts/planner.md`. There is no periodic
  model polling, and repeated idle observations do not cause a busy loop.
- Planner output is a validated typed goal using the existing goal schema,
  or an explicit idle/no-goal outcome. Invalid output cannot reach the
  executor and retries are bounded. The executor remains fresh per goal;
  the planner receives compact outcomes and relevant memory, not unbounded
  executor transcripts.
- A small scheduler has player and autonomous lanes. A new actionable
  player request cancels autonomous execution, waits for its `/stop` cleanup,
  and services the player lane before re-queuing the interrupted autonomous
  goal. There is never more than one executor controlling the body. Danger
  is handled before autonomous work; no immediate retry loop on danger or
  repeated goal failure. A fresh request can also cancel slow autonomous
  planning instead of sitting behind it. Document what is resumed/replanned
  and how unsupported or impossible requests are answered.
- Read player chat/whispers and death from `/events` with a durable cursor,
  including missed/reset handling and ignoring Clyde's own chat. Do not use
  judge interrupts as a reliable chat transport. Persist accepted requests
  and goal outcomes so restarting does not duplicate answered requests or
  lose queued autonomous work. In-flight work is reconciled against live
  state on restart, not blindly marked successful or re-executed.
- `harness/memory` stores episodic goals, outcomes, deaths, chat/request
  status and costs in SQLite. Spatial memory is maintained by observed
  code data (last-seen players and named POIs/home/resource locations),
  with position, dimension and observation time. No model-invented
  coordinates become observations. Semantic memory is bounded Markdown
  under `memory/`, curated by the planner with traceable supporting episode
  IDs. Runtime data is gitignored, and all memory survives restart.
- The planner can propose or revise a skill using slice 5's exact skill
  format. Candidates stay outside the admitted library until the existing
  gym gate passes. Failure cannot overwrite an admitted skill or its
  receipt. Save candidates for explicit local gate/promotion commands, not
  automatic mutation of trusted skills during ordinary play. Gate tests run
  on a separate throwaway gym instance, never by resetting the world the
  live planner is using. Demonstrate one model-authored candidate passing
  its gate and a failing candidate staying unadmitted.
- `clyde serve` runs planner/scheduler/executor against an already running
  body, with configurable memory paths and graceful shutdown. Keep
  `clyde run` and `clyde mcp` working. Write a short runbook with budget
  limits and safe local stop/start commands; no production deployment.
- Offline tests with a scripted provider and a controllable executor verify
  preemption ordering/no concurrent body ownership, autonomous re-queueing,
  chat deduplication across restart, bounded invalid planning, idle
  quiescence, memory persistence, and failed skill promotion. Prefer
  observable outcomes over internal call-order mocks.
- A live throwaway-gym demo shows two completed sequential goals, an
  autonomous goal interrupted by a player request, subsequent autonomous
  resumption, and a restart that retains memory without answering the same
  request twice. Capture the event/goal transcript and server-checked
  outcomes, plus tokens/cost and player-request response latency. Also
  exercise a failure/death and its post-mortem. Do not claim survival or
  general planning from these controlled scenarios. All build/test and CI
  checks pass at the shipped head.

## Starting point

Slices 1–4 are on main: body HTTP API, gym and shared predicates, judge,
executor and MCP. `clyde run` runs a single goal and returns a structured
outcome; it does not schedule goals or persist history. SQLite and memory
curation do not exist. Slice 5 adds skill loading, execution and admission.
Read its plan and, once shipped, its As built section before integration.

Preparation can run concurrently with slice 5: own `harness/planner`,
`harness/memory`, scheduler tests, and `prompts/planner.md`. Settle the
skill contract with its thread through the coordinator. Do not edit
`harness/executor`, `harness/cmd/clyde`, gym runner/agents or skill files
until slice 5 ships; rebase onto its actual contracts before wiring serve
and running the integrated demo. Do not create a competing skill loader
or admission store. SQLite driver choice should fit Go 1.27 and CI/orbs;
keep SQL/schema ownership in the memory package.

## Notes

- Mineflayer remains the body for now; mc26-kit research is non-blocking.
- Keep planning event-triggered and bounded in spend, context and retries.
  Reuse agent-harness instead of constructing another provider/tool loop.
- Treat player chat and retrieved memory as untrusted content, not system
  instructions or permission to change safety rules. Body safety remains
  authoritative. Scope semantic writes to the configured memory directory.
- Persist request IDs before dispatch. Document the crash window between
  sending a chat reply and recording it: without server acknowledgements,
  do not claim exactly-once delivery across that boundary.
- Production safety/kill-switch latching belongs to slice 7. Never run the
  live demo against Bonnie's server.

## Not in this slice

Multi-agent play, production deployment, classifier distillation, vector
search infrastructure, a new Minecraft client, or broad autonomous
curriculum/skill generation without budgets and the existing gym gate.
