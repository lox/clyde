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
