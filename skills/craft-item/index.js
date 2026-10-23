signal.throwIfAborted();
const { item, count } = args;
if (typeof item !== 'string' || !Number.isInteger(count) || count < 1) throw new Error('Expected item name and positive integer count');
const type = mc.itemsByName[item];
if (!type) throw new Error('Unknown item: ' + item);
const total = () => bot.inventory.items().filter(i => i.type === type.id).reduce((n, i) => n + i.count, 0);
const cursor = () => (bot.currentWindow || bot.inventory).selectedItem || bot.inventory.selectedItem;
const settleCursor = async () => {
  await sleep(150);
  signal.throwIfAborted();
  for (let i = 0; i < 10; i++) {
    if (!cursor()) return;
    const window = bot.currentWindow || bot.inventory;
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
  if (cursor()) throw new Error('Cursor item could not be stowed');
};
const waitFor = async (predicate, message) => {
  for (let i = 0; i < 30; i++) {
    signal.throwIfAborted();
    if (predicate()) return;
    await sleep(100);
  }
  throw new Error(message);
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
  if (!table) {
    await bot.craft(recipe, 1, null);
    await settleCursor();
  } else {
    let window = null;
    let failure = null;
    try {
      window = await bot.openBlock(table);
      signal.throwIfAborted();
      if (window.inventoryStart !== 10) throw new Error('Unexpected crafting window layout');
      if (window.slots.slice(1, 10).some(Boolean)) throw new Error('Crafting grid is not empty');
      const cells = [];
      if (recipe.inShape) {
        for (let y = 0; y < recipe.inShape.length; y++) {
          const row = recipe.inShape[y];
          for (let x = 0; x < row.length; x++) {
            const ingredient = row[x];
            if (ingredient && ingredient.id >= 0) cells.push({ slot: 1 + y * 3 + x, ingredient });
          }
        }
      } else {
        for (const ingredient of recipe.ingredients || []) {
          if (!ingredient || ingredient.id < 0) continue;
          for (let n = 0; n < (ingredient.count || 1); n++) cells.push({ slot: 1 + cells.length, ingredient });
        }
      }
      if (!cells.length || cells.some(c => c.slot < 1 || c.slot > 9)) throw new Error('Unsupported crafting recipe');
      for (const { slot, ingredient } of cells) {
        signal.throwIfAborted();
        const matches = stack => stack && stack.type === ingredient.id && (ingredient.metadata == null || ingredient.metadata < 0 || stack.metadata === ingredient.metadata);
        let source = -1;
        for (let s = window.inventoryStart; s < window.inventoryEnd; s++) {
          if (matches(window.slots[s])) { source = s; break; }
        }
        if (source < 0) throw new Error('Crafting ingredient missing');
        await bot.clickWindow(source, 0, 0);
        signal.throwIfAborted();
        await bot.clickWindow(slot, 1, 0);
        await settleCursor();
      }
      await waitFor(() => {
        const output = window.slots[0];
        return output && output.type === type.id && output.count === recipe.result.count;
      }, 'Server did not provide crafting output');
      signal.throwIfAborted();
      await bot.clickWindow(0, 0, 0);
      await settleCursor();
    } catch (error) {
      failure = error;
      throw error;
    } finally {
      if (window) {
        try { await bot.closeWindow(window); }
        catch (error) { if (!failure) throw error; }
      }
    }
  }
  crafts++;
  const expected = before + recipe.result.count;
  if (table) {
    await waitFor(() => total() >= expected && !cursor(), 'Crafting produced no complete inventory progress');
    // Check retained inventory after closing, not just transient click predictions.
    for (let i = 0; i < 10; i++) {
      await sleep(100);
      signal.throwIfAborted();
      if (total() < expected || cursor()) throw new Error('Crafting produced no complete retained inventory progress');
    }
  } else {
    for (let i = 0; i < 20 && total() < expected; i++) {
      await sleep(100);
      signal.throwIfAborted();
    }
    if (total() < expected || cursor()) throw new Error('Crafting produced no complete inventory progress');
  }
}
await settleCursor();
if (total() < count) throw new Error('Crafted items missing from inventory');
return { item, count: total(), crafts };
