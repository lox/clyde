signal.throwIfAborted();
const { item, count } = args;
if (typeof item !== 'string' || !Number.isInteger(count) || count < 1) throw new Error('Expected item name and positive integer count');
const type = mc.itemsByName[item];
if (!type) throw new Error('Unknown item: ' + item);
const total = () => bot.inventory.items().filter(i => i.type === type.id).reduce((n, i) => n + i.count, 0);
const settleCursor = async () => {
  await sleep(150);
  signal.throwIfAborted();
  for (let i = 0; i < 10; i++) {
    const window = bot.currentWindow || bot.inventory;
    if (!window.selectedItem && !bot.inventory.selectedItem) return;
    let slot = -1;
    for (let s = window.inventoryStart; s < window.inventoryEnd; s++) {
      if (window.slots[s] === null) { slot = s; break; }
    }
    if (slot < 0) throw new Error('Inventory full; cannot safely stow cursor item');
    signal.throwIfAborted();
    await bot.clickWindow(slot, 0, 0);
    await sleep(150);
    signal.throwIfAborted();
  }
  if ((bot.currentWindow || bot.inventory).selectedItem || bot.inventory.selectedItem) throw new Error('Cursor item could not be stowed');
};
let crafts = 0;
await settleCursor();
while (total() < count) {
  signal.throwIfAborted();
  let table = null;
  let recipes = bot.recipesFor(type.id, null, 1, null);
  if (!recipes.length) {
    const def = mc.blocksByName.crafting_table;
    table = def && bot.findBlock({ matching: def.id, maxDistance: 32 });
    if (!table) throw new Error('No hand recipe with available ingredients, and no nearby crafting table');
    await goto(new goals.GoalNear(table.position.x, table.position.y, table.position.z, 3));
    signal.throwIfAborted();
    table = bot.blockAt(table.position);
    if (!table || table.name !== 'crafting_table') throw new Error('Crafting table disappeared');
    recipes = bot.recipesFor(type.id, null, 1, table);
  }
  const recipe = recipes[0];
  if (!recipe || !recipe.result || !Number.isInteger(recipe.result.count) || recipe.result.count < 1) throw new Error('No usable recipe with available ingredients for ' + item);
  await settleCursor();
  const before = total();
  await bot.craft(recipe, 1, table);
  crafts++;
  await settleCursor();
  const expected = before + recipe.result.count;
  for (let i = 0; i < 20 && total() < expected; i++) {
    await sleep(100);
    signal.throwIfAborted();
  }
  if (total() < expected) throw new Error('Crafting produced no complete inventory progress');
}
await settleCursor();
if (total() < count) throw new Error('Crafted items missing from inventory');
return { item, count: total(), crafts };
