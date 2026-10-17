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
  in the commit message.

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

## As built

Where the implementation differs from or adds to the plan above:

- `api/action.schema.json` defines the `POST /action` bodies (`goto`,
  `follow`, `collect`, `dig`, `place`, `chat`, `eval`, `stop`); the server
  validates against it. TS types for both schemas are generated into
  `body/src/api/` with `npm run gen`.
- State card field names are the draft's. Additions: `action` and
  `lastAction` carry `distanceRemaining` and `lastMovedAt` (goto, follow) and
  `collected` (collect, blocks broken so far) beside the `progress` text.
  Dropped item entities are named by item (`oak_sapling`), not `item`.
  `blocks` lists only blocks with a face against air or a transparent block
  of another type, so buried ores and the inside of lakes are not on it.
- `POST /eval` runs as the current action, like `POST /action {type: eval}`,
  but answers with the final status. `{type: stop}` is the same as
  `POST /stop` and does not become an action.
- Preemption is synchronous: the running action is marked aborted and its
  `action.aborted` event is pushed before the new `action.start`.
- `GET /events?since=N` returns `{events, latestSeq, missed}`; a cursor ahead
  of `latestSeq` (from an earlier body process) reads from the start.
- Event types: `spawn`, `chat`, `whisper`, `damage`, `death`, `pickup`,
  `kicked`, `error`, `stop`, `safety.refused`, `action.start`, `action.log`,
  `action.succeeded`, `action.failed`, `action.aborted`.
- Safety wraps `bot.dig`, `bot._genericPlace`, and `bot.attack`, so actions,
  eval code, pathfinder, and collectblock all hit the same checks, and adds
  exclusion areas to both pathfinder Movements so paths avoid protected
  blocks and stop digging at the cap. `BODY_PROTECTED_REGIONS` is a JSON
  array of `{name, min: [x,y,z], max: [x,y,z], dimension?}`. `BODY_OPS` lists
  players whose `<bot name> stop` chat message halts the bot (no latch).
- mineflayer 4.39 on Paper 26.1 workarounds: `bot.oxygenLevel` is set from any
  entity's air metadata, so the card reads the bot's own `air_supply`; and
  the server rubber-bands moves that end exactly touching a wall, so the
  physics half-width is 0.3001.
- No reconnect loop: the body exits on disconnect for its supervisor to
  restart.

## Not in this slice

Judge, executor, skills, production join through Velocity, Bedrock.
