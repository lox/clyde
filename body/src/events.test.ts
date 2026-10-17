import { test } from 'node:test'
import assert from 'node:assert/strict'
import { EventLog } from './events.js'

test('since returns only events after the cursor, and latestSeq is the next cursor', () => {
  const log = new EventLog(10)
  for (const t of ['a', 'b', 'c']) log.push(t, t)

  const first = log.since(0)
  assert.deepEqual(first.events.map((e) => e.type), ['a', 'b', 'c'])
  assert.equal(first.latestSeq, 3)
  assert.equal(first.missed, 0)

  log.push('d', 'd')
  const next = log.since(first.latestSeq)
  assert.deepEqual(next.events.map((e) => [e.seq, e.type]), [[4, 'd']])
  assert.deepEqual(log.since(next.latestSeq), { events: [], latestSeq: 4, missed: 0 })
})

test('a cursor older than the buffer reports how many events it missed', () => {
  const log = new EventLog(3)
  for (let i = 1; i <= 7; i++) log.push('e', String(i))
  // Seqs 5, 6, 7 remain; a reader at 2 never saw 3 and 4.
  const page = log.since(2)
  assert.deepEqual(page.events.map((e) => e.seq), [5, 6, 7])
  assert.equal(page.missed, 2)
  assert.equal(log.since(4).missed, 0)
  assert.equal(log.since(0).missed, 4)
})

test('a cursor from an earlier process reads from the start', () => {
  const log = new EventLog(10)
  log.push('spawn', 'spawned')
  const page = log.since(500)
  assert.deepEqual(page.events.map((e) => e.seq), [1])
  assert.equal(page.latestSeq, 1)
})

test('recent returns the newest events inside the age window, oldest first', () => {
  let now = 0
  const log = new EventLog(10, () => now)
  for (const [ts, type] of [[1_000, 'old'], [20_000, 'a'], [25_000, 'b'], [29_000, 'c']] as const) {
    now = ts
    log.push(type, type)
  }
  assert.deepEqual(log.recent(2, 10_000, 30_000).map((e) => e.type), ['b', 'c'])
  assert.deepEqual(log.recent(10, 10_000, 30_000).map((e) => e.type), ['a', 'b', 'c'])
})
