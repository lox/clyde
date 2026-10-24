# AGENTS.md

Clyde is an AI-driven Minecraft player. Read `docs/plans/roadmap.md` first;
it is the source of truth for architecture, decisions, and slice order.

## Layout

- `api/`: JSON Schemas for the state card, actions, and goals; generated TS
  and Go types. Change the schema, then regenerate; never hand-edit generated
  files.
- `body/`: TypeScript mineflayer bot with an HTTP API (tier 0).
- `harness/`: Go module. `judge/`, `executor/`, `planner/`, `mcp/`, shared
  `llm/` and `jev/` clients.
- `skills/<name>/{skill.md,index.js,test.yaml}`: model-authored skills gated
  by gym tasks.
- `prompts/`: model-facing prompt files, one per role.
- `gym/`: throwaway Paper server runner, world templates, `tasks/*.yaml`,
  scoreboard.
- `deploy/`: compose and config for production in `lox/selfhosted-ovh`.
- `docs/plans/`: roadmap and one plan per slice.

## Build and test

- Body: `cd body && npm ci && npm test && npm run build`.
- Harness: `cd harness && go build ./... && go test ./...`.
- Gym server: `gym/bin/paper start|stop|logs` (installed by `.agents/setup`;
  Paper runs offline mode on 127.0.0.1:25565 with seed `mcbot`).
- Gym tasks: `gym/bin/run <task>` prints a scoreboard; include it in the commit message.

Tools installed by `.agents/setup` live under `~/tools` (Go, JDK); put
`~/tools/go/bin` and `~/tools/jdk/bin` on `PATH` if a shell does not have
them.

## Conventions

- Minecraft protocol version is pinned in `body/src/config.ts` (`26.1`). Do
  not bump it without checking the mineflayer ceiling.
- Models are pluggable: never call a provider SDK outside `harness/llm` or
  `harness/jev`. New backends implement the existing interface.
- Safety limits live in the body, not in prompts.
- Secrets come from the environment (`TYPESAFE_API_KEY`, `ANTHROPIC_API_KEY`,
  `OPENAI_API_KEY`). Never commit them or print them.
- Prefer small, boring code. The intelligence belongs to the models and the
  skill library.

## Design principle: defer to models

- Models own language understanding, conversational replies, clarification,
  priorities, project decomposition, strategy, recovery and deciding what to
  remember or turn into a skill. Prompts and model-authored skills are the
  normal extension points for improving playing ability.
- Code supplies accurate observations, tools, durable state, independent goal
  verification and bounded execution. The body enforces hard safety; the
  harness enforces ownership, cancellation, budgets and admission contracts.
  These limits must not silently choose Minecraft tactics or override a
  model's useful decision merely because the harness lacks a way to express it.
- Do not add keyword intent handlers, scripted dialogue, task-specific plans,
  hard-coded recovery tactics or benchmark exceptions to the harness. Simple
  addressing/routing and factual signal processing are not semantic policy.
  Before adding a heuristic, ask whether the model instead needs better
  evidence, a clearer prompt, a tool or a tested reusable skill.
- Deferring to models does not permit bypassing safety, fabricating progress,
  trusting self-reported success, unbounded calls or automatic skill admission.
  Evaluate changes on observed failures and matched gym tasks; retain negative
  results rather than coding around a benchmark.

## Thread workflow

- One coordinator thread owns the roadmap. It writes slice plans, creates one
  thread per slice with `create_thread`, reviews results, and updates the
  roadmap. It does not implement.
- Changes land by committing to `main` (Amp's Ship / merge-to-main
  workflow), not through pull requests. Before shipping: run Auto-review and
  fix its findings, run the build and tests above, rebase onto
  `origin/main`, then push. Keep commits small and logical; prefix the
  subject with the area (`body:`, `gym:`, `judge:`, `plans:`).
- Slice threads implement one `docs/plans/NN-name.md`, update that plan if
  reality differs, ship to `main`, and confirm CI on the pushed commit is
  green, fixing forward if it is not.
- Commit messages carry evidence: test output, and for anything touching
  behavior, a gym scoreboard or smoke-script transcript.
- Never deploy to Bonnie's server from a thread; production changes go
  through `lox/selfhosted-ovh` PRs.
