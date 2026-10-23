# Slice 7: simple private deployment on media-ovh

Goal: run the full Clyde on our Minecraft server: Sol 6.1 planner and
executor, Jev judge, persistent memory and admitted skills. Keep deployment
boring and private, not a production-hardening project.

## Done when

- Clyde has a reproducible container build and one opt-in service example.
  Node body and the Go harness may share one container. Package prompts,
  schemas and the skill library alongside the binaries; use the existing
  CLI rather than a new service controller. No public HTTP/MCP port,
  dashboard, registry service or distributed orchestration.
- Startup waits for a ready body, then runs `clyde serve` with Sol 6.1 low
  for both model slots and Jev using environment-provided keys. A failed
  child stops the service; SIGTERM stops the planner before the body and
  leaves no orphan bot. Memory lives on a persistent writable mount.
- Explain a simple, non-production-world-reset bootstrap for admitting the
  five bundled skills at their exact shipped contents. Do not fabricate
  admission receipts or silently advertise unadmitted skills as available.
  Test the packaged skill discovery and an actual invocation. Never run a
  skill gate against Bonnie's world.
- Support the existing inventory/arrival goals immediately. Check whether
  private Paper RCON is already provisioned for build/delivery predicates;
  use it if available without publishing it. If absent, document the exact
  capability limitation and the smallest configuration needed, rather than
  quietly claiming full build/delivery verification. Do not weaken the
  planner, model, skills or memory just to simplify packaging.
- Reuse existing safety limits and protected-region configuration. Start
  only one controller. The reliable initial kill switch is stopping the
  whole service: document it, and make shutdown work. Chat-stop latching is
  deferred, not a prerequisite for this attended first deployment. No
  speculative access-control framework or new region-management system.
- Local container verification proves readiness, an actual goal, persistent
  memory after restart, admitted skill invocation and clean service stop.
  Run Auto-review, relevant body/harness checks and CI. Ship small commits
  to Clyde main with transcripts and image/build instructions.
- The `lox/selfhosted-ovh` deployment thread adds the opt-in Compose service
  through that repo's PR flow, then runs it on media-ovh under Lachlan's
  authorization. The final owner-selected target is a fresh separate play
  world, not Bonnie: private direct Java, new world/memory/RCON, and Bedrock
  Geyser/Floodgate access. Do not restart Bonnie/public Velocity or unrelated
  services. Verify Clyde joins and completes one modest request. Record the
  result and real limitations; owner iPad gameplay is a separate check, not
  established by a signaling probe or console whisper.

## Starting point

Slices 1–6 are shipped. `deploy/README.md` is still a placeholder; no Clyde
container exists. `harness/README.md` documents `clyde serve`, budgets,
memory and stop ordering. `skills/README.md` documents admission and isolated
gates. Preserve existing behavior; package it rather than redesign it.

The checked-in OVH proxy layout has `minecraft-bonnie-bot-proxy:25565` on
`minecraft_bonnie_net`, opt-in profile `clyde`, no published bot port, with
ViaVersion/ViaBackwards on Paper. Host preflight found the public proxy
healthy, but the private bot proxy and offline whitelist were still pending;
verify the live state with its owning deployment thread, not assumptions
from an old PR. The offline identity for `Clyde` must be whitelisted with
its offline UUID, not Mojang name resolution. Forwarding secrets stay with
the proxy; the bot does not need them.

## Notes

- This thread packages/tests Clyde only; it never deploys to Bonnie.
  Host configuration and launch belong to `lox/selfhosted-ovh`.
- Lachlan explicitly wants an intelligent bot with simple deployment.
  Full planner/memory/skills are not optional replacements for a basic
  one-goal client. Existing budgets remain configurable, not a permanent
  artificial reduction in capability.
- Secrets stay in private environment/files; never images, commits or logs.
- Deployment source and image revision must be identifiable. No automatic
  latest-image updater is needed for the first run.

## As built

- `deploy/Dockerfile` packages the compiled body and pure-Go `clyde`, prompts,
  schemas and all five bundled skills. One short Bash entrypoint waits for a
  spawned `/state`, then runs existing `serve --judge jev`. Sol 6.1 low
  defaults for both model slots, safety settings and planner/executor budgets
  remain configurable and unchanged. No new controller or public API/MCP.
- `deploy/compose.example.yaml` is private and opt-in (`clyde` profile),
  `init: true`, no auto-restart and no ports. It persists the whole memory
  directory at `/opt/clyde/memory` as UID/GID 1000. TERM stops/joins planner
  before body with 35/5-second child limits and a 50-second Compose grace.
  Unexpected child exit fails the unit. Stopping the whole service remains
  the reliable kill switch; chat latching is deliberately deferred.
- Admission uses the existing model-free isolated gym gate, never Bonnie.
  `deploy/export-admissions` exports only five genuine receipts and the source
  revision. Build checks that revision and each source digest through new
  read-only `clyde skill verify`; startup rechecks all five. Missing/stale
  receipts fail clearly, rather than quietly dropping skills. Normal startup
  does not gate, reset worlds, or promote candidates. Docker context excludes
  local source receipts and accepts only the explicit export. The full
  gate/export/build/transfer/start/stop runbook is `deploy/README.md`.
- Local gates on the disposable orb Paper 26.1.2 passed **5/5**: collect-logs
  19s (4/4 logs), craft-item 2s (12/9 planks), craft-place-table 2s (1/1
  blocks), deposit-items 1s (7/7 container planks), eat-food 2s (1/1 bowl).
  Exact three-file skill sources were not changed by packaging.
- Packed-filesystem verification used the compiled CGO-disabled Go binary,
  compiled body with pruned production dependencies, and image-layout assets,
  not tsx/source execution. MCP discovered all five; real `run_skill` crafted
  12 planks from three logs, independently checked over RCON. A real Sol 6.1
  low/Jev executor discovered/invoked craft-item for a table: succeeded in
  7.88s, 4,002 tokens, $0.0065289, RCON 1/1 table.
- Full packaged `serve` with the shipped planner prompt completed a wooden
  pickaxe goal and Alex's real chat request for supplied-log planks. RCON
  checked 1/1 pickaxe and 15/15 planks; player acknowledgement took 2889ms.
  The general craft-item skill failed once for the pickaxe with no inventory
  progress; the executor recovered through ordinary eval and independently
  reached the goal. This existing skill limitation was not hidden or changed.
  Restart preserved all 17 existing episode rows, home, reported usage
  (41,922 tokens / $0.0359202) and one reply, without duplicate reply or
  new model usage. Test-only budgets were one autonomous goal / 60s executor;
  deployment defaults were not reduced. This is not a broad survival claim.
- Direct entrypoint lifecycle tests against real Paper/body/serve passed:
  TERM 0.22s/status 0, planner kill 0.12s/status 1, body kill 0.12s/status 1,
  all children gone; TERM during pre-spawn readiness never started planner;
  unreachable Minecraft failed before planner. Missing Jev and stale skill
  contents failed clearly. Orb supervisor stop signals its entire cgroup,
  unlike Docker init forwarding; shutdown ordering was therefore tested by
  signalling the entrypoint directly, not inferred from supervisor stop.
- Body 21 tests/build and harness build/vet/test pass. Auto-review corrected
  Docker ignore directory exceptions that could include extra export files;
  Docker's pattern matcher confirmed private files/local receipts excluded
  while source and exported receipts remain included. CI now builds the
  gated image and checks admission plus missing-Jev startup without secrets.
- Host Docker/Compose acceptance passed on exact source `91348f47` and image
  `sha256:de20f61423903e9ce95c5635fd0c1114b4aff702900b85d7d25c0432194ca551`:
  Node 24/UID 1000, five admissions, real skill/full planner task, memory
  restart without duplicate reply/new usage, TERM and both child-failure
  shutdowns, plus pre-readiness/missing-key/stale-admission negatives.
- Lachlan chose a separate persistent play world before interacting on
  Bonnie. `lox/selfhosted-ovh` PR 11 merged at `1ea028ec`; the host started
  only new `minecraft-clyde` and relocated `clyde`, with fresh world and
  `clyde-play-memory`, private stable RCON file and native Geyser HTTPS/
  NetherNet at `minecraft.loxflix.xyz:19134`. Paper 26.2 build 134 and pinned
  ViaBackwards accept the unchanged 26.1 body. Copied TLS requires PKCS#8
  and attended renewal before its 2027-01-15 expiry; recreate only the new
  server after replacing file mounts. Host runbook is `docs/minecraft-clyde.md`
  in selfhosted-ovh. Existing Bonnie/proxies/Traefik stayed unchanged.
- Live autonomous gather-four was interrupted as stalled after one birch
  log (server checked), not claimed successful. A genuine admin-console
  whisper requested a table from that log: acknowledgement 3062ms, admitted
  skill planks 678ms/table 650ms, independent RCON inventory 1/1 table.
  Returned planner/executor usage snapshot was 28,458 tokens/$0.036456,
  excluding total Jev/unknown billing and ongoing calls. The world and bot
  were left running. External certificate-trusted signaling passed, but
  **real owner iPad authentication/UDP gameplay remains unverified**. No
  broad survival claim or production skill gates follow from these checks.

## Not in this slice

Dashboards, public MCP/API, HA, distributed leases, automatic skill promotion,
classifier distillation, client migration, comprehensive survival evaluation,
and a broader production-hardening programme.
