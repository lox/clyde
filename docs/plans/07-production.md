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
  authorization. It must not restart Paper/public Velocity or unrelated
  services. Clyde joins the private bot proxy, chats and completes one
  modest non-destructive request. Record the result and real limitations;
  no broad autonomous survival claim from one smoke test.

## Starting point

Slices 1–6 are shipped. `deploy/README.md` is still a placeholder; no Clyde
container exists. `harness/README.md` documents `clyde serve`, budgets,
memory and stop ordering. `skills/README.md` documents admission and isolated
gates. Preserve existing behavior; package it rather than redesign it.

The checked-in OVH proxy layout has `minecraft-bonnie-bot-proxy:25565` on
`minecraft_bonnie_net`, opt-in profile `clyde`, no published bot port, with
ViaVersion/ViaBackwards on Paper. The proxy rollout is already deployed;
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

## Not in this slice

Dashboards, public MCP/API, HA, distributed leases, automatic skill promotion,
classifier distillation, client migration, comprehensive survival evaluation,
and a broader production-hardening programme.
