signal.throwIfAborted();
const name = args.block === undefined ? 'oak_log' : args.block;
const target = args.count;
if (typeof name !== 'string' || !Number.isInteger(target) || target < 1 || !/(_log|_stem)$/.test(name)) throw new Error('Expected a log/stem block name and positive integer count');
const def = mc.blocksByName[name], item = mc.itemsByName[name];
if (!def || !item) throw new Error('Unknown log type: ' + name);
const origin = bot.entity.position.clone();
const total = () => bot.inventory.items().filter(i => i.type === item.id).reduce((n, i) => n + i.count, 0);
let broken = 0;
const abort = () => {
  try { bot.collectBlock.targets.clear(); } catch (_) {}
  try {
    const pending = bot.collectBlock.cancelTask();
    if (pending && typeof pending.catch === 'function') void pending.catch(() => {});
  } catch (_) {}
  try {
    const pending = bot.stopDigging();
    if (pending && typeof pending.catch === 'function') void pending.catch(() => {});
  } catch (_) {}
};
signal.addEventListener('abort', abort, { once: true });
try {
  while (total() < target) {
    signal.throwIfAborted();
    if (!bot.collectBlock || typeof bot.collectBlock.collect !== 'function') throw new Error('collectBlock plugin unavailable');
    const needed = target - total();
    const positions = bot.findBlocks({ point: origin, matching: def.id, maxDistance: 32, count: needed * 4 }).slice(0, needed);
    const blocks = positions.map(p => bot.blockAt(p)).filter(b => b && b.type === def.id);
    if (!blocks.length) throw new Error('Insufficient accessible ' + name + '; inventory has ' + total() + ' of ' + target);
    const before = total();
    signal.throwIfAborted();
    await bot.collectBlock.collect(blocks, { ignoreNoPath: true });
    signal.throwIfAborted();
    for (const block of blocks) {
      const remaining = bot.blockAt(block.position);
      if (remaining && remaining.type !== def.id) broken++;
    }
    for (let i = 0; i < 20 && total() < target; i++) {
      await sleep(100);
      signal.throwIfAborted();
    }
    if (total() <= before) throw new Error('Log collection made no inventory progress');
  }
  signal.throwIfAborted();
  return { item: name, count: total(), broken };
} finally {
  signal.removeEventListener('abort', abort);
}
