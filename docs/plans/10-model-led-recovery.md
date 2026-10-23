# Slice 10: model-led recovery and judge evidence

Goal: correct the Oracle's demonstrated responsibility leaks: the planner
chooses how to recover or idle, the judge evaluates recovery with adequate
evidence, and the harness bounds execution without choosing Minecraft tactics.

## Done when

- Reproduce on unchanged code that a failed goal's planner-proposed recovery
  is discarded, and that persistent code-detected danger prevents a useful
  response from executing. Exercise the actual scheduler/executor/judge path,
  not merely a scripted executor returning success. Retain before transcripts.
- After an ordinary failure/stuck interruption, the planner may select a new
  approach or explicit idle. Do not unconditionally delete its proposed goal.
  Keep retries bounded in the harness: start with at most two consecutive
  recovery executions in a durable recovery episode, with existing per-goal
  time/step limits; define reset/exhaustion semantics and test them. Restart
  must not replenish exhausted allowances. Planner refusals/idle and failures
  must not create periodic planning or an unbounded model-call loop.
- Danger preempts ordinary work but allows model-selected recovery. Existing
  persistent danger alone must not immediately cancel its own response in
  either scheduler or executor. Keep the judge active: renewed/worsening
  danger, a truly stalled recovery, budget exhaustion and player cancellation
  still have defined interrupt behaviour. Do not encode eat/flee/swim target
  selection in Go or grant recovery a general safety bypass. Body restrictions
  and single-controller cleanup remain authoritative.
- Give the planner/executor the context needed to distinguish ordinary goals,
  failure recovery and danger response through the existing contracts. Choose
  the smallest coherent representation shared by consumers, not scattered
  per-caller booleans or a second executor/scheduler implementation. Generated
  schema types, if affected, come from their schema. Preserve run/MCP clients
  and existing durable memory safely; document compatibility and semantics.
- Reproduce the stone-variant interruption from slice 9, keeping model,
  thresholds, task and budgets fixed. Determine whether missing temporal
  evidence contributes. Where justified, supply bounded factual time/action
  progress deltas through the judge's own observation adapter and adjust its
  prompt to match the evidence. Do not invent trend observations in prompts,
  teach task-specific exceptions, or tune confidence thresholds to one run.
  If the hypothesis fails, record that rather than claiming a fix.
- Offline regressions through public behaviours cover bounded recovery/idle,
  restart persistence, player preemption and cleanup before the next owner,
  recovery during persistent danger and interruption of a failed recovery.
  Live isolated scenarios prove at least one changed approach after failure
  and one useful recovery under persistent danger, with independent server
  checks. Use ordinary survival mechanics to produce danger, not a fake-safe
  card or disabled judge. Include a no-progress negative control. Report
  real latency, failures, usage and all supplied scenario prerequisites.
- Repeat slice 9 base/variant tasks with fixed matched model/settings/budgets
  against the unchanged baseline and final code, at least three trials for
  the previously failing stone variant. Preserve original and failed attempts.
  Verify existing deterministic skill gates and executor controls, then full
  planner preemption/resumption and ordered shutdown with no action owner left.
- Run Auto-review, relevant body/harness builds/tests/vet/race/generation and
  exact-head CI. Update As built with commands, comparisons, bounded policy,
  evidence and unresolved risks. Ship small logical commits to Clyde main;
  no live image rebuild or deployment is authorized by this slice.

## Starting point

Read the Oracle review in coordinator thread
`T-01a14705-d53e-712a-b7ce-957c5292d68a` and the slice 6/9 As built reports.
At `179ec808`, `harness/planner/scheduler.go` discards post-mortem goals,
pauses autonomy on failure and vetoes execution during danger. Executor judge
danger cancellation is unconditional too. The judge receives only the current
raw state card even though its prompt asks about change over time.

Ownership paths: `harness/planner/{planner,scheduler}.go`,
`harness/executor/executor.go`, `harness/judge/{judge,precheck,config}.go`,
their prompts and CLI wiring. The body already supplies confirmed placement
progress and owns safety/cancellation. Skill crafting stays model-authored;
no demonstrated duplicate table-crafting consumer warrants a new body API.
Do not rework keyword retrieval or observed spatial naming without evidence.

## Notes

- Keep Sol 6.1 low for both roles and Jev. Improve decision boundaries, not
  models or deployment. Initial experiment bound: US$5 reported provider
  usage including Jev where available; report before extending. Unreturned
  billing is unknown, so this is not a hard dollar cap.
- The two-recovery allowance is an initial operational bound, not strategy
  or a claim of adequate survival. Make reset conditions explicit; new danger
  notifications must not silently refill it. Avoid adding configuration
  surfaces unless existing callers actually require different limits.
- State-read deadlines are a separate observed limitation, not a proven
  crafting or judge regression. Time the existing observation boundary if
  they recur; avoid speculative caching, services or unrelated optimization.
- Coordinator owns roadmap acceptance. Run all experiments in disposable orb
  worlds with separate memory. media-ovh play world and Bonnie stay untouched.

## Not in this slice

Body safety weakening, hard-coded recovery tactics, deployment/hardening,
automatic skill promotion, judge distillation, body migration, unbounded
self-improvement or claims of general autonomous survival.
