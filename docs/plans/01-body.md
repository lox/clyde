# Slice 1: body v0

Goal: a mineflayer bot wrapped in an HTTP API that the harness and an Amp
thread can drive, with hard safety limits, proven against a local Paper
server.

## Done when

- `npm run dev` in `body/` connects to the gym Paper server and serves
  `GET /state`, `POST /action`, `POST /eval`, `GET /events` on
  `BODY_LISTEN_HOST:BODY_LISTEN_PORT` (default 127.0.0.1:7777).
- A new `POST /action` aborts the running one within one tick and the
  aborted action appears in `/events`.
- `/state` returns the state card JSON and `?format=text` returns the rendered
  card. The JSON validates against `api/state.schema.json`.
- Safety: `POST /stop` halts everything; `BODY_MAX_BREAKS_PER_MIN` caps block
  breaking; players are never attacked; configured protected regions are
  refused for dig/place.
- Node tests cover: action preemption, event cursor semantics, state card
  rendering from a fixture, and the safety caps. These run without a server.
- An end-to-end script (`body/scripts/smoke.sh`) starts against the gym
  server and exercises goto, collect, dig, chat, eval, and stop; its output is
  in the PR.

## Starting point

`body/src/` already contains draft `config.ts`, `events.ts`, `actions.ts`,
`eval.ts`, and `state.ts` from the spec thread. They have never been compiled
or run. Missing: `server.ts`, `index.ts`, tests, schema, smoke script.
Treat the drafts as a sketch, not a contract; keep what survives review.

## Notes

- Versions: mineflayer 4.39.0, mineflayer-pathfinder 2.4.5,
  mineflayer-collectblock 1.6.0, bot pinned to `version: '26.1'`.
- minecraft-data entity `kind` values are `'Hostile mobs'` and
  `'Passive mobs'`.
- Progress deltas, not stuck detection: `goto` reports distance remaining and
  last-moved timestamp; `collect` reports items so far. The judge decides.
- Keep `/eval` sandboxed with `node:vm` and a timeout; expose `bot`, `mc`,
  `Vec3`, `goals`, `Movements`, `goto`, `sleep`, `log`, `signal`.

## Not in this slice

Judge, executor, skills, production join through Velocity, Bedrock.
