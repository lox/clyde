signal.throwIfAborted();
const at = args.at;
if (!at || !['x', 'y', 'z'].every(k => Number.isInteger(at[k]))) throw new Error('Expected integer args.at coordinates');
const pos = new Vec3(at.x, at.y, at.z);
const validFloor = () => {
  const air = bot.blockAt(pos), floor = bot.blockAt(pos.offset(0, -1, 0));
  return air && ['air', 'cave_air', 'void_air'].includes(air.name) && floor && floor.boundingBox === 'block' ? floor : null;
};
const check = async () => {
  for (let i = 0; i < 20; i++) {
    signal.throwIfAborted();
    const floor = validFloor();
    if (floor) return floor;
    await sleep(100);
  }
  signal.throwIfAborted();
  const floor = validFloor();
  if (!floor) throw new Error('Target must be loaded air above a solid floor');
  return floor;
};
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
await check();
await settleCursor();
const tableId = mc.itemsByName.crafting_table.id;
const total = id => bot.inventory.items().filter(i => i.type === id).reduce((n, i) => n + i.count, 0);
const heldTable = () => bot.inventory.items().find(i => i.type === tableId);
const craftOnce = async recipe => {
  signal.throwIfAborted();
  if (!recipe || !recipe.result || !Number.isInteger(recipe.result.count) || recipe.result.count < 1) throw new Error('Invalid crafting recipe');
  await settleCursor();
  const id = recipe.result.id;
  const expected = total(id) + recipe.result.count;
  await bot.craft(recipe, 1, null);
  await settleCursor();
  for (let i = 0; i < 20 && total(id) < expected; i++) {
    await sleep(100);
    signal.throwIfAborted();
  }
  if (total(id) < expected) throw new Error('Crafting produced no complete inventory progress');
};
if (!heldTable()) {
  let recipes = bot.recipesFor(tableId, null, 1, null);
  let plankTypes;
  let crafts = 0;
  while (!recipes.length) {
    signal.throwIfAborted();
    if (++crafts > 64) throw new Error('Plank crafting limit exceeded');
    if (!plankTypes) plankTypes = mc.itemsArray.filter(i => i.name.endsWith('_planks'));
    let plankRecipe = null;
    for (const type of plankTypes) {
      const found = bot.recipesFor(type.id, null, 1, null);
      if (found.length) { plankRecipe = found[0]; break; }
    }
    if (!plankRecipe) throw new Error('Insufficient compatible planks or craftable logs for a crafting table');
    await craftOnce(plankRecipe);
    recipes = bot.recipesFor(tableId, null, 1, null);
  }
  await craftOnce(recipes[0]);
}
signal.throwIfAborted();
await goto(new goals.GoalNear(pos.x, pos.y, pos.z, 3));
signal.throwIfAborted();
const floor = await check();
const table = heldTable();
if (!table) throw new Error('Crafting table missing from inventory');
await bot.equip(table, 'hand');
signal.throwIfAborted();
await bot.placeBlock(floor, new Vec3(0, 1, 0));
signal.throwIfAborted();
for (let i = 0; i < 20 && bot.blockAt(pos)?.name !== 'crafting_table'; i++) {
  await sleep(100);
  signal.throwIfAborted();
}
if (bot.blockAt(pos)?.name !== 'crafting_table') throw new Error('Table was not placed at the requested coordinates');
await settleCursor();
return { block: 'crafting_table', at: { x: pos.x, y: pos.y, z: pos.z } };
