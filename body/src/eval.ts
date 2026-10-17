import vm from 'node:vm'
import { inspect } from 'node:util'
import pathfinderPkg from 'mineflayer-pathfinder'
import type { goals as Goals } from 'mineflayer-pathfinder'
import { Vec3 } from 'vec3'
import type { ActionContext } from './actions.js'
import { gotoGoal } from './actions.js'

const { goals, Movements } = pathfinderPkg

/**
 * Code-as-action. Runs `code` as the body of an async function with the bot
 * in scope. This is not a security boundary: the code runs in-process with
 * full access to the bot. It exists so a model can write JavaScript that is
 * not limited to the fixed action vocabulary. The safety wrappers on
 * bot.dig, placement, and bot.attack still apply.
 *
 * In scope: bot, mc (registry), Vec3, goals, Movements, goto(goal), sleep(ms),
 * log(...args), signal (AbortSignal), and the timer functions.
 *
 * The timeout bounds the whole run, and also the synchronous part before the
 * first await. A synchronous infinite loop after an await blocks the process.
 */
export async function runCode(ctx: ActionContext, code: string, timeoutMs: number): Promise<{ value: unknown; logs: string[] }> {
  const { bot } = ctx
  const logs: string[] = []
  // Aborted by the runner (preempted or stopped) or by the timeout, so sleep()
  // and goto() in code that outlives its timeout stop as well.
  const controller = new AbortController()
  const signal = controller.signal
  const onOuterAbort = () => controller.abort(ctx.signal.reason)
  ctx.signal.addEventListener('abort', onOuterAbort, { once: true })
  const timer = setTimeout(() => controller.abort(new Error(`timed out after ${timeoutMs}ms`)), timeoutMs)
  const stopped = new Promise<never>((_, reject) => {
    signal.addEventListener('abort', () => reject(signal.reason), { once: true })
  })
  stopped.catch(() => {}) // Settles only through the race below.
  const evalCtx: ActionContext = { ...ctx, signal }

  const sleep = (ms: number) =>
    new Promise<void>((resolve, reject) => {
      if (signal.aborted) return reject(signal.reason)
      const onAbort = () => {
        clearTimeout(t)
        reject(signal.reason)
      }
      const t = setTimeout(() => {
        signal.removeEventListener('abort', onAbort)
        resolve()
      }, ms)
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
    goto: (goal: Goals.Goal) => gotoGoal(evalCtx, goal),
    sleep,
    log,
    signal,
    console: { log },
    setTimeout,
    clearTimeout,
    setInterval,
    clearInterval,
  })

  try {
    let running: Promise<unknown>
    try {
      running = vm.runInContext(`(async () => {\n${code}\n})()`, context, { filename: 'eval.js', timeout: timeoutMs })
    } catch (err) {
      // The error may come from the context's realm, so check the name, not instanceof.
      if ((err as Error)?.name === 'SyntaxError') throw new Error(`syntax error: ${(err as Error).message}`)
      throw err
    }
    // If the timeout or an abort wins the race, the code's eventual rejection has no other handler.
    running.catch(() => {})
    const value = await Promise.race([running, stopped])
    return { value: serializable(value), logs }
  } catch (err) {
    ctx.signal.throwIfAborted()
    // Errors thrown by eval code come from the context's realm and fail instanceof Error.
    const message = typeof err === 'object' && err !== null && 'message' in err ? String(err.message) : String(err)
    throw new Error(logs.length ? `${message}\nlogs:\n${logs.join('\n')}` : message)
  } finally {
    clearTimeout(timer)
    ctx.signal.removeEventListener('abort', onOuterAbort)
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
