# Slice 14: persistent player

Lachlan approved the architecture pivot in
[the coordinator discussion](https://ampcode.com/threads/T-01a1940d-be92-7625-9329-7da018f30d0d).
The product is a long-lived Minecraft player, not a succession of isolated
jobs. Implement and validate in disposable worlds; this is not permission to
change the deployed image, production memory or world.

## Problem and direction

The current planner remembers projects but hands goals to fresh executors.
Success, danger and exhausted goal budgets terminate those conversations;
the scheduler's two-recovery lifecycle can close continued work. SQLite
episodes preserve outcomes, not the executor's detailed experience. Slice 13
improved continuity inside a run without removing those boundaries.

Replace that orchestration with one durable agent session. The same model
conversation owns dialogue, projects, planning, investigation and tool use.
Thinking and actions remain bounded; reaching a boundary yields or pauses
execution without discarding identity, history or intent. Models choose
Minecraft tactics, what to remember and when to wait. Code supplies accurate
evidence, exclusive ownership, safe cancellation, durable state and limits.

Keep the TypeScript Mineflayer body, Go harness/provider interface, admitted
skills, MCP tools, independent predicates and isolated gym. Start with the
pinned `lox/agent-harness`, whose Thread/WithMessages interfaces already carry
conversation between calls. Do not replace languages or introduce another
agent framework merely to get continuity. If a necessary upstream contract is
missing, report the exact gap to the coordinator; dependency changes belong
in that repository and must be pinned, not vendored or patched in the cache.

## Runtime contract

### One session, bounded attention

- `clyde serve` becomes the persistent-player entry point. There is one model
  conversation and one exclusive body-action owner, not independent planner
  and executor agents. Keep `clyde run` as the bounded single-goal gym adapter
  and MCP as a development interface; they must still exclude another owner.
- A durable inbox supplies addressed player turns, relevant world events,
  action outcomes and wake deadlines. Apply each accepted event once to the
  conversation; detect and record body event-ring gaps. Model output is not
  authority to alter safety, trusted instructions or resource limits.
- Each inference receives a fresh bounded observation, current action and
  authoritative budget/stop state. Do not repeatedly append complete world
  snapshots to the active conversation. Keep enough timestamped evidence to
  audit decisions and distinguish stale observations from current facts.
- Action tools return promptly with an identity/status when work continues.
  Results arrive in the same session. Chat and read-only observations must
  not replace walking or other useful work. Reuse the body API rather than
  implementing a second action runner; adapt existing blocking tool waits
  only where needed for conversation responsiveness.
- New chat can interrupt stale inference, without stopping useful body work
  merely because a message arrived. Model-selected replacement cancels and
  joins prior work before dispatch. Cancellation must leave complete matched
  tool-call/result history, including explicitly withheld calls.
- Judge danger/stuck evidence changes attention, not session identity. A
  valid stuck interruption stops only its observed action incarnation. Keep
  hard safety; do not repeatedly cancel a model-selected response to the
  same persistent danger before it can act. No scripted escape/eat/flee policy.
- Provide explicit waiting for events and/or a bounded model-selected wake
  time. Persist the wait across restart. Idle waiting incurs no frontier
  polling calls, while urgent events and player messages can still wake it.
  Coalesce repetitive factual events instead of feeding an unbounded inbox.

### Durable experience and working memory

- Persist provider-compatible messages, tool intents/results, inbox progress,
  active-action identity, waits and resource accounting. Checkpoint before
  dispatching side effects. Preserve provider-native continuation data where
  required; a flattened text transcript must not corrupt subsequent calls.
- A crash can leave an action's outcome unknown. On restart stop/reconcile
  stale ownership, inspect live state and report uncertainty in the session.
  Never replay an ambiguous mutation or chat just because no receipt exists.
  Retain at-most-once chat attempts rather than claiming exactly-once delivery.
- Compact at bounded context pressure, keeping recent complete exchanges and
  model-written working notes: commitments, current approach, failed attempts,
  unresolved questions and references to supporting history. Archive original
  messages before replacing active context; publish a compaction atomically.
  Compaction failure must not lose history or create an unbounded retry loop.
- Give the model bounded paginated search/read access to original session
  history, including pre-compaction records. Keep observed places and verified
  outcomes distinct from model-authored notes; summaries are not proof.
- Reuse existing semantic notes and skill staging where appropriate. Models
  may propose reusable skills, but only isolated gym gates and explicit
  operator promotion admit executable library changes. No arbitrary host shell.

### Limits without an artificial lifetime

- Remove the old goal-count and two-recovery closure from persistent play.
  Failed actions, milestones and inference-burst boundaries do not start fresh
  identities or erase investigation history. Bounded bursts yield back to the
  event loop; neither an assistant end-turn nor a burst limit silently closes
  the project. The model can continue, wait, ask for help or abandon honestly.
- Configure finite session-wide inference allowances, charged durably before
  dispatch, plus per-call and per-action timeouts. Include compaction and any
  model-based monitoring in documented accounting. Restart, chat, failure and
  compaction must not replenish allowances. Operator extension is explicit.
  Exhaustion pauses work, stops owned action safely and preserves the session.
- Record returned tokens/cost and unknown billing separately. Do not advertise
  a hard dollar cap from post-hoc usage estimates. Use enforceable request/token
  limits or conservative reservations where supported; document their scope.
- Add an explicit authenticated/operator control to latch stop and resume.
  A stop survives restart and prevents automatic action or inference dispatch.
  Ordinary untrusted chat cannot grant resume, safety changes or more budget.
  Reuse existing operator identity configuration if it exists; do not invent
  public keyword authority. Local CLI control is sufficient for this slice.

### Verification is evidence, not the conversation lifecycle

- Expose existing goal predicates as independent checks of model-chosen
  milestones. A true result enters the conversation; it does not destroy it.
  Unsupported checks remain explicit errors, not model-scored success.
- Support only what observers can actually verify: inventory/arrival without
  RCON, construction/delivery with RCON. Keep the reserved `judged` predicate
  unsupported. Long-horizon projects need not fit one predicate; milestone
  selection and continuation belong to the model.

## Delivery and migration

One slice thread owns implementation. Work in useful, reviewable phases:

1. Implement the durable session/inbox/accounting and recovery of uncertain
   tool outcomes behind the existing tool/provider boundaries. Add behavior
   tests for restart, interruption and exact history preservation.
2. Wire persistent `serve`, asynchronous action/status, fresh observations,
   waiting, verification, memory/search and compaction. Remove the superseded
   planner scheduler from the production path rather than maintaining two
   orchestration modes indefinitely. Reuse coherent memory/candidate code.
3. Validate continuity on the real isolated stack, compare with unchanged
   source, then update CLI/container runbooks and evidence. No live rollout.

Likely owners are `harness/cmd/clyde/serve.go`, a coherent session package,
`harness/memory`, existing `harness/executor` tool definitions,
`harness/judge`, `harness/planner` shared candidate utilities, and a unified
prompt under `prompts/`. Keep body/schema changes limited to contracts the
existing HTTP API cannot express. Regenerate types for any schema changes.

Keep old episodes/notes/places intact. Import legacy project intent and pending
commitments as explicitly unverified context, not as commands to replay; old
recovery closures must not disable the new runtime. Legacy executor transcripts
do not exist and must not be fabricated. Version new persistent state and
refuse incompatible reads. Test migration on copied disposable databases.
Document stopped whole-memory backup and rollback requirements; the old binary
must not run against incompatible new state. Update obsolete flags/env settings
and the container entrypoint together, with explicit migration errors instead
of silently ignoring old budgets or changing models. Keep current model/effort
for matched trials so the experiment measures architecture, not a model switch.

## Acceptance evidence

Use the same disposable world/project and fixed disturbances for baseline and
candidate. Start with a useful short continuity smoke, then run an hour-long
project in each arm with a finite declared inference/spending allowance. Freeze
the scenario before tuning: independently check supplies, placement/delivery
milestones, remembered requirements and changes made after a player revision.
Keep a reproducible runner and machine-readable event/evidence record; do not
claim that remaining alive for an hour proves useful continuity.

During the candidate run, exercise addressed conversation during observed
movement, a revised request, an obstruction and impossible-action control,
explicit waiting/wakeup, forced compaction, and a process restart. Require
resumption without the player restating the project. Use deterministic tests
for tight crash/cancellation races and bounded limit exhaustion rather than
trying to induce all of them in one paid-model run. Force compaction through
the same production path, not a test-only transcript substitution.

Report verified progress, missed/forgotten commitments, repeated failed
approaches, reply latency during work, waiting/model-call counts, usage and
unknown billing. Preserve negative results. The initial experiment should show
continuity through compaction/restart and useful adaptation without regressing
ownership or inventing completed work; it is not proof of days-long autonomy.
If the hour-long arm exhausts its declared allowance, report that as a limit
result rather than silently topping up or replacing the run.

Retain unchanged executor/curriculum controls and all five skill gates. Add
public-contract regressions covering history after interrupted multi-tool
turns; duplicate events; unknown side effects after crash; atomic compaction
and history retrieval; waits across restart; stop/resume authority; durable
allowance exhaustion; stale action cancellation; and unsupported verification.
Do not preserve obsolete recovery-policy assertions as new session requirements.

Before shipping, run Auto-review and fix grounded findings; body tests/build;
harness build/vet/tests, changed-package race checks and generated-code checks;
real smoke/scoreboards; then rebase onto `origin/main`, ship logical commits to
`main` and confirm exact-head CI. Include evidence in commit messages. Update
this plan with actual decisions/results; the coordinator owns roadmap acceptance.

## Provider history contract and resolved prerequisite

Initial inspection at `666ea86d0bae50df44a8c75ae27a26ca25375331` found an
upstream contract gap in the pinned agent-harness
`v0.3.1-0.20261017002310-8117cc9dbc59`. The original negative probe is retained
below; it was a request-construction diagnostic, not a paid-provider trial.

The OpenAI adapter's `convertResponse` retains only `response_id` in
`Message.ProviderData`, not native output/reasoning items. `buildRequest`
automatically selects the latest retained assistant response ID and sends only
the messages after it. A locally compacted prefix therefore does not replace
the provider's previous context. Setting `Options["previous_response_id"]` to
an empty string disables that selection, but `convertMessages` then reconstructs
only text/function calls/results; the native reasoning is already lost.

A temporary public-API `httptest` probe returned a synthetic reasoning item
with encrypted content and a function call, JSON-serialized/restored the
assistant message, and submitted working notes plus the retained exchange.
`go test ./llm -run TestContinuityProbe -v -count=1` produced:

```text
persisted provider_data: {"response_id":"resp_before_compaction"}
automatic continuation: previous_response_id=true compacted_notes=false native_reasoning=false
explicit empty continuation: previous_response_id=false compacted_notes=true native_reasoning=false
PASS (diagnostic execution, not a passing continuity contract)
```

The probe used a local HTTP fixture, no provider credentials or paid inference.
Its source is attached in the
[implementation thread](https://ampcode.com/threads/T-01a19f2c-7b41-7474-aff7-9c2cbc8338df).
This demonstrates request construction and information loss, not a live API
rejection. OpenAI's [reasoning guide](https://platform.openai.com/docs/guides/reasoning)
documents preserving native output items for manual replay, including reasoning
and assistant phase; text/tool reconstruction is not equivalent native history.

The coordinator approved the published upstream revision
`582660e24f3798d364f6cec16366433f60d0441a`, now pinned as
`v0.3.1-0.20261103022158-582660e24f37`. It preserves ordered native output in
`ProviderData` and requests encrypted reasoning from the first call. Clyde
sets `Options["previous_response_id"]=""` for full replay, preserves the
complete active user/tool exchange, and removes reset after appending a new
response. The asserted `llm.TestNativeHistorySurvivesCompactionReset` passes
against this pin, including serialized reasoning and subsequent continuation.
No module-cache patches, model changes or flattened replacement history.

Upstream PR [agent-harness#13](https://github.com/lox/agent-harness/pull/13)
is published but unmerged: the Amp Review app failed before findings and its
authorized rerun returned 403. Upstream tests/CI passed; the coordinator
explicitly approved isolated integration of the published revision despite
that delivery limitation.

## Implemented journal and runtime

`harness/session` adds versioned tables alongside existing memory, leaving the
legacy checkpoint, episodes and places intact. Active messages retain opaque
`ProviderData`; original history has stable IDs and paginated search. A single
transaction publishes checkpoint changes, history, consumed inbox IDs and usage
receipts. Duplicate source keys cannot re-enter the conversation after restart.
The event loop coalesces factual batches and bounds the pending user turn to
32KB while stopped/exhausted, retaining original events in history. Search
returns at most 32 records/24KB of content with per-record truncation flags;
byte-offset paging can retrieve the rest of a truncated record. Native opaque
fields remain untouched on disk, not copied into the text view.

Inference and compaction reserve the same finite request allowance before
dispatch. Reservations survive missing receipts and restart; returned usage
and unknown billing remain distinct. Operator stop and explicit extensions use
separate SQL columns, so stale runtime checkpoints cannot overwrite them. The
provider path disables hidden SDK retries for persistent play only. These are
request limits, not a dollar cap. Output/deadline limits are documented below.

Restart reconciliation stops the body and obtains a fresh observation before
publishing any state. Started tools without receipts become explicitly unknown;
unstarted calls receive withheld results. Neither replays, and the original
assistant message stays intact. Quiet waits survive; interrupted work wakes for
attention while the operator stop remains latched. Failed stop/read prevents
reconciliation. Tight crash windows have HTTP-fixture coverage; the real-stack
restart evidence below is a clean process restart, not a crash simulation.

Validation: harness build/vet/tests passed; `go test -race ./session -count=10`
passed. Regressions cover journal restart/atomic publication, old memory retention,
duplicate events, history retrieval after active-context replacement, operator
stop persistence, durable accounting, concurrent allowance exhaustion, incompatible
versions, and unknown/withheld tool results without replay. The context replacement
test validates storage only, not production model compaction. Local Auto-review
found no material issue in this foundation. Copied legacy-database CLI tests
also preserve checkpoints/episodes and import only once without replay/refill.
Roadmap acceptance remains with the coordinator.

### Runtime decisions

- `serve` calls the pinned provider's public `Chat` boundary once per attention
  turn, rather than wrapping an unjournaled multi-tool `Run`. The session owns
  durable dispatch boundaries and cancellation; it reuses executor tool
  definitions/body actions. `run` and MCP keep their existing behavior. Local
  advisory body/memory leases exclude other local Clyde owners; remote or
  aliased HTTP controllers remain an operator responsibility, not a distributed
  lease guarantee.
- Defaults: 240 shared durable reservations, 45s frontier calls, 4096 output
  tokens, 128KiB serialized-history compaction pressure. Current observations
  are bounded to 32KB. Body action limits remain; tool dispatch is bounded to
  12s. Profile changes fail on restart instead of silently changing settings.
- Every Jev attention tick reserves one request conservatively, including
  precheck-only ticks. Jev runs before frontier attention, not as paid idle
  polling. Local deterministic danger/stall checks poll at 500ms. This cadence
  change is part of the architecture experiment, not a model/effort switch.
- First observed danger stops its action once; persistent danger does not
  repeatedly veto useful responses. Stalls still stop only the observed token,
  including when danger remains active. Player messages cancel stale inference
  without stopping movement. All withheld tool calls receive matched results.
- Operator `control status|stop|resume|extend|compact` uses local filesystem
  authority. Stop is independent of runtime checkpoints; dispatch intents are
  serialized against it. Extension does not clear stop. Resume clears an error
  pause without granting requests. Real compaction failure preserves history
  and pauses instead of automatically retrying forever. Attention cancellation
  supersedes compaction without pausing or refunding the charged request.
- New version-2 `player_*` tables retain legacy memory. Project/dialogue/jobs
  import once as unverified context; recovery closure is not imported as policy.
  No historical executor messages are fabricated. Runbooks require a stopped
  whole-memory backup and matching image+snapshot for rollback.

Public runtime regressions cover cancelled inference, stale multi-tool batches,
wait/restart, stop authority and extension, allowance exhaustion, native
compaction/reset and failure, historical retrieval, and independent milestone
success/errors without ending the session. Changed-package race tests and full
harness build/vet/tests pass; body tests pass 26/26, build/generation pass with
no generated drift. Auto-review identified and corrected a stale extension
overwriting operator stop, incomplete receipt decoding of active tokens,
unbounded paused-event accumulation, restart gap detection, danger masking a
stall, and an ignored `--judge-log` flag. Final targeted review also corrected
dispatch after the last reservation exhausted the allowance, and added paging
beyond truncated history records. The exhaustion regression failed with one
extra action before the fix and passes with zero after it. A public-provider
429 regression confirms one reserved call cannot hide SDK retries. Auto-review
and its final targeted recheck found no remaining material issue.

Commands: `go build ./...`, `go vet ./...`, `go test ./...`,
`go test -race ./session ./cmd/clyde ./llm ./executor -count=10`,
`go generate ./...`, and body `npm test`, `npm run build`, `npm run gen`.
All passed; generated types have no drift. Docker lifecycle/host rollout was
not exercised locally; the deployment runbook keeps that separately required.

### Matched isolated evidence

`gym/continuity.mjs` freezes a short smoke and two hour-long arms on disposable
Paper worlds with the same seed, arena and scripted disturbances. An external
forwarding proxy leaves provider requests unchanged and caps each full arm at
120 frontier dispatches (including SDK retries); candidate reservations also
bound Jev/compaction. It records requests, native item types, RCON results,
player messages/action events and restart timing. Models remain Sol 6.1 low/Jev.
No production image, memory, world or roadmap was changed.

Both full arms completed 3,600 seconds on 2026-11-03. Baseline was unchanged
`666ea86d0bae50df44a8c75ae27a26ca25375331`. [14-evidence.json](14-evidence.json)
contains the frozen scenario/source/binary hashes, request/response metadata,
timestamped chat, independent RCON evidence, milestones, compaction notes,
usage and scoreboards. The runner was formatted after launch; its original
and formatted hashes are retained, with no scenario changes. Final source
adds three safeguards after the candidate binary was frozen: last-reservation
action withholding, history byte-offset paging and attention-safe compaction
supersession. They have public regression coverage, not a second paid hour-long
run. The added retry regression changes tests only.

| Check | Baseline | Persistent player |
|---|---|---|
| First supplies checkpoint | Four planks and table in inventory | Failed: six planks, no table |
| Revised destination (12,80,0) | Within two blocks | Within two blocks; independently verified |
| Table at (3,80,3), eight planks in chest | Both verified | Both verified; recovered from earlier supply failure |
| Obstruction: six new trunk logs | Six added (13 → 19) | Three added (16 → 19); earlier work already consumed three trunk logs; accurately reported incomplete |
| Impossible bedrock request | Declined without attempting | Attempted once, reported failure, did not repeat |
| Row after restart, without restating wood/revision | Three birch blocks verified | Three birch blocks verified after compaction and restart |
| Remember birch/avoid oak and destination revision | Retained | Retained; final reply also retained incomplete/failed work |
| Status reply during observed movement | 2.746s | 3.693s |
| Revision acknowledgment | 4.342s | 7.327s |
| Frontier calls during four-minute quiet-wait probe | 0 | 0 |
| Frontier dispatches / external cap | 52 / 120 | 78 / 120 |
| Shared durable reservations | Legacy policy | 155 / 240: 77 inference, 77 monitor, 1 compaction |
| Returned frontier + Jev cost | $0.373757836 | $2.440643418 |

Compaction used the production operator control. Candidate request 62 replayed
native reasoning/tool history and returned HTTP 200 with 6,262-character notes;
request 63 reset onto notes plus the complete retained exchange; request 64
continued from request 63's new response ID. The same database survived restart.
The next player request omitted the agreed wood and old/revised coordinates;
Clyde recalled both and the independent `built` check returned `3/3 blocks match`.
RCON chat markers corroborated the table and row at later/final checkpoints;
an empty response to `execute ... run say` alone is not treated as verification.
Candidate finished waiting, with 319 archived records, no error pause, no unknown
reservations and no allowance extension. This demonstrates bounded continuity,
not a capability, latency, cost or days-long-autonomy gain.

The unchanged 12-task executor curriculum passed **12/12 before and 12/12
after**. All five unchanged model-free skill gates passed: collect-logs,
craft-item, craft-place-table, deposit-items and eat-food. Gates used the same
skill sources in both checkouts; this is one shared five-gate run, not two.
No skill was authored/promoted and no tactical benchmark exception was added.

Retained negatives: smoke 1 hit raw-crafting failures and an experiment RCON
empty-response deadline, fixed before full arms. Its first proxy also failed
to decode gzip and labelled replies `streamed`; the durable journal supplies
its returned usage. Smoke 2 completed native replay/restart and recalled the
project, but final RCON inventory contained eight planks, not the model's
claimed twelve. Intermediate verification is timestamped evidence, not proof
of final inventory. Neither smoke was discarded from accounting.

Returned estimates including both smokes, both full arms and both control
sets total **$3.976797222**. The full baseline has one frontier dispatch without
returned usage (request 40), plus two Jev timeout records; baseline controls
have two more Jev timeouts. Control frontier calls were not proxy-instrumented,
so their unreturned billing cannot be bounded from outcome summaries. The
candidate and smokes have no unknown journal reservations. These are returned
usage estimates, not an invoice or hard dollar cap. Neither arm hit its declared
allowance; the $5 review threshold was not presented as enforcement.

Reproduce only on disposable servers: build the baseline/current binaries,
prepare the same `mcbot` Paper template on local MC/RCON ports 25567/25577 and
25565/25575 respectively, install body dependencies and gate the unchanged
skills. Keep OpenAI Sol 6.1 low and Jev; remove model overrides and obsolete
planner overrides for the candidate. Run
`node gym/continuity.mjs before|after BINARY SOURCE_ROOT NEW_OUTPUT_DIR`.
The runner refuses to overwrite earlier output. Its source documents the fixed
disturbances; actual provider model/effort is recorded per request. Run controls
on a third disposable server so world resets cannot contaminate either arm.
Roadmap acceptance and any separately authorized host rollout remain with the
coordinator.

## Coordinator follow-up: compaction supersession

The coordinator found that an addressed message during compaction latched an
operator-only pause. Inference now distinguishes attention cancellation from
provider/deadline failure. Superseded compaction preserves the original active
history and archived partial response, settles its receipt, and consumes pending
events without operator resume. Automatic pressure may require another charged
compaction with those events before continuing. No allowance is refunded or
extended; operator stop remains authoritative.

`TestPlayerChatSupersedesCompactionWithoutOperatorResume` failed before the fix
for both requested and automatic compaction (`condition did not arrive`). After
the fix, both modes reply once and enter explicit waiting, retain the earlier
birch requirement and original archive, exclude the stale summary, and keep the
limit at 10. Requested compaction charges 2 requests and 48 input/10 output
tokens; automatic compaction charges 3 requests and 71 input/15 output tokens,
including the cancelled receipt. The existing failure regression now also
checks deadline and empty-summary cases: each retains history and pauses after
one attempt. These are public runtime tests with fixture providers, not paid
model observations or additions to the frozen hour-long scoreboard.

Validation: `go test -race ./session -run Compaction -count=10` passes, as do
full harness build/vet/tests and changed-package race tests (`session`,
`cmd/clyde`, `llm`, `executor`) ten times. Body tests remain 26/26 and its build
passes; generation has no drift. Local Auto-review of the follow-up, including
its final targeted recheck, found no remaining material findings.

The local harness runbook/package map now describes the persistent player and
accepted startup flags, controls and migration boundaries. Root/skill/deployment
docs agree; historical slice plans are unchanged. No production changes or
roadmap acceptance are implied.

## Boundaries

No production rollout, multi-agent system, new vision stack, provider switch,
automatic skill promotion or tactical recovery library. Do not turn the new
session into a second planner/executor scheduler with different names. If a
required safety or persistence contract cannot be implemented with the existing
interfaces, surface the precise blocker rather than weakening it to finish.
