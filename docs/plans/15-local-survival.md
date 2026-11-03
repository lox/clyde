# Slice 15: local survival during frontier latency

Lachlan approved this slice in
[the coordinator discussion](https://ampcode.com/threads/T-01a1940d-be92-7625-9329-7da018f30d0d).
Implement and validate in disposable worlds only. This is not permission to
deploy, change production memory/worlds, or enable new live background behaviour.

**Blocked on a third-party controller cancellation/drain contract.** The audit
and unchanged-source reproduction below are complete. No production code,
dependency, schema, prompt or admitted skill changed. The coordinator accepted
the controller-handoff prerequisite but will seek the user's decision before
opening upstream work. Do not implement arming or weaken exclusive ownership
while that decision is outstanding. Paid authoring/integration/judge calls: **0**.

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

The probe delays completion of `bot.equip` while **the real pathfinder physics
callback** navigates through a wall or builds a bridge. The real equip/dig/place
functions remain in use. After stopping/replacing the action, it releases the
old equip continuation and records the goal at the late mutation. RCON checks
the changed block independently of bot events.

- Replacement: the old route dug stone `(2,81,0)` under the new refuge goal.
- Operator `POST /stop`: releasing the old equip still dug `(2,81,0)` with
  `goal=null`, without dispatching any replacement action.
- Eval deadline: after the 1.5s timeout and stop, the old route still dug
  `(2,81,0)` under the replacement goal.
- Placement: after stop, **goal=null, moving=false, mining=false,
  building=false, targetDigBlock=null**. Nevertheless, releasing old equip
  placed dirt at `(2,79,0)` under the replacement goal.

All four server assertions returned `Test passed` for the **unwanted** world
mutation. These are failures of cancellation, not passing survival gates.
The initial manual dig probe was followed by the retained scripted reproduction;
the six survival trials were not rerun or altered after these probes.

Pathfinder's `monitorMovement` listener is registered outside ActionRunner's
`AsyncLocalStorage`. Its asynchronous chains can outlive the action without
the aborted signal. Merely wrapping the next `bot.dig`/`placeBlock` call would
still leave private `resetPath`, locks, `placing`, `returningPos` and related
continuations live. Idle flags and a resolved `goto` cannot prove a drain.

### Required prerequisite contract; upstream decision pending

The coordinator approved a separate correctness-preserving prerequisite commit,
but **no fix has been implemented or accepted**. The necessary contract is:

1. Revoking a controller incarnation synchronously prevents it from initiating
   more movement, digging, placement or item use. Every post-await mutation and
   shared-state cleanup is generation-bound, including physics-event work.
2. Stop-and-drain returns a promise covering every controller-owned asynchronous
   chain through its final shared-state cleanup, not only the current `goto`
   receipt or wrapped primitive promise. No new owner is admitted before that
   promise establishes quiescence. Cancellation must not depend on another
   goal's events to settle.
3. A bounded body handoff deadline **fails closed**. Expiry is not successful
   drain; pending actions are refused and ownership remains unavailable until
   quiescence is verified or the body process is restarted. Stop, timeout and
   disconnect cannot reopen admission or silently transfer ownership.
4. Tests must release delayed equip both before and after cancellation, exercise
   delayed dig/place/shared-state cleanup, operator stop and eval expiry, and
   retain an uncooperative continuation past the deadline. Prove no world
   mutation/new movement while revoked or draining, successful handoff only
   after actual drain, and no delayed cleanup cancelling the new owner's goal.

The installed public pathfinder API has no such join or generation primitive.
The coordinator chose pursuing an upstream stop-and-drain contract over
rejecting all handoffs once navigation has entered an asynchronous mutation;
the latter would not deliver gathering-to-retreat survival. Opening that
third-party work awaits the user. Do not fork/replace pathfinder, patch caches
or treat a quiet interval as proof of quiescence. No async-hook framework was
introduced as a substitute for the missing dependency contract.
After that boundary is sound, resume the original single-response plan and
its unchanged paid-request caps; no arming API is selected yet.

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
When implementing the upstream fix, adapt the probe to release blocked equip
while a stop/drain request is pending, then assert **unchanged** server blocks
and admission ordering. Do not reinterpret the current capture-only probe's
exit code as a regression-test pass.

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
