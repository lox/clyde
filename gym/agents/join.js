// Joins the gym server as MC_USERNAME and stands still until stopped.
// Shared by the idle and cheat agents; it is not the body.
import mineflayer from 'mineflayer'

const bot = mineflayer.createBot({
  host: process.env.MC_HOST ?? '127.0.0.1',
  port: Number(process.env.MC_PORT ?? 25565),
  username: process.env.MC_USERNAME ?? 'clyde',
  auth: 'offline',
  version: '26.1', // pinned like the body; see body/src/config.ts
})

bot.once('spawn', () => {
  const p = bot.entity.position
  console.log(`joined as ${bot.username} at ${p.x.toFixed(1)} ${p.y.toFixed(1)} ${p.z.toFixed(1)}`)
})
bot.on('kicked', (reason) => console.error('kicked:', JSON.stringify(reason)))
bot.on('error', (err) => console.error('error:', err.message))
bot.on('end', (reason) => {
  console.log('disconnected:', reason)
  process.exit(stopping ? 0 : 1)
})

let stopping = false
for (const sig of ['SIGTERM', 'SIGINT']) {
  process.on(sig, () => {
    stopping = true
    bot.quit()
    setTimeout(() => process.exit(0), 2000).unref()
  })
}
