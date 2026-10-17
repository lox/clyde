import { test } from 'node:test'
import assert from 'node:assert/strict'
import type { Bot } from 'mineflayer'
import { Vec3 } from 'vec3'
import { ActionRunner } from './actions.js'
import { EventLog } from './events.js'
import { Safety } from './safety.js'

/** Just enough bot for chat and eval actions, recording what halt() touches. */
function setup() {
  const calls: string[] = []
  let pendingGoto: ((err: Error) => void) | undefined
  const bot = {
    registry: {},
    entity: { position: new Vec3(0, 64, 0) },
    players: { Bonnie: { entity: { position: new Vec3(5, 64, 0) } } },
    targetDigBlock: { name: 'stone' },
    pathfinder: {
      // Like mineflayer-pathfinder, a goal change rejects a pending goto.
      setGoal: (goal: unknown) => {
        calls.push(`setGoal(${goal === null ? null : goal?.constructor.name})`)
        pendingGoto?.(new Error('GoalChanged'))
      },
      goto: (goal: object) => {
        calls.push(`goto(${goal.constructor.name})`)
        return new Promise((_, reject) => (pendingGoto = reject))
      },
    },
    stopDigging: () => calls.push('stopDigging'),
    clearControlStates: () => calls.push('clearControlStates'),
    chat: (message: string) => calls.push(`chat ${message}`),
  } as unknown as Bot
  const events = new EventLog()
  const runner = new ActionRunner(bot, events, new Safety({ maxBreaksPerMin: 60, regions: [] }, events))
  const types = () => events.since(0).events.map((e) => `${e.type} #${e.data?.id ?? ''}`)
  return { runner, events, calls, types }
}

function within<T>(ms: number, p: Promise<T>): Promise<T> {
  return Promise.race([p, new Promise<never>((_, reject) => setTimeout(() => reject(new Error(`not settled within ${ms}ms`)), ms))])
}

test('a new action aborts the running one before it starts, and the abort is in the events', async () => {
  const { runner, calls, types } = setup()
  const first = runner.start({ type: 'eval', code: 'await sleep(60000); log("still running")' })
  calls.length = 0

  const second = runner.start({ type: 'chat', message: 'hi' })

  // Synchronous: no await between the two starts and these checks.
  assert.deepEqual(types(), ['action.start #1', 'action.aborted #1', 'action.start #2'])
  assert.equal(runner.lastStatus?.id, 1)
  assert.equal(runner.lastStatus?.status, 'aborted')
  assert.match(runner.lastStatus?.error ?? '', /superseded by chat #2/)
  assert.deepEqual(calls.slice(0, 3), ['setGoal(null)', 'stopDigging', 'clearControlStates'])

  // The old action's promise settles within a tick (50 ms) and keeps its aborted status.
  const firstFinal = await within(50, first.done)
  assert.equal(firstFinal.status, 'aborted')
  assert.equal((await within(50, second.done)).status, 'succeeded')
  assert.ok(calls.includes('chat hi'))
  assert.ok(!types().some((t) => t.startsWith('action.log')), 'aborted eval code must not keep running')
})

test('an aborted action that finishes later does not overwrite its status or emit a second outcome', async () => {
  const { runner, types } = setup()
  // Ignores the abort signal and resolves after 30 ms anyway.
  const stubborn = runner.start({ type: 'eval', code: 'await new Promise((r) => setTimeout(r, 30)); return 1' })
  runner.start({ type: 'eval', code: 'await sleep(60000)' })
  assert.equal((await stubborn.done).status, 'aborted')
  await new Promise((r) => setTimeout(r, 60))
  assert.deepEqual(types().filter((t) => t.endsWith('#1')), ['action.start #1', 'action.aborted #1'])
  assert.equal(runner.currentStatus?.id, 2)
  runner.stop()
})

test('an aborted follow does not clear the goal of the action that replaced it', async () => {
  const { runner, calls } = setup()
  runner.start({ type: 'follow', player: 'Bonnie' })
  runner.start({ type: 'goto', x: 10, y: 64, z: 0 })
  await new Promise((r) => setTimeout(r, 10))
  const after = calls.slice(calls.indexOf('goto(GoalNear)'))
  assert.ok(after.length > 0, `goto never set its goal: ${calls}`)
  assert.ok(!after.includes('setGoal(null)'), `goal cleared after goto started: ${after}`)
  runner.stop()
})

test('stop aborts the running action and halts movement and digging', async () => {
  const { runner, calls, types } = setup()
  const running = runner.start({ type: 'eval', code: 'await sleep(60000)' })
  calls.length = 0

  const stopped = runner.stop('test')

  assert.equal(stopped?.status, 'aborted')
  assert.equal(runner.currentStatus, null)
  assert.deepEqual(calls, ['setGoal(null)', 'stopDigging', 'clearControlStates'])
  assert.deepEqual(types(), ['action.start #1', 'action.aborted #1', 'stop #'])
  assert.equal((await within(50, running.done)).status, 'aborted')
})

test('eval returns its value and logs, and fails with the message from eval code', async () => {
  const { runner } = setup()
  const ok = await runner.start({ type: 'eval', code: 'log("hello", { a: 1 }); return { sum: 1 + 2 }' }).done
  assert.equal(ok.status, 'succeeded')
  assert.deepEqual(ok.result, { value: { sum: 3 }, logs: ['hello { a: 1 }'] })

  const bad = await runner.start({ type: 'eval', code: 'throw new Error("no wood")' }).done
  assert.equal(bad.status, 'failed')
  assert.equal(bad.error, 'no wood')

  const syntax = await runner.start({ type: 'eval', code: 'return (' }).done
  assert.equal(syntax.status, 'failed')
  assert.match(syntax.error ?? '', /^syntax error/)
})

test('eval timeouts stop never-settling and synchronously looping code', async () => {
  const { runner } = setup()
  const hung = await within(500, runner.start({ type: 'eval', code: 'await new Promise(() => {})', timeoutMs: 50 }).done)
  assert.equal(hung.status, 'failed')
  assert.match(hung.error ?? '', /timed out after 50ms/)

  const spin = await within(1000, runner.start({ type: 'eval', code: 'while (true) {}', timeoutMs: 100 }).done)
  assert.equal(spin.status, 'failed')
  assert.match(spin.error ?? '', /timed out/)
})

test('code that outlives its eval timeout stops at its next sleep', async () => {
  const { runner, types } = setup()
  const loop = await runner.start({ type: 'eval', code: 'for (;;) { log("tick"); await sleep(20) }', timeoutMs: 50 }).done
  assert.equal(loop.status, 'failed')
  const ticks = types().filter((t) => t.startsWith('action.log')).length
  await new Promise((r) => setTimeout(r, 100))
  assert.equal(types().filter((t) => t.startsWith('action.log')).length, ticks)
})

test('a query runs beside the running action without aborting it', async () => {
  const { runner, types } = setup()
  runner.start({ type: 'eval', code: 'await sleep(60000)' })

  const q = await within(500, runner.query('log("looking"); return 6 * 7', 1000))

  assert.deepEqual(q, { status: 'succeeded', result: { value: 42, logs: ['looking'] } })
  assert.equal(runner.currentStatus?.status, 'running')
  assert.deepEqual(types(), ['action.start #1'])
  assert.deepEqual(await runner.query('throw new Error("no such item")', 1000), { status: 'failed', error: 'no such item' })
  runner.stop()
})
