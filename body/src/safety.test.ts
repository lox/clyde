import { test } from 'node:test'
import assert from 'node:assert/strict'
import type { Bot } from 'mineflayer'
import type { Movements } from 'mineflayer-pathfinder'
import { Vec3 } from 'vec3'
import { parseRegions } from './config.js'
import { EventLog } from './events.js'
import { Safety, SafetyError } from './safety.js'

const regions = parseRegions('[{"name":"spawn","min":[10,70,10],"max":[0,60,0]},{"name":"portal","min":[0,0,0],"max":[5,5,5],"dimension":"the_nether"}]')

/** A bot whose dig/place/attack succeed and record calls, with the safety wrappers installed. */
function setup(maxBreaksPerMin = 60) {
  let now = 1_000_000
  const calls: string[] = []
  const bot = {
    game: { dimension: 'overworld' },
    dig: async (block: { position: Vec3 }) => {
      if (block.position.x === 666) throw new Error('dig interrupted')
      calls.push(`dig ${block.position}`)
    },
    _genericPlace: async (ref: { position: Vec3 }, face: Vec3) => {
      calls.push(`place ${ref.position.plus(face)}`)
    },
    attack: (entity: { name?: string }) => {
      calls.push(`attack ${entity.name}`)
    },
  }
  const events = new EventLog()
  const safety = new Safety({ maxBreaksPerMin, regions }, events, () => now)
  safety.install(bot as unknown as Bot)
  const dig = (x: number, y: number, z: number) => bot.dig({ position: new Vec3(x, y, z) })
  const place = (ref: Vec3, face: Vec3) => bot._genericPlace({ position: ref }, face)
  const refusals = () => events.since(0).events.filter((e) => e.type === 'safety.refused').map((e) => e.text)
  return { bot, safety, calls, dig, place, refusals, advance: (ms: number) => (now += ms) }
}

test('break cap refuses the break that would exceed it and recovers after a minute', async () => {
  const { dig, calls, refusals, advance } = setup(3)
  await dig(100, 64, 0)
  advance(20_000)
  await dig(101, 64, 0)
  await dig(102, 64, 0)
  await assert.rejects(dig(103, 64, 0), (err: Error) => err instanceof SafetyError && /3 blocks per minute/.test(err.message))
  assert.equal(calls.length, 3, 'the refused dig must not reach the server')
  assert.equal(refusals().length, 1)

  // The first break leaves the window 60 s after it happened; the other two are still inside.
  advance(40_000)
  await dig(103, 64, 0)
  await assert.rejects(dig(104, 64, 0), SafetyError)
})

test('failed digs do not use up the break budget', async () => {
  const { dig } = setup(1)
  await assert.rejects(dig(666, 64, 0), /dig interrupted/)
  await dig(100, 64, 0)
})

test('protected regions refuse dig and place inside inclusive bounds, in their dimension only', async () => {
  const { bot, dig, place, calls } = setup()
  // Bounds were given max-first; parseRegions normalises them.
  await assert.rejects(dig(10, 70, 10), /inside protected region spawn/)
  await assert.rejects(dig(0, 60, 0), /spawn/)
  // Fractional coordinates are floored to the block.
  await assert.rejects(dig(10.9, 65.5, 0.2), /spawn/)
  // Placing on top of (5,59,5) targets (5,60,5), inside the region, though the reference block is outside.
  await assert.rejects(place(new Vec3(5, 59, 5), new Vec3(0, 1, 0)), /place at 5,60,5 is inside protected region spawn/)
  assert.deepEqual(calls, [])

  await dig(11, 65, 5)
  await dig(-0.5, 65, 5) // floors to -1
  await place(new Vec3(5, 70, 5), new Vec3(0, 1, 0)) // targets y=71, above the region
  await dig(2, 2, 2) // portal region is nether-only
  bot.game.dimension = 'minecraft:the_nether'
  await assert.rejects(dig(2, 2, 2), /portal/)
  assert.equal(calls.length, 4)
})

test('players are never attacked; mobs are', () => {
  const { bot, calls, refusals } = setup()
  assert.throws(() => bot.attack({ type: 'player', username: 'Bonnie', name: 'player' } as never), /never allowed \(Bonnie\)/)
  bot.attack({ type: 'hostile', name: 'zombie' } as never)
  assert.deepEqual(calls, ['attack zombie'])
  assert.equal(refusals().length, 1)
})

test('guarded movements make the pathfinder treat protected and over-cap blocks as unbreakable', async () => {
  const { safety, dig } = setup(1)
  const movements = { exclusionAreasBreak: [], exclusionAreasPlace: [] } as unknown as Movements
  safety.guardMovements(movements, () => 'overworld')
  const breakCost = (x: number, y: number, z: number) =>
    movements.exclusionAreasBreak.reduce((sum, f) => sum + f({ position: new Vec3(x, y, z) } as never), 0)
  const placeCost = (x: number, y: number, z: number) =>
    movements.exclusionAreasPlace.reduce((sum, f) => sum + f({ position: new Vec3(x, y, z) } as never), 0)

  // mineflayer-pathfinder refuses to break at a weight of 100 or more.
  assert.ok(breakCost(5, 65, 5) >= 100)
  assert.ok(placeCost(5, 65, 5) >= 100)
  assert.equal(breakCost(50, 65, 5), 0)
  assert.equal(placeCost(50, 65, 5), 0)
  await dig(50, 65, 5)
  assert.ok(breakCost(50, 65, 6) >= 100, 'at the cap, no block is breakable')
  assert.equal(placeCost(50, 65, 6), 0)
})
