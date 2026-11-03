// Loaded by cancellation.go through /query in a disposable body. Instrumentation
// only: real plugins remain installed. Restart the body between cases.
const p = bot.cancellationProbe = { events: [], phase: 'setup', released: false };
p.mark = (kind, data = {}) => p.events.push({ kind, at: Date.now(), ...JSON.parse(JSON.stringify(data)) });
p.gate = async (phase) => {
  p.phase = phase;
  p.mark('blocked', { phase });
  await new Promise((resolve, reject) => { p.release = (fail = false) => {
    p.released = true;
    p.mark('released', { fail });
    fail ? reject(new Error('injected late failure')) : resolve();
  }; });
};
p.profile = () => {
  const m = new Movements(bot);
  m.canDig = false;
  m.scafoldingBlocks = [];
  m.allow1by1towers = false;
  m.canOpenDoors = false;
  m.allowParkour = true;
  return m;
};
const write = bot._client.write;
bot._client.write = function(name, data) {
  if (['block_dig', 'block_place', 'window_click', 'held_item_slot'].includes(name)) {
    p.mark('packet', { name, data, goal: bot.pathfinder.goal, revoked: !!p.revoked });
  }
  const result = write.call(this, name, data);
  // The real inventory implementation waits before a hotbar click when a dig
  // was recent. Set that clock after the source click, not before original
  // equip. Its private clickWindow/moveSlotItem continuation is still real.
  if (p.mode === 'equip' && name === 'window_click' && !p.equipPaused) {
    p.equipPaused = true;
    bot.lastDigTime = Date.now();
    p.phase = 'equip';
    p.mark('blocked', { phase: 'equip-recent-dig-clock' });
  }
  return result;
};
const setControlState = bot.setControlState;
bot.setControlState = function(name, value) {
  if (value && p.retreatAt && !p.commandedAt) {
    p.commandedAt = Date.now();
    p.mark('first-command', { name });
  }
  return setControlState.call(this, name, value);
};
bot.on('physicsTick', () => {
  if (p.retreatAt && !p.observedAt && bot.entity.position.distanceTo(p.anchor) > 0.2) {
    p.observedAt = Date.now();
    p.mark('first-observed', { position: bot.entity.position });
  }
});
const lookAt = bot.lookAt;
bot.lookAt = async (...args) => {
  // Delay completion *inside* real dig/place, after the real look has run.
  await lookAt.apply(bot, args);
  if (p.delayLook) {
    p.delayLook = false;
    await p.gate('look');
  }
};
const dig = bot.dig;
bot.dig = async (...args) => {
  p.mark('dig-enter', { position: args[0].position });
  try { return await dig.apply(bot, args); }
  finally { p.mark('dig-settled'); }
};
const collect = bot.collectBlock.collect;
bot.collectBlock.collect = async (...args) => {
  p.mark('collect-enter');
  try { return await collect.apply(bot.collectBlock, args); }
  finally { p.mark('collect-settled'); }
};
p.retreat = async (signal, goto) => {
  signal.throwIfAborted();
  p.retreatAt = Date.now();
  p.anchor = bot.entity.position.clone();
  p.mark('handoff');
  bot.pathfinder.setMovements(p.profile());
  await goto(new goals.GoalNear(-12, 80, 0, 1));
  p.mark('arrived');
};
p.run = async (mode, signal, goto, prepareOnly = false) => {
  p.mode = mode;
  const block = () => bot.blockAt(new Vec3(0, 80, -4));
  if (mode.startsWith('path-')) {
    const movementOnly = mode.endsWith('-movement');
    if (movementOnly) bot.pathfinder.setMovements(p.profile());
    else {
      if (mode === 'path-place') {
        const m = new Movements(bot); m.canDig = false; m.allowParkour = false;
        bot.pathfinder.setMovements(m);
      }
      const equip = bot.equip;
      bot.equip = async (...args) => { await p.gate('path-equip'); return equip.apply(bot, args); };
    }
    await goto(new goals.GoalBlock(5, 80, 0));
  } else if (mode === 'dig-look' || mode === 'place-look') {
    if (mode === 'place-look') await bot.equip(bot.inventory.items().find(i => i.name === 'dirt'), 'hand');
    p.delayLook = true;
    if (mode === 'dig-look') await bot.dig(block());
    else await bot.placeBlock(bot.blockAt(new Vec3(1, 79, 0)), new Vec3(0, 1, 0));
  } else if (mode === 'equip') {
    await bot.equip(bot.inventory.items().find(i => i.name === 'diamond_axe'), 'hand');
    p.mark('equip-settled');
  } else if (mode === 'active-dig') {
    p.phase = 'active-dig';
    await bot.dig(block(), 'ignore');
  } else if (mode === 'collector-profile') {
    bot.pathfinder.setMovements(p.profile());
    bot.collectBlock.movements = p.profile();
    if (prepareOnly) return;
    await bot.collectBlock.collect([block()]);
    p.mark('profile-result', { block: block()?.name, sameProfile: bot.pathfinder.movements === bot.collectBlock.movements });
  } else if (mode === 'collector-drop' || mode === 'collector-pickup' || mode === 'collector-error') {
    // Native collector cancellation probes retain canDig=true: canDig=false
    // also rejects *explicit* harvesting in this plugin (tested above).
    if (mode === 'collector-error') {
      const equip = bot.tool.equipForBlock;
      bot.tool.equipForBlock = async (...args) => { await equip.apply(bot.tool, args); await p.gate('collector-error'); };
    }
    if (mode === 'collector-drop') {
      const emit = bot.emit;
      p.resumeTicks = () => { p.holdTicks = false; };
      bot.emit = function(name, ...args) {
        if (name === 'physicsTick' && p.holdTicks) return false;
        return emit.call(this, name, ...args);
      };
      bot.once('diggingCompleted', () => { p.holdTicks = true; p.phase = 'collector-drop'; });
    }
    if (prepareOnly) return;
    if (mode === 'collector-pickup') {
      const go = bot.pathfinder.goto;
      bot.pathfinder.goto = async goal => {
        await go.call(bot.pathfinder, goal);
        if (goal.constructor.name === 'GoalFollow') { p.phase = 'collector-pickup'; p.mark('pickup-wait'); }
      };
      await bot.collectBlock.collect(Object.values(bot.entities).find(e => e.name === 'item'));
    } else await bot.collectBlock.collect([block()]);
  } else if (mode.startsWith('local-')) {
    // One foreground fixture, no arming API or independent action owner. Only
    // movement-only navigation and explicit harvesting are used. Await every
    // entered primitive; cancellation checks separate them. The local trigger
    // revokes collection, not the ActionRunner owner that will retreat.
    bot.pathfinder.setMovements(p.profile());
    bot.collectBlock.movements = p.profile();
    p.trigger = () => {
      if (p.revoked) return;
      p.revoked = Date.now();
      p.mark('trigger');
      bot.pathfinder.setGoal(null);
      if (bot.targetDigBlock) bot.stopDigging();
      bot.clearControlStates();
      p.mark('revocation');
    };
    const check = () => { signal.throwIfAborted(); if (p.revoked) throw new Error('collection revoked'); };
    try {
      if (mode === 'local-navigation') {
        p.phase = 'navigation';
        await goto(new goals.GoalNear(0, 80, -4, 2));
      } else {
        if (mode === 'local-equip') {
          // Same real multi-click boundary as the primitive probe.
          p.mode = 'equip';
          await bot.equip(bot.inventory.items().find(i => i.name === 'diamond_axe'), 'hand');
          p.mark('equip-settled');
          check();
        }
        if (mode === 'local-look' || mode === 'local-stuck') p.delayLook = true;
        await bot.lookAt(block().position.offset(.5, .5, .5), true);
        check();
        // No await between the cancellation check and Mineflayer installing
        // stopDigging: 'ignore' deliberately removes its internal look await.
        p.phase = 'active-dig';
        await bot.dig(block(), 'ignore');
        check();
        p.phase = 'drop';
        await bot.waitForTicks(10);
        check();
      }
    } catch (err) {
      p.mark('collection-ended', { error: String(err) });
      if (!p.revoked) throw err;
    }
    signal.throwIfAborted();
    if (p.revoked && Date.now() - p.revoked > 500) {
      p.mark('handoff-expired');
      throw new Error('collection did not yield within the 500ms handoff allowance');
    }
    if (p.revoked) await p.retreat(signal, goto);
  }
  p.phase = 'done';
};
return true;
