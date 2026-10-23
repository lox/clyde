# Slice 9: harder gym tasks and evidence-led capability improvements

Goal: improve Clyde's actual multi-step play, using reproducible gym failures
and matched before/after runs rather than more deployment machinery.

## Done when

- Add three harder tasks using existing gym YAML and independent RCON goals:
  empty inventory to a stone pickaxe (wood, table, wooden tool, stone, final
  craft); a small specified shelter checked as actual placed blocks; and
  gathering from uneven terrain followed by crafted-item delivery to a chest.
  Supply reachable resources in the fixtures, not prerequisite inventory or
  a prescribed sequence of tool calls. Keep coordinates/geometry asymmetric
  so incorrect navigation, placement and inventory handling are observable.
- Prove each checker with the existing cheat positive control and idle
  negative control. Preserve existing primitive/multi-step tasks as regression
  controls. No model self-report may stand in for a server-checked outcome.
- Run the unchanged executor with Sol 6.1 low, Jev and the five admitted
  skills three times per new task. Freeze fixtures, prompts, time/turn limits
  and model settings for the baseline. Record pass rates, failure reasons,
  wall time, returned tokens/cost, actual skill calls and interrupt reasons.
  Investigate the live gather interruption hypothesis on the actual gym
  path before blaming Jev, progress accounting or the skill.
- Use failures to choose a small first batch of fixes to the owning body,
  executor, judge, prompts or skills. Fix mechanisms, not task names or known
  coordinates. Do not disable the judge, weaken safety/checkers, add free
  resources, or increase budgets only for the final arm. Model-author new
  or revised skill bodies through the existing provider, gate exact contents
  on isolated instances, and preserve failed attempts/authorship evidence.
- Repeat the same three runs per task after fixes. Include one held-out
  geometry/resource variant for each task, withheld from the first tuning
  batch. Report unchanged failures and regressions, not just best runs.
  If another refinement is justified, bound it to one further batch and
  distinguish previously seen variants from genuinely held-out evidence.
- Exercise the full planner on one harder scenario with real player chat
  preempting autonomous work, a server-verified player goal, and autonomous
  resumption or an explicit documented pause. Use the existing serve/event
  path, not executor-only success as evidence of planner reliability. Preserve
  separate memory and report acknowledgement latency and actual outcomes.
- Run Auto-review and fix grounded findings; body tests/build, harness
  build/vet/tests and relevant race checks pass, as do existing deterministic
  skill gates and CI at the pushed head. Update this plan's As built with
  commands, revision pins, full scoreboards, limits and the next bottleneck.
  Keep permanent tests only for demonstrated regression contracts.

## Starting point

Slices 1–6 and container packaging are shipped. Read their As built sections,
`gym/tasks/README.md`, `skills/README.md` and `harness/README.md`. Existing
`logs-to-table` and `logs-to-chest` fixtures are small, flat, peaceful arenas;
they do not establish resource-chain or construction competence. Reuse
`gym/bin/run`, existing agents, typed goals and independent predicates.

The persistent Clyde play world on media-ovh is running the accepted image.
Its first gather-four-birch-logs goal was interrupted as stalled after one
log; a console-whisper request then successfully crafted a table from that
log. This is a useful failing observation, not an established root cause.
The new work runs entirely in this slice thread's disposable orb gym.

## Notes

- Keep Sol 6.1 low for both roles; improve systems before changing models.
  Match judge configuration, skill admission and budgets between arms. Freeze
  task fixtures before measuring; if a fixture is wrong, repair it and rerun
  both arms rather than comparing different tasks.
- Start with at most US$5 of provider-reported model usage for this iteration;
  stop and report before extending the experiment. Track Jev usage separately
  where available; unreturned/cancelled billing is not a hard spending cap.
- Prefer three useful tasks and one coherent fix batch over a general
  benchmark framework or a large speculative skill library. If new predicate
  semantics are truly needed, first try expressing the observable outcome
  through existing items/built/delivered checks.
- Ship small logical commits to Clyde main through the repository workflow,
  with evidence in commit messages. Reply to the coordinator when CI is green.
  This does not authorize rebuilding or deploying the live image.

## Not in this slice

Production/play-world experiments or resets, media-ovh deployment, model
migration, judge distillation, automatic skill promotion, new infrastructure,
general autonomous-survival claims or unbounded benchmark spending.

## As built

### Scope and reproducibility (2026-10-23)

Measured from [the frozen starting revision](https://github.com/lox/clyde/commit/8e40bb7b42339f31439cf5a41e3cad968500a19c).
The three base tasks and three variants were frozen before the baseline;
their SHA256 hashes, full per-trial outcomes/tool transcripts, judge usage,
errors and interrupts are in [09-evidence.json](09-evidence.json).
[09-skill-authorship.json](09-skill-authorship.json) preserves both complete
author prompts, provider responses, exact candidate sources and gate logs,
including the rejected attempt. These are experiment records, not a new
benchmark framework. Full state-card tick streams remain in ignored gym runs.

All work used disposable orb Paper worlds. Nothing connected to media-ovh,
reset the live play world, rebuilt its image or deployed anything.

- Paper 26.1.2 build 74 (`e4e17fc`), seed/template `mcbot`, protocol 26.1;
  mineflayer 4.39.0 from the unchanged lockfile.
- Node 26.12.1, Go 1.27.2, Temurin Java 25.0.5; agent-harness pinned at
  `v0.3.1-0.20261017002310-8117cc9dbc59`.
- Executor and planner: `gpt-6.1-sol`, low effort, through existing
  `harness/llm`; executor 40 turns / 300 seconds in every measured arm.
- Jev enabled throughout: default 2 Hz / 800ms deadline, 10s stalled
  precheck, confidence 0.5 / corroboration 0.7. No judge, safety, executor,
  prompt, checker or budget changes. The bounded second batch changes only
  the body's eval placement-progress accounting.
- Five admitted skills in every valid arm; only `craft-item` contents changed.
  Original digest `ddaf7d69cd697ac455f1dc87c3dab80dbf65ec973aae45092b396a0e74f6f708`;
  revised digest `61b6bb28440fcec554302f788dfa7372c36874a42395725315ba47a1b4495a4f`.
- Main gym: `~/paper`, ports 25565/25575, body 7787. Separate disposable
  diagnostic gym: `~/paper-diagnostic`, ports 25567/25577, body 7789.
  Independent worlds allow concurrent trials, not concurrent gates: gates
  mutate the installed admission receipts shared by both bodies.

The fixtures start empty-handed. Pickaxe requires actual server inventory;
shelter requires 23 placed planks plus four air blocks (interior/doorway);
delivery requires 16 crafted planks inside an elevated chest after hillside
gathering. Variants change wood species, resource locations and geometry,
including a west-facing instead of south-facing doorway. No tool sequence
is prescribed. Peaceful arenas establish these bounded tasks, not survival.

### Baseline and selected batch

Commands, from the repository root with `~/tools/{go,jdk}/bin` on PATH:

```sh
gym/bin/gate-skills
gym/bin/run --all --agent cheat
gym/bin/run --all --agent idle --timeout 2s  # expected nonzero
gym/bin/run stone-pickaxe small-shelter terrain-delivery --agent executor --runs 3
```

Checker controls: cheat **12/12**, idle **0/12**, including all six existing
tasks. All goals were initially unsatisfied. Baseline deterministic gates:
**5/5**. Baseline model trials: **9/9**, with no danger/stuck interrupts;
one Jev backend error. Pickaxe times 137/137/112s, shelter 76/89/76s,
delivery 48/72/42s. Returned Sol usage 309,372 tokens / $0.1453935;
Jev 1,673,728 input tokens / $0.070296576.

There was no demonstrated pass-rate deficit to fix. However, every pickaxe
trial had table-crafting errors (`Crafting produced no complete inventory
progress`) and recovered with extra model turns/eval. The selected batch
therefore targets recovery overhead, not an invented capability failure:
model-author just `craft-item` to use public window clicks for table recipes,
wait for actual output, and check retained inventory with an empty cursor.
Hand recipes retain `bot.craft`. Recipes remain generic, without task names,
known resource coordinates or pickaxe special cases. Windows close on errors.

The first authored proposal failed its exact gate immediately because
`bot.openCraftingTable` does not exist. It was not promoted. The second
corrected public API use to `bot.openBlock` and passed the two-furnace gate
from asymmetric 19+3 cobblestone stacks (8s); its exact contents were then
explicitly promoted using the existing candidate/admission path. Authoring
costs were $0.029736 and $0.034261. Craft-only all-skill gates passed **5/5**:
collect 18s, craft two furnaces 8s, place table 2s, deposit 1s, eat 2s.

Short-chain diagnostics sometimes passed and sometimes reproduced missing
table output. RCON `clear ... 0` counting was a synchronization hypothesis,
but read-only `execute if items` polling also reproduced failure. No observer
or upstream dependency patch was justified. The diagnostic marker-goal
timeouts are intentional probes, not hidden failed model trials; their
transcripts and packet evidence are preserved separately.

### Craft-only comparison and second-batch selection

The clean craft-only arm (`20261023T114646Z`) passed **7/9**: pickaxe 3/3
(70/96/76s), shelter 1/3 (two precheck-stalled failures at 21/27 and 20/27,
one 77s pass), delivery 3/3 (36/37/55s). Pickaxe median dropped from 137s
to 76s, but this is not an overall capability improvement: baseline shelter
had passed 3/3. The first genuinely held-out arm (`20261023T115417Z`)
passed **8/9**, failing the rotated shelter at 25/27 with the same precheck.
Full per-trial scoreboards, tokens, costs, skill calls and errors remain in
the exported evidence, including every failure.

The first attempted final arm (`20261023T113859Z`, 9/9) is **excluded**:
concurrent installed-skill gates withdrew admission during pickaxe trial 3.
It was retained and charged, not replaced with a best-of comparison. The
entire arm was repeated after gates completed. Gates must be serialized
against measurements even when Paper worlds/ports are isolated.

The base shelter failure trace supplies a concrete second-batch target:
stationary eval consumed planks from 24 to 7 while placing real blocks,
but action progress never changed and the 10s stall precheck fired at 10.5s.
No inventory heuristic or increased timeout is justified. A model-free
control placing one plank every two seconds reproduced it at **5/8 blocks,
11.9s**. The body must expose confirmed placement progress for the owning
eval; a genuinely idle eval must still stall. This is not evidence for the
historical birch-collection cause. The frozen variants become previously
seen controls in the second batch, not newly held-out evidence.

The second batch changes the existing body action runner, not the judge.
Pinned mineflayer emits `blockPlaced` after server-confirmed placement;
the owning eval now exposes a monotonically increasing placement count in
its existing progress field. Async action ownership prevents unrelated or
late events from refreshing another action. No inventory-change heuristic,
task-specific exception or longer stall timeout was added. The same live
stationary probe now passes **8/8 at 21.0s**; truly idle eval still interrupts
at **11.7s, 0/8**. The regression test failed before the change and now passes,
including foreign/stale-event controls. Repeated deterministic gates passed
**5/5** (19/8/2/1/2s) before model measurements resumed.

### Final matched comparison

The final runtime combines the [authored craft revision](https://github.com/lox/clyde/commit/ef8da88)
with [owning-eval placement progress](https://github.com/lox/clyde/commit/ed783dd).
All nine frozen base trials passed at the same budgets; the three previously
seen variants passed **8/9**, not fresh held-out evidence. Each table cell is
passes and median time **among passes**:

| Base task | Unchanged baseline | Craft-only | Craft + placement progress |
|---|---:|---:|---:|
| Stone pickaxe | 3/3, 137s | 3/3, 76s | 3/3, 78s |
| Specified shelter | 3/3, 76s | 1/3, 77s | 3/3, 89s |
| Terrain delivery | 3/3, 48s | 3/3, 37s | 3/3, 43s |
| Total | 9/9 | 7/9 | 9/9 |

Final pickaxe times were 78/89/75s, shelter 89/97/88s, and delivery
43/69/41s. Pickaxe mean returned tokens fell from 70,596 to 26,997, but
shelter median became slower than baseline; there is no general pass-rate
improvement over the 9/9 baseline. Three trials per task, stochastic models,
and shared orb load are not strong latency or generalization guarantees.
The stronger result is the reproduced stationary-placement contract repair.

Both shelter geometries now pass **6/6**. The remaining variant failure was
stone collection: the bot descended from y=79 to y=76, acquiring dirt while
target-stone progress remained zero. At action elapsed **5,468ms**, Jev's
model confidence 0.36 and corroborating stuck score 0.71 triggered the
unchanged model path, **not** the stall precheck. No third fix batch or
threshold change was attempted. Resource targeting/navigation versus early
judge interpretation is the next bounded investigation, not a settled cause.

Final base usage: Sol **198,348 tokens / $0.0911589**, Jev **1,854,542 input
tokens / $0.077890764**. Seen variants: Sol **222,272 / $0.0999428**, Jev
**1,768,773 / $0.074288466**. Neither final arm had judge errors; only the
one variant interrupt occurred. Every trial's exact scoreboard and transcript
is in the evidence JSON, including craft-only failures and the discarded arm.
The parallel final commands happened to share a second-resolution run-directory
name, but used disjoint task subdirectories and independent worlds/ports.
The first summary was captured before the other overwrote it; both exact
summaries are preserved. No trial was discarded for that metadata collision.

### Live gather hypothesis

The [historical live trace](https://ampcode.com/threads/T-01a14894-8624-72d8-af3c-2c59e8fb5d5b)
used primitive `act collect birch_log count=4 radius=16 wait=60`, not
`collect-logs`. It picked up one log and was stopped at 16,410ms after
`judge: stuck (precheck stalled)`. The full historical terrain/state card
is unavailable. The exact primitive passed **3/3** small birch-gym probes
(20/19/19s), with Jev enabled ($0.007180866, 170,973 input tokens).
This does not reproduce or explain the live failure. No change to progress
accounting, Jev or collection follows from that observation; the separate
placement fix follows from the measured shelter trace.

### Full planner/player scenario

The real `clyde serve` path used separate SQLite memory under
`memory/slice9-planner`, the unchanged planner prompt plus a session-only
terrain-delivery objective, and a real offline Alex client. The exported
evidence includes the complete prompt and disposable drivers to replay it.
Alex waited for active autonomous gathering with two birch logs, then sent:
“Clyde, craft four sticks and keep them in your inventory, then resume your
work.” No scripted tool sequence or injected scheduler goal served as the
planner's answer.

Observed: autonomous `auto-1` cancelled/requeued; planner acknowledged;
player goal succeeded with **4/4 sticks** (also independently polled over
RCON); the original delivery predicate remained **16 birch planks in the
same chest**; `auto-1` resumed, gathered more wood and deposited all 16.
Independent gym outcome: **pass, 125s**. Actual send-to-observed-ack was
**12.586s**; scheduler accept-to-reply-attempt was **11.325s**. These measure
different boundaries, not contradictory latencies.

Limitation: the gym terminates the process group as soon as its predicate
holds. That raced the scheduler's final ownership release (`/stop` connection
reset); memory retains the autonomous job as running for live reconciliation
on restart, not a durable `goal_succeeded` entry. The server delivery and
resumption were observed; clean completion/restart was not established.
The judge also logged state-read deadlines during the session. Neither the
successful player request nor the final chest inventory justifies claiming
uninterrupted reflex coverage or general planner reliability.

Cleanup was investigated separately, not excused by task success. The
original deposit action #9 had already emitted `action.succeeded` before
the checker terminated the group. `gym.agentProc.stop` sends SIGTERM to
the whole body/serve/player group, waits up to 5s, then SIGKILLs remaining
members. The scheduler's independent `/stop` therefore raced body death.
No detached body service or new executor survived that group's cleanup.
This trigger is a gym teardown artifact; a dead/unreachable body can also
make ordinary `serve` fail its stop. In that case the scheduler fails
closed rather than starting another controller. Its worker is joined on
exit and the next session stops/reconciles before running work. These local
leases do not protect against arbitrary remote controllers.
No action after cleanup or overlapping controllers was observed. `/stop`
cancels the action runner and halts movement/digging; it is not a rollback
of packets already sent or a security sandbox for arbitrary eval JavaScript.
A stop timeout alone cannot prove a still-live remote body is quiescent:
standalone `serve` has no such network-partition guarantee. The existing
container entrypoint subsequently terminates the body. The observed failed
cleanup killed the body itself; it did not demonstrate this conditional case.

An ordered replay kept the disposable body alive until `serve` exited,
matching `deploy/entrypoint`'s planner-before-body shutdown. It passed the
same server delivery at **130s**, durably recorded `auto-1 goal_succeeded`,
acknowledged three stop requests, captured a cleanup card with `action=null`,
then stopped the managed body; its port was closed and service removed.
Acknowledgement was **6.837s** in that replay, not a replacement for the
original 12.586s measurement. No lifecycle code change was needed.

The ordered scenario was repeated once on the final body with fresh memory
(`memory/slice9-planner-final`): **pass, 141s**, real player preemption,
**4/4 sticks**, original `auto-1` resumption and durable success. The gym
observed the original at-least-16 delivery predicate; serve's final check
reported 18/16. Actual acknowledgement was **6.484s**. Final cleanup again
recorded three stop events after deposit success and `action=null`, then
removed the body service and closed its port. There were **0 state-read
deadlines in 247 ticks**, with one Jev backend deadline. This does not replace
the original acknowledgement or conceal its failed group-kill cleanup.

The original judge recorded **54 state-read deadlines in 166 ticks**, plus
one backend error. These were mid-run `GET /state` requests exceeding the
unchanged 800ms tick deadline, not just cleanup failures. The first occurred
during unchanged `collect-logs`, before revised `craft-item` ran. The ordered
same-code replay had **0 state-read deadlines in 229 ticks**, plus one
backend error. State building synchronously scans nearby blocks and the
diagnostic player additionally polls at 5 Hz; load/instrumentation are
hypotheses, not an established cause. The ordinary serve observation path
can encounter such deadlines; neither new-code regression nor continuous
reflex coverage was demonstrated. These traces remain in the evidence.

### Validation and review

Local Auto-review covered the full base-to-final diff with ship-risk and
simplicity lenses, including generic crafting/window cleanup, independent
fixture predicates, native placement-event semantics, stale action ownership,
and the observed planner shutdown path. No material code finding remained.
The measured model failures are retained capability limits, not passing
tests or evidence that every layout is solved.

```sh
(cd body && npm test && npm run build)  # 22/22 tests; build passes
(cd harness && go build ./... && go vet ./... && go test ./...)
(cd harness && go test -race -count=1 ./... && go generate ./...)
git diff --exit-code -- api body/src/api harness/body
gym/bin/gate-skills  # 5/5, serialized before model arms
gym/.bin/clyde skill verify collect-logs craft-item craft-place-table deposit-items eat-food
```

All commands passed. Exact authored/promoted three-file content comparison
also passed. One permanent body regression checks observable progress and
foreign/stale-event isolation; the longer live controls remain experiment
evidence rather than a new slow test framework. No generated API changes,
dependencies, runtime configuration or deployment changes were needed.

The six existing primitive/multi-step executor tasks passed **6/6** after
each batch. Final times were collect 27s, goto 52s, place 4s, stock chest 7s,
logs-to-table 39s and logs-to-chest 29s, with no judge errors. The same
commands, limits and fixtures were used; no regression failure was discarded.
The final body was also reinstalled from the unchanged lockfile before its
22-test/build check, and the installed native placement source hash matched
the measured version.

### Spend, retained evidence and limits

All recorded experiment usage, including the contaminated arm, both author
attempts, both refinement batches, regressions, three planner scenarios and
the primitive/placement/idle controls:

| Provider | Returned tokens | Cost from returned usage and repository tariffs |
|---|---:|---:|
| Sol 6.1 | 1,537,546 | $0.8159269 |
| Jev | 12,400,824 input | $0.520834608 |
| Combined | — | **$1.336761508** |

This is below the initial $5 experiment bound. It is not a billing hard cap:
cancelled/unreturned requests may incur unreported charges. The per-arm ledger
is in `09-evidence.json`; model-free crafting diagnostics incurred no Sol
usage. Failed controls and interrupted trials are not presented as passes.
The entire contaminated arm remains excluded from comparison but included
in spend. Selected failed-shelter tick tails are committed; complete raw
tick streams, agent logs, planner memory and control traces are retained in
the slice thread's compressed evidence artifact as well as ignored run data.
Archive: `slice9-raw-evidence.tar.gz` (2.1 MiB), SHA256
`0e9cd9fd2e926ad5c2e2790e613b3bcce0dd83aace2fcd5eaaebebe0c66b6c2f`.

Next bottleneck: investigate the layout-dependent stone-collection/model-judge
failure with a bounded, controlled reproduction before changing targeting or
calibration. Intermittent state-read deadlines also remain an observation-path
limit, not an established skill regression. The historical live gather stall
is still unexplained. These peaceful gym results do not establish unattended
survival or authorize rollout. The coordinator owns roadmap review and any
separate live deployment decision; the accepted media-ovh image is untouched.
