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
}

function env(name: string, fallback: string): string {
  const v = process.env[name]
  return v === undefined || v === '' ? fallback : v
}

export function loadConfig(): Config {
  const auth = env('MC_AUTH', 'offline')
  if (auth !== 'offline' && auth !== 'microsoft') {
    throw new Error(`MC_AUTH must be "offline" or "microsoft", got ${JSON.stringify(auth)}`)
  }
  return {
    host: env('MC_HOST', '127.0.0.1'),
    port: Number(env('MC_PORT', '25565')),
    version: env('MC_VERSION', '26.1'),
    username: env('MC_USERNAME', 'BonnieBot'),
    auth,
    profilesFolder: env('MC_PROFILES_DIR', './profiles'),
    listenHost: env('BODY_LISTEN_HOST', '127.0.0.1'),
    listenPort: Number(env('BODY_LISTEN_PORT', '7777')),
    scanRadius: Number(env('BODY_SCAN_RADIUS', '24')),
  }
}
