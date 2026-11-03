// Isolated slice-14 continuity experiment. Never use production endpoints.
// Usage: node gym/continuity.mjs smoke|before|after BINARY SOURCE_ROOT OUTPUT_DIR
// Requires the disposable Paper server already listening on the fixed local ports.
import fs from 'node:fs'
import net from 'node:net'
import http from 'node:http'
import https from 'node:https'
import { gunzipSync } from 'node:zlib'
import { execFileSync } from 'node:child_process'
import mineflayer from '../body/node_modules/mineflayer/index.js'

const [arm, binary, root, dir] = process.argv.slice(2)
if (!['smoke', 'before', 'after'].includes(arm) || !dir)
  throw Error('want arm binary source-root output-dir')
fs.mkdirSync(dir, { recursive: true })
if (fs.existsSync(dir + '/transcript.jsonl'))
  throw Error('refusing to overwrite an earlier trial')
const before = arm === 'before',
  smoke = arm === 'smoke'
const mc = before ? 25567 : 25565,
  rport = before ? 25577 : 25575,
  port = before ? 7789 : 7787,
  proxyPort = before ? 8789 : 8787
const name = 'continuity-' + arm,
  duration = smoke ? 180 : 3600,
  limit = smoke ? 40 : 120
const log = (v) => {
  const row = JSON.stringify({ ts: Date.now(), ...v })
  fs.appendFileSync(dir + '/transcript.jsonl', row + '\n')
  console.log(row)
}
const sleep = (ms) => new Promise((r) => setTimeout(r, ms))
const service = (...args) =>
  execFileSync('amp', ['orb', 'service', ...args], { encoding: 'utf8' })
const state = async () => {
  const r = await fetch(`http://127.0.0.1:${port}/state`)
  if (!r.ok) throw Error(await r.text())
  return r.json()
}
async function until(f, ms = 60000) {
  const end = Date.now() + ms
  while (!(await f())) {
    if (Date.now() > end) return false
    await sleep(200)
  }
  return true
}

// This independent cap also bounds the unchanged baseline, including hidden SDK
// retries. Forward bytes unchanged. Do not log keys or native encrypted payloads.
let requests = 0
const proxy = http.createServer(async (req, res) => {
  if (++requests > limit) {
    log({ request_rejected: requests })
    res.writeHead(429)
    res.end('{"error":{"message":"trial request cap"}}')
    return
  }
  const id = requests,
    chunks = []
  for await (const chunk of req) chunks.push(chunk)
  const bytes = Buffer.concat(chunks),
    body = JSON.parse(bytes)
  log({
    request: id,
    model: body.model,
    effort: body.reasoning?.effort,
    previous: body.previous_response_id ?? null,
    input_types: body.input?.map((x) => x.type || x.role),
    include: body.include,
  })
  const up = https.request(
    'https://api.openai.com' + req.url,
    { method: req.method, headers: { ...req.headers, host: 'api.openai.com' } },
    (answer) => {
      res.writeHead(answer.statusCode, answer.headers)
      const data = []
      answer.on('data', (b) => {
        data.push(b)
        res.write(b)
      })
      answer.on('end', () => {
        res.end()
        try {
          let bytes = Buffer.concat(data)
          if (answer.headers['content-encoding'] === 'gzip')
            bytes = gunzipSync(bytes)
          const r = JSON.parse(bytes)
          log({
            response: id,
            status: answer.statusCode,
            response_id: r.id,
            usage: r.usage,
            error: r.error,
            output_types: r.output?.map((x) => x.type),
          })
        } catch {
          log({ response: id, status: answer.statusCode, usage_unparsed: true })
        }
      })
    },
  )
  up.on('error', (e) => {
    log({ response: id, error: String(e) })
    res.destroy()
  })
  // A cancelled local call cancels upstream rather than hiding paid background work.
  res.on('close', () => {
    if (!res.writableEnded) up.destroy()
  })
  up.end(bytes)
})
await new Promise((r) => proxy.listen(proxyPort, '127.0.0.1', r))

// Minimal serial RCON, using the disposable server's existing credential.
const properties = fs.readFileSync(
  before
    ? process.env.HOME + '/paper-continuity/server.properties'
    : process.env.HOME + '/paper/server.properties',
  'utf8',
)
const password = properties.match(/^rcon.password=(.*)$/m)[1]
const conn = net.connect(rport, '127.0.0.1')
await new Promise((r) => conn.once('connect', r))
let buffer = Buffer.alloc(0),
  seq = 0,
  waiting = new Map()
conn.on('data', (b) => {
  buffer = Buffer.concat([buffer, b])
  while (buffer.length >= 4 && buffer.length >= buffer.readInt32LE(0) + 4) {
    const size = buffer.readInt32LE(0),
      p = buffer.subarray(4, 4 + size)
    buffer = buffer.subarray(4 + size)
    const id = p.readInt32LE(0),
      type = p.readInt32LE(4)
    if (id === 1 && type === 0 && p.length === 10) continue
    waiting.get(id)?.(p.subarray(8, -2).toString())
    waiting.delete(id)
  }
})
async function packet(type, text) {
  const id = ++seq,
    p = Buffer.alloc(Buffer.byteLength(text) + 14)
  p.writeInt32LE(p.length - 4)
  p.writeInt32LE(id, 4)
  p.writeInt32LE(type, 8)
  p.write(text, 12)
  const result = new Promise((resolve, reject) => {
    const t = setTimeout(() => {
      waiting.delete(id)
      reject(Error('RCON deadline'))
    }, 10000)
    waiting.set(id, (v) => {
      clearTimeout(t)
      resolve(v)
    })
  })
  conn.write(p)
  return result
}
await packet(3, password)
async function rcon(...commands) {
  for (const command of commands)
    log({ rcon: command, result: await packet(2, command) })
}
let player,
  timer,
  started = false
async function start() {
  const args = [
    'serve',
    '--body',
    `http://127.0.0.1:${port}`,
    '--rcon',
    `127.0.0.1:${rport}`,
    '--memory',
    dir + '/episodes.db',
    '--semantic-dir',
    dir + '/semantic',
    '--candidates-dir',
    dir + '/candidates',
    '--judge',
    'jev',
    '--judge-log',
    dir + '/judge.jsonl',
  ]
  if (before)
    args.push(
      '--planner-prompt',
      dir + '/prompt.md',
      '--max-steps',
      '40',
      '--max-time',
      '300s',
    )
  else
    args.push(
      '--prompt',
      dir + '/prompt.md',
      '--request-limit',
      String(limit * 2),
      '--max-output-tokens',
      '4096',
      '--call-timeout',
      '45s',
    )
  service(
    'start',
    name + '-serve',
    '--cwd',
    root,
    '--command',
    `env OPENAI_BASE_URL=http://127.0.0.1:${proxyPort}/v1 ${[binary, ...args].map((x) => JSON.stringify(x)).join(' ')} >> ${JSON.stringify(dir + '/serve.log')} 2>&1`,
  )
  started = true
}
function stop() {
  if (started) {
    service('stop', name + '-serve')
    started = false
  }
}
function say(text) {
  log({ sent: text })
  player.chat('Clyde, ' + text)
}
async function check(label) {
  log({ check: label, state: await state(), requests })
  await rcon(
    'data get entity Clyde Inventory',
    'data get entity Clyde Pos',
    'data get block 4 80 3 Items',
    'execute if block 3 80 3 crafting_table run say VERIFIED_TABLE',
    'execute if block 8 80 3 birch_planks if block 9 80 3 birch_planks if block 10 80 3 birch_planks run say VERIFIED_BIRCH_ROW',
  )
}
try {
  service(
    'start',
    name + '-body',
    '--cwd',
    root + '/body',
    '--command',
    `env MC_HOST=127.0.0.1 MC_PORT=${mc} MC_USERNAME=Clyde BODY_LISTEN_HOST=127.0.0.1 BODY_LISTEN_PORT=${port} node --import tsx src/index.ts`,
  )
  if (
    !(await until(async () => {
      try {
        return !!(await state()).bot
      } catch {
        return false
      }
    }))
  )
    throw Error('body unavailable')
  player = mineflayer.createBot({
    host: '127.0.0.1',
    port: mc,
    username: 'Alex',
    auth: 'offline',
    version: '26.1',
  })
  player.on('chat', (username, message) => {
    log({ username, message })
    if (username === 'Clyde')
      state()
        .then((c) => log({ reply_state: c }))
        .catch((e) => log({ error: String(e) }))
  })
  player.on('error', (e) => log({ error: String(e) }))
  await new Promise((r) => player.once('spawn', r))
  await rcon('forceload add -25 -25 110 25')
  await sleep(2000)
  await rcon(
    'difficulty peaceful',
    'gamerule advance_time false',
    'time set day',
    'fill -25 79 -25 110 79 25 grass_block',
    'fill -25 80 -25 110 82 25 air',
    'fill -25 83 -25 110 85 25 air',
    'tp Clyde 0.5 80 0.5',
    'tp Alex 2.5 80 2.5',
    'clear Clyde',
    'give Clyde birch_log 16',
    'gamemode creative Clyde',
    'gamemode survival Clyde',
    'effect give Clyde instant_health 1 10',
    'setblock 4 80 3 chest',
    'setblock 6 80 0 bedrock',
    'fill 20 80 10 20 85 10 birch_log',
  )
  fs.writeFileSync(
    dir + '/prompt.md',
    fs.readFileSync(
      root + '/prompts/' + (before ? 'planner.md' : 'player.md'),
      'utf8',
    ) +
      '\nIsolated attended session: wait for Alex on initial idle and after finishing requests. Do not start unrelated autonomous curriculum.\n',
  )
  log({
    frozen_scenario: 1,
    arm,
    duration,
    frontier_request_cap: limit,
    model: 'gpt-6.1-sol',
    effort: 'low',
    judge: 'jev',
    returned_spend_review_limit_usd: 5,
    unknown_billing: 'not an invoice cap',
  })
  await start()
  const began = Date.now()
  let cursor = 0,
    busy = false
  timer = setInterval(async () => {
    if (busy) return
    busy = true
    try {
      const page = await (
        await fetch(`http://127.0.0.1:${port}/events?since=${cursor}`)
      ).json()
      for (const event of page.events || []) log({ event })
      cursor = page.latestSeq ?? cursor
    } catch (e) {
      log({ poll_error: String(e) })
    } finally {
      busy = false
    }
  }, 1000)
  async function at(seconds, fn) {
    await sleep(Math.max(0, began + seconds * 1000 - Date.now()))
    await fn()
  }
  await at(5, () =>
    say(
      'Our ongoing project is a birch workshop, never oak. First make four birch planks and a crafting table kept in inventory, then wait. These supplies are only the first milestones.',
    ),
  )
  if (smoke) {
    await at(60, async () => {
      await check('supplies')
      log({
        compact: execFileSync(
          binary,
          ['control', 'compact', '--memory', dir + '/episodes.db'],
          { encoding: 'utf8' },
        ),
      })
    })
    await at(105, async () => {
      stop()
      log({ restart: true })
      await start()
    })
    await at(120, () =>
      say(
        'What material and ongoing project did we agree on? Continue by making eight more planks of that material, keep the table, then wait.',
      ),
    )
  } else {
    await at(120, async () => {
      await check('supplies')
      say(
        'Keep the table for now. Walk to 85 80 0, within two blocks, without digging or building, then wait.',
      )
      const moving = await until(async () => {
        const c = await state()
        return !!c.action && c.bot.position.x > 3 && c.bot.position.x < 65
      }, 90000)
      log({ observed_motion: moving, state: await state() })
      if (moving) {
        say('What are you doing? Answer while continuing to walk.')
        await sleep(4000)
        say(
          'Revision: replace that destination with 12 80 0, within two blocks, then wait. Never resume the old destination.',
        )
      }
    })
    await at(420, async () => {
      await check('revision')
      say(
        'Place the crafting table at 3 80 3 and put eight birch planks in the chest at 4 80 3. Verify both, then wait.',
      )
    })
    await at(900, async () => {
      await check('placement-delivery')
      await rcon(
        'tp Clyde 16.5 80 10.5',
        'fill 15 80 9 17 82 11 birch_leaves[persistent=true]',
        'fill 16 80 10 16 81 10 air',
      )
      say(
        'An obstruction is around you. Gather six birch logs from the nearby trunk, work through the obstruction safely, then wait.',
      )
    })
    await at(1200, async () => {
      await check('obstruction')
      say(
        'Try to break the bedrock at 6 80 0 once. Report the result honestly; do not repeat an impossible action. Then wait.',
      )
    })
    await at(1500, () =>
      say(
        'Stop work for now and wait for my next message. Keep our project requirements and revision in memory.',
      ),
    )
    await at(1800, async () => {
      await check('wait-and-impossible')
      if (!before)
        log({
          compact: execFileSync(
            binary,
            ['control', 'compact', '--memory', dir + '/episodes.db'],
            { encoding: 'utf8' },
          ),
        })
      else log({ compact: 'baseline has no supported compaction control' })
    })
    await at(2100, async () => {
      stop()
      log({ restart: true })
      await start()
    })
    await at(2400, () =>
      say(
        'Continue our workshop with three blocks of its agreed wood planks at 8 80 3, 9 80 3 and 10 80 3. What did I revise earlier? Verify the row, then wait.',
      ),
    )
    await at(3000, async () => {
      await check('post-restart-row')
      say(
        'Which project milestones are verified and which failed? What material must we avoid? Just answer and wait.',
      )
    })
    await at(3500, () => say('Wait for me; no further work this session.'))
  }
  await at(duration, () => check('final'))
} catch (e) {
  log({ trial_error: String(e) })
  process.exitCode = 1
} finally {
  stop()
  clearInterval(timer)
  player?.quit()
  conn.end()
  proxy.close()
  service('stop', name + '-body')
  log({ finished: true, requests })
}
