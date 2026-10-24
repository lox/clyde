// Generated from api/action.schema.json by body/scripts/gen-types.mjs. Do not edit.

/**
 * Body of POST /action. A new action aborts the running one, except immediate non-replacing chat.
 */
export type ActionRequest =
  GotoAction | FollowAction | CollectAction | DigAction | PlaceAction | ChatAction | EvalAction | StopAction

/**
 * Walk until within range blocks of (x, y, z).
 */
export interface GotoAction {
  type: 'goto'
  x: number
  y: number
  z: number
  range?: number
}
/**
 * Keep within range blocks of a player until aborted.
 */
export interface FollowAction {
  type: 'follow'
  player: string
  range?: number
}
/**
 * Find up to count blocks of a type within radius, break them, and pick up the drops.
 */
export interface CollectAction {
  type: 'collect'
  block: string
  count?: number
  radius?: number
}
/**
 * Break the block at (x, y, z), walking within reach first.
 */
export interface DigAction {
  type: 'dig'
  x: number
  y: number
  z: number
}
/**
 * Place a block item from the inventory at (x, y, z), walking within reach first.
 */
export interface PlaceAction {
  type: 'place'
  item: string
  x: number
  y: number
  z: number
}
/**
 * Send chat beside work. Returns its own final receipt; does not replace current or last exclusive-action status.
 */
export interface ChatAction {
  type: 'chat'
  message: string
}
/**
 * Run JavaScript as the body of an async function with the bot in scope.
 */
export interface EvalAction {
  type: 'eval'
  code: string
  timeoutMs?: number
}
/**
 * Abort the running action and stand still. Same as POST /stop.
 */
export interface StopAction {
  type: 'stop'
}
