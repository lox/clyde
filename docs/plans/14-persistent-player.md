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

## Boundaries

No production rollout, multi-agent system, new vision stack, provider switch,
automatic skill promotion or tactical recovery library. Do not turn the new
session into a second planner/executor scheduler with different names. If a
required safety or persistence contract cannot be implemented with the existing
interfaces, surface the precise blocker rather than weakening it to finish.
