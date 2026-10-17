#!/usr/bin/env bash
# End-to-end smoke test: starts the body against the gym server and exercises
# state, chat, eval, goto, preemption, collect, dig, place, safety refusals,
# and stop. Needs the gym server running (gym/bin/paper start), curl, and jq.
#
# Usage: body/scripts/smoke.sh   (exits non-zero on the first failed check)
set -euo pipefail
cd "$(dirname "$0")/.."

export BODY_LISTEN_HOST=127.0.0.1
export BODY_LISTEN_PORT=${BODY_LISTEN_PORT:-7777}
export MC_USERNAME=${MC_USERNAME:-ClydeSmoke}
# Nothing below y=55 may be broken or placed into, anywhere in the overworld.
export BODY_PROTECTED_REGIONS='[{"name":"deep","min":[-30000000,-64,-30000000],"max":[30000000,55,30000000],"dimension":"overworld"}]'
BASE="http://$BODY_LISTEN_HOST:$BODY_LISTEN_PORT"
LOG=$(mktemp -t body-smoke.XXXXXX.log)

if curl -fsS -o /dev/null "$BASE/state" 2>/dev/null; then
  echo "something is already serving $BASE; stop it or set BODY_LISTEN_PORT" >&2
  exit 1
fi

# Same as npm run dev, as a single process so the trap can stop it.
node --import tsx src/index.ts >"$LOG" 2>&1 &
BODY_PID=$!
cleanup() { kill "$BODY_PID" 2>/dev/null || true; }
trap cleanup EXIT

fail() {
  echo "FAIL: $*" >&2
  echo "--- body log ($LOG) ---" >&2
  tail -20 "$LOG" >&2
  exit 1
}
step() { printf '\n== %s\n' "$*"; }
post() { curl -sS -X POST "$BASE$1" -H 'content-type: application/json' -d "$2"; }
act() {
  local r
  r=$(post /action "$1")
  printf '> POST /action %s\n%s\n' "$1" "$r" >&2
  jq -er .id <<<"$r"
}
# Prints the final event for action $1 once it has one, waiting up to $2 seconds.
wait_action() {
  local id=$1 deadline=$((SECONDS + $2)) ev
  while ((SECONDS < deadline)); do
    ev=$(curl -fsS "$BASE/events?since=0" | jq -c --argjson id "$id" \
      'first(.events[] | select(.data.id == $id and (.type | test("^action\\.(succeeded|failed|aborted)$")))) // empty')
    if [ -n "$ev" ]; then
      jq -c '{type, text} + (if .data.result then {result: .data.result} else {} end)' <<<"$ev"
      return
    fi
    sleep 0.5
  done
  fail "action $id did not finish within $2s"
}
expect() { # expect <json> <jq filter> <description>
  jq -e "$2" <<<"$1" >/dev/null || fail "$3: $1"
}

step "start body (log: $LOG)"
for _ in $(seq 1 60); do
  curl -fsS -o /dev/null "$BASE/state" 2>/dev/null && break
  kill -0 "$BODY_PID" 2>/dev/null || fail "body exited"
  sleep 1
done
curl -fsS -o /dev/null "$BASE/state" || fail "body did not come up"
head -1 "$LOG"

step "GET /state validates against api/state.schema.json"
curl -fsS "$BASE/state" | node --input-type=module -e '
  import { Ajv } from "ajv"
  import { readFileSync } from "node:fs"
  const validate = new Ajv({ allErrors: true }).compile(JSON.parse(readFileSync("../api/state.schema.json", "utf8")))
  const card = JSON.parse(readFileSync(0, "utf8"))
  if (!validate(card)) { console.error(JSON.stringify(validate.errors, null, 2)); process.exit(1) }
  console.log("valid: bot", card.bot.name, "at", JSON.stringify(card.bot.position))
' || fail "state card does not validate"

step "GET /state?format=text"
curl -fsS "$BASE/state?format=text"

step "chat"
id=$(act '{"type":"chat","message":"smoke test: hello"}')
r=$(wait_action "$id" 10); echo "$r"
expect "$r" '.type == "action.succeeded"' "chat failed"
sleep 1
curl -fsS "$BASE/events?since=0" | jq -c 'first(.events[] | select(.type == "chat" and (.text | contains("smoke test: hello")))) // error("no chat echo")' \
  || fail "chat did not echo back"

step "POST /eval"
echo '> POST /eval {"code":"return { pos: bot.entity.position.floored(), below: bot.blockAt(bot.entity.position.offset(0, -1, 0)).name }"}'
r=$(post /eval '{"code":"return { pos: bot.entity.position.floored(), below: bot.blockAt(bot.entity.position.offset(0, -1, 0)).name }"}')
echo "$r" | jq -c '{status, value: .result.value}'
expect "$r" '.status == "succeeded" and (.result.value.pos.y | type == "number")' "eval failed"

step "goto a standable spot 6-16 blocks away"
pick=$(post /eval '{"code":"const me = bot.entity.position; const air = (p) => bot.blockAt(p)?.boundingBox === \"empty\"; const spot = bot.findBlocks({ matching: mc.blocksByName.grass_block.id, maxDistance: 16, count: 400 }).find((p) => p.distanceTo(me) >= 6 && p.y > 55 && air(p.offset(0, 1, 0)) && air(p.offset(0, 2, 0))); if (!spot) throw new Error(\"no standable grass nearby\"); return spot.offset(0, 1, 0)"}')
target=$(jq -ce '.result.value' <<<"$pick") || fail "could not pick a goto target: $pick"
id=$(act "$(jq -c '{type: "goto", x, y, z, range: 1}' <<<"$target")")
sleep 1
curl -fsS "$BASE/state?format=text" | grep '^action:' || true
r=$(wait_action "$id" 60); echo "$r"
expect "$r" '.type == "action.succeeded"' "goto failed"

step "preemption: a new action aborts the running goto"
here=$(curl -fsS "$BASE/state" | jq -c '.bot.position')
id=$(act "$(jq -c '{type: "goto", x: (.x + 60 | floor), y, z, range: 1}' <<<"$here")")
sleep 1
curl -fsS "$BASE/state?format=text" | grep '^action:' || true
id2=$(act '{"type":"chat","message":"smoke test: interrupted"}')
r=$(wait_action "$id" 2); echo "$r"
expect "$r" ".type == \"action.aborted\" and (.text | contains(\"superseded by chat #$id2\"))" "goto was not aborted by the new action"
curl -fsS "$BASE/events?since=0" | jq -c --argjson a "$id" --argjson b "$id2" \
  '[.events[] | select(.data.id == $a or .data.id == $b) | select(.type != "action.log") | .text]'
wait_action "$id2" 10 >/dev/null

step "collect 2 grass_block"
id=$(act '{"type":"collect","block":"grass_block","count":2,"radius":16}')
r=$(wait_action "$id" 90); echo "$r"
expect "$r" '.type == "action.succeeded" and .result.collected >= 1' "collect failed"
sleep 1
curl -fsS "$BASE/state" | jq -c '{inventory: .inventory.items}'

step "dig a nearby solid block"
pick=$(post /eval '{"code":"const me = bot.entity.position.floored(); for (const d of [[1,-1,0],[-1,-1,0],[0,-1,1],[0,-1,-1]]) { const b = bot.blockAt(me.offset(...d)); if (b && b.boundingBox === \"block\" && b.position.y > 55 && bot.canDigBlock(b)) return b.position } throw new Error(\"no diggable block next to the bot\")"}')
target=$(jq -ce '.result.value' <<<"$pick") || fail "could not pick a dig target: $pick"
id=$(act "$(jq -c '{type: "dig", x, y, z}' <<<"$target")")
r=$(wait_action "$id" 30); echo "$r"
expect "$r" '.type == "action.succeeded"' "dig failed"

step "place dirt back where the block was"
id=$(act "$(jq -c '{type: "place", item: "dirt", x, y, z}' <<<"$target")")
r=$(wait_action "$id" 30); echo "$r"
expect "$r" '.type == "action.succeeded"' "place failed"

step "safety: protected region refuses dig and place"
here=$(curl -fsS "$BASE/state" | jq -c '.bot.position | map_values(floor)')
id=$(act "$(jq -c '{type: "dig", x, y: 50, z}' <<<"$here")")
r=$(wait_action "$id" 5); echo "$r"
expect "$r" '.type == "action.failed" and (.text | contains("inside protected region deep"))' "dig in protected region was not refused"
id=$(act "$(jq -c '{type: "place", item: "dirt", x, y: 50, z}' <<<"$here")")
r=$(wait_action "$id" 5); echo "$r"
expect "$r" '.type == "action.failed" and (.text | contains("inside protected region deep"))' "place in protected region was not refused"

step "safety: players are never attacked, even from eval"
echo '> POST /eval {"code":"bot.attack({ type: \"player\", username: \"Bonnie\" })"}'
r=$(post /eval '{"code":"bot.attack({ type: \"player\", username: \"Bonnie\" })"}')
echo "$r" | jq -c '{status, error}'
expect "$r" '.status == "failed" and (.error | contains("attacking players is never allowed"))' "attack on a player was not refused"

step "stop: POST /stop halts a running goto"
here=$(curl -fsS "$BASE/state" | jq -c '.bot.position')
id=$(act "$(jq -c '{type: "goto", x: (.x - 60 | floor), y, z, range: 1}' <<<"$here")")
sleep 1.5
echo "> POST /stop"
r=$(post /stop ''); echo "$r" | jq -c '{stopped: (.stopped | {id, type, status, error})}'
expect "$r" ".stopped.id == $id and .stopped.status == \"aborted\"" "stop did not abort the goto"
sleep 1
p1=$(curl -fsS "$BASE/state" | jq -c '{action, pos: .bot.position}')
sleep 1
p2=$(curl -fsS "$BASE/state" | jq -c '{action, pos: .bot.position}')
echo "after stop: $p1 then $p2"
expect "$p2" '.action == null' "an action is still running after stop"
[ "$(jq -c .pos <<<"$p1")" = "$(jq -c .pos <<<"$p2")" ] || fail "bot still moving after stop"

step "events (tail)"
curl -fsS "$BASE/events?since=0" | jq -r '.events[-12:][] | "\(.seq) \(.type): \(.text)"'

printf '\nSMOKE OK\n'
