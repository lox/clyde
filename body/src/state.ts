import type { Bot } from 'mineflayer'
import type { Block } from 'prismarine-block'
import type { Entity } from 'prismarine-entity'
import { Vec3 } from 'vec3'
import type { ActionStatus, NearbyEntity, StateCard, Vec } from './api/state.js'
import type { BodyEvent } from './events.js'

/**
 * The state card is the one observation the rest of the system sees. The JSON
 * form (api/state.schema.json) is for code; the text form (renderCard) is
 * what judges and models read. Keep it compact, stable in layout, and free of
 * anything a model cannot act on.
 */
export type { NearbyEntity, StateCard, Vec }

/** Blocks worth noticing. Everything else (stone, dirt, grass, leaves) is noise. */
export const NOTABLE_BLOCKS = [
  'oak_log', 'birch_log', 'spruce_log', 'jungle_log', 'acacia_log', 'dark_oak_log', 'mangrove_log', 'cherry_log',
  'coal_ore', 'iron_ore', 'copper_ore', 'gold_ore', 'diamond_ore', 'emerald_ore', 'lapis_ore', 'redstone_ore',
  'deepslate_coal_ore', 'deepslate_iron_ore', 'deepslate_copper_ore', 'deepslate_gold_ore', 'deepslate_diamond_ore',
  'deepslate_emerald_ore', 'deepslate_lapis_ore', 'deepslate_redstone_ore',
  'crafting_table', 'furnace', 'chest', 'bed', 'red_bed', 'white_bed',
  'water', 'lava', 'obsidian', 'spawner', 'torch',
  'wheat', 'carrots', 'potatoes', 'sugar_cane', 'pumpkin', 'melon',
]

const MAX_ENTITIES_PER_GROUP = 8
/** How many recent events the card carries, and how old they may be. */
export const CARD_EVENT_LIMIT = 12
export const CARD_EVENT_WINDOW_MS = 30_000

export interface StateInputs {
  bot: Bot
  action: ActionStatus | null
  lastAction: ActionStatus | null
  /** Recent events, oldest first; see EventLog.recent. */
  events: BodyEvent[]
  scanRadius: number
  now?: number
}

export function buildStateCard({ bot, action, lastAction, events, scanRadius, now = Date.now() }: StateInputs): StateCard {
  const me = bot.entity
  const pos = me.position
  const timeOfDay = bot.time.timeOfDay

  const groups: StateCard['entities'] = { players: [], hostiles: [], passives: [], items: [] }
  for (const entity of Object.values(bot.entities)) {
    if (entity === me || !entity.position) continue
    const distance = entity.position.distanceTo(pos)
    if (distance > scanRadius) continue
    const group = classify(entity)
    if (!group) continue
    groups[group].push(describeEntity(entity, pos, distance))
  }
  for (const list of Object.values(groups)) {
    list.sort((a, b) => a.distance - b.distance)
    list.splice(MAX_ENTITIES_PER_GROUP)
  }

  const blockIds = NOTABLE_BLOCKS.map((n) => bot.registry.blocksByName[n]?.id).filter((id): id is number => id !== undefined)
  // Only blocks a player could see: at least one face against air or a
  // transparent block of another type. Filtering inside findBlocks keeps buried ores from
  // using up the count.
  const found = bot.findBlocks({
    matching: blockIds,
    maxDistance: scanRadius,
    count: 256,
    useExtraInfo: (block: Block) =>
      FACES.some((f) => {
        // Unloaded neighbours count as opaque. Water next to water is the
        // inside of a lake, not a visible face.
        const next = bot.blockAt(block.position.plus(f))
        return next !== null && next.type !== block.type && seeThrough(next)
      }),
  })
  const byName = new Map<string, StateCard['blocks'][number]>()
  for (const p of found) {
    const block = bot.blockAt(p)
    if (!block) continue
    const distance = p.distanceTo(pos)
    const entry = byName.get(block.name)
    if (!entry) {
      byName.set(block.name, { name: block.name, count: 1, nearest: round1(distance), nearestPos: vec(p) })
    } else {
      entry.count++
      if (distance < entry.nearest) {
        entry.nearest = round1(distance)
        entry.nearestPos = vec(p)
      }
    }
  }
  const blocks = [...byName.values()].sort((a, b) => a.nearest - b.nearest)

  const itemCounts = new Map<string, number>()
  for (const item of bot.inventory.items()) {
    itemCounts.set(item.name, (itemCounts.get(item.name) ?? 0) + item.count)
  }

  return {
    ts: now,
    bot: {
      name: bot.username,
      dimension: bot.game.dimension,
      position: vec(pos),
      yaw: round1(me.yaw),
      pitch: round1(me.pitch),
      health: bot.health,
      food: bot.food,
      oxygen: oxygenOf(bot),
      xpLevel: bot.experience.level,
      gameMode: bot.game.gameMode,
      onGround: me.onGround,
      // Set on the bot's own entity by prismarine-physics; not in the Entity typings.
      inWater: Boolean((me as Entity & { isInWater?: boolean }).isInWater),
      inLava: Boolean((me as Entity & { isInLava?: boolean }).isInLava),
    },
    world: {
      timeOfDay,
      phase: phaseOf(timeOfDay),
      raining: bot.isRaining,
      thundering: bot.thunderState > 0,
    },
    action,
    lastAction,
    inventory: {
      used: bot.inventory.items().length,
      total: bot.inventory.inventoryEnd - bot.inventory.inventoryStart,
      held: bot.heldItem?.name ?? null,
      items: [...itemCounts].map(([name, count]) => ({ name, count })).sort((a, b) => b.count - a.count),
    },
    entities: groups,
    blocks,
    events: events.map((e) => ({ ageMs: now - e.ts, type: e.type, text: e.text })),
  }
}

const FACES = [new Vec3(1, 0, 0), new Vec3(-1, 0, 0), new Vec3(0, 1, 0), new Vec3(0, -1, 0), new Vec3(0, 0, 1), new Vec3(0, 0, -1)]

/** Air, water, grass, glass, leaves, and the like. */
function seeThrough(block: Block): boolean {
  return block.boundingBox === 'empty' || block.transparent
}

/**
 * Air in half-hearts (0-20) from the bot's own air_supply metadata. Not
 * bot.oxygenLevel: mineflayer 4.39 sets that from any entity's air_supply,
 * so a nearby squid (4800 ticks) reports 320. Full until the server sends it.
 */
function oxygenOf(bot: Bot): number {
  const index = bot.registry.entitiesByName.player?.metadataKeys?.indexOf('air_supply') ?? 1
  const air = (bot.entity.metadata as unknown[] | undefined)?.[index]
  return typeof air === 'number' ? Math.max(0, Math.min(20, Math.round(air / 15))) : 20
}

function classify(entity: Entity): keyof StateCard['entities'] | null {
  if (entity.type === 'player') return 'players'
  if (entity.name === 'item') return 'items'
  if (entity.kind === 'Hostile mobs') return 'hostiles'
  if (entity.kind === 'Passive mobs') return 'passives'
  return null
}

function describeEntity(entity: Entity, from: Vec, distance: number): NearbyEntity {
  const name =
    entity.type === 'player'
      ? (entity.username ?? 'player')
      : entity.name === 'item'
        ? (entity.getDroppedItem()?.name ?? 'item')
        : (entity.name ?? 'unknown')
  const dx = entity.position.x - from.x
  const dz = entity.position.z - from.z
  return {
    name,
    distance: round1(distance),
    direction: compass(dx, dz),
    dy: Math.round(entity.position.y - from.y),
    position: vec(entity.position),
    health: entity.type === 'player' || entity.kind ? entity.health : undefined,
  }
}

/** Compass direction of (dx, dz) in Minecraft coordinates: -z is north, +x is east. */
export function compass(dx: number, dz: number): NearbyEntity['direction'] {
  if (Math.abs(dx) < 0.5 && Math.abs(dz) < 0.5) return 'here'
  const angle = (Math.atan2(dx, -dz) * 180) / Math.PI // 0 = north, 90 = east
  const dirs = ['N', 'NE', 'E', 'SE', 'S', 'SW', 'W', 'NW'] as const
  return dirs[Math.round(((angle + 360) % 360) / 45) % 8]
}

export function phaseOf(timeOfDay: number): StateCard['world']['phase'] {
  const t = ((timeOfDay % 24000) + 24000) % 24000
  if (t < 12000) return 'day'
  if (t < 13000) return 'dusk'
  if (t < 23000) return 'night'
  return 'dawn'
}

function vec(v: { x: number; y: number; z: number }): Vec {
  return { x: round1(v.x), y: round1(v.y), z: round1(v.z) }
}

function round1(n: number): number {
  return Math.round(n * 10) / 10
}

/** Text form of the card. This is what the judge and the executor read. */
export function renderCard(card: StateCard): string {
  const b = card.bot
  const lines: string[] = []
  lines.push(
    `bot ${b.name} | ${b.dimension} | pos (${b.position.x}, ${b.position.y}, ${b.position.z}) | health ${b.health}/20 | food ${b.food}/20` +
      ` | xp ${b.xpLevel} | ${b.gameMode}` +
      (b.inLava ? ' | IN LAVA' : '') +
      (b.inWater ? ` | in water (air ${b.oxygen}/20)` : '') +
      (b.onGround ? '' : ' | airborne'),
  )
  lines.push(
    `world: ${card.world.phase} (t=${card.world.timeOfDay})` +
      (card.world.thundering ? ', thunderstorm' : card.world.raining ? ', raining' : ''),
  )
  lines.push(`action: ${renderAction(card.action, card.ts)}`)
  if (card.lastAction && card.lastAction.id !== card.action?.id) {
    lines.push(`last action: ${renderAction(card.lastAction, card.ts)}`)
  }
  const inv = card.inventory
  lines.push(
    `inventory (${inv.used}/${inv.total} slots, holding ${inv.held ?? 'nothing'}): ` +
      (inv.items.length ? inv.items.map((i) => `${i.name} x${i.count}`).join(', ') : 'empty'),
  )
  for (const [label, list] of [
    ['players', card.entities.players],
    ['hostiles', card.entities.hostiles],
    ['passives', card.entities.passives],
    ['dropped items', card.entities.items],
  ] as const) {
    if (list.length === 0) continue
    lines.push(`${label}: ${list.map(renderEntity).join(', ')}`)
  }
  if (card.blocks.length) {
    lines.push(
      'blocks nearby: ' +
        card.blocks
          .map((bl) => `${bl.name}${bl.count > 1 ? ` x${bl.count}` : ''} (nearest ${bl.nearest}m at ${bl.nearestPos.x},${bl.nearestPos.y},${bl.nearestPos.z})`)
          .join(', '),
    )
  }
  if (card.events.length) {
    lines.push('recent events:')
    for (const e of card.events) {
      lines.push(`  -${(e.ageMs / 1000).toFixed(1)}s ${e.type}: ${e.text}`)
    }
  }
  return lines.join('\n')
}

function renderAction(a: ActionStatus | null, now: number): string {
  if (!a) return 'none'
  const params = Object.entries(a.params)
    .map(([k, v]) => `${k}=${typeof v === 'string' && v.length > 60 ? v.slice(0, 60) + '…' : String(v)}`)
    .join(' ')
  let s = `${a.type} ${params} [${a.status}, ${(a.elapsedMs / 1000).toFixed(1)}s]`
  if (a.status === 'running') {
    if (a.progress) s += ` ${a.progress}`
    if (a.distanceRemaining !== undefined) s += ` | ${a.distanceRemaining}m to go`
    if (a.lastMovedAt !== undefined) s += ` | last moved ${((now - a.lastMovedAt) / 1000).toFixed(1)}s ago`
    if (a.collected !== undefined) s += ` | collected ${a.collected}`
  }
  if (a.error) s += ` error: ${a.error}`
  return s
}

function renderEntity(e: NearbyEntity): string {
  let s = `${e.name} ${e.distance}m ${e.direction}`
  if (e.dy !== 0) s += ` ${e.dy > 0 ? '+' : ''}${e.dy}y`
  if (e.health !== undefined && e.health > 0) s += ` hp${Math.round(e.health)}`
  return s
}
