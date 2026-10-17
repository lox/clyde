signal.throwIfAborted();
const { item, count, container: at } = args;
if (typeof item !== 'string' || !Number.isInteger(count) || count < 1 || !at || !['x', 'y', 'z'].every(k => Number.isInteger(at[k]))) throw new Error('Expected item, positive count, and integer container coordinates');
const type = mc.itemsByName[item];
if (!type) throw new Error('Unknown item: ' + item);
const total = () => bot.inventory.items().filter(i => i.type === type.id).reduce((n, i) => n + i.count, 0);
if (total() < count) throw new Error('Insufficient inventory items');
const pos = new Vec3(at.x, at.y, at.z);
await goto(new goals.GoalNear(pos.x, pos.y, pos.z, 3));
signal.throwIfAborted();
const block = bot.blockAt(pos);
if (!block || block.boundingBox === 'empty') throw new Error('Container block unavailable');
let window;
let before;
try {
  window = await bot.openContainer(block);
  signal.throwIfAborted();
  before = total();
  if (before < count) throw new Error('Insufficient inventory items');
  await window.deposit(type.id, null, count);
  signal.throwIfAborted();
} finally {
  if (window) window.close();
}
signal.throwIfAborted();
const expected = before - count;
for (let i = 0; i < 20 && total() !== expected; i++) {
  await sleep(100);
  signal.throwIfAborted();
}
const remaining = total();
if (remaining !== expected) throw new Error('Deposit did not transfer the requested amount');
return { item, deposited: count, remaining };
