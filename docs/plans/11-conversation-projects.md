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
