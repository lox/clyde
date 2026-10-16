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
- Gym tasks: `gym/bin/run <task>` prints a scoreboard; include it in PRs.

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

## Thread workflow

- One coordinator thread owns the roadmap. It writes slice plans, creates one
  thread per slice with `create_thread`, reviews results, and updates the
  roadmap. It does not implement.
- Slice threads implement one `docs/plans/NN-name.md`, update that plan if
  reality differs, and finish with Auto-review, a PR written with the
  PR-description skill, and Babysit until green. Title PRs
  `clyde: slice NN – name`.
- PRs carry evidence: test output, and for anything touching behavior, a gym
  scoreboard or smoke-script transcript.
- Never push to `main` directly. Never deploy to Bonnie's server from a
  thread; production changes go through `lox/selfhosted-ovh` PRs.
