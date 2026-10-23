# Slice 9: harder gym tasks and evidence-led capability improvements

Goal: improve Clyde's actual multi-step play, using reproducible gym failures
and matched before/after runs rather than more deployment machinery.

## Done when

- Add three harder tasks using existing gym YAML and independent RCON goals:
  empty inventory to a stone pickaxe (wood, table, wooden tool, stone, final
  craft); a small specified shelter checked as actual placed blocks; and
  gathering from uneven terrain followed by crafted-item delivery to a chest.
  Supply reachable resources in the fixtures, not prerequisite inventory or
  a prescribed sequence of tool calls. Keep coordinates/geometry asymmetric
  so incorrect navigation, placement and inventory handling are observable.
- Prove each checker with the existing cheat positive control and idle
  negative control. Preserve existing primitive/multi-step tasks as regression
  controls. No model self-report may stand in for a server-checked outcome.
- Run the unchanged executor with Sol 6.1 low, Jev and the five admitted
  skills three times per new task. Freeze fixtures, prompts, time/turn limits
  and model settings for the baseline. Record pass rates, failure reasons,
  wall time, returned tokens/cost, actual skill calls and interrupt reasons.
  Investigate the live gather interruption hypothesis on the actual gym
  path before blaming Jev, progress accounting or the skill.
- Use failures to choose a small first batch of fixes to the owning body,
  executor, judge, prompts or skills. Fix mechanisms, not task names or known
  coordinates. Do not disable the judge, weaken safety/checkers, add free
  resources, or increase budgets only for the final arm. Model-author new
  or revised skill bodies through the existing provider, gate exact contents
  on isolated instances, and preserve failed attempts/authorship evidence.
- Repeat the same three runs per task after fixes. Include one held-out
  geometry/resource variant for each task, withheld from the first tuning
  batch. Report unchanged failures and regressions, not just best runs.
  If another refinement is justified, bound it to one further batch and
  distinguish previously seen variants from genuinely held-out evidence.
- Exercise the full planner on one harder scenario with real player chat
  preempting autonomous work, a server-verified player goal, and autonomous
  resumption or an explicit documented pause. Use the existing serve/event
  path, not executor-only success as evidence of planner reliability. Preserve
  separate memory and report acknowledgement latency and actual outcomes.
- Run Auto-review and fix grounded findings; body tests/build, harness
  build/vet/tests and relevant race checks pass, as do existing deterministic
  skill gates and CI at the pushed head. Update this plan's As built with
  commands, revision pins, full scoreboards, limits and the next bottleneck.
  Keep permanent tests only for demonstrated regression contracts.

## Starting point

Slices 1–6 and container packaging are shipped. Read their As built sections,
`gym/tasks/README.md`, `skills/README.md` and `harness/README.md`. Existing
`logs-to-table` and `logs-to-chest` fixtures are small, flat, peaceful arenas;
they do not establish resource-chain or construction competence. Reuse
`gym/bin/run`, existing agents, typed goals and independent predicates.

The persistent Clyde play world on media-ovh is running the accepted image.
Its first gather-four-birch-logs goal was interrupted as stalled after one
log; a console-whisper request then successfully crafted a table from that
log. This is a useful failing observation, not an established root cause.
The new work runs entirely in this slice thread's disposable orb gym.

## Notes

- Keep Sol 6.1 low for both roles; improve systems before changing models.
  Match judge configuration, skill admission and budgets between arms. Freeze
  task fixtures before measuring; if a fixture is wrong, repair it and rerun
  both arms rather than comparing different tasks.
- Start with at most US$5 of provider-reported model usage for this iteration;
  stop and report before extending the experiment. Track Jev usage separately
  where available; unreturned/cancelled billing is not a hard spending cap.
- Prefer three useful tasks and one coherent fix batch over a general
  benchmark framework or a large speculative skill library. If new predicate
  semantics are truly needed, first try expressing the observable outcome
  through existing items/built/delivered checks.
- Ship small logical commits to Clyde main through the repository workflow,
  with evidence in commit messages. Reply to the coordinator when CI is green.
  This does not authorize rebuilding or deploying the live image.

## Not in this slice

Production/play-world experiments or resets, media-ovh deployment, model
migration, judge distillation, automatic skill promotion, new infrastructure,
general autonomous-survival claims or unbounded benchmark spending.
