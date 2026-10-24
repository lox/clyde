# Slice 11: conversation and remembered projects

Goal: Clyde can converse across turns, clarify a request and carry out a
remembered multi-step project. Models choose meaning and strategy; code owns
faithful context, durable execution and safety, not a dialogue or crafting tree.

## Done when

- Reproduce the current limitations on the real chat/event/planner path:
  a follow-up requiring dialogue context, a clarification exchange, and a
  project whose intermediate goal does not complete the original request.
  Capture unchanged transcripts/outcomes before selecting the interface.
- Provide bounded recent dialogue with speaker/request identity and relevant
  durable project/memory context to the model. The model can answer, clarify,
  update/cancel/continue intent, choose a measurable next action or explicitly
  wait/idle. Do not force every conversational turn into a Minecraft goal,
  invent completion predicates for speech, or parse intents with keywords.
  Preserve simple addressed public chat/whisper routing; do not react to all
  ambient player conversation or invoke models repeatedly while waiting.
- Persist the model's ongoing project intent and progress separately from a
  single executor goal. On goal results the model chooses the next step,
  revises the project, asks for help, reports verified completion or idles.
  A statement of intent is not an observed fact or proof of completion. Do
  not add deterministic wood-to-tools/building recipes or auto-advance steps
  because the harness recognises an item/task name. Start with one active
  project rather than a general workflow engine or many competing projects.
- Natural follow-ups can refer to prior answers, project context and observed
  locations. A question or clarification does not inherently cancel a useful
  running action; the model decides whether requested work changes. Genuine
  revised instructions can preempt/replan safely. Conversational replies must
  not inadvertently replace movement via the existing chat-as-action path.
  Resolve that at the owning boundary with the smallest coherent change.
- Restart preserves the dialogue/project state and the established reply
  intent, event deduplication, predicate reconciliation and ownership rules.
  Do not claim exactly-once delivery or replay arbitrary side effects. Older
  memory/checkpoints remain readable or receive a narrow explicit migration.
- Demonstrate useful remembered preferences/agreements after restart and
  context pressure: at least one earlier project fact lies beyond the recent
  dialogue window, yet affects correct action or explanation. The model owns
  curation/selection of semantic context; code preserves size limits, evidence
  provenance and observed spatial facts. Reuse existing SQLite/notes before
  adding retrieval services or an unbounded conversation transcript prompt.
- The experience-to-skill path remains model-led and testable: use a repeated
  project procedure to elicit one candidate or a reasoned decision that an
  existing skill suffices. If a candidate is authored, capture authorship,
  gate exact contents in a separate throwaway world, and explicitly promote
  only on pass; a failed candidate must leave the installed library unchanged.
  Do not build automatic promotion or require gratuitous new skills.
- Offline observable tests cover conversational no-action, clarification and
  follow-up, intent replacement versus preservation, restart/deduplication,
  bounded context and bounded idle behaviour. Extend existing tests. Live
  isolated scenarios use a real mineflayer player through ordinary chat:
  discuss/clarify a project, complete at least two model-chosen intermediate
  goals with independent server checks, answer during work, handle a revised
  instruction, restart, then recall/continue the agreed project correctly.
  Include a failure/impossible-request case and honest reply, not fake success.
- Run matched before/after scenarios plus existing executor/gym controls,
  skill gates and slice 10 recovery/preemption invariants. Record full dialogue,
  supplied prerequisites, actual outcomes, response latency, tokens/cost and
  all failures. Keep tasks/checkers/settings/budgets fixed across comparison;
  no wording-specific strategy, weaker judge or enlarged final-only budgets.
- Run Auto-review and relevant builds/tests/vet/race/generation; ship small
  logical commits to main and verify exact-head CI. Update this plan and
  runbooks with actual interfaces, evidence, limits and next bottleneck.
  Request focused Oracle review if the ownership boundary remains unsettled.

## Starting point

At `d283216`, planner input has one Request, up to 16 recent episodes,
observed places and bounded semantic notes. Decision is one Goal or Idle,
one Reply, notes and an optional skill candidate. Scheduler persists reply
intent before sending but removes a player job after its executor goal;
ongoing intent is not first-class. Chat actions replace the body's current
action. Inspect these paths rather than layering around them:
`harness/planner/{planner,scheduler}.go`, `harness/memory`,
`harness/cmd/clyde/serve.go`, body chat/event/action APIs and role prompts.

Slice 10 fixed model-led bounded recovery. Preserve its durable allowance,
active recovery judge, safety and preemption semantics. Global normal-Jev
history was rejected after regressions; do not reintroduce it incidentally.
Read slice 6/9/10 As built and `AGENTS.md`'s model-deference principle.

## Notes

- Keep Sol 6.1 low for planner/executor and Jev. Initial experiment allowance
  is US$5 of reported provider usage including Jev where available; report
  before exceeding. Unknown/unreturned billing is not a hard spending cap.
- Prefer extending the current planner contract and memory owner over adding
  a conversational agent/service, project DSL, semantic classifier or second
  scheduler. New abstractions must have a current responsibility and consumer.
- Player text and retrieved memories remain untrusted data; they cannot set
  recovery modes, change admission/body safety, invent observations or reset
  execution budgets. Awaiting clarification must not consume unbounded calls.
- Experiments run only in this slice thread's disposable orb worlds and
  separate memory. The media-ovh world/image and Bonnie remain untouched.
  Coordinator accepts the result and decides any later live rollout.

## Not in this slice

Deployment, unrestricted ambient chat, multiple concurrent projects/agents,
unbounded dialogue history, automatic trusted-skill mutation, model training,
hard-coded Minecraft strategy, a new orchestration framework or claims of
general long-session survival.

## As built

### Demonstrated gap and selected boundary

Baseline is [5e466d0](https://github.com/lox/clyde/commit/5e466d0).
Real Alex chat already clarified supplies, chained independently checked birch
planks into a retained crafting table, curated preferences, and recalled birch
after seven no-task turns and restart. Those are counterevidence to rebuilding
language understanding or memory retrieval, not new capabilities of this slice.
The demonstrated deficits were ownership and latency: a non-changing wood
question cancelled/restarted the table goal; a question during player-owned
travel waited **44.173s** for arrival in the original reproduction. The matched
expanded baseline also deferred revised travel instructions until old travel
finished. Its eventual status reply was contaminated by later queued dialogue.

The existing planner and SQLite checkpoint remain the owners. There is no new
agent/service, classifier, recipe tree, project DSL or skill admission path.
The coordinator's focused read-only Oracle endorsed this boundary before
implementation; that endorsement was not a source review.

- One read-only planner may run beside one exclusive executor. Accepted turns
  and replies remain serialized by the scheduler. The model selects
  `work: preserve` (default) or `replace`. No goal + preserve is conversation,
  not terminal cancellation. A new goal + preserve suspends old work; replace
  discards old goals after cancellation/join, including on crash/restart.
- Input adds the last **12** addressed speaker/request turns, goal-bearing
  current/suspended work, and one **4096-byte** model-written project summary.
  Omitted/null project preserves it; empty string clears it. Source request
  identity is retained. This is untrusted intent, never certified progress or
  an observed location. Existing 16 recent episodes, code-observed places and
  cited 4KiB/note, 16KiB-total semantic memory remain unchanged.
- Decisions, project updates, exact reply text and send intent are checkpointed
  before sending. There is at-most-once **attempt**, not guaranteed delivery:
  a crash after intent but before network send can lose the reply. Old
  checkpoints default new fields without discarding jobs, intents or recovery.
- Executor completion, replacement, death and danger transitions invalidate a
  delayed decision atomically: no stale goal, reply, notes or project applies.
  Ordinary position updates do not invalidate it. One refresh is permitted per
  player request, sharing the original **30s** deadline, charged durably before
  dispatch; restart does not mint calls. Each plan retains the existing two
  validation attempts. Exhaustion answers honestly rather than looping.
- Body chat returns its own immediate final receipt/events and preserves both
  current and retained last exclusive-action status. It cannot abort movement
  or hide a just-finished result from `waitAction`. Other actions still replace.
  The executor prompt leaves dialogue/revised goals to the planner, rather than
  creating a second responder from observed chat.
- Slice 10's two-execution recovery allowance, harness-assigned contexts, active
  Jev, body safety, independent predicates and cleanup-before-new-owner remain.
  Preserve conversation does not refill/close recovery or clear its pause.
  Explicit recovery stop/post-mortem idle still closes it; only successful
  execution plus fresh safe observation resets it. Allowance admission happens
  before cancellation, so a refused preserve goal leaves the charged executor
  running. Explicit no-goal replacement closes the existing episode even when
  the requesting player has ordinary context in a safe state. The dispatch-time
  allowance guard remains. No global normal-Jev history or numerical damage
  veto was introduced.

The current operator contract, bounded planning and crash windows are in the
[local runbook](../../harness/README.md). Full dialogue, supplied fixtures,
server checks, model decisions, source/task/skill hashes, usage and exclusions
are retained in [11-evidence.json](11-evidence.json).

### Real-player comparison and controls

Matched trials used disposable orb worlds and separate memory, Sol 6.1 low for
both roles, active Jev, five unchanged admitted skills, executor 40 steps/300s,
planner 30s and the same task/checker files. Full-chat arms use the same driver,
wording, supplied 16 birch logs and peaceful forceloaded grass arena. The only
test curriculum addition, identical in both arms, suppresses unrelated initial
autonomy and asks the model to explain existing-skill sufficiency when requested.
The player names desired supplies; models choose goal boundaries, predicates
and tools. No executor program, injected planner decision or crafting strategy
is supplied by the fixture.

| Observation | Unchanged matched arm | Reviewed final arm |
|---|---|---|
| Non-changing wood question | 4.131s; table goal cancelled/requeued | 2.772s; work preserved |
| Status during player travel goal | 32.838s; answered later revised destination | 3.101s; answered current task without replacing it |
| Revised destination request | 43.464s; eventual reply drifted to later diamond request | 9.542s; revised arrival independently checked at 1.76 blocks, radius2 |
| Supplies | Independently verified planks4, then table1 | Independently verified planks4, then table1 |
| Seven no-task turns, restart, recall/continue | Birch preference recalled; planks8/table1 | Birch/project recalled beyond 12-turn window; planks8/table1 |

Latencies above are durable request acceptance to send attempt, not guaranteed
delivery or a latency distribution. In the clean final full run the status
answer preceded actual movement; its executor was already working on travel.
A supplemental final probe therefore waited for **observed movement**, not a
fixed sleep: the answer took **2.553s**, with the same running action **3** as
x advanced **5.2 → 18.2**. A genuine subsequent model-selected replacement
cancelled/joined that action rather than resuming the old destination (85,80,0).
RCON/predicate independently verified arrival **1.74 blocks** from (12,80,0),
radius2.
Its unchanged companion is a retained **negative**, not an invalid fixture:
the model falsely claimed ordinary walking had exhausted its allowance and
never moved. It supplies no paired motion-latency number. No keyword handler,
budget override or prompt exception was added to force that probe to pass.

The full final dialogue honestly refused obtaining a diamond without digging
or receiving items. Asked about the repeated procedure, the model explained
that `craft-item` already supports planks and tables, so repetition alone did
not justify another skill. No candidate was authored, automatically gated or
promoted. Existing deterministic gates were explicitly rerun in the separate
disposable world, serialized after model trials: **5/5 before and after**
(collect18s, craft8s, place2s, deposit1s, eat2s). Frozen original/final executor
controls each passed **12/12**. Their reported Sol usage was respectively
154,985/$0.0750734 and 190,733/$0.089012; Jev $0.067981494/$0.075329898.
This is one trial per task, not a claim of improved general reliability.

Combined returned usage, **including every excluded/failed arm**, was
**1,763,692 Sol tokens / $1.8624802**, plus **$0.249694284 Jev**, total
**$2.112174484** of the initial $5 allowance. Matched full-chat usage was
176,037/$0.2007069 + Jev$0.009760884 unchanged, versus
205,906/$0.256735 + Jev$0.007657482 final. Bounded richer context costs more;
there is no cost-reduction claim. Unreturned/cancelled billing remains unknown.

Preserved exclusions: initial unloaded-chunk/fall fixture; the first 12/12
executor arm run while source/prompts changed; the preliminary final before
review tightened its shared deadline from60s to30s; a confirmation started
with a fresh database but stale body event ring; and a motion probe starting
before body spawn (HTTP503, no model call). These are not recast as capability
failures or improvements. Valid model failures remain visible. The final
full-chat run had one Jev backend deadline gap; original controls had one.
No judge threshold or checker was weakened. `player_request` Jev decisions are
still notifications: the existing executor only cancels for danger/stuck.

### Validation and remaining limits

Auto-review's sequential ship-risk/simplicity pass and targeted recheck found
and fixed crash-durable supersession, project-clear final reply suppression,
and cancellation-time checkpoint restoration. No material finding remains.
Coordinator source review subsequently found cancellation before allowance
admission and a safe-state replacement-stop that left recovery open. Both
regressions failed on the initial shipped code and pass after the narrow
fix-forward. Tests hold execution two running during rejected preserve and
explicit replace, and stop execution one while safe, then restart and process
fresh danger/death without replanning, execution or allowance refill. No new
provider experiment was needed; the matched model transcripts above describe
the original measured build, not a rerun of this deterministic fix-forward.
The body regression fails on unchanged code (chat replaces goto identity).
Final `npm ci`, tests **23/23**, build and type generation passed. Harness
`go build ./...`, `go test ./...`, `go vet ./...`, `go test -race ./...` and
`go generate ./...` passed; planner/executor/judge/memory also passed **ten**
consecutive race runs. Generated files have no drift. Existing recovery tests
plus extended public scheduler tests cover preserve/replace and cleanup,
stale result rejection, exact reply intent before send, restart/deduplication,
bounded dialogue/project and planning allowance, and persistent/exhausted
danger without laundering. One parallel body run hit its unchanged 20ms eval
timeout test; the unchanged rerun passed, without weakening that test.

The next bottleneck is model/context reliability, not more orchestration.
Remembered intent is not a complete verified progress ledger: both full arms
correctly declined to certify older steps no longer supported by recent
evidence. Notes/project retain useful preferences, but do not promise general
long-session recall or automatic continuation while explicitly waiting. Turns
are serialized and model/provider latency still applies; there is no universal
response-time guarantee. Old checkpoints remain readable, not safely
downgradable after accepting new replacement intents. Startup still needs a
spawned body; fresh experiment memory needs a fresh event ring. Recovery,
admission and body safety are unchanged. No production world, media-ovh/Bonnie
service, live image or deployment was touched; coordinator acceptance and
any future rollout remain separate.
