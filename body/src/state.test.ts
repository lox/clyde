import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { Ajv } from 'ajv'
import type { Bot } from 'mineflayer'
import { Vec3 } from 'vec3'
import type { ActionStatus } from './api/state.js'
import { buildStateCard, renderCard } from './state.js'

const NOW = 1_800_000_000_000

/** A bot at (0.5, 64, 0.5) at dusk in the rain, with a player, a zombie, a dropped sapling, and some logs and coal nearby. */
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
  const blocks: Record<string, string> = { '3,64,0': 'oak_log', '3,65,0': 'oak_log', '-5,60,2': 'coal_ore' }
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
      blocksByName: { oak_log: { id: 1 }, coal_ore: { id: 2 } },
      entitiesByName: { player: { metadataKeys: ['shared_flags', 'air_supply'] } },
    },
    findBlocks: () => [new Vec3(3, 64, 0), new Vec3(3, 65, 0), new Vec3(-5, 60, 2)],
    blockAt: (p: Vec3) => ({ name: blocks[`${p.x},${p.y},${p.z}`] }),
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
  assert.deepEqual(card.inventory.items, [
    { name: 'dirt', count: 12 },
    { name: 'oak_log', count: 5 },
  ])
})

test('renderCard renders the fixture card', () => {
  assert.equal(
    renderCard(fixtureCard()),
    [
      'bot Clyde | overworld | pos (0.5, 64, 0.5) | health 18.5/20 | food 17/20 | xp 2 | survival',
      'world: dusk (t=12500), raining',
      'action: goto x=10 y=64 z=-20 range=1 [running, 4.2s] walking to 10,64,-20 | 12.3m to go | last moved 0.8s ago',
      'last action: dig x=3 y=64 z=0 [failed, 1.0s] error: refused: break at 3,64,0 is inside protected region spawn',
      'inventory (3/36 slots, holding dirt): dirt x12, oak_log x5',
      'players: Bonnie 10m S +1y hp20',
      'hostiles: zombie 5m NE hp20',
      'dropped items: oak_sapling 0.7m SE',
      'blocks nearby: oak_log x2 (nearest 2.5m at 3,64,0), coal_ore (nearest 7m at -5,60,2)',
      'recent events:',
      '  -2.5s chat: <Bonnie> hi clyde',
      '  -1.0s action.start: #4 goto x=10 y=64 z=-20 range=1',
    ].join('\n'),
  )
})
