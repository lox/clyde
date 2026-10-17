#!/usr/bin/env bash
# Soak test for issue #2: start a collect, preempt it with a goto after a
# random delay, and check the goto. Repeats N times (default 60) and counts
# gotos that fail with "goal was changed". Needs the gym server running
# (gym/bin/paper start), curl, and jq. It digs up the area around spawn, so
# run gym/bin/paper reset first when repeating it.
#
# Usage: body/scripts/soak-preempt.sh [N]   (SOAK_LOAD=4 adds 4 busy CPU loops)
# Exits non-zero if any goto failed with "goal was changed".
set -euo pipefail
cd "$(dirname "$0")/.."

N=${1:-60}
export BODY_LISTEN_HOST=127.0.0.1
export BODY_LISTEN_PORT=${BODY_LISTEN_PORT:-7779}
export MC_USERNAME=${MC_USERNAME:-ClydeSoak}
export BODY_MAX_BREAKS_PER_MIN=600
BASE="http://$BODY_LISTEN_HOST:$BODY_LISTEN_PORT"
LOG=$(mktemp -t body-soak.XXXXXX.log)

if curl -fsS -o /dev/null "$BASE/state" 2>/dev/null; then
  echo "something is already serving $BASE; stop it or set BODY_LISTEN_PORT" >&2
  exit 1
fi
node --import tsx src/index.ts >"$LOG" 2>&1 &
pids=($!)
cleanup() { kill "${pids[@]}" 2>/dev/null || true; }
trap cleanup EXIT
for _ in $(seq 1 60); do
  curl -fsS -o /dev/null "$BASE/state" 2>/dev/null && break
  sleep 1
done
curl -fsS -o /dev/null "$BASE/state" || { echo "body did not come up; see $LOG" >&2; exit 1; }
for _ in $(seq 1 "${SOAK_LOAD:-0}"); do
  (while :; do :; done) &
  pids+=($!)
done

post() { curl -sS -X POST "$BASE$1" -H 'content-type: application/json' -d "$2"; }
# Final event for action $1, waiting up to $2 seconds; empty if none.
final() {
  local deadline=$((SECONDS + $2)) ev
  while ((SECONDS < deadline)); do
    ev=$(curl -fsS "$BASE/events?since=0" | jq -c --argjson id "$1" \
      'first(.events[] | select(.data.id == $id and (.type | test("^action\\.(succeeded|failed|aborted)$")))) // empty')
    [ -n "$ev" ] && { echo "$ev"; return; }
    sleep 0.25
  done
}

# Two standable spots 5-10 blocks apart to walk between.
pick=$(post /eval '{"code":"const me = bot.entity.position.floored(); const air = (p) => bot.blockAt(p)?.boundingBox === \"empty\"; const ok = (p) => air(p.offset(0, 1, 0)) && air(p.offset(0, 2, 0)); const far = bot.findBlocks({ matching: (b) => b.boundingBox === \"block\", maxDistance: 12, count: 4000 }).find((p) => p.distanceTo(me) >= 5 && p.distanceTo(me) <= 10 && ok(p)); if (!far) throw new Error(\"no standable spot nearby\"); return [me, far.offset(0, 1, 0)]"}')
spots=$(jq -ce '.result.value' <<<"$pick") || { echo "could not pick spots: $pick" >&2; exit 1; }
echo "walking between $spots; body log $LOG; load ${SOAK_LOAD:-0}"

preempted=0 still=0 ok=0 changed=0
declare -A other
for i in $(seq 1 "$N"); do
  cid=$(post /action '{"type":"collect","block":"grass_block","count":6,"radius":16}' | jq -r .id)
  sleep "$(awk -v r=$RANDOM 'BEGIN { printf "%.2f", 0.3 + 2.2 * r / 32767 }')"
  running=$(curl -fsS "$BASE/state" | jq --argjson id "$cid" '.action.id == $id')
  spot=$(jq -c ".[$((i % 2))] | {type: \"goto\", x, y, z, range: 2}" <<<"$spots")
  gid=$(post /action "$spot" | jq -r .id)
  if [ "$running" = true ]; then preempted=$((preempted + 1)); else still=$((still + 1)); fi
  ev=$(final "$gid" 30)
  [ -n "$ev" ] || ev='{}'
  type=$(jq -r '.type // "timeout"' <<<"$ev")
  text=$(jq -r '.data.error // ""' <<<"$ev")
  case "$type:$text" in
    action.succeeded:*) ok=$((ok + 1)) ;;
    *"goal was changed"*) changed=$((changed + 1)); echo "  #$i goto $gid: $text" ;;
    *) other["$type: $text"]=$((${other["$type: $text"]:-0} + 1)); echo "  #$i goto $gid: $type $text" ;;
  esac
  printf '\r%d/%d: preempted %d, goto ok %d, goal changed %d' "$i" "$N" "$preempted" "$ok" "$changed" >&2
done
echo >&2
echo "iterations: $N (collect still running when preempted: $preempted; already finished: $still)"
echo "goto succeeded: $ok"
echo "goto failed with 'goal was changed': $changed"
for k in "${!other[@]}"; do echo "other goto outcome: $k x${other[$k]}"; done
[ "$changed" -eq 0 ]
