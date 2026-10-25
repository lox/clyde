import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { Ajv } from 'ajv'
import type { Bot } from 'mineflayer'
import { Vec3 } from 'vec3'
import type { ActionStatus } from './api/state.js'
import { buildStateCard, renderCard } from './state.js'

const NOW = 1_800_000_000_000

/** A bot at (0.5, 64, 0.5) at dusk in the rain, with a player, a zombie, a dropped sapling, and logs and ores nearby. */
function fixtureBot(): Bot {
  const me = {
    position: new Vec3(0.5, 64, 0.5),
    yaw: 1.234,
    pitch: -0.05,
    onGround: true,
    isInWater: false,
    isInLava: false,
    metadata: [0, 150], // air_supply 150 of 300 ticks
  }
  // Solid stone at y <= 55, air above, plus these blocks. The diamond is buried
  // in stone; the iron touches a one-block air pocket at (-3,51,4). Of the two
  // water blocks only the top one has a visible face.
  const blocks: Record<string, string> = {
    '3,64,0': 'oak_log',
    '3,65,0': 'oak_log',
    '-5,60,2': 'coal_ore',
    '-2,50,0': 'diamond_ore',
    '-3,50,4': 'iron_ore',
    '-3,51,4': 'air',
    '10,55,0': 'water',
    '10,54,0': 'water',
  }
  const blockAt = (p: Vec3) => {
    const name = blocks[`${p.x},${p.y},${p.z}`] ?? (p.y <= 55 ? 'stone' : 'air')
    const empty = name === 'air' || name === 'water'
    return { name, type: name, position: p, boundingBox: empty ? 'empty' : 'block', transparent: empty, shapes: empty ? [] : [[0, 0, 0, 1, 1, 1]] }
  }
  const candidates = [new Vec3(3, 64, 0), new Vec3(3, 65, 0), new Vec3(-5, 60, 2), new Vec3(-2, 50, 0), new Vec3(-3, 50, 4), new Vec3(10, 55, 0), new Vec3(10, 54, 0)]
  return {
    username: 'Clyde',
    entity: me,
    entities: {
      1: me,
      2: { type: 'player', username: 'Bonnie', position: new Vec3(0.5, 65, 10.5), health: 20 },
      3: { type: 'hostile', kind: 'Hostile mobs', name: 'zombie', position: new Vec3(4.5, 64, -2.5), health: 20 },
      4: { type: 'object', name: 'item', position: new Vec3(1, 64, 1), getDroppedItem: () => ({ name: 'oak_sapling' }) },
      5: { type: 'hostile', kind: 'Hostile mobs', name: 'creeper', position: new Vec3(40, 64, 0) }, // outside the scan radius
      6: { type: 'object', name: 'arrow', position: new Vec3(2, 64, 2) }, // not a group the card shows
    },
    registry: {
      blocksByName: { oak_log: { id: 1 }, coal_ore: { id: 2 }, iron_ore: { id: 3 }, diamond_ore: { id: 4 }, water: { id: 5 } },
      entitiesByName: { player: { metadataKeys: ['shared_flags', 'air_supply'] } },
    },
    // Like mineflayer, a useExtraInfo function filters the matching blocks.
    findBlocks: ({ useExtraInfo }: { useExtraInfo?: (block: unknown) => boolean }) =>
      candidates.filter((p) => !useExtraInfo || useExtraInfo(blockAt(p))),
    blockAt,
    inventory: {
      items: () => [
        { name: 'oak_log', count: 3 },
        { name: 'dirt', count: 12 },
        { name: 'oak_log', count: 2 },
      ],
      inventoryStart: 9,
      inventoryEnd: 45,
    },
    heldItem: { name: 'dirt' },
    game: { dimension: 'overworld', gameMode: 'survival' },
    health: 18.5,
    food: 17,
    oxygenLevel: 274, // mineflayer copies other entities' air here; the card must not use it
    experience: { level: 2 },
    time: { timeOfDay: 12500 },
    isRaining: true,
    thunderState: 0,
  } as unknown as Bot
}

const running: ActionStatus = {
  id: 4,
  token: 'running-owner',
  type: 'goto',
  params: { x: 10, y: 64, z: -20, range: 1 },
  status: 'running',
  startedAt: NOW - 4200,
  elapsedMs: 4200,
  progress: 'walking to 10,64,-20',
  distanceRemaining: 12.3,
  lastMovedAt: NOW - 800,
}

const failed: ActionStatus = {
  id: 3,
  token: 'failed-owner',
  type: 'dig',
  params: { x: 3, y: 64, z: 0 },
  status: 'failed',
  startedAt: NOW - 6000,
  endedAt: NOW - 5000,
  elapsedMs: 1000,
  error: 'refused: break at 3,64,0 is inside protected region spawn',
}

function fixtureCard() {
  return buildStateCard({
    bot: fixtureBot(),
    action: running,
    lastAction: failed,
    events: [
      { seq: 7, ts: NOW - 2500, type: 'chat', text: '<Bonnie> hi clyde' },
      { seq: 8, ts: NOW - 1000, type: 'action.start', text: '#4 goto x=10 y=64 z=-20 range=1' },
    ],
    scanRadius: 24,
    now: NOW,
  })
}

test('the state card built from a bot validates against api/state.schema.json', () => {
  const schema = JSON.parse(readFileSync(new URL('../../api/state.schema.json', import.meta.url), 'utf8'))
  const validate = new Ajv({ allErrors: true }).compile(schema)
  const card = fixtureCard()
  // Round-trip through JSON: that is what GET /state sends.
  const ok = validate(JSON.parse(JSON.stringify(card)))
  assert.ok(ok, JSON.stringify(validate.errors, null, 2))
  assert.equal(card.bot.oxygen, 10, 'oxygen comes from the bot\'s own air_supply metadata')
  assert.deepEqual(
    card.blocks.map((b) => b.name),
    ['oak_log', 'coal_ore', 'water', 'iron_ore'],
    'the buried diamond_ore is not on the card; the iron_ore beside an air pocket is',
  )
  assert.equal(card.blocks.find((b) => b.name === 'water')?.count, 1, 'water under water is not a visible face')
  assert.deepEqual(card.inventory.items, [
    { name: 'dirt', count: 12 },
    { name: 'oak_log', count: 5 },
  ])
})

test('renderCard renders the fixture card', () => {
  const card = fixtureCard()
  delete card.collision // Checked separately below, including partial and unknown shapes.
  assert.equal(
    renderCard(card),
    [
      'bot Clyde | overworld | pos (0.5, 64, 0.5) | health 18.5/20 | food 17/20 | xp 2 | survival',
      'world: dusk (t=12500), raining',
      'action: goto x=10 y=64 z=-20 range=1 [running, 4.2s] owner=running-owner walking to 10,64,-20 | 12.3m to go | last moved 0.8s ago',
      'last action: dig x=3 y=64 z=0 [failed, 1.0s] owner=failed-owner error: refused: break at 3,64,0 is inside protected region spawn',
      'inventory (3/36 slots, holding dirt): dirt x12, oak_log x5',
      'players: Bonnie 10m S +1y hp20',
      'hostiles: zombie 5m NE hp20',
      'dropped items: oak_sapling 0.7m SE',
      'blocks nearby: oak_log x2 (nearest 2.5m at 3,64,0), coal_ore (nearest 7m at -5,60,2), water (nearest 13.1m at 10,55,0), iron_ore (nearest 14.8m at -3,50,4)',
      'recent events:',
      '  -2.5s chat: <Bonnie> hi clyde',
      '  -1.0s action.start: #4 goto x=10 y=64 z=-20 range=1',
    ].join('\n'),
  )
})

test('local collision distinguishes air, canopy, partial shapes and unknown without a volume scan', () => {
  const bot = fixtureBot()
  const at = bot.blockAt.bind(bot)
  bot.blockAt = ((p: Vec3) => {
    if (p.equals(new Vec3(0, 66, 0))) return { ...at(p), name: 'birch_leaves', shapes: [[0, 0, 0, 1, 1, 1]] }
    if (p.equals(new Vec3(1, 64, 0))) return { ...at(p), name: 'stone_slab', shapes: [[0, 0, 0, 1, 0.5, 1]] }
    if (p.equals(new Vec3(0, 65, 1))) return { ...at(p), shapes: Array.from({ length: 17 }, () => [0, 0, 0, 1, 1, 1]) }
    if (p.equals(new Vec3(-1, 65, 0))) return null
    return at(p)
  }) as Bot['blockAt']
  const card = buildStateCard({ bot, action: null, lastAction: failed, events: [], scanRadius: 24 })
  const cells = card.collision!
  assert.equal(cells.length, 12)
  assert.deepEqual(cells.find((c) => c.position.x === 1 && c.position.y === 64)?.shapes, [[0, 0, 0, 1, 0.5, 1]])
  assert.deepEqual(cells.find((c) => c.position.x === 0 && c.position.y === 64)?.shapes, [])
  assert.equal(cells.find((c) => c.position.x === -1 && c.position.y === 65)?.name, null)
  assert.equal(cells.find((c) => c.position.x === -1 && c.position.y === 65)?.shapes, null)
  assert.equal(cells.find((c) => c.position.y === 66)?.name, 'birch_leaves')
  const truncated = cells.find((c) => c.position.z === 1 && c.position.y === 65)!
  assert.equal(truncated.shapes?.length, 16)
  assert.equal(truncated.truncated, true)
  assert.ok(cells.every((c) => Math.abs(c.position.x) + Math.abs(c.position.z) <= 1))
  assert.ok(!cells.some((c) => c.name === 'diamond_ore'))
  const text = renderCard(card)
  assert.match(text, /stone_slab \[\[0,0,0,1,0.5,1\]\]/)
  assert.match(text, /unknown unknown collision/)
  assert.match(text, /birch_leaves/)
})
