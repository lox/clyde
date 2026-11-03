# Slice 15: local survival during frontier latency

Lachlan approved this slice in
[the coordinator discussion](https://ampcode.com/threads/T-01a1940d-be92-7625-9329-7da018f30d0d).
Implement and validate in disposable worlds only. This is not permission to
deploy, change production memory/worlds, or enable new live background behaviour.

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
