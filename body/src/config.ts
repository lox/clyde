import type { Region } from './safety.js'

export interface Config {
  host: string
  port: number
  version: string
  username: string
  auth: 'offline' | 'microsoft'
  profilesFolder: string
  listenHost: string
  listenPort: number
  /** Radius in blocks for entity and block scans in the state card. */
  scanRadius: number
  /** Hard cap on blocks broken in any rolling 60 seconds, by any path. */
  maxBreaksPerMin: number
  /** Regions where the bot never breaks or places blocks. */
  protectedRegions: Region[]
  /** Players whose "<bot name> stop" chat message halts the bot. */
  ops: string[]
}

function env(name: string, fallback: string): string {
  const v = process.env[name]
  return v === undefined || v === '' ? fallback : v
}

function num(name: string, fallback: number): number {
  const raw = env(name, String(fallback))
  const n = Number(raw)
  if (!Number.isFinite(n)) throw new Error(`${name} must be a number, got ${JSON.stringify(raw)}`)
  return n
}

export function loadConfig(): Config {
  const auth = env('MC_AUTH', 'offline')
  if (auth !== 'offline' && auth !== 'microsoft') {
    throw new Error(`MC_AUTH must be "offline" or "microsoft", got ${JSON.stringify(auth)}`)
  }
  return {
    host: env('MC_HOST', '127.0.0.1'),
    port: num('MC_PORT', 25565),
    // Pinned: mineflayer 4.39.0 tops out at 26.1. Do not bump without checking.
    version: env('MC_VERSION', '26.1'),
    username: env('MC_USERNAME', 'Clyde'),
    auth,
    profilesFolder: env('MC_PROFILES_DIR', './profiles'),
    listenHost: env('BODY_LISTEN_HOST', '127.0.0.1'),
    listenPort: num('BODY_LISTEN_PORT', 7777),
    scanRadius: num('BODY_SCAN_RADIUS', 24),
    maxBreaksPerMin: num('BODY_MAX_BREAKS_PER_MIN', 60),
    protectedRegions: parseRegions(env('BODY_PROTECTED_REGIONS', '[]')),
    ops: env('BODY_OPS', '')
      .split(',')
      .map((s) => s.trim())
      .filter(Boolean),
  }
}

/**
 * BODY_PROTECTED_REGIONS is a JSON array of
 * {"name": "spawn", "min": [x, y, z], "max": [x, y, z], "dimension": "overworld"}.
 * Bounds are inclusive block coordinates; dimension is optional.
 */
export function parseRegions(raw: string): Region[] {
  const fail = (why: string): never => {
    throw new Error(`BODY_PROTECTED_REGIONS: ${why}`)
  }
  let parsed: unknown
  try {
    parsed = JSON.parse(raw)
  } catch (err) {
    return fail(`not JSON: ${(err as Error).message}`)
  }
  if (!Array.isArray(parsed)) return fail('must be a JSON array')
  const triple = (v: unknown, what: string) => {
    if (!Array.isArray(v) || v.length !== 3 || !v.every((n) => typeof n === 'number' && Number.isFinite(n))) {
      fail(`${what} must be [x, y, z]`)
    }
    const [x, y, z] = v as number[]
    return { x, y, z }
  }
  return parsed.map((r, i) => {
    if (typeof r !== 'object' || r === null) return fail(`region ${i} must be an object`)
    const { name, min, max, dimension } = r as Record<string, unknown>
    const a = triple(min, `region ${i} min`)
    const b = triple(max, `region ${i} max`)
    return {
      name: typeof name === 'string' && name ? name : `region ${i}`,
      min: { x: Math.min(a.x, b.x), y: Math.min(a.y, b.y), z: Math.min(a.z, b.z) },
      max: { x: Math.max(a.x, b.x), y: Math.max(a.y, b.y), z: Math.max(a.z, b.z) },
      ...(typeof dimension === 'string' && dimension && { dimension }),
    }
  })
}
