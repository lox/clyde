# Slice 5: skills and multi-step gym tasks

Goal: Clyde can discover and run reusable JavaScript skills, and only skills
that pass their gym task are admitted. Add harder tasks so the gym measures
more than a single primitive action.

## Done when

- `skills/<name>/{skill.md,index.js,test.yaml}` has a documented format in
  `skills/README.md`. The description covers inputs, preconditions, runtime,
  results, and failure modes. `index.js` executes as an async function body
  through the existing body eval runtime, with JSON arguments and the same
  `bot`, `mc`, `Vec3`, `goto`, `sleep`, `log`, and `signal` bindings. No Node
  module loader or mineflayer compatibility layer is needed.
- `harness/skills` owns loading, discovery, and admission. Extend the existing
  `find_skill` tool and add `run_skill(name, args)` to the executor and MCP.
  They share the same loader. `run_skill` reads the code from disk, passes
  it to the existing body `/eval` action, and uses its timeout/cancellation
  and safety limits rather than adding another script process. Reject
  invalid names/path traversal and report missing skills as tool errors.
- Admission is tied to the exact contents of all three skill files. A gym
  gate command runs `test.yaml` in a reset world and records a local receipt
  only after the machine-checkable goal passes. Discovery/execution expose
  only admitted skills; editing code, description, or test invalidates the
  receipt. Receipts and failed candidates are not evidence merely because
  an LLM says a task succeeded. Document where receipts live and how a fresh
  checkout re-runs gates. Candidate testing can execute the candidate before
  admission without exposing it to normal executor runs.
- Ship five useful initial skills, authored with the executor model rather
  than hard-coding strategy in Go: craft an item, collect a requested log
  count, craft and place a crafting table, deposit items in a container,
  and eat available food. Refine the names/parameters if live tests show a
  better boundary. Each has a gym test with prerequisites and an independent
  server-side success check. Add a minimal shared predicate only if the
  current items/arrived/built/delivered predicates cannot check the result;
  do not substitute a model's self-reported success.
- Add at least two repeatable multi-step gym tasks, including logs → planks
  → table → placement starting without the crafted items, and a resource
  gathering/crafting/delivery task. Do not prescribe a sequence of model
  tool calls; a single general eval may legitimately solve a whole task.
  The difficulty comes from the world and prerequisites, not forcing turns.
- Run Sol 6.1 at low effort on the new tasks three times each with skills
  disabled, and three times with admitted skills enabled, from the same
  reset fixtures and budgets. Record pass rate, time, model tokens/cost,
  and actual skill use. Report honestly if skills do not improve results;
  do not tune the checkers or make unsupported capability claims.
- Offline tests exercise discovery, changed-content rejection and execution
  via the public loader/tool contract, plus refusal after failed gating.
  The five gates pass live. Existing body and harness checks pass; CI runs
  deterministic skill gates without a model/API key. The live comparison
  and model-authorship transcript go in commit messages.

## Starting point

Read the As built sections of 01–04. `harness/executor/tools.go` already
searches `skills/*/skill.md`, but does not load or execute code. The body
eval API, tool adapters shared with MCP, and `gym/bin/run` with repeated
runs and cost reporting already exist. Reuse them, including `GYM_READY`.
The skill test format should reuse the gym task loader rather than invent
a second scenario language. No skill library exists yet.

Slice 6 is preparing in parallel. You own `harness/skills`, `skills/`,
skill tools, gym tasks/gates and their CLI wiring. Tell the coordinator when
the skill invocation and receipt contracts settle; slice 6 integrates after
you ship. Avoid changes to the planner or memory packages.

## Notes

- Lachlan chose mineflayer for now. The mc26-kit spike continues separately
  and must not block this work. No portability shim or body migration.
- Call provider adapters only through `harness/llm`; use `gpt-6.1-sol`.
  Models author skills; deterministic code loads and gates them.
- A receipt is a development admission check, not a hostile-code security
  boundary. Body eval is trusted model code and `/query` is read-only by
  convention, not an enforced sandbox.
- A skill must not outlive its eval action. Exercise cancellation and the
  existing safety limits with a skill invocation, not just direct eval.
- The existing four primitive gym tasks remain useful regression controls.

## Not in this slice

Autonomous skill authoring or promotion by the planner, priority lanes,
persistent planner memory, production deployment, and replacing mineflayer.
