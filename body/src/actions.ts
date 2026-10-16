import type { Bot } from 'mineflayer'
import { goals } from 'mineflayer-pathfinder'
import { Vec3 } from 'vec3'
import { EventLog } from './events.js'
import { runCode } from './eval.js'

export type ActionRequest =
  | { type: 'goto'; x: number; y: number; z: number; range?: number }
  | { type: 'follow'; player: string; range?: number }
  | { type: 'collect'; block: string; count?: number; radius?: number }
  | { type: 'dig'; x: number; y: number; z: number }
  | { type: 'chat'; message: string }
  | { type: 'eval'; code: string; timeoutMs?: number }

export type ActionState = 'running' | 'succeeded' | 'failed' | 'aborted'

export interface ActionStatus {
  id: number
  type: ActionRequest['type']
  params: Record<string, unknown>
  status: ActionState
  startedAt: number
  endedAt?: number
  elapsedMs: number
  /** Short, human-readable description of what the action is doing right now. */
  progress?: string
  result?: unknown
  error?: string
}

export interface ActionContext {
  bot: Bot
  signal: AbortSignal
  progress: (text: string) => void
  log: (text: string) => void
}

class AbortedError extends Error {
  constructor(reason: string) {
    super(`aborted: ${reason}`)
    this.name = 'AbortedError'
  }
}

/**
 * Runs at most one action at a time. Starting a new action aborts the current
 * one; that is the interrupt channel for the harness.
 */
export class ActionRunner {
  private nextId = 1
  private current: { status: ActionStatus; controller: AbortController } | null = null
  private last: ActionStatus | null = null

  constructor(
    private readonly bot: Bot,
    private readonly events: EventLog,
  ) {}

  get currentStatus(): ActionStatus | null {
    if (!this.current) return null
    return { ...this.current.status, elapsedMs: Date.now() - this.current.status.startedAt }
  }

  get lastStatus(): ActionStatus | null {
    return this.last
  }

  async stop(reason = 'stop requested'): Promise<void> {
    if (!this.current) return
    this.current.controller.abort(new AbortedError(reason))
    // Give the action a tick to observe the abort and tear down.
    await new Promise((r) => setTimeout(r, 0))
  }

  start(request: ActionRequest): ActionStatus {
    void this.stop('superseded by new action')
    const { type, ...params } = request
    const controller = new AbortController()
    const status: ActionStatus = {
      id: this.nextId++,
      type,
      params,
      status: 'running',
      startedAt: Date.now(),
      elapsedMs: 0,
    }
    this.current = { status, controller }
    this.events.push('action.start', `${type} ${describe(params)}`, { id: status.id })

    const ctx: ActionContext = {
      bot: this.bot,
      signal: controller.signal,
      progress: (text) => {
        status.progress = text
      },
      log: (text) => this.events.push('action.log', text, { id: status.id }),
    }

    this.run(request, ctx).then(
      (result) => this.finish(status, 'succeeded', { result }),
      (err: unknown) => {
        if (controller.signal.aborted) {
          const reason = controller.signal.reason
          this.finish(status, 'aborted', { error: reason instanceof Error ? reason.message : String(reason) })
        } else {
          this.finish(status, 'failed', { error: err instanceof Error ? err.message : String(err) })
        }
      },
    )
    return { ...status }
  }

  private finish(status: ActionStatus, state: ActionState, extra: { result?: unknown; error?: string }) {
    if (status.status !== 'running') return
    status.status = state
    status.endedAt = Date.now()
    status.elapsedMs = status.endedAt - status.startedAt
    if (extra.result !== undefined) status.result = extra.result
    if (extra.error !== undefined) status.error = extra.error
    if (this.current?.status.id === status.id) this.current = null
    this.last = status
    this.events.push(`action.${state}`, `${status.type} ${state}${status.error ? `: ${status.error}` : ''}`, {
      id: status.id,
      elapsedMs: status.elapsedMs,
    })
  }

  private async run(request: ActionRequest, ctx: ActionContext): Promise<unknown> {
    // Every action starts from a clean slate so a superseded action's
    // pathfinder goal or dig does not leak into the new one.
    this.bot.pathfinder.stop()
    if (this.bot.targetDigBlock) this.bot.stopDigging()
    this.bot.clearControlStates()
    switch (request.type) {
      case 'goto':
        return goto(ctx, new Vec3(request.x, request.y, request.z), request.range ?? 1)
      case 'follow':
        return follow(ctx, request.player, request.range ?? 3)
      case 'collect':
        return collect(ctx, request.block, request.count ?? 1, request.radius ?? 32)
      case 'dig':
        return dig(ctx, new Vec3(request.x, request.y, request.z))
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
    .map(([k, v]) => `${k}=${typeof v === 'string' && v.length > 40 ? v.slice(0, 40) + '…' : String(v)}`)
    .join(' ')
}

/** Resolves when the goal is reached; rejects on abort or when no path exists. */
export async function gotoGoal(ctx: ActionContext, goal: goals.Goal): Promise<void> {
  const { bot, signal } = ctx
  signal.throwIfAborted()
  const onAbort = () => bot.pathfinder.stop()
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

async function goto(ctx: ActionContext, target: Vec3, range: number): Promise<unknown> {
  const update = () => ctx.progress(`moving, ${ctx.bot.entity.position.distanceTo(target).toFixed(1)}m to go`)
  update()
  const timer = setInterval(update, 500)
  try {
    await gotoGoal(ctx, new goals.GoalNear(target.x, target.y, target.z, range))
    return { arrived: ctx.bot.entity.position.floored() }
  } finally {
    clearInterval(timer)
  }
}

async function follow(ctx: ActionContext, name: string, range: number): Promise<unknown> {
  const { bot, signal } = ctx
  const target = bot.players[name]?.entity
  if (!target) throw new Error(`player ${name} is not visible`)
  ctx.progress(`following ${name}`)
  bot.pathfinder.setGoal(new goals.GoalFollow(target, range), true)
  await new Promise<void>((resolve) => {
    signal.addEventListener('abort', () => resolve(), { once: true })
  })
  bot.pathfinder.stop()
  signal.throwIfAborted()
  return {}
}

async function collect(ctx: ActionContext, blockName: string, count: number, radius: number): Promise<unknown> {
  const { bot, signal } = ctx
  const blockType = bot.registry.blocksByName[blockName]
  if (!blockType) throw new Error(`unknown block ${blockName}`)
  const positions = bot.findBlocks({ matching: blockType.id, maxDistance: radius, count })
  if (positions.length === 0) throw new Error(`no ${blockName} within ${radius} blocks`)
  const blocks = positions.map((p) => bot.blockAt(p)).filter((b) => b !== null)
  ctx.progress(`collecting ${blocks.length} ${blockName}`)
  const onAbort = () => void bot.collectBlock.cancelTask()
  signal.addEventListener('abort', onAbort, { once: true })
  try {
    await bot.collectBlock.collect(blocks, { ignoreNoPath: true })
  } catch (err) {
    signal.throwIfAborted()
    throw err
  } finally {
    signal.removeEventListener('abort', onAbort)
  }
  return { collected: blocks.length, block: blockName }
}

async function dig(ctx: ActionContext, pos: Vec3): Promise<unknown> {
  const { bot, signal } = ctx
  const block = bot.blockAt(pos)
  if (!block || block.name === 'air') throw new Error(`no block at ${pos}`)
  if (!bot.canDigBlock(block)) {
    ctx.progress(`walking to ${block.name}`)
    await gotoGoal(ctx, new goals.GoalLookAtBlock(pos, bot.world, { reach: 4 }))
  }
  const tool = bot.pathfinder.bestHarvestTool(block)
  if (tool) await bot.equip(tool, 'hand')
  ctx.progress(`digging ${block.name}`)
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
  return { dug: block.name, at: pos }
}
