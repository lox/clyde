import type { Bot } from 'mineflayer'
import type { Movements } from 'mineflayer-pathfinder'
import type { Vec } from './api/state.js'
import type { EventLog } from './events.js'

export interface Region {
  name: string
  /** Inclusive block coordinates; min <= max on every axis. */
  min: Vec
  max: Vec
  /** Applies to every dimension when absent. */
  dimension?: string
}

export class SafetyError extends Error {
  constructor(message: string) {
    super(`refused: ${message}`)
    this.name = 'SafetyError'
  }
}

/** Pathfinder treats an exclusion weight of 100 or more as impassable. */
const EXCLUDED = 1000
const WINDOW_MS = 60_000

/**
 * Hard limits that hold no matter which code path asks: actions, eval code,
 * pathfinder digging and scaffolding, and collectblock all go through the
 * bot methods that install() wraps. This is not a sandbox; eval code that
 * writes raw packets can bypass it.
 */
export class Safety {
  private breaks: number[] = []

  constructor(
    private readonly opts: { maxBreaksPerMin: number; regions: Region[] },
    private readonly events: EventLog,
    private readonly now: () => number = Date.now,
  ) {}

  breaksLastMinute(): number {
    const cutoff = this.now() - WINDOW_MS
    while (this.breaks.length && this.breaks[0] <= cutoff) this.breaks.shift()
    return this.breaks.length
  }

  regionAt(pos: Vec, dimension: string): Region | undefined {
    const x = Math.floor(pos.x)
    const y = Math.floor(pos.y)
    const z = Math.floor(pos.z)
    const dim = stripNamespace(dimension)
    return this.opts.regions.find(
      (r) =>
        (r.dimension === undefined || stripNamespace(r.dimension) === dim) &&
        x >= r.min.x && x <= r.max.x &&
        y >= r.min.y && y <= r.max.y &&
        z >= r.min.z && z <= r.max.z,
    )
  }

  /** Throws SafetyError if breaking the block at pos is not allowed right now. */
  checkBreak(pos: Vec, dimension: string): void {
    const region = this.regionAt(pos, dimension)
    if (region) this.refuse(`break at ${fmt(pos)} is inside protected region ${region.name}`)
    if (this.breaksLastMinute() >= this.opts.maxBreaksPerMin) {
      this.refuse(`break cap of ${this.opts.maxBreaksPerMin} blocks per minute reached`)
    }
  }

  checkPlace(pos: Vec, dimension: string): void {
    const region = this.regionAt(pos, dimension)
    if (region) this.refuse(`place at ${fmt(pos)} is inside protected region ${region.name}`)
  }

  checkAttack(target: { type?: string; username?: string; name?: string }): void {
    if (target.type === 'player') this.refuse(`attacking players is never allowed (${target.username ?? 'player'})`)
  }

  /** Wraps bot.dig, bot._genericPlace (placeBlock, placeEntity), and bot.attack. Call once plugins are injected. */
  install(bot: Bot): void {
    const dim = () => bot.game?.dimension ?? 'overworld'
    const dig = bot.dig.bind(bot)
    bot.dig = (async (block: Parameters<Bot['dig']>[0], ...rest: unknown[]) => {
      this.checkBreak(block.position, dim())
      await (dig as (...args: unknown[]) => Promise<void>)(block, ...rest)
      this.breaks.push(this.now())
    }) as Bot['dig']

    const internal = bot as unknown as {
      _genericPlace: (ref: { position: Vec & { plus(v: Vec): Vec } }, face: Vec, options: unknown) => Promise<unknown>
    }
    const place = internal._genericPlace.bind(bot)
    internal._genericPlace = async (ref, face, options) => {
      this.checkPlace(ref.position.plus(face), dim())
      return place(ref, face, options)
    }

    const attack = bot.attack.bind(bot)
    bot.attack = ((target: Parameters<Bot['attack']>[0], ...rest: unknown[]) => {
      this.checkAttack(target)
      return (attack as (...args: unknown[]) => void)(target, ...rest)
    }) as Bot['attack']
  }

  /** Makes the pathfinder plan around protected regions and stop digging at the break cap. */
  guardMovements(movements: Movements, dimension: () => string): void {
    movements.exclusionAreasBreak.push((block) =>
      this.regionAt(block.position, dimension()) || this.breaksLastMinute() >= this.opts.maxBreaksPerMin ? EXCLUDED : 0,
    )
    movements.exclusionAreasPlace.push((block) => (this.regionAt(block.position, dimension()) ? EXCLUDED : 0))
  }

  private refuse(why: string): never {
    this.events.push('safety.refused', why)
    throw new SafetyError(why)
  }
}

function stripNamespace(dimension: string): string {
  return dimension.replace(/^minecraft:/, '')
}

function fmt(p: Vec): string {
  return `${Math.floor(p.x)},${Math.floor(p.y)},${Math.floor(p.z)}`
}
