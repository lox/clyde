signal.throwIfAborted();
const target = args.targetFood === undefined ? 20 : args.targetFood;
if (!Number.isInteger(target) || target < 0 || target > 20) throw new Error('targetFood must be an integer from 0 to 20');
const excluded = new Set(['poisonous_potato', 'rotten_flesh', 'spider_eye', 'pufferfish', 'raw_chicken', 'chicken', 'chorus_fruit', 'suspicious_stew']);
const eaten = [];
const abort = () => { bot.deactivateItem(); };
signal.addEventListener('abort', abort, { once: true });
try {
  while (bot.food < target) {
    signal.throwIfAborted();
    const options = bot.inventory.items().filter(i => !excluded.has(i.name) && mc.foodsByName[i.name] && mc.foodsByName[i.name].foodPoints > 0);
    options.sort((a, b) => mc.foodsByName[b.name].foodPoints - mc.foodsByName[a.name].foodPoints);
    const food = options[0];
    if (!food) throw new Error('No safe ordinary food available; hunger is ' + bot.food + ', target is ' + target);
    await bot.equip(food, 'hand');
    signal.throwIfAborted();
    const before = bot.food;
    await bot.consume();
    signal.throwIfAborted();
    eaten.push(food.name);
    for (let i = 0; i < 20 && bot.food <= before; i++) {
      await sleep(100);
      signal.throwIfAborted();
    }
    if (bot.food <= before) throw new Error('Consumption did not increase hunger');
  }
  return { food: bot.food, targetFood: target, consumed: eaten };
} finally {
  signal.removeEventListener('abort', abort);
}
