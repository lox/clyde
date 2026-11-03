# Slice 15: local survival during frontier latency

Lachlan approved this slice in
[the coordinator discussion](https://ampcode.com/threads/T-01a1940d-be92-7625-9329-7da018f30d0d).
Implement and validate in disposable worlds only. This is not permission to
deploy, change production memory/worlds, or enable new live background behaviour.

**Blocked: the model-free cancellation experiment fails admission gates.**
Movement-only navigation and a serialized foreground fixture can reach the
refuge, but native collection, replacement during equip, and ordinary danger
preemption remain incompatible with that candidate. The responsible layers and
the smallest proposed next boundary are recorded below; an upstream pathfinder
drain is neither proven necessary nor sufficient. No runtime fix, arming API or
model-authored response has been accepted. The 18 candidate survival trials
were not run; paid authoring/integration/judge calls remain **0**.

## Problem and first outcome

The persistent player preserves context while the frontier thinks, but its
danger precheck can only stop current work and request new attention. Stopping
resource collection is not a survival response when inference takes seconds.

Mineflayer already supplies local physics; the installed pathfinder steers,
jumps, swims during navigation and replans. Collectblock already combines
navigation, tool choice, mining and pickup. Reuse those controllers rather than
implementing their mechanics again. The missing behaviour is local activation
and safe ownership transfer for a response chosen before danger appears.

Deliver one independently tested response: a hostile approaches during
resource collection while frontier inference is delayed; Clyde interrupts
collection and retreats to a reachable refuge selected by the model in advance.
The same conversation receives the outcome and decides what to do next.
This is a narrow survival capability, not autonomous combat or proof of
days-long unattended play.

## Audit and reproduce before designing the API

- Inspect the installed Mineflayer/pathfinder/collectblock implementations
  and existing combat/auto-eating plugins. Record exact versions/revisions,
  sources and relevant capabilities, protocol compatibility evidence,
  cancellation behaviour and control ownership. Do not infer compatibility
  from a README or install a new dependency merely because it exists.
- Check whether the response can use existing admitted skills and body action
  ownership. Adopt a new plugin only if it removes real work and passes the
  cancellation/safety checks. No competing automatic movement/item controllers.
- Reproduce the unchanged bot's behaviour in a disposable Paper scenario.
  Select one hostile type and a reachable refuge; freeze the scenario and
  independent success/failure criteria before evaluating the candidate.
  Inject 5, 15 and 30 seconds of frontier latency through the provider boundary,
  without changing models or putting scenario tactics in production code.
- Record the observed baseline before proceeding. If the proposed failure
  cannot be reproduced, report that and narrow the problem from evidence;
  do not claim improvement by changing the scenario after seeing the results.

## Ownership and model authority

The frontier selects the admitted response, its bounded parameters and its
applicability before danger. Tactical trigger/response logic belongs in an
admitted model-authored skill, not keyword handlers or hard-coded escape rules
in the harness. Local code supplies factual observations, deadlines, action
ownership, cancellation, telemetry and hard safety.

Use the smallest extension justified by the reproduction. If an armed response
is needed beside foreground work, its observation phase must not mutate the
world. Activation must obtain exclusive action ownership before acting:

- Interrupt the foreground action and prevent its late work from issuing
  movement, digging, item-use or other mutations that conflict with the new
  owner. Exercise actual plugin behaviour, not just outer API mocks. Preserve
  the repository's trusted-code boundary; do not claim hostile-code isolation.
- Record activation and the new action incarnation so a stale model response
  or stale danger/stall verdict cannot take over or cancel the response.
  Do not remove danger checks globally or accept a skill's claim that danger
  has been handled as proof of safety.
- Operator stop, disarm, expiry, cancellation, disconnect and allowance
  exhaustion must prevent subsequent activations and stop response-owned
  work. Disarming an inactive response must not stop unrelated foreground
  work; operator stop still stops everything. Bound arming lifetime and
  response duration; no automatic rearming, budget refill or unlimited
  repeated triggers. Define replacement and repeated-threat semantics in
  the plan before implementing the contract.
- Keep provider history/tool exchanges complete when an activation supersedes
  inference. Report trigger facts, cancelled work, outcome and uncertainty
  into the existing persistent session. Do not automatically replay interrupted
  gathering, rebuild a separate recovery agent, or close the conversation.
- On restart, reconcile and clear expired/uncertain ownership rather than
  replaying an armed response or assuming it remains active. Restart must not
  extend a reaction's lifetime or grant resources.

Keep existing schema generation, body safety, protected regions, no-player-
attack rules and explicit skill admission. Runtime play may stage a candidate
but may not admit it. This slice authorizes isolated development gating and
explicit promotion of the tested candidate, not production promotion.

## Implementation boundaries

Likely owners are `body/src/actions.ts`, `body/src/eval.ts`, the body API/schema
if needed, `harness/session` attention/tools/journal, the shared skill loader,
one skill and one reproducible gym scenario. Inspect their contracts before
selecting the changes. Change schemas first and regenerate affected types.
Do not introduce a second skill loader, action runner, background agent,
behaviour-tree framework, tactical rule engine or new model tier.

The slice thread owns audit, baseline, implementation and evidence. Update
this plan with the chosen minimal contract and evidence before broadening
changes. If the required contract is materially larger than this first
capability, bring that decision to the coordinator instead of silently
building a general reaction system.

## Validation and acceptance

Use matched unchanged/candidate runs with the same world setup, timings,
equipment, threat type and model configuration. Test all three injected
latencies and more than one threat position. Preserve failed trials and report
numerators/denominators, not a general survival claim from one favourable run.

Measure threat observation, activation, first actual defensive movement,
damage, death, independently verified refuge arrival and subsequent session
continuity. Use server/world evidence for outcomes, not skill return values.
Separate event-detection latency from controller handoff and movement latency;
the nominal 50ms physics tick is not an end-to-end latency guarantee.
Prove the response begins before the deliberately delayed frontier can return,
with no new provider call needed to choose or execute that response.

Controls must cover no threat/no activation, blocked refuge, repeated threat,
operator stop during response, expiry/disarm and late old-controller work.
Retain protected-region/player-attack safety and unchanged skill admission.
Check that a generic danger stop does not repeatedly cancel the response and
that stale inference cannot dispatch a conflicting action. Use deterministic
public-interface regressions for ownership races, cancellation and restart.

Author/refine the candidate through the existing `harness/llm` path with the
current Sol 6.1 low model; record provenance and gate evidence. Start with at
most 12 frontier authoring/refinement requests, then at most 24 frontier
requests for one attended real-model integration after deterministic gates.
These are request caps, not dollar guarantees; account separately for judge
calls, returned usage and unknown billing. Give the attended session at most
64 shared durable reservations, including monitoring and compaction. Stop and
report if those caps are insufficient rather than silently running more paid
trials. No paid call is needed for baseline delayed-provider fixtures or
repeated mechanics controls.

Before source acceptance: Auto-review and address grounded findings; run body
tests/build, Go build/tests/vet, targeted race checks, generated-code drift
checks, unchanged executor controls and existing skill gates. Include gym
scoreboards or smoke transcripts in commits. Ship small logical commits to
`main` per repository workflow, then confirm exact-head CI. The coordinator
reviews source/evidence and updates the roadmap; production rollout remains
a separate decision.

## Audit and unchanged-source evidence (2026-11-03)

The source under test was `17a5eaf76bb278775cb806322d9d011a20e3a37d`.
[15-evidence.json](15-evidence.json) preserves all six baseline results, damage
and action events, independent server position/health samples, raw transcript
hashes and the controller probes. `gym/local-survival.go` reproduces these
observations through the real body, Paper and persistent session. It uses a
fixture at the `agent.Provider` boundary, never a paid model or a scripted
production response. Exit zero means the capture completed, **not** that Clyde
survived or ownership was safe; inspect the recorded RCON and event evidence.

### Installed controllers do not expose a drain boundary

Versions below are installed lockfile resolutions except the two explicitly
uninstalled candidates. Sources were inspected from their npm release tarballs;
the links identify the corresponding published `gitHead`.

| Component | Version / source revision | Relevant behaviour and cancellation |
|---|---|---|
| Mineflayer | [4.39.0 / c168635](https://github.com/PrismarineJS/mineflayer/tree/c168635cf2f6602069fc5e408bec864702335581) | Physics/control states, inventory/equip, dig/place. `dig` can await looking before installing `stopDigging`; stopping is not a universal join of in-flight work. Emits both `physicsTick` and deprecated `physicTick`. |
| Pathfinder | [2.4.5 / ca35a00](https://github.com/PrismarineJS/mineflayer-pathfinder/tree/ca35a00ec18e7d3095280ffe2dc194e7c81b55eb) | Local A*, steering, jumps/parkour, water navigation, digging/scaffolding and replanning. `setGoal(null)` resets path state synchronously; `stop()` only sets a stop flag. Neither joins private equip→dig/place chains. `goto` settles on goal change, not controller quiescence. |
| Collectblock | [1.6.0 / 05eab37](https://github.com/TheDudeFromCI/mineflayer-collectblock/tree/05eab37a47b81c5b4f26ed7168188bc89094dfe0) | Navigates, equips through tool, digs and collects drops; waits ten physics ticks for drops. `cancelTask` requests pathfinder stop and waits for `collectBlock_finished`; empty targets return immediately. Clyde already clears private targets and guards late `goto`/`dig`, but this does not join pathfinder callbacks. |
| Tool | [1.2.0 / 8d6aa2a](https://github.com/TheDudeFromCI/mineflayer-tool/tree/8d6aa2a2701577fbdcf0eef806670d303a95e88f) | Tool choice/equip and optional chest retrieval, called by collectblock. No action incarnation or abort contract. |
| Physics | 1.11.1 (shared by Mineflayer/pathfinder) | Local movement simulation, not survival strategy or ownership arbitration. |
| PVP, **not installed** | [1.3.2 / 0b4006d](https://github.com/TheDudeFromCI/mineflayer-pvp/tree/0b4006de03f33f901ab9005c99278cfb24bb22e0) | Pursuit, attack cooldown and shield control. Subscribes to deprecated `physicTick`. Stop can wait five seconds, then removes all `path_stop` listeners and clears the shared goal. Adds competing movement/item control; unnecessary for retreat. |
| Auto-eat, **not installed** | [5.0.3 / 9d45ea9](https://github.com/linkle69/mineflayer-auto-eat/tree/9d45ea9f2fbe0425739141e73a22c0027d259054) | Threshold-triggered food selection/equip/use and optional restoration. `disableAuto` removes polling, not an in-flight equip/eat; cancellation binding is installed after equip, and finally may re-equip the old item. Adds competing item control; unnecessary for retreat. |

Actual compatibility evidence: Mineflayer protocol **26.1**, Paper
**26.1.2-74-ver/26.1.2@e4e17fc**, Java **25.0.5+8-LTS**, installed pathfinder,
collectblock and tool ran all six trials plus the probes. Movement across the
two-block moat, collection, digging and scaffolding worked on this stack.
This is not protocol validation of PVP or auto-eat: those packages were only
read under `/tmp`, never loaded or added as dependencies. No cache or
`node_modules` source was edited.

### Delayed inference reproduces the survival gap

The arena starts Clyde at `(0.5,80,0.5)`, empty inventory, full health/food,
collecting five oak logs at `(0,80..84,-4)`. Normal difficulty, fixed midnight,
no natural regeneration or ambient spawning; one adult zombie is summoned at
`(4.5,80,0)` or `(4.5,80,3)` one second into the delayed request. A two-block
moat at `x=-8..-7` separates the reachable destination `(-12,80,0)`.
Geometry is rebuilt between runs; mob AI/random state is not replayed. No mob
equipment was explicitly assigned. These are six small trials, not a survival
rate estimate for general play.

The fixture first requests collection, then delays the next two frontier
requests by 5, 15 or 30 seconds. Cancellation is honoured. Those delayed
responses request the same `goto`; subsequent fixture responses only wait.
Death/respawn and fresh danger can cancel the second retreat request too.
The fixture does not perform unlimited recovery after death. This limitation
does not affect the observed first-death outcome. The destination is currently
**fixture-selected**, not yet model-selected; no candidate or authoring call
has run. The planned model-selected response still requires separate evidence.

Success means no death through `delay + 12s` after summon and an independent
RCON position within two blocks of the destination. Skill returns never certify
arrival. Values below are seconds after summon; damage is before the first
death, or over the full trial when no death occurred.

| Delay | Zombie z | Hostile first sampled | Collection stopped | Retreat dispatched | RCON arrival | First death | Damage | Survived + arrived |
|---:|---:|---:|---:|---:|---:|---:|---:|---|
| 5 | 0 | 0.200 | 0.772 | 7.015 | 9.021 | — | 18 | yes, health 2 |
| 5 | 3 | 0.189 | 2.155 | 8.254 | 10.105 | — | 18 | yes, health 2 |
| 15 | 0 | 0.177 | 0.665 | — | — | 7.774 | 20 | no |
| 15 | 3 | 0.212 | 2.224 | — | — | 8.718 | 20 | no |
| 30 | 0 | 0.173 | 0.639 | — | — | 8.039 | 20 | no |
| 30 | 3 | 0.215 | 3.207 | — | — | 10.417 | 20 | no |

Overall **2/6** survived and arrived; **0/4** at the longer delays. The first
sampled hostile is not necessarily within the danger threshold yet. RCON
samples retain movement, but do not separate knockback from intentional
movement; no first-defensive-movement latency claim is made. The full runs
retain 3 or 5 deaths in the failed cases rather than hiding respawns. All
trials used fresh 64-reservation journals, no extensions and no paid judge.

### Real-plugin probes invalidate the existing handoff guarantee

The probe pauses **before calling the original `bot.equip`**, while **the real
pathfinder physics callback** navigates through a wall or builds a bridge.
It does not pause inside Mineflayer's inventory implementation. The real
equip/dig/place functions remain in use. After stopping/replacing the action,
it releases the old equip continuation and records the goal at the late
mutation. RCON checks the changed block independently of bot events.

- Replacement: the old route dug stone `(2,81,0)` under the new refuge goal.
- Operator `POST /stop`: releasing the old equip still dug `(2,81,0)` with
  `goal=null`, without dispatching any replacement action.
- Eval deadline: after the 1.5s timeout and stop, the old route still dug
  `(2,81,0)` under the replacement goal.
- Placement: after stop, **goal=null, moving=false, mining=false,
  building=false, targetDigBlock=null**. Nevertheless, releasing old equip
  placed dirt at `(2,79,0)` under the replacement goal. The release occurred
  **after installing that replacement**, not while the idle snapshot held.

All four server assertions returned `Test passed` for the **unwanted** world
mutation. These are failures of cancellation, not passing survival gates.
The initial manual dig probe was followed by the retained scripted reproduction;
the six survival trials were not rerun or altered after these probes.

Pathfinder's `monitorMovement` listener is registered outside ActionRunner's
`AsyncLocalStorage`. Its asynchronous chains can outlive the action without
the aborted signal. Merely wrapping the next `bot.dig`/`placeBlock` call would
still leave private `resetPath`, locks, `placing`, `returningPos` and related
continuations live. Idle flags and a resolved `goto` cannot prove a drain.

### Oracle correction and approved cancellation experiment

The user requested an independent oracle review and approved its narrower
next step. The local-response direction remains justified, but a pathfinder
stop-and-drain API is neither proven necessary nor sufficient. The oracle
reproduced direct Mineflayer `dig` issuing a start packet after operator stop
when its internal `lookAt` promise was released. This used the real body runner
and digging implementation with an in-memory bot, not a live world. Placement
also awaits looking before sending, equip can span inventory transactions, and
collectblock owns separate equip/dig/drop/pickup work. Those boundaries need
testing independently of pathfinder's private continuations.

The required invariant is **no residual conflicting authority at handoff**:
old work must not issue conflicting commands or modify the new owner's goal,
controls, locks or collector targets. Already-sent requests can produce later
server effects; stop cannot undo them, prevent knockback or promise an instantly
unchanged world. Define which already-entered operations are noninterruptible
and await their bounded completion with admission closed. Reconcile ambiguous
effects from observations. A timeout never proves safe handoff. Harmless
observers with revoked mutation authority need not all settle.

Run this model-free experiment before designing an arming API:

1. Convert the retained capture cases into assertion-bearing conformance
   checks, preserving original failure evidence. Add direct dig with delayed
   look, placement with delayed look, non-hotbar equip, active digging,
   collector drop/pickup waits, and late success/error cleanup. Include stop,
   replacement, timeout and disconnect; release blocked continuations afterward
   to detect delayed effects. Use real implementations at the risky boundary.
2. Try an explicit movement-only navigation profile for **both** collection
   navigation and retreat: disable navigation-owned digging, scaffolding and
   door use while retaining movement/parkour. Collectblock installs its own
   Movements, so changing only retreat's configuration is insufficient. Keep
   resource digging explicit and test its cancellation; do not claim this
   profile alone fixes lower Mineflayer operations. Do not silently impose
   permanent restrictions on all existing actions.
3. Exercise actual collect-to-retreat handoffs at controlled collection phases
   in the frozen arena. A single foreground skill/fixture may compose the
   response if that avoids a background arming framework. Fixture tactics
   remain gym-only; model-authored admission is a later gate. Integrate or
   account for ordinary danger preemption rather than disabling it globally.
4. Separately measure local trigger, revocation, safe handoff, first commanded
   movement, first observed movement and independent server outcome. Retain
   threat timing, equipment, arena and the original six baseline runs.

Freeze these preliminary go/no-go criteria before candidate trials:

- No new unauthorized effectful operations after revocation except explicitly
  documented completion of already-entered noninterruptible operations. No
  replacement while they can conflict; no stale cleanup stealing new ownership.
- Stop, deadline and disconnect remain closed with uncooperative continuations.
  Uncertain timeout is a failure, never a successful drain. No raw-packet or
  arbitrary trusted-JavaScript sandbox guarantee is implied.
- In each preliminary arena run, safe handoff within **500ms** and first
  commanded defensive movement within **1s** of the local trigger. These are
  proposed engineering acceptance thresholds, not established guarantees.
- Only after conformance passes, require survival and independently verified
  refuge arrival for three repeats of each 5/15/30s delay and threat-position
  cell. Preserve all failures; these 18 candidate trials are still not a
  general survival-rate estimate. Disclose any changed navigation capability
  and add matched controls when needed to attribute the result.

Use existing body/skill ownership mechanisms and make only narrow local changes
needed to test this supported boundary. No paid calls, dependency additions,
fork/cache patches, upstream PR, general reaction framework or production
changes are part of this experiment. A remaining candidate ownership escape,
unworkable route, stale cleanup or excessive handoff latency is a no-go for
that candidate, not grounds to skip the other bounded experiments. Identify
the responsible layer and recommend the smallest fix from its failing tests;
do not assume pathfinder is that layer. The coordinator reviews the outcome
before resuming model-authored response admission and the previously bounded
paid trials.

### Reproduction commands

Use a disposable orb only: the runner resets blocks and entities on local
Paper ports 25565/25575 and controls the local body on 7777. It reads the RCON
credential from `~/paper/server.properties` without printing it. Output
directories must be new; existing transcripts are never overwritten.

```sh
gym/bin/paper start
(cd body && npm ci && npm run build)
amp orb service start survival-body --cwd "$PWD/body" --command 'node dist/index.js'
export PATH="$HOME/tools/go/bin:$PATH"
(cd harness && go build -o /tmp/local-survival ../gym/local-survival.go)
for delay in 5 15 30; do
  for z in 0 3; do
    /tmp/local-survival "$delay" "$z" ".amp/in/survival/before-$delay-$z"
  done
done
for mode in handoff handoff-stop handoff-timeout handoff-place; do
  amp orb service restart survival-body
  sleep 3
  /tmp/local-survival "$mode" 0 ".amp/in/survival/$mode-before"
done
```

Restart the body after instrumentation probes; cleanup restores bot methods
but does not certify that the vulnerable plugin's private state is clean.
When evaluating a cancellation candidate, adapt the probe to release blocked
equip while cancellation is pending, then assert **unchanged** server blocks
for operations not yet issued and verify admission ordering. Do not
reinterpret the current capture-only probe's exit code as a regression pass.

### Evidence-only publication checks

- `cd body && npm test && npm run build`: **26/26**, build passed. Existing
  safety/admission behaviour is unchanged, not newly certified by these probes.
- `cd harness && go build ./... && go test ./... && go vet ./...`: passed.
- `go vet ../gym/local-survival.go` and a `go build -race` of that standalone
  runner: passed. All four controller probes were repeated with that binary;
  the same unwanted mutations were server-confirmed, with no Go race report.
  The timeout recheck additionally captured `action=null` and retained
  `lastAction.error="timed out after 1500ms"` before releasing old equip.
- `go test -race ./session ./skills ./executor`: passed.
- `go generate ./...`, `npm run gen`, and scoped generated-file diffs: no
  drift. `docs/plans/roadmap.md` is untouched.
- Local Auto-review applied ship-risk and simplicity lenses to this evidence
  diff. Tightened mode validation, bounded body HTTP reads, removed duplicate
  probe flow and asserted actual eval timeout before recording that case.
  Targeted recheck found no remaining material findings in the publication.

No runtime fix is being accepted. The unchanged paid executor curriculum and
real-model integration were not run: this publication changes only this plan,
the evidence JSON and the standalone capture runner. Existing model-free CI
gym/skill gates still apply to the pushed evidence commit. The original six
baseline transcripts remain unchanged; `probe_rechecks` records the separate
verification captures instead of replacing earlier evidence.

## Narrow cancellation experiment (2026-11-03)

Runtime source is unchanged from `3c65a5a5abca9071c7b11b7fcab6d325eca09b7f`.
The new standalone `gym/cancellation.go` and embedded
`gym/cancellation-fixture.js` exercise the real HTTP ActionRunner, Mineflayer,
pathfinder and collectblock in disposable Paper. They instrument bot instances,
not dependency files, and restart the body between cases. They are not loaded
by production or admitted as skills. Unlike the original capture runner, a
failed conformance assertion exits **1**. A missed fixture phase is inconclusive
and also exits 1; it must not be counted as passing cancellation.

New evidence is separate in [15-cancellation-evidence.json](15-cancellation-evidence.json).
It retains assertions, command/packet timestamps, independent server outcomes
and hashes of new transcripts. The original six baseline runs and all original
controller evidence are unchanged. These are phase-controlled mechanics
experiments, **not** the gated 18 delayed-inference survival trials.

The final matrix is **12/30 passing, 18/30 no-go**, plus **2/2** targeted
disconnect-alone rechecks. Three further collector rechecks after `npm ci`
still failed exact replacement-target preservation. Forty-two development
captures are retained but not pooled into those totals. Five no-threat local
handoff phases reached the server-verified refuge with health 20:

| Revoked collection phase | Handoff ms | First command ms | First observed movement ms |
|---|---:|---:|---:|
| Navigation | 1 | 6 | 134 |
| Explicit look | 200 | 207 | 253 |
| Active dig | 1 | 8 | 126 |
| Drop wait | 183 | 319 | 364 |
| Entered equip | 236 | 276 | 373 |

Times start at the local phase trigger, not hostile observation. First movement
is a bot physics sample more than 0.2 blocks from the handoff position; RCON
checks final arrival independently. No-threat phase success is not survival
evidence. The uncooperative-look control remained closed before and after late
release, failed the latency/arrival gates, and was not counted as a success.

### What the narrower experiment establishes

- **Pathfinder:** the four original escape cases still fail assertion-bearing
  checks. Late dig/place packets and unwanted server block changes occur after
  stop, replacement or timeout. Movement-only navigation separately reaches
  the east-side test destination around the wall and across the three-block
  gap without world/inventory mutation packets. Parkour remains enabled;
  digging, scaffolding, towers and door use are disabled. This removes the
  exercised navigation-owned mutation chains without modifying pathfinder.
- **Mineflayer:** direct `dig` and `placeBlock` still issue packets after their
  internal `lookAt` completion is held, the owner is revoked, and the promise
  is released. The probe first calls the real look; it holds its completion
  inside the real primitive. Active digging with `dig(block, 'ignore')` stops
  without a late start/finish or server block removal. This distinction is why
  a pathfinder-only fix cannot establish primitive cancellation.
- **Inventory:** non-hotbar equip sends its source-slot click, then resumes its
  private `moveSlotItem`/`clickWindow` sequence after stop or replacement. The
  probe sets `lastDigTime=Date.now()` immediately after the first real click to
  exercise inventory's existing recent-dig wait before the hotbar click. It
  does not replace equip or clickWindow. This is an injected clock boundary,
  not evidence of natural 500ms equip latency: digging currently writes
  `performance.now()` while inventory compares against wall time. Ordinary
  26.1 inventory clicks otherwise often complete through microtasks without
  waiting for server acknowledgements.
- **Collectblock compatibility:** setting **both** navigation profiles to
  movement-only also prevents explicit harvesting: `mineBlock` consults
  `movements.safeToBreak`, which returns false when `canDig=false`. The real
  body `collect` fails with zero harvested blocks. No global action
  restriction or private-policy override was introduced to hide this result.
- **Collector cleanup:** the real body `collect` is stopped in its ten-tick
  drop wait or after tool selection, then a replacement installs a sentinel
  target and retreats. Resuming ticks or rejecting the held tool continuation
  clears that replacement target. A real collector entity-pickup wait, invoked
  through eval because the body collect action accepts blocks, has the same
  shared-target failure after the unpickable item is removed. The sentinel
  measures shared-state interference; it is not a second full collection job.
  The drop probe holds `physicsTick` delivery after real digging completion,
  then resumes it; no claim is made that this delay is naturally occurring.
- **Disconnect:** the production body exits on connection end; the orb
  supervisor restarts an empty body. A release scheduled on the old instance
  does not survive that process boundary. Test restart state and unchanged
  server blocks, not permanent HTTP unavailability. Earlier experimental
  captures incorrectly treated the supervised restart as a failure; they are
  retained separately. Final disconnect rechecks do not send operator stop
  first, so they test disconnect alone.

### A foreground fixture works locally, but is not admission-ready

The smallest tested composition retains one ordinary eval owner. It uses
movement-only navigation, explicit tool equip, an explicit awaited look,
`signal.throwIfAborted()`, then `dig(block, 'ignore')`. That last call installs
dig cancellation synchronously rather than awaiting another look before the
first dig packet. It waits for entered operations before moving to the refuge.
The trigger is an external gym phase control; the refuge is fixture-selected.
This is a collection-phase fixture, not a complete replacement collector or
a model-authored skill.

The proposed noninterruptible boundary is an **already-entered inventory equip
sequence**, including its remaining clicks and cursor cleanup. That sequence
may finish after collection revocation, but the fixture does not grant retreat
until equip settles. Already-sent world effects are not undone. Explicit look
completion must be followed by a cancellation check before a new dig; active
dig cancellation is awaited; drop observers cannot dispatch new work after
revocation. The fixture rejects handoff after its 500ms allowance, including
after a deliberately uncooperative look is finally released. Passing that
fail-closed check does not pass the handoff-latency or survival gates.

Remaining candidate no-go cases are independent:

1. **Body ownership:** replacing this same fixture during the entered equip
   grants a new ActionRunner owner before the old click sequence settles.
   A single well-behaved eval cannot close admission to arbitrary subsequent
   body actions. Do not describe eval's abort-race completion as a drain.
2. **Harness coordination:** a conditional danger stop for the still-current
   eval token cancels its retreat too. The control summons the same adult
   zombie and explicitly calls the same `StopAction(token)` used by session;
   it does not run a second full session or measure threat-detection latency.
   The response does not reach the refuge. Danger checks were not disabled.
3. **Collector API:** movement-only `canDig=false` cannot be used unchanged
   with native collectblock harvesting; late shared-target cleanup also needs
   to settle or be isolated before another collector can own those targets.

**Recommendation for coordinator decision:** pursue a narrowly scoped body
operation/ownership boundary for explicit collection primitives before an
upstream navigation project. The new tests must prove that an entered equip
has bounded completion with replacement admission closed, and that timeout or
disconnect never converts an unresolved operation into permission to mutate.
An unresolved timeout must leave admission closed until quiescence is proved
or the body is restarted; returning an error alone does not revoke a callback.
Pair an explicit look/check/dig boundary with movement-only navigation. Keep
existing general actions available outside that opt-in contract. A local
activation also needs a new action incarnation, so a danger verdict observing
the old collection cannot cancel its replacement; a verdict actually observing
the response still requires the existing safety policy. No generic danger
exemption or self-reported "handled" flag is justified by these results.

This is a proposed next contract, not implemented source or permission for a
broader arming framework. No third-party change has been shown necessary for
this smaller composition. Model-authored admission remains with the coordinator
after the failures above are resolved and conformance is rerun.

### Repeat the conformance matrix

Use the same isolated Paper/body startup commands above. Each invocation resets
the arena and consumes **zero provider requests**. Restart the instrumented body
between cases. Do not reuse output directories. The loop intentionally keeps
going after no-go results and prints each exit code; its own exit status is not
an aggregate pass. No candidate survival runs are launched automatically.

```sh
export PATH="$HOME/tools/go/bin:$PATH"
(cd harness && go build -race -o /tmp/cancellation ../gym/cancellation.go)
for spec in \
  'path-dig stop' 'path-dig replace' 'path-dig timeout' 'path-place replace' \
  'path-dig-movement complete' 'path-place-movement complete' \
  'dig-look stop' 'dig-look replace' 'dig-look timeout' \
  'place-look stop' 'place-look replace' 'equip stop' 'equip replace' \
  'active-dig stop' 'collector-profile complete' 'collector-drop replace' \
  'collector-pickup replace' 'collector-error replace' \
  'local-navigation local' 'local-look local' 'local-dig local' \
  'local-drop local' 'local-equip local' 'local-equip replace' \
  'local-look stop' 'local-look timeout' 'local-stuck local' \
  'local-dig danger' 'dig-look disconnect' 'local-look disconnect'; do
  set -- $spec
  amp orb service restart survival-body
  sleep 3
  /tmp/cancellation "$1" "$2" ".amp/in/survival/new-run/$1-$2"
  echo "exit=$? $spec"
done
```

### Narrow experiment publication checks

- `cd body && npm ci --no-audit --no-fund && npm test && npm run build && npm run gen`:
  **26/26 tests**, build and generation passed. Lockfile versions remain
  Mineflayer 4.39.0, pathfinder 2.4.5, collectblock 1.6.0, tool 1.2.0 and
  physics 1.11.1 on protocol 26.1. Reinstallation confirms no dependency/cache
  patch is required by the probes.
- `cd harness && go build ./... && go test ./... && go vet ./... && go test -race ./session ./skills ./executor && go generate ./...`:
  passed. Existing admission/executor controls are unchanged.
- `go vet ../gym/cancellation.go` and `go build -race -o /tmp/cancellation ../gym/cancellation.go`
  from `harness`: passed. The final matrix and disconnect rechecks used the
  race-built probe with no race report; intentional no-go exits are retained.
- Generated-file diffs are empty; original six transcript hashes match, and
  `15-evidence.json`, `gym/local-survival.go` and the coordinator-owned roadmap
  are unchanged. All disposable services were stopped after verification.
- Local Auto-review used ship-risk and simplicity lenses. Tightened assertions
  to use the body's actual terminal timestamp rather than an HTTP marker,
  checked allowed equip completion precedes handoff and exact replacement
  target identity, added independent server block checks, and corrected
  disconnect checks for supervisor restart. The targeted recheck found no
  remaining material issue in this evidence-only publication. Runtime
  conformance remains **blocked**, not approved.

No paid curriculum, model authoring, skill admission, upstream project,
dependency change or production action was performed. The original request
caps remain untouched: 12 authoring, 24 integration, 64 shared reservations
for a later approved attended session. CI gym/skill gates are model-free;
exact-head CI must be confirmed after this evidence is pushed.
