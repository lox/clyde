import mineflayer from 'mineflayer'
import pathfinderPkg from 'mineflayer-pathfinder'
import collectBlockPkg from 'mineflayer-collectblock'
import type {} from 'mineflayer-collectblock'
import { ActionRunner } from './actions.js'
import { loadConfig } from './config.js'
import { EventLog } from './events.js'
import { Safety } from './safety.js'
import { createServer } from './server.js'
import { buildStateCard, CARD_EVENT_LIMIT, CARD_EVENT_WINDOW_MS } from './state.js'

const { pathfinder, Movements } = pathfinderPkg

const config = loadConfig()
const events = new EventLog()
const safety = new Safety({ maxBreaksPerMin: config.maxBreaksPerMin, regions: config.protectedRegions }, events)

const bot = mineflayer.createBot({
  host: config.host,
  port: config.port,
  username: config.username,
  auth: config.auth,
  version: config.version,
  profilesFolder: config.profilesFolder,
})
bot.loadPlugin(pathfinder)
bot.loadPlugin(collectBlockPkg.plugin)

const runner = new ActionRunner(bot, events, safety)
let spawned = false

bot.once('spawn', () => {
  // Plugins are injected by now; wrap before any action can run.
  safety.install(bot)
  runner.guardStaleCode()
  // Paper 26.1 rubber-bands any move that leaves the player exactly touching a
  // wall, which prismarine-physics does every time it walks into one; the bot
  // then never gets out of a trench. A 0.1 mm gap avoids the contact.
  Object.assign(bot.physics, { playerHalfWidth: 0.3001 })
  const dimension = () => bot.game.dimension
  const movements = new Movements(bot)
  safety.guardMovements(movements, dimension)
  bot.pathfinder.setMovements(movements)
  // collectblock swaps in its own Movements while collecting.
  safety.guardMovements(bot.collectBlock.movements!, dimension)
  spawned = true
})

bot.on('spawn', () => {
  const p = bot.entity.position
  events.push('spawn', `spawned at ${Math.floor(p.x)},${Math.floor(p.y)},${Math.floor(p.z)} in ${bot.game.dimension}`)
})

bot.on('chat', (username, message) => {
  events.push('chat', `<${username}> ${message}`, { username, message })
  if (config.ops.includes(username) && message.trim().toLowerCase() === `${bot.username.toLowerCase()} stop`) {
    runner.stop(`chat stop from ${username}`)
  }
})
bot.on('whisper', (username, message) => {
  events.push('whisper', `<${username}> ${message}`, { username, message })
})

let lastHealth: number | null = null
bot.on('health', () => {
  if (lastHealth !== null && bot.health < lastHealth) {
    const amount = Math.round((lastHealth - bot.health) * 10) / 10
    events.push('damage', `took ${amount} damage, health ${bot.health}/20`, { amount, health: bot.health })
  }
  lastHealth = bot.health
})
bot.on('death', () => {
  events.push('death', 'died')
  runner.stop('died')
  lastHealth = null
})
bot.on('playerCollect', (collector, collected) => {
  if (collector !== bot.entity) return
  const item = collected.getDroppedItem()
  if (item) events.push('pickup', `picked up ${item.name} x${item.count}`, { item: item.name, count: item.count })
})
bot.on('kicked', (reason) => {
  events.push('kicked', String(reason))
})
bot.on('error', (err) => {
  console.error('bot error:', err)
  events.push('error', err.message)
})
// No reconnect loop in v0: exit and let the supervisor restart the body.
bot.on('end', (reason) => {
  console.error(`disconnected: ${reason}`)
  process.exit(1)
})

// Eval code can leave promises unhandled; record them instead of crashing.
process.on('unhandledRejection', (err) => {
  const message = typeof err === 'object' && err !== null && 'message' in err ? String(err.message) : String(err)
  console.error('unhandled rejection:', err)
  events.push('error', `unhandled rejection: ${message}`)
})

const server = createServer({
  runner,
  events,
  card: () =>
    spawned
      ? buildStateCard({
          bot,
          action: runner.currentStatus,
          lastAction: runner.lastStatus,
          events: events.recent(CARD_EVENT_LIMIT, CARD_EVENT_WINDOW_MS),
          scanRadius: config.scanRadius,
        })
      : null,
})
server.listen(config.listenPort, config.listenHost, () => {
  console.log(`body listening on http://${config.listenHost}:${config.listenPort}, joining ${config.host}:${config.port} as ${config.username}`)
})
