/**
 * Ring buffer of things that happened to the bot. The harness reads it with
 * GET /events?since=<seq>, and the state card includes the most recent ones.
 */
export interface BodyEvent {
  seq: number
  ts: number
  type: string
  text: string
  data?: Record<string, unknown>
}

export class EventLog {
  private events: BodyEvent[] = []
  private nextSeq = 1

  constructor(private readonly capacity = 500) {}

  push(type: string, text: string, data?: Record<string, unknown>): BodyEvent {
    const event: BodyEvent = { seq: this.nextSeq++, ts: Date.now(), type, text, data }
    this.events.push(event)
    if (this.events.length > this.capacity) {
      this.events.splice(0, this.events.length - this.capacity)
    }
    return event
  }

  /** Events with seq greater than `since`, oldest first. */
  since(since: number): BodyEvent[] {
    return this.events.filter((e) => e.seq > since)
  }

  /** The newest `limit` events that are at most `maxAgeMs` old, oldest first. */
  recent(limit: number, maxAgeMs: number, now = Date.now()): BodyEvent[] {
    const cutoff = now - maxAgeMs
    const out: BodyEvent[] = []
    for (let i = this.events.length - 1; i >= 0 && out.length < limit; i--) {
      const e = this.events[i]
      if (e.ts < cutoff) break
      out.push(e)
    }
    return out.reverse()
  }

  get latestSeq(): number {
    return this.nextSeq - 1
  }
}
