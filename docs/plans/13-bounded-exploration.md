# Slice 13: bounded model-led exploration

Goal: give Clyde room to investigate failures, try different approaches and
complete a requested task, without scripted Minecraft tactics, unbounded
retries or invented permission limits. Models choose experiments; code supplies
observations, ownership, independent checks and truthful durable budgets.

## Contract

Keep the existing 40-model-turn/300-second execution and outer two-recovery
policy. An action failure is evidence for another experiment, not by itself a
reason to terminate the executor. Continue in the same conversation and budget;
neither a failed tool nor a stuck notification starts a fresh allowance.

1. **Make stuck action-local.** A terminal action returns its failure normally;
   a stuck verdict for an absent or terminal action is inapplicable. A valid
   live-stall verdict stops only the action it observed, then delivers the
   reason and fresh state to the executor before further stale plans run.
   Cover actions whose tool already returned "still running". Danger, caller
   cancellation, predicate success and run-budget exhaustion retain their
   existing run-level effects.
2. **Make cancellation ownership-safe.** Give each action an opaque unique
   token, carried through status, judge evidence and conditional stop. Numeric
   action IDs restart at one and are insufficient. The body atomically stops
   only a matching current owner; a mismatch clears no controls. Terminal
   failures clean up their movement before another action can start. Preserve
   unconditional operator stop. Update schemas first and regenerate types.
3. **Show actual permission, not a guessed story.** Planner/executor input
   exposes current ownership, current-request admission eligibility, attempts,
   closed status and closure provenance. Future closure reasons distinguish
   voluntary idle, explicit stop and exhaustion; old unknown provenance stays
   unknown. Closed with zero attempts is not numerical exhaustion. A newly
   accepted safe player request can select an ordinary bounded goal without
   resetting recovery, unpausing background autonomy or refilling anything.
4. **Give the model useful physical evidence.** Add a compact body-generated
   local collision observation, including adjacent feet/head cells and unknown
   cells, plus actual action target/error where available. Prefer this over
   new tools. Any executor inspection via `/query` uses fixed reviewed
   read-only code; the concurrent planner receives data, never arbitrary JS.
   Models decide whether to break, move, reroute, change target or ask for help.

Implement ownership/cleanup and observations first, then judge/executor
continuation and planner facts/prompts. Validate the combined behavior in
matched isolated trials before considering any change to outer recovery policy.
No new service, workflow DSL, agent-harness upgrade or budget ledger is needed.

## Done when

- Reproduce the wood incident on unchanged source in a disposable world:
  partial gathering, a path/search failure, a previously closed zero-attempt
  recovery episode, and the owner's follow-up instruction. Separate the
  terminal-action judge interruption, missing physical evidence and false
  allowance explanation; do not assume one change fixes all three.
- A failed action returns its actual result and fresh relevant observations to
  the model with an opportunity to inspect and revise its approach. A stale
  stuck verdict about a completed/replaced action cannot kill later work.
  Genuine stalls still stop their owning action promptly. Danger, explicit
  cancellation, body safety and bounded execution remain effective.
- Planner and executor receive authoritative control facts: current owner,
  actual budget/admission state, paused versus exhausted status, and the
  recorded cause/provenance of a pause. Model-written project summaries and
  old replies cannot establish those facts. A safe player request must not
  be refused merely because an unrelated earlier recovery voluntarily idled.
- Supply compact local physical evidence sufficient to distinguish an open
  space from a canopy/wall/overhang. Respect no-x-ray, unloaded/unknown cells,
  collision shapes and limits on size/latency. Keep resource counts distinct
  from reachability. Reuse existing executor tools before adding capabilities;
  do not expose arbitrary `/query` JavaScript to the concurrent planner as if
  it were enforced read-only.
- Demonstrate that useful adaptation retains the same conversation and original
  40-turn/300-second run budget. Test terminal timeout followed by false stuck,
  genuine live stall, and late verdicts across action replacement and body
  restart. Conditional mismatch must never stop the new action. Retain the
  existing charged-before-dispatch outer recovery accounting and restart
  semantics; no notification/chat/preemption refills it. A legacy closed
  zero-attempt episode must be represented truthfully during safe follow-up
  planning. No periodic model calls while paused or keyword renewal handler.
- Prove useful model-selected changes of approach in more than one physical
  fixture, including the enclosed-tree reproduction and a different obstruction
  or unreachable target. Verify the original task outcome independently, not
  merely that an escape action ran. Include genuinely impossible, exhausted,
  real-danger and motionless-stall controls; never weaken safety/checkers or
  supply a fixture-specific escape program.
- Preserve slice 11 conversation during work, real replacement/cleanup,
  durable reply intent, restart/deduplication and bounded memory. Run unchanged
  executor/curriculum controls and five deterministic skill gates. Report
  failures, supplied prerequisites, latency, turns and returned cost for fixed
  before/after arms, not a success-only demonstration.
- Run Auto-review, body/harness builds/tests/vet/race/generation and exact-head
  CI. Update this plan/runbooks with actual semantics, compatibility and
  evidence. Ship through Clyde's main workflow; coordinator accepts capability
  evidence separately from any future infrastructure/image rollout.

## Starting point

Deployed source is `7700696`; source main adds plans through `2faa38d`.
The owner has now successfully joined from Java. This incident was investigated
read-only by the [host owner](https://ampcode.com/threads/T-01a14894-8624-72d8-af3c-2c59e8fb5d5b)
and reported in `.amp/in/artifacts/clyde-wood-incident-20261025.md` there.
Do not copy private raw captures, credentials or player identifiers into Git.

The real request promised eight birch logs. The executor collected two, then
body action4 failed after15.059s with `Took to long to decide path to goal!`.
Jev returned stuck0.97 after the action failed; all preceding33 ticks were
non-interrupting. Current feet/head cells were air with cardinal leaf enclosure;
the exact failed target/path was not recorded. The normal notable-block card
excludes leaves. The collect plugin's `ignoreNoPath` is not proof of skipping
unreachable targets; the installed plugin has an unimplemented branch there.

No recovery execution was charged for this incident. The durable episode was
`attempts:0, closed:true` from an earlier voluntary post-mortem idle; subsequent
failure therefore queued no recovery. The owner's leaf-clearing instruction
received ordinary safe context, but the model returned idle and falsely said
its attempts were exhausted. No proposed action was rejected by admission.
The cause of that model assertion is unrecorded, not proven to be the missing
status alone. Planner input currently lacks the full global recovery state.

Relevant owners are `harness/executor/{executor,tools}.go`,
`harness/judge/{judge,precheck}.go`, `harness/planner/{planner,scheduler}.go`,
their prompts, `body/src/{actions,state}.ts`, `api/` and existing tests.
The executor already has arbitrary code-as-action plus admitted skills.
`harness.Run` already retains tool errors for another model turn; executor
currently cancels its whole context for both danger and stuck. Judge records
have a card timestamp but no action identity on the interrupt contract.

## Notes

- Oracle recommends this smaller first slice: the incident demonstrates a
  premature executor exit, not two charged recoveries being exhausted. The old
  closed episode still prevents a post-mortem after a run ends; this limitation
  remains. If matched trials show useful exploration repeatedly hitting that
  outer boundary, propose a separate durable allowance contract with evidence.
  Better facts cannot guarantee the model chooses useful work or speaks truly.
- Keep Sol6.1-low for both roles and active Jev; no threshold/model/resource
  tuning to make a single benchmark pass. Initial implementation-experiment
  allowance is $5 returned provider usage including Jev; report before extending.
  This is not a hard invoice cap when cancelled/unreturned billing is unknown.
- Experiments belong in isolated disposable worlds/memory, never the attended
  live Clyde world or Bonnie. This plan authorizes no live reset, move, allowance
  refill, prompt/config edit or image deployment.

## Not in this slice

Scripted tree escape, keyword tactics, a workflow DSL or new agent service,
automatic skill promotion, arbitrary concurrent world mutations, hard-dollar
billing guarantees, durable project-budget redesign, raising recovery limits,
broad pathfinder rewrites, latency tuning or deployment. Lachlan approved
implementation and isolated validation of this plan; live rollout remains a
separate decision.

## Implemented semantics

- Every action receipt/status has a UUID `token`, including chat receipts;
  exclusive actions own the body. Conditional `POST /action` stop compares
  that token atomically before any abort, halt or event. Terminal/replacement/
  restart mismatches return `stopped:null` and do not clear controls.
  Unconditional operator `/stop` is unchanged. Every terminal exclusive owner
  expires detached work and halts movement/digging before replacement.
- Judge stuck applies only to an observed running, token-bearing action.
  Terminal/absent-owner verdicts are logged as inapplicable. The stall clock
  keys on tokens, not restartable numeric IDs. The executor serializes action
  dispatch with conditional stop, but never holds that lock while waiting for
  an action. Primitive, eval and admitted-skill dispatch all use this path.
- An applicable stuck stop feeds its actual result/reason and fresh state into
  the same conversation. Failed/aborted tools also attach fresh state, including
  immediate failures. A stop during inference withholds that stale response,
  including an end-turn, while preserving returned usage and charging its
  original turn. There is exactly one provider call per harness turn, no
  nested retry loop and no new time/turn allowance. Danger, cancellation,
  predicate success and exhaustion remain run-level controls.
- The card adds exactly twelve local contact cells: below, feet, head,
  overhead, and four cardinal feet/head neighbours. Names and actual collision
  boxes distinguish air, solid/partial collision and unknown/unloaded cells;
  at most sixteen boxes per cell, with explicit truncation. This is not a
  volume/resource scan or reachability claim. Wrapped path/dig calls record
  the actual attempted target where available. No arbitrary concurrent query
  capability was added; executor JS remains an exclusive body action.
- Planner and executor receive harness-computed admission/ownership snapshots,
  attempts/limit/closed, autonomy pause and closure provenance. Snapshots are
  recomputed per call, not persisted as checkpoint authority. Future closures
  record the first cause (`voluntary_idle`, `explicit_stop`, `exhausted`);
  legacy missing provenance stays `unknown`. Ordinary safe follow-up can be
  admitted while recovery is closed and autonomy paused. Acceptance alone
  neither charges/refills recovery nor unpauses autonomy; the existing
  successful-execution plus fresh-safe-observation reset remains unchanged.

Upgrade body and harness together: old bodies cannot provide incarnation-safe
ownership. Checkpoint additions are additive. The coupled upgrade and matching
image/whole-memory rollback requirement are documented in `deploy/README.md`.
Models, reasoning effort, Jev configuration/thresholds, five admitted skills,
40 turns/300 seconds and outer two charged-before-dispatch recoveries are
unchanged. No tactical escape code or new skill was added.

## Isolated evidence

The durable machine-readable record is [13-evidence.json](13-evidence.json):
fixed-arm scoreboards, outcomes, transcripts, returned usage, source hashes,
validation logs and reproducible scratch drivers. Only disposable orb worlds
and offline test identities appear there; no host captures/playerdata were
copied. Before source is `ddfb9ee6ac2ae756e1feed6c29c24fa59d70abf0`, frozen
with `git archive`, not the changing working tree. Both arms use Sol6.1-low,
active Jev and identical fixtures/prerequisites. Gym checkers independently
verify the original item/build/delivery/arrival predicates over RCON.

### Fixed obstruction arms

Two repetitions each, no selected reruns. Seconds are executor wall time,
excluding world reset; scoreboard detection can lag by a second. Costs below
are returned Sol plus Jev, not invoice guarantees.

| Fixture/arm | Original predicate | Turns | Seconds | Sol tokens | Sol + Jev USD |
|---|---|---|---|---|---|
| Tree before 1 | fail, 5/8 birch logs, stuck exit | 6 | 53.097 | 12,815 | 0.015284258 |
| Tree before 2 | pass, 8/8 | 14 | 69.772 | 39,943 | 0.022461184 |
| Tree after 1 | pass, 8/8 | 3 | 65.598 | 8,133 | 0.023051322 |
| Tree after 2 | pass, 8/8 | 3 | 66.142 | 8,041 | 0.018745216 |
| Wall before 1 | fail, 0/4 oak logs, stuck exit | 1 | 8.636 | 1,764 | 0.001929486 |
| Wall before 2 | fail, 0/4, stuck exit | 1 | 8.335 | 1,764 | 0.001838388 |
| Wall after 1 | pass, 4/4 | 6 | 44.155 | 28,958 | 0.027339548 |
| Wall after 2 | pass, 4/4 | 8 | 58.158 | 45,238 | 0.034779052 |

Both after-tree collect actions actually failed after five logs; the model
repositioned and collected the remaining three in the same run. Both wall
runs stopped a live stalled collect action, then model-selected digging,
pillaring or repositioning obtained the original four logs. These are useful
same-conversation adaptations, not merely successful escape actions. Four
after successes versus one before success are bounded demonstrations, not a
general reliability gain or a claim that failed runs became faster.

After obstruction-arm state reads had p50 150–155ms and p95 257–359ms,
versus p50 152–201ms and p95 263–643ms before. These include the existing
notable-block scan and are not an isolated collision-overhead benchmark.
Real follow-up/probe cards serialized the collision addition in 960–1,006
bytes for those ordinary shapes; schema limits remain twelve cells/sixteen
boxes rather than an unbounded scan. Partial, unknown and truncated shapes
are separately checked by body tests.

### Incident reconstruction and negative controls

The exact live failed target was never recorded, so this is a reconstruction,
not an exact replay. Both arms actually gathered two birch logs, then targeted
a known occupied cell and produced `Took to long to decide path to goal!`
(7.628s before, 7.076s after). The before partial collect also timed out after
obtaining two logs; the after partial collect succeeded. To exercise active
Jev rather than the healthy-idle skip, the driver supplied seven damage points
(health thirteen, above the danger threshold), then healed before follow-up.
Jev chose terminal stuck at confidence 0.96 before and 0.95 after: unchanged
code emitted stuck; new code logged it as inapplicable.

The same legacy `attempts:0, closed:true`, autonomy-paused checkpoint and
explicitly synthetic old false-exhaustion dialogue were then supplied to both
arms. On the leaf-clearing/eight-log follow-up, before idled and falsely claimed
exhaustion. After accepted an ordinary goal, cleared leaves, encountered a
failed collect and chose repositioning/direct digging; RCON verified eight
birch logs. Checkpoints retained closed-zero and paused autonomy throughout
that execution, until successful-safe reset. The scheduler regression also
checks the exact authoritative planner/executor inputs and restart/no-periodic-
planning behavior; checkpoint snapshots alone are not captures of model input.
The after executor used eleven turns; RCON first verified eight logs 74.540s
after the follow-up was sent (including planning/dispatch). Initial owner reply
latency was 3.901s before / 5.666s after.

- Sealed bedrock: honest failure at 0/4 in both arms; 3 turns, 8.823s before,
  12.972s after. No impossible completion or new allowance was fabricated.
- Real adjacent zombie: ordinary execution interrupted by unchanged danger
  precheck before any model turn, 0.510s before / 0.321s after.
- Exhaustion/cancellation: executable regressions retain the original turn
  limit after live stuck, wall-time/caller cancellation, body safety, two
  charged-before-dispatch recoveries and restart/preemption/no-refill behavior.
- Motionless real action: active Jev preceded the deterministic ten-second
  stall guard; conditional stop reached its owning action at 10.316s.
- Real-body stale-stop smoke: terminal and replaced tokens left later live
  controls set; a restarted body's reused numeric ID had a different token and
  rejected the old stop without touching controls. Matching live stop halted
  it (566ms including HTTP checks). A first invalid scratch driver used the
  wrong goto binding and crashed only its disposable Probe body; its failure
  is retained, not counted as a passing application test.
- The new executor regression fails in all three modes on unchanged code and
  passes after: terminal failure/false stuck, live stop while waiting, and live
  stop after a tool returned. The last forces a verdict during inference and
  verifies three original turns and all 2,128 returned tokens, including the
  withheld response. No allowance is restarted.

### Conversation, controls and validation

The full slice 11 chat driver was reused with only local ports/paths changed,
plus matching attended-idle prompt additions in both arms. Both arms clarified
the workshop request, verified four birch planks and an inventory crafting
table, performed real destination replacement with joined cleanup, retained
birch/workshop context across seven no-task turns and a graceful same-DB
restart, and verified eight additional planks. Both refused the impossible
diamond request and found existing skills sufficient; none was promoted.
Supplied prerequisites were sixteen birch logs and a peaceful flat arena.
The full driver's status question preceded movement in both arms, so a separate
fixed observed-motion probe checks conversation beside a moving action.

That probe replied in 3.932s before / 3.905s after while preserving the same
live action: x6.9 → x22.5 before and x4.5 → x11.3 after. Before then replaced
and reached radius two of (12,80,0). After correctly cancelled/joined the old
owner but chose idle based on its earlier near-target position; Clyde had
moved to x20 before stop, and the new return target was not completed. The
probe timed out after ninety seconds. This negative model/stale-position result
is retained alongside the passing full-driver replacement, not counted as a
success or repaired with a fixture-specific rule. No general conversational
latency, replacement-success or reliability guarantee is claimed.

| Fixed conversation arm | Returned Sol tokens | Sol USD | Jev USD, including terminal probe |
|---|---:|---:|---:|
| Full before | 212,651 | 0.2871929 | 0.007831908 |
| Full after | 236,563 | 0.3290395 | 0.007448238 |
| Legacy follow-up before | 5,653 | 0.0147910 | 0.000097986 |
| Legacy follow-up after | 65,652 | 0.0456753 | 0.009297876 |
| Moving-status before | 50,830 | 0.0702319 | 0.004531842 |
| Moving-status after, including failed return | 50,410 | 0.0675298 | 0.002457630 |

The twelve unchanged executor/curriculum controls passed 11/12 before and
12/12 after, one trial each. The before negative was small-shelter (4/27
blocks, interrupted); after verified 27/27. This is not uniformly faster:
terrain-delivery took 34.674s before / 50.532s after. Full per-task turns,
seconds, tokens, Sol cost and Jev cost are retained in the evidence JSON.
The five unchanged deterministic skill gates passed 5/5 before and 5/5 after
(`gym/bin/gate-skills`); no skill was authored or promoted.

Total returned usage across all 36 model gym trials, six conversation arms,
terminal Jev probes and the motionless-stall probe was **$1.342987860**:
$1.087015500 Sol (1,158,048 tokens) plus $0.255972360 Jev. Failures and the
failed moving-status return are included, priced at configured provider rates.
Deterministic tests/gates use no provider. This stays inside the initial $5
experimental allowance; billing for
cancelled/unreturned calls is unknown, not zero or a hard invoice cap.

Local validation: body `npm ci`, `npm test` (26/26), `npm run build`,
`npm run gen`; harness `go build ./...`, `go vet ./...`, `go test ./...`,
`go test -race ./...`, `go generate ./...`. TS/Go regeneration has no drift;
ten repeated changed-package race runs and later focused race rechecks pass.
Sequential local Auto-review found and fixed an immediate-failure path that
omitted fresh state; its targeted ownership/continuation recheck found no
remaining issue there. Final schema-to-tool review additionally found that
`act(stop)` advertised the new token but still used unconditional stop. The
handler now validates/forwards the token and uses the schema's stop description;
the public-tool regression fails before this fix and passes after, including
empty-token rejection. Full harness build/vet/test/race/generation and three
executor race repetitions were rerun. These measured model arms precede that
last narrow forwarding/description fix; none supplied an `act(stop)` token.
The evidence separates measured-source and final-source hashes rather than
claiming another model rerun. No material finding remains after the targeted
recheck. No delegated review or new thread was used.

Exact pushed source and CI confirmation are linked in the
[implementation thread](https://ampcode.com/threads/T-01a171da-b4f8-70d6-b9ab-590c106c5878).

Coordinator acceptance and any production rollout remain separate. The old
closed episode still prevents another post-mortem after this bounded run ends;
this slice does not grant a durable exploration allowance or guarantee that a
model uses accurate observations well.
