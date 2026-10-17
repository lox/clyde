import { AsyncLocalStorage } from 'node:async_hooks'
import type { Bot } from 'mineflayer'
import type { Block } from 'prismarine-block'
import pathfinderPkg from 'mineflayer-pathfinder'
import type { goals as Goals, GoalPlaceBlockOptions } from 'mineflayer-pathfinder'
import type {} from 'mineflayer-collectblock'
import { Vec3 } from 'vec3'
import type { ActionRequest, StopAction } from './api/action.js'
import type { ActionStatus } from './api/state.js'
import type { EventLog } from './events.js'
import { runCode } from './eval.js'
import type { Safety } from './safety.js'

const { goals } = pathfinderPkg
type Goal = Goals.Goal

export type { ActionStatus }
export type RunnableAction = Exclude<ActionRequest, StopAction>
type ActionState = ActionStatus['status']
type Progress = Partial<Pick<ActionStatus, 'progress' | 'distanceRemaining' | 'lastMovedAt' | 'collected'>>

export interface ActionContext {
  bot: Bot
  safety: Safety
  signal: AbortSignal
  /** Updates the progress fields shown on the state card. */
  update: (progress: Progress) => void
  log: (text: string) => void
}

/** How often movement actions refresh distanceRemaining and lastMovedAt. */
const PROGRESS_INTERVAL_MS = 250

/**
 * The abort signal of the action whose code is running. Async work started
 * by an action (its awaits, timers, and plugin loops) carries it, so code
 * that outlives its action can be refused; see guardStaleCode.
 */
const actionSignal = new AsyncLocalStorage<AbortSignal>()

/**
 * Runs at most one action at a time. Starting a new action aborts the current
 * one synchronously, so the abort is in /events before the new action starts;
 * that is the harness's interrupt channel.
 */
export class ActionRunner {
  private nextId = 1
  private current: { status: ActionStatus; controller: AbortController } | null = null
  private last: ActionStatus | null = null

  constructor(
    private readonly bot: Bot,
    private readonly events: EventLog,
    private readonly safety: Safety,
  ) {}

  get currentStatus(): ActionStatus | null {
    if (!this.current) return null
    return { ...this.current.status, elapsedMs: Date.now() - this.current.status.startedAt }
  }

  get lastStatus(): ActionStatus | null {
    return this.last
  }

  /**
   * Ignores pathfinder goals and refuses gotos and digs from code whose
   * action was aborted. Without this, mineflayer-collectblock's loop could
   * pick up a drop that landed after its collect was preempted and path to
   * it, replacing the next action's goal (issue #2). Call once plugins are
   * injected.
   */
  guardStaleCode(): void {
    const stale = () => actionSignal.getStore()?.aborted ?? false
    const reason = () => new Error(`refused: ${actionSignal.getStore()?.reason?.message ?? 'its action was aborted'}`)
    const pf = this.bot.pathfinder
    const { setGoal, goto } = pf
    // Ignored rather than thrown: a throw from a stale timer callback would
    // be an uncaught exception.
    pf.setGoal = (goal, dynamic) => {
      if (!stale()) setGoal.call(pf, goal, dynamic)
    }
    pf.goto = (goal) => (stale() ? Promise.reject(reason()) : goto.call(pf, goal))
    const dig = this.bot.dig
    this.bot.dig = ((...args: Parameters<Bot['dig']>) => (stale() ? Promise.reject(reason()) : dig.apply(this.bot, args))) as Bot['dig']
  }

  /** Aborts the running action, if any, and stops all movement and digging. */
  stop(reason = 'stop requested'): ActionStatus | null {
    const aborted = this.abortCurrent(reason)
    this.halt()
    this.events.push('stop', reason)
    return aborted
  }

  /** Starts an action, aborting the running one. `done` settles with the final status. */
  start(request: RunnableAction): { status: ActionStatus; done: Promise<ActionStatus> } {
    const { type, ...params } = request
    const id = this.nextId++
    this.abortCurrent(`superseded by ${type} #${id}`)
    this.halt()

    const controller = new AbortController()
    const status: ActionStatus = { id, type, params, status: 'running', startedAt: Date.now(), elapsedMs: 0 }
    this.current = { status, controller }
    this.events.push('action.start', `#${id} ${type} ${describe(params)}`.trimEnd(), { id, type, params })

    const ctx: ActionContext = {
      bot: this.bot,
      safety: this.safety,
      signal: controller.signal,
      update: (progress) => {
        if (status.status === 'running') Object.assign(status, progress)
      },
      log: (text) => this.events.push('action.log', text, { id }),
    }

    const done = actionSignal.run(controller.signal, () => this.run(request, ctx)).then(
      (result) => this.finish(status, 'succeeded', { result }),
      (err: unknown) => this.finish(status, 'failed', { error: err instanceof Error ? err.message : String(err) }),
    )
    return { status: { ...status }, done }
  }

  /**
   * Runs eval code beside the running action instead of replacing it, for
   * read-only lookups such as recipes. Nothing stops the code from acting,
   * so callers keep it read-only; it ends only by finishing or timing out.
   */
  async query(code: string, timeoutMs: number): Promise<Pick<ActionStatus, 'status' | 'result' | 'error'>> {
    const ctx: ActionContext = {
      bot: this.bot,
      safety: this.safety,
      signal: new AbortController().signal,
      update: () => {},
      log: () => {},
    }
    try {
      return { status: 'succeeded', result: await runCode(ctx, code, timeoutMs) }
    } catch (err) {
      return { status: 'failed', error: err instanceof Error ? err.message : String(err) }
    }
  }

  private abortCurrent(reason: string): ActionStatus | null {
    if (!this.current) return null
    const { status, controller } = this.current
    this.finish(status, 'aborted', { error: reason })
    controller.abort(new Error(`aborted: ${reason}`))
    return { ...status }
  }

  /**
   * Leaves no goal, dig, or held control behind. setGoal(null) rather than
   * pathfinder.stop(): stop() only sets a flag that would also cancel the
   * next action's goal.
   */
  private halt(): void {
    this.bot.pathfinder.setGoal(null)
    if (this.bot.targetDigBlock) this.bot.stopDigging()
    this.bot.clearControlStates()
  }

  /** First call wins: an aborted action's late rejection does not overwrite its status. */
  private finish(status: ActionStatus, state: ActionState, extra: { result?: unknown; error?: string }): ActionStatus {
    if (status.status !== 'running') return { ...status }
    status.status = state
    status.endedAt = Date.now()
    status.elapsedMs = status.endedAt - status.startedAt
    if (extra.result !== undefined) status.result = extra.result as ActionStatus['result']
    if (extra.error !== undefined) status.error = extra.error
    if (this.current?.status.id === status.id) {
      const { controller } = this.current
      this.current = null
      // Eval can start detached plugin/timer work. Its ownership expires on
      // completion and timeout too, not only on explicit preemption.
      if (status.type === 'eval' && state !== 'aborted') {
        controller.abort(new Error(`eval ${state}`))
        this.halt()
      }
    }
    this.last = status
    this.events.push(`action.${state}`, `#${status.id} ${status.type} ${state}${status.error ? `: ${status.error}` : ''}`, {
      id: status.id,
      type: status.type,
      elapsedMs: status.elapsedMs,
      ...(status.result !== undefined && { result: status.result }),
      ...(status.error !== undefined && { error: status.error }),
    })
    return { ...status }
  }

  private async run(request: RunnableAction, ctx: ActionContext): Promise<Record<string, unknown>> {
    switch (request.type) {
      case 'goto':
        return goto(ctx, new Vec3(request.x, request.y, request.z), request.range ?? 1)
      case 'follow':
        return follow(ctx, request.player, request.range ?? 3)
      case 'collect':
        return collect(ctx, request.block, request.count ?? 1, request.radius ?? 32)
      case 'dig':
        return dig(ctx, new Vec3(request.x, request.y, request.z))
      case 'place':
        return place(ctx, request.item, new Vec3(request.x, request.y, request.z))
      case 'chat':
        ctx.bot.chat(request.message)
        return { sent: request.message }
      case 'eval':
        return runCode(ctx, request.code, request.timeoutMs ?? 60_000)
      default: {
        const never: never = request
        throw new Error(`unknown action ${JSON.stringify(never)}`)
      }
    }
  }
}

function describe(params: Record<string, unknown>): string {
  return Object.entries(params)
    .map(([k, v]) => {
      const s = typeof v === 'string' ? v.replace(/\s+/g, ' ') : String(v)
      return `${k}=${s.length > 40 ? s.slice(0, 40) + '…' : s}`
    })
    .join(' ')
}

/** Resolves when the goal is reached; rejects on abort or when no path exists. */
export async function gotoGoal(ctx: ActionContext, goal: Goal): Promise<void> {
  const { bot, signal } = ctx
  signal.throwIfAborted()
  const onAbort = () => bot.pathfinder.setGoal(null)
  signal.addEventListener('abort', onAbort, { once: true })
  try {
    await bot.pathfinder.goto(goal)
  } catch (err) {
    signal.throwIfAborted()
    throw err
  } finally {
    signal.removeEventListener('abort', onAbort)
  }
}

/** Reports distance to a moving or fixed target and when the bot last moved, until the returned stop is called. */
function trackMovement(ctx: ActionContext, target: () => Vec3 | null): () => void {
  let anchor = ctx.bot.entity.position.clone()
  ctx.update({ lastMovedAt: Date.now() })
  const tick = () => {
    const pos = ctx.bot.entity.position
    const t = target()
    if (pos.distanceTo(anchor) >= 0.5) {
      anchor = pos.clone()
      ctx.update({ lastMovedAt: Date.now() })
    }
    if (t) ctx.update({ distanceRemaining: round1(pos.distanceTo(t)) })
  }
  tick()
  const timer = setInterval(tick, PROGRESS_INTERVAL_MS)
  return () => {
    tick()
    clearInterval(timer)
  }
}

async function goto(ctx: ActionContext, target: Vec3, range: number): Promise<Record<string, unknown>> {
  ctx.update({ progress: `walking to ${target.x},${target.y},${target.z}` })
  const stop = trackMovement(ctx, () => target)
  try {
    await gotoGoal(ctx, new goals.GoalNear(target.x, target.y, target.z, range))
  } finally {
    stop()
  }
  return { arrived: vec(ctx.bot.entity.position) }
}

async function follow(ctx: ActionContext, name: string, range: number): Promise<Record<string, unknown>> {
  const { bot, signal } = ctx
  const entity = () => bot.players[name]?.entity ?? null
  const first = entity()
  if (!first) throw new Error(`player ${name} is not in view`)
  ctx.update({ progress: `following ${name}` })
  bot.pathfinder.setGoal(new goals.GoalFollow(first, range), true)
  const stop = trackMovement(ctx, () => entity()?.position ?? null)
  try {
    // Runs until aborted, or fails when the player leaves view.
    await new Promise<void>((_, reject) => {
      const check = setInterval(() => {
        if (!entity()) {
          clearInterval(check)
          reject(new Error(`lost sight of ${name}`))
        }
      }, PROGRESS_INTERVAL_MS)
      signal.addEventListener('abort', () => {
        clearInterval(check)
        reject(signal.reason)
      }, { once: true })
    })
  } finally {
    stop()
    // When aborted, the runner has already halted and the next action may own
    // the pathfinder; clearing the goal now would cancel its goal.
    if (!signal.aborted) bot.pathfinder.setGoal(null)
  }
  return {}
}

async function collect(ctx: ActionContext, blockName: string, count: number, radius: number): Promise<Record<string, unknown>> {
  const { bot, signal } = ctx
  const blockType = bot.registry.blocksByName[blockName]
  if (!blockType) throw new Error(`unknown block ${blockName}`)
  const dimension = bot.game.dimension
  const positions = bot
    .findBlocks({ matching: blockType.id, maxDistance: radius, count: count * 4 })
    .filter((p) => !ctx.safety.regionAt(p, dimension))
    .slice(0, count)
  if (positions.length === 0) throw new Error(`no ${blockName} outside protected regions within ${radius} blocks`)
  const blocks = positions.map((p) => bot.blockAt(p)).filter((b): b is Block => b !== null)

  // Count targets that are no longer the block type: diggingCompleted reports
  // the new (air) block, and mineflayer clears its listeners on death.
  const brokenCount = () => blocks.filter((b) => bot.blockAt(b.position)?.type !== blockType.id).length
  ctx.update({ progress: `collecting ${blocks.length} ${blockName}`, collected: 0 })
  const timer = setInterval(() => ctx.update({ collected: brokenCount() }), PROGRESS_INTERVAL_MS)
  // cancelTask alone leaves collectblock's loop running: after the current
  // step it would path to the next target and steal the next action's goal.
  // Emptying its (private) target list ends the loop.
  const onAbort = () => {
    ;(bot.collectBlock as unknown as { targets: { clear(): void } }).targets.clear()
    void bot.collectBlock.cancelTask()
  }
  signal.addEventListener('abort', onAbort, { once: true })
  try {
    await bot.collectBlock.collect(blocks, { ignoreNoPath: true })
  } catch (err) {
    signal.throwIfAborted()
    throw err
  } finally {
    clearInterval(timer)
    signal.removeEventListener('abort', onAbort)
  }
  const collected = brokenCount()
  ctx.update({ collected })
  if (collected === 0) throw new Error(`could not collect any of ${blocks.length} ${blockName} found`)
  return { collected, block: blockName }
}

async function dig(ctx: ActionContext, pos: Vec3): Promise<Record<string, unknown>> {
  const { bot, signal } = ctx
  ctx.safety.checkBreak(pos, bot.game.dimension)
  const block = bot.blockAt(pos)
  if (!block || block.boundingBox === 'empty') throw new Error(`no solid block at ${pos.x},${pos.y},${pos.z}`)
  if (!bot.canDigBlock(block)) {
    ctx.update({ progress: `walking to ${block.name}` })
    await gotoGoal(ctx, new goals.GoalLookAtBlock(pos, bot.world, { reach: 4 }))
  }
  const tool = bot.pathfinder.bestHarvestTool(block)
  if (tool) await bot.equip(tool, 'hand')
  signal.throwIfAborted()
  ctx.update({ progress: `digging ${block.name}` })
  const onAbort = () => bot.stopDigging()
  signal.addEventListener('abort', onAbort, { once: true })
  try {
    await bot.dig(block)
  } catch (err) {
    signal.throwIfAborted()
    throw err
  } finally {
    signal.removeEventListener('abort', onAbort)
  }
  return { dug: block.name, at: vec(pos) }
}

const FACES = [new Vec3(0, -1, 0), new Vec3(0, 1, 0), new Vec3(0, 0, -1), new Vec3(0, 0, 1), new Vec3(-1, 0, 0), new Vec3(1, 0, 0)]

async function place(ctx: ActionContext, itemName: string, pos: Vec3): Promise<Record<string, unknown>> {
  const { bot, signal } = ctx
  ctx.safety.checkPlace(pos, bot.game.dimension)
  const item = bot.inventory.items().find((i) => i.name === itemName)
  if (!item) throw new Error(`no ${itemName} in inventory`)
  const target = bot.blockAt(pos)
  if (!target) throw new Error(`${pos.x},${pos.y},${pos.z} is not loaded`)
  if (target.boundingBox !== 'empty') throw new Error(`${pos.x},${pos.y},${pos.z} is occupied by ${target.name}`)
  // Click the face of a solid neighbour that points at the target.
  const face = FACES.find((f) => bot.blockAt(pos.plus(f))?.boundingBox === 'block')
  if (!face) throw new Error(`nothing solid next to ${pos.x},${pos.y},${pos.z} to place against`)
  const reference = bot.blockAt(pos.plus(face))!
  ctx.update({ progress: `walking to place ${itemName}` })
  await gotoGoal(ctx, new goals.GoalPlaceBlock(pos, bot.world, { range: 4 } as GoalPlaceBlockOptions))
  await bot.equip(item, 'hand')
  signal.throwIfAborted()
  ctx.update({ progress: `placing ${itemName}` })
  await bot.placeBlock(reference, face.scaled(-1))
  return { placed: itemName, at: vec(pos) }
}

function vec(v: { x: number; y: number; z: number }) {
  return { x: round1(v.x), y: round1(v.y), z: round1(v.z) }
}

function round1(n: number): number {
  return Math.round(n * 10) / 10
}
