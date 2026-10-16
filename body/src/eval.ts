import vm from 'node:vm'
import { inspect } from 'node:util'
import { goals, Movements } from 'mineflayer-pathfinder'
import { Vec3 } from 'vec3'
import type { ActionContext } from './actions.js'
import { gotoGoal } from './actions.js'

/**
 * Code-as-action. Runs `code` as the body of an async function with the bot
 * in scope. This is not a security boundary: the code runs in-process with
 * full access to the bot. It exists so a model can write JavaScript that is
 * not limited to the fixed action vocabulary.
 *
 * In scope: bot, mc (registry), Vec3, goals, Movements, goto(goal), sleep(ms),
 * log(...args), signal (AbortSignal).
 */
export async function runCode(ctx: ActionContext, code: string, timeoutMs: number): Promise<unknown> {
  const { bot, signal } = ctx
  const logs: string[] = []

  const sleep = (ms: number) =>
    new Promise<void>((resolve, reject) => {
      if (signal.aborted) return reject(signal.reason)
      const t = setTimeout(() => {
        signal.removeEventListener('abort', onAbort)
        resolve()
      }, ms)
      const onAbort = () => {
        clearTimeout(t)
        reject(signal.reason)
      }
      signal.addEventListener('abort', onAbort, { once: true })
    })

  const log = (...args: unknown[]) => {
    const line = args.map((a) => (typeof a === 'string' ? a : inspect(a, { depth: 3 }))).join(' ')
    logs.push(line)
    ctx.log(line)
  }

  const context = vm.createContext({
    bot,
    mc: bot.registry,
    Vec3,
    goals,
    Movements,
    goto: (goal: goals.Goal) => gotoGoal(ctx, goal),
    sleep,
    log,
    signal,
    console: { log },
  })

  let fn: (...args: unknown[]) => Promise<unknown>
  try {
    fn = vm.runInContext(`(async () => {\n${code}\n})`, context, { filename: 'eval.js' })
  } catch (err) {
    throw new Error(`syntax error: ${err instanceof Error ? err.message : String(err)}`)
  }

  const timeout = new Promise<never>((_, reject) => {
    const t = setTimeout(() => reject(new Error(`timed out after ${timeoutMs}ms`)), timeoutMs)
    signal.addEventListener('abort', () => clearTimeout(t), { once: true })
  })

  try {
    const value = await Promise.race([fn(), timeout])
    return { value: serializable(value), logs }
  } catch (err) {
    signal.throwIfAborted()
    const message = err instanceof Error ? err.message : String(err)
    throw new Error(logs.length ? `${message}\nlogs:\n${logs.join('\n')}` : message)
  }
}

function serializable(value: unknown): unknown {
  if (value === undefined) return null
  try {
    return JSON.parse(JSON.stringify(value))
  } catch {
    return inspect(value, { depth: 2 })
  }
}
