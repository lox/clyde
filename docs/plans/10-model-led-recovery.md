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

## As built

### Responsibility boundary and durable policy

The scheduler now executes the planner's changed approach rather than deleting
every post-mortem goal. The shared `executor.ExecutionContext` carries mode
(`recovery`, `danger`, or empty ordinary work), cause, remaining executions and
the initial fired danger checks through planner input, durable jobs and executor
options. The CLI supplies it for `serve`; existing run/MCP clients retain the
ordinary zero value. No schema, generated contract, body, skill, provider or
retrieval changes were needed.

An episode permits two recovery **executions**, charged durably before dispatch.
Planning does not spend an execution. Idle/refusal or exhaustion closes it.
Only a successful execution plus a fresh card with no fired danger prechecks
resets the episode and autonomous pause. A bowl predicate while health is low
does not reset it. Player requests, death/danger notifications, danger flaps
and restart do not mint attempts. Safe player work remains possible while an
episode is closed. A player-preempted recovery keeps its goal/context and spends
another attempt if resumed; unfinished running recovery after a crash is not
replayed for free. Restart still independently reconciles held predicates.
Older checkpoints decode without new fields and preserve their existing pause.

The initial implementation allowed persistent checks but cancelled on any
decrement/new check. The starvation trace showed why that still recreated the
original veto: health fell before the first action, without reliable evidence
of its cause. A focused Oracle follow-up rejected that policy. In the selected
implementation, **during recovery only**, deterministic danger checks are
evidence for the active judge, not unconditional interrupts. Its question is
whether this current recovery should stop/replan given its goal/action and
observations. New danger reaches that tick's model and may interrupt immediately
with normal confidence handling. Ordinary-work danger preemption/prechecks,
the existing deterministic stall guard, budgets, cancellation, body safety and
single-owner cleanup remain authoritative. There is no numeric grace threshold
or Go-selected eating/fleeing/swimming strategy.

The judge-owned `recovery` observation adds the typed goal, preceding/current
fired checks, appeared/cleared checks and preceding successful state observation;
the current card supplies action/events. Damage cause is explicitly unknown.
JSONL retains the raw card, exact enriched observation, answers/interrupt and
`state_ms` timing. Backend/state failures are recorded gaps, not a survival
guarantee. See the updated [local runbook](../../harness/README.md).

### Reproductions and rejected temporal change

Baseline is [e55df97](https://github.com/lox/clyde/commit/e55df97931a0e99797f32ada1e1284571755edcb).
All experiments used disposable orb Paper worlds, not media-ovh or Bonnie.
[10-evidence.json](10-evidence.json) retains fixtures, model decisions/tool
transcripts, server checks, failed attempts, judge summaries/interrupt records,
selected recovery tick streams and disposable drivers. Full raw ordinary tick
streams remain in ignored gym runs. No deployment or live-image rebuild occurred.

- Unchanged full planner/scheduler/executor/Jev path: an impossible diamond
  goal failed; the real planner proposed four birch planks, but the scheduler
  discarded it (`before-failure2`). In real low-health survival it proposed
  eating stew/retaining the bowl, again discarded (`before-danger2`). Direct
  unchanged executor execution was also precheck-interrupted at step zero in
  0.293s. The earlier `before-danger` hunger setup was invalid (food remained
  20, saturation 3), and is retained but excluded from the danger claim.
- Unchanged six-task slice-9 base/variant comparison passed **18/18**, including
  stone variant **3/3**. The historical stone interruption was therefore not
  consistently reproduced live. Its retained final card replayed with the
  original questions gave **none 3/3** raw versus **stuck 3/3** with history.
  Missing history is **not an established cause**.
- The first history implementation repeated action programs in samples and
  failed all six valid stone/base-stone trials. One compact factual-samples
  refinement still failed stone variant **3/3**. Both were rejected: the global
  temporal feature and its ordinary stuck/progress prompt changes are absent
  from the shipped implementation. No confidence/stall/safety thresholds were
  changed. Later startup attempts with incorrectly named isolation environment
  variables collided on disposable ports; those setup errors are retained and
  excluded, not counted as model failures or improvements.

### Real recovery and controls

Both frontier roles use Sol 6.1 low, with Jev active. Curriculum arms use the
same six tasks, admission receipts, 40 turns and 300s. Recovery smoke runs use
40 turns/40s per execution and a 100s session, with separate durable memory.
The survival fixture uses normal difficulty/survival, real high-amplifier hunger
for ten seconds to deplete saturation/food, then a 17-block fall onto a flat
grass arena. It supplies eight cooked beef and one mushroom stew. No fake-safe
card, invulnerability or disabled judge establishes the result.

- Selected failure path: the diamond goal failed honestly after 31.163s;
  model-chosen four-plank crafting then completed in **6.129s**, independently
  RCON-checked. A fresh safe card reset the episode, then the planner idled.
- Selected danger path: RCON began at health 5, food/saturation 0. During the
  recovery, health fell **2 → 1** before its first body action; Jev continued
  rather than vetoing its own response. The model chose eating and the RCON
  bowl predicate completed in **9.628s**. Final RCON food was **6**, health **1**;
  the body also retained one bowl. The model then explicitly idled, correctly
  noting that the milestone was not restored health. The episode closed after
  one execution with no refill. This is useful recovery action, **not successful
  survival restoration**. A later `inventory.*` slot query was invalid for a
  bowl in the hotbar; it is retained, not substituted for the actual RCON goal
  checker (`clear clyde minecraft:bowl 0`).
- Failed earlier danger runs remain evidence. In `final-danger`, the first
  attempt was deterministically cancelled at step zero on falling health;
  the second ate one beef (food **0 → 8**) but model-stuck interruption at
  **10.269s** prevented its bowl predicate. The episode exhausted two attempts.
  Food rising did not make this a successful recovery or prove stuck wrong.
- Real-body/real-Jev negative controls deliberately run a motionless 30s eval
  under a recovery wait goal. Introducing lava produced a **model danger**
  interruption (confidence **0.77**, action elapsed **2.717s**), with `in_lava`
  present in that tick's appeared checks. Without a new threat, the genuinely
  motionless wait produced **model stuck** at **6.559s** (confidence 0.41,
  corroborating noul 0.73). Cleanup stopped each action. These deliberately
  scripted negative fixtures test the adapter/judge, not frontier survival
  planning. Separate executor tests prove that model interrupts cancel once.

Offline tests also cover falling-health recovery with scripted judge `none`,
new checks reaching the same tick's judge, the existing stall guard, durable
idle/exhaustion/restart, safe-success reset, player preemption without allowance
laundering, and cleanup before another owner. Model/backend outages still limit
survival; the supported predicates do not directly express restored hunger or
health. No stronger autonomous-survival capability is claimed.

### Matched curriculum and usage

The frozen unchanged arm (`20261023T220619Z`) passed **18/18**. The selected
arm (`20261023T224723Z`) passed **17/18**, without reruns or threshold changes.
Both use the same tasks/receipts, Sol 6.1 low, active Jev and 40-turn/300s limits.
Medians below include successful trials only, not a fabricated completion time
for the failed stone run.

| Task | Unchanged | Selected | Unchanged median | Selected median |
|------|----------:|---------:|-----------------:|----------------:|
| stone-pickaxe | 3/3 | 3/3 | 72s | 69s |
| small-shelter | 3/3 | 3/3 | 75s | 69s |
| terrain-delivery | 3/3 | 3/3 | 36s | 41s |
| stone-pickaxe-variant | 3/3 | 2/3 | 72s | 70.5s |
| small-shelter-variant | 3/3 | 3/3 | 83s | 67s |
| terrain-delivery-variant | 3/3 | 3/3 | 48s | 38s |

The selected stone failure reached 26 turns/158.059s before the unchanged
deterministic stall guard interrupted collection, after a crafting/resource
detour and correcting birch to oak. This is not the historical early model
interruption. The trace has no state/backend errors. The selected arm otherwise
records four Jev deadlines in a passing stone-variant trial and one state-read
deadline in a passing shelter trial. They remain logged gaps, not evidence for
changing thresholds. One base-stone executor outcome is `cancelled` because the
gym stopped it as the independent server predicate held; both records remain.
No general pass-rate or latency gain, or stone-root-cause fix, is claimed.

All returned experiment usage, including failed/rejected arms and controls:
Sol **$0.710476600**, Jev **$0.434624064**, total **$1.145100664**. The initial
$5 allowance has **$3.854899336 remaining**. Cancelled/unreturned provider
billing is unknown; this is not a hard invoice cap. No additional model
experiments were added after the planned comparison.

### Planner ownership, timing and validation

The full `serve` run used the slice-9 stepped-hillside/chest task and Alex's
real chat client. Alex interrupted active collection after two birch logs,
requested four sticks, and received one acknowledgement. The player predicate
completed **4/4 sticks**; the original `auto-1` delivery predicate resumed
unchanged. The independent gym server check passed **16/16 birch planks in
the chest in 111.002s**. Ordered SIGTERM stopped/joined `serve` before its body:
the capture has `action:null`, four retained sticks, and a closed body port.
The interrupted ordinary job remains durably `running` for live reconciliation
on restart rather than falsely recording an executor completion after shutdown.

Selected survival ticks had state-read median **241ms**, p95/max **360ms**,
with **0/18** recorded state/backend errors. The full planner stream had median
**262ms**, p95 **453ms**, max **691ms**, and **0/190** errors. These time the
existing observation boundary only; they do not prove an upstream latency cause
or justify caching/body migration. Ordinary curriculum summaries retain their
own errors and latency distributions separately.

Auto-review's sequential ship-risk/simplicity passes corrected the conflicting
absolute-danger question, preserved model-only danger intent, removed the
rejected global history and an unreachable old preemption branch, and rechecked
the recovery lifecycle/callers. A race repetition exposed an existing timeout
test that assigned the entire tick deadline to backend latency; its assertion
now checks total tick time, matching the existing deadline contract. A recovery
test also now waits for its durable completed state rather than observing an
initially empty database. No runtime deadline or confidence threshold changed.

Verification commands (repository root, Go/JDK on PATH):

```sh
(cd body && npm ci && npm test && npm run build)
(cd harness && go build ./... && go test ./... && go vet ./...)
(cd harness && go test -race ./... && go generate ./...)
(cd harness && go test -race ./planner ./executor ./judge -count=10)
gym/bin/gate-skills
gym/bin/run --all --agent cheat
gym/bin/run --all --agent idle --timeout 2s # expected nonzero, no passes
```

Body: **22/22 tests**, build passed. Harness: build, tests, vet, full race and
generation passed with no generated drift; the repeated planner/executor/judge
race checks passed. The preemption negative control also waits long enough to
observe a spurious cancellation instead of checking an immediately empty
channel. No new permanent experiment runner or configuration surface was added.

Final deterministic gates: **5/5** (collect 18s, craft 8s, place 2s, deposit
1s, eat 2s). Checker controls: cheat **12/12**, idle **0/12**, as expected.
Gates were serialized after the model comparison because admission receipts
are shared. The temporary baseline CLI wrapper was removed before rebuilding
the final gate binary; the initial build refusal is retained in the logs.

The evidence JSON's `drivers` retains the disposable source, not a new supported
runner. Original comparison invocation, from the repository root:

```sh
PAPER_DIR="$HOME/paper" GYM_PAPER_NAME=paper GYM_MC_PORT=25565 \
  GYM_RCON_PORT=25575 BODY_LISTEN_PORT=7787 gym/.bin/gym --root "$PWD" \
  stone-pickaxe-variant stone-pickaxe small-shelter terrain-delivery \
  small-shelter-variant terrain-delivery-variant \
  --agent 'bash /tmp/slice10/executor-final' --runs 3
```

Recovery/control drivers run from `harness/` with the **separate disposable**
Paper/body already listening on 25567/25577/7789. They set up and independently
inspect the physical survival fixture over RCON. Example original invocations:

```sh
export PAPER_DIR="$HOME/paper-recovery" GYM_PAPER_NAME=paper-recovery
export GYM_MC_PORT=25567 GYM_RCON_PORT=25577 BODY_LISTEN_PORT=7789
go run /tmp/slice10/recovery.go /tmp/slice10/selected-danger danger
go run /tmp/slice10/recovery.go /tmp/slice10/selected-failure failure
go run /tmp/slice10/controls.go new-threat /tmp/slice10/control-new-threat
go run /tmp/slice10/controls.go stalled /tmp/slice10/control-stalled
go run /tmp/slice10/planner.go
```

Use fresh memory/output directories when reproducing, restore the disposable
world between cases, and never point these fixture drivers at a live world.
