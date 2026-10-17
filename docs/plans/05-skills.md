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

## As built

`harness/skills.Store{Dir}` owns captured three-file snapshots, ranked
discovery, content-bound receipts, invocation and explicit promotion. Both
executor and MCP expose the same `find_skill`/`run_skill` tools. An empty
`--skills-dir` (gym: `CLYDE_SKILLS_DIR=''`) removes both tools. Invocation
uses the existing body eval API, with JSON object arguments and its normal
safety, timeout and abort handling. Strategy stays in JS; gates call no model.

`gym.Gate(ctx, store, name, Runner)` revokes only that store's receipt,
executes a frozen candidate through the existing executor gym agent, checks
the ordinary independent RCON goal, then admits only unchanged contents.
Receipts are `<root>/<name>/.admission.json`, ignored at arbitrary roots.
Stage replacements in a separate candidate `Store`, gate there, then call
`live.Promote(candidate, name)` or `clyde skill promote --from ... --dir ...`.
Failed gating and stale/failed promotion preserve the working installed
files and receipt; offline tests and a live deliberately lying candidate
confirmed this. See `skills/README.md` for the exact format, digest and CLI.

The five JS bodies and descriptions are verbatim Sol 6.1 low-effort outputs
from four author/refinement calls through `harness/llm`. Live feedback found
findBlock palette predicates without positions, craft output left on the
cursor, and single-block collect/drop handling on vertical trunks. Final
model revisions use numeric block searches, bounded cursor stowing and batch
collectblock targets. Initial gate rounds passed 1/5, 3/5 and 2/5; the final
five gates pass without provider keys: collect logs 19s, craft item 2s,
craft/place table 2s, deposit 1s, eat 2s. Eating stew is witnessed by the
server-side empty bowl, unavailable in setup; no new goal predicate was
needed. The model-authorship prompts/responses and scoreboards are retained
in commit messages, not admission receipts checked into Git.

Final comparisons use identical fixtures, 180s task budgets, the same
executor prompt, `gpt-6.1-sol`, low effort, three runs per cell. Tokens/cost
are means per run (including failures); times are medians of passes:

| task | skills | pass | time | tokens/run | cost/run | skill calls |
|------|--------|------|------|------------|----------|-------------|
| logs-to-table | off | 3/3 | 20s | 9583 | $0.0060 | 0 |
| logs-to-table | on | 3/3 | 18s | 6188 | $0.0052 | 5 |
| logs-to-chest | off | 2/3 | 47.5s | 14528 | $0.0086 | 0 |
| logs-to-chest | on | 3/3 | 35s | 11502 | $0.0055 | 9 |

These small samples suggest lower token use, not general reliability proof.
The disabled chest run failed to find crafted planks for its deposit, then
the judge ended its retry as stuck; that run did not inspect the cursor.
An earlier comparison with the old prompt had zero skill calls:
off passed 6/6, enabled 4/6 (table 1/3, chest 3/3); it did **not** show a
benefit. A general prompt preference to search/use fitting admitted skills
was then added, and **both** final arms were rerun. No checker or fixture was
relaxed. Normal shutdown can log a failed `/stop` when the gym has already
terminated the body's process group; this did not determine goal outcomes.

The existing Paper lifecycle now accepts `GYM_PAPER_NAME`, `GYM_MC_PORT`,
`GYM_RCON_PORT`, `PAPER_DIR`, `GYM_TEMPLATES` and `BODY_LISTEN_PORT`;
`gym.Runner.Env` passes consistent per-instance overrides without global
environment mutation. A second live server (`paper-live`, 25566/25576,
body 7788) retained its Java/body PIDs, online player, exact position,
seven diamonds and diamond marker through five candidate resets/gates
(`paper-candidates`, 25567/25577, body 7789), with distinct template roots.
The working environment example is in `skills/README.md`. `PAPER_DIR` alone
is explicitly insufficient isolation.

A live admitted collect skill confirmed timeout, context cancellation,
protected-region refusal and the one-block break cap; a diagnostic candidate
confirmed the cap's explicit body refusal (collectblock itself reports
generic no-progress). The body now expires completed/timed-out eval ownership
as well as preempted ownership, preventing detached pathfinder work from
overriding the next action; its regression test failed before the fix.
Offline loader/tool/promotion/failure tests, body tests/build and harness
build/vet/test/race pass. CI adds all five deterministic model-free gates.
The six task checkers pass with cheat and fail with idle at a 2s timeout
(including all four primitive controls), as expected.
This is still trusted local code and serialized local promotion, not a
hostile-code sandbox or concurrent registry. Mineflayer remains the runtime;
there was no production deployment or planner implementation in this slice.
