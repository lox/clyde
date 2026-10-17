import http from 'node:http'
import { readFileSync } from 'node:fs'
import { Ajv, type ValidateFunction } from 'ajv'
import type { ActionRequest, EvalAction } from './api/action.js'
import type { StateCard } from './api/state.js'
import type { ActionRunner } from './actions.js'
import type { EventLog } from './events.js'
import { renderCard } from './state.js'

export interface ServerDeps {
  runner: ActionRunner
  events: EventLog
  /** The current state card, or null until the bot has spawned. */
  card: () => StateCard | null
}

const MAX_BODY_BYTES = 1 << 20

class HttpError extends Error {
  constructor(
    readonly status: number,
    message: string,
  ) {
    super(message)
  }
}

/**
 * GET /state[?format=text], POST /action, POST /eval, POST /query,
 * GET /events?since=N, POST /stop. Requests are validated against
 * api/action.schema.json.
 */
export function createServer({ runner, events, card }: ServerDeps): http.Server {
  const validate = actionValidator()

  const ready = (): StateCard => {
    const c = card()
    if (!c) throw new HttpError(503, 'bot has not spawned yet')
    return c
  }

  const routes: Record<string, (req: http.IncomingMessage, url: URL) => Promise<unknown>> = {
    'GET /state': async (_req, url) => {
      const c = ready()
      return url.searchParams.get('format') === 'text' ? renderCard(c) + '\n' : c
    },
    'POST /action': async (req) => {
      ready()
      const action = validate(await readJson(req))
      if (action.type === 'stop') return { stopped: runner.stop('stop action') }
      return runner.start(action).status
    },
    'POST /eval': async (req) => {
      ready()
      const body = await readJson(req)
      // Runs as the current action, like POST /action, but answers with the final status.
      const action = validate({ ...(isObject(body) ? body : {}), type: 'eval' }) as EvalAction
      return runner.start(action).done
    },
    'POST /query': async (req) => {
      ready()
      const body = await readJson(req)
      // Same request as POST /eval, but runs beside the current action instead of replacing it.
      const action = validate({ ...(isObject(body) ? body : {}), type: 'eval' }) as EvalAction
      return runner.query(action.code, action.timeoutMs ?? 10_000)
    },
    'GET /events': async (_req, url) => {
      const raw = url.searchParams.get('since') ?? '0'
      const since = Number(raw)
      if (!Number.isInteger(since) || since < 0) throw new HttpError(400, `since must be a non-negative integer, got ${raw}`)
      return events.since(since)
    },
    'POST /stop': async () => ({ stopped: runner.stop('POST /stop') }),
  }

  return http.createServer(async (req, res) => {
    const url = new URL(req.url ?? '/', 'http://body')
    const route = routes[`${req.method} ${url.pathname}`]
    try {
      if (!route) throw new HttpError(404, `no route ${req.method} ${url.pathname}`)
      const out = await route(req, url)
      if (typeof out === 'string') {
        res.writeHead(200, { 'content-type': 'text/plain; charset=utf-8' }).end(out)
      } else {
        res.writeHead(200, { 'content-type': 'application/json' }).end(JSON.stringify(out) + '\n')
      }
    } catch (err) {
      const status = err instanceof HttpError ? err.status : 500
      const message = err instanceof Error ? err.message : String(err)
      res.writeHead(status, { 'content-type': 'application/json' }).end(JSON.stringify({ error: message }) + '\n')
    }
  })
}

function isObject(v: unknown): v is Record<string, unknown> {
  return typeof v === 'object' && v !== null && !Array.isArray(v)
}

async function readJson(req: http.IncomingMessage): Promise<unknown> {
  const chunks: Buffer[] = []
  let size = 0
  for await (const chunk of req) {
    size += (chunk as Buffer).length
    if (size > MAX_BODY_BYTES) throw new HttpError(413, 'request body too large')
    chunks.push(chunk as Buffer)
  }
  try {
    return JSON.parse(Buffer.concat(chunks).toString('utf8'))
  } catch {
    throw new HttpError(400, 'request body must be JSON')
  }
}

/**
 * Picks the schema branch by `type` so errors name the field that is wrong
 * instead of listing every oneOf branch.
 */
function actionValidator(): (body: unknown) => ActionRequest {
  const schema = JSON.parse(readFileSync(new URL('../../api/action.schema.json', import.meta.url), 'utf8'))
  const ajv = new Ajv({ allErrors: true })
  ajv.addSchema(schema)
  const byType = new Map<string, ValidateFunction>()
  for (const [name, def] of Object.entries<{ properties: { type: { const: string } } }>(schema.definitions)) {
    byType.set(def.properties.type.const, ajv.getSchema(`${schema.$id}#/definitions/${name}`)!)
  }
  return (body) => {
    const type = isObject(body) ? body.type : undefined
    const check = typeof type === 'string' ? byType.get(type) : undefined
    if (!check) throw new HttpError(400, `type must be one of ${[...byType.keys()].join(', ')}`)
    if (!check(body)) throw new HttpError(400, ajv.errorsText(check.errors, { dataVar: String(type) }))
    return body as ActionRequest
  }
}
