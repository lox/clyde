// Generated from api/state.schema.json by body/scripts/gen-types.mjs. Do not edit.

/**
 * GET /state: the one observation the rest of the system sees. Distances are in blocks, times in epoch milliseconds unless named *Ms.
 */
export interface StateCard {
  /**
   * When the card was built.
   */
  ts: number
  bot: {
    name: string
    /**
     * For example overworld, the_nether, the_end.
     */
    dimension: string
    position: Vec
    yaw: number
    pitch: number
    health: number
    food: number
    oxygen: number
    xpLevel: number
    gameMode: string
    onGround: boolean
    inWater: boolean
    inLava: boolean
  }
  world: {
    timeOfDay: number
    phase: 'day' | 'dusk' | 'night' | 'dawn'
    raining: boolean
    thundering: boolean
  }
  /**
   * The running action, or null when idle.
   */
  action: ActionStatus | null
  /**
   * The most recently finished action, or null.
   */
  lastAction: ActionStatus | null
  inventory: {
    /**
     * Occupied slots.
     */
    used: number
    total: number
    held: string | null
    /**
     * Item totals across slots, largest count first.
     */
    items: {
      name: string
      count: number
    }[]
  }
  /**
   * Nearby entities within the scan radius, nearest first, at most eight per group.
   */
  entities: {
    players: NearbyEntity[]
    hostiles: NearbyEntity[]
    passives: NearbyEntity[]
    items: NearbyEntity[]
  }
  /**
   * Notable blocks within the scan radius, nearest first.
   */
  blocks: {
    name: string
    count: number
    nearest: number
    nearestPos: Vec
  }[]
  /**
   * Recent events, oldest first. GET /events has the full log.
   */
  events: {
    ageMs: number
    type: string
    text: string
  }[]
}
export interface Vec {
  x: number
  y: number
  z: number
}
export interface ActionStatus {
  id: number
  type: 'goto' | 'follow' | 'collect' | 'dig' | 'place' | 'chat' | 'eval'
  /**
   * The request without its type.
   */
  params: {
    [k: string]: unknown
  }
  status: 'running' | 'succeeded' | 'failed' | 'aborted'
  startedAt: number
  endedAt?: number
  elapsedMs: number
  /**
   * What the action is doing right now, for models.
   */
  progress?: string
  /**
   * goto and follow: blocks to the target.
   */
  distanceRemaining?: number
  /**
   * goto and follow: when the bot last moved at least half a block.
   */
  lastMovedAt?: number
  /**
   * collect: blocks broken so far.
   */
  collected?: number
  /**
   * Action-specific result on success.
   */
  result?: {
    [k: string]: unknown
  }
  error?: string
}
export interface NearbyEntity {
  /**
   * Username for players, entity type otherwise.
   */
  name: string
  distance: number
  direction: 'here' | 'N' | 'NE' | 'E' | 'SE' | 'S' | 'SW' | 'W' | 'NW'
  /**
   * Height difference, positive when above the bot.
   */
  dy: number
  position: Vec
  health?: number
}
