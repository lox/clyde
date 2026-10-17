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

export interface EventPage {
  /** Events with seq greater than the cursor, oldest first. */
  events: BodyEvent[]
  /** Pass this as the next `since`. */
  latestSeq: number
  /** Events after the cursor that were evicted from the buffer before this read. */
  missed: number
}

export class EventLog {
  private events: BodyEvent[] = []
  private nextSeq = 1

  constructor(
    private readonly capacity = 500,
    private readonly now: () => number = Date.now,
  ) {}

  push(type: string, text: string, data?: Record<string, unknown>): BodyEvent {
    const event: BodyEvent = { seq: this.nextSeq++, ts: this.now(), type, text, ...(data && { data }) }
    this.events.push(event)
    if (this.events.length > this.capacity) {
      this.events.splice(0, this.events.length - this.capacity)
    }
    return event
  }

  /**
   * Events after `since`. A cursor ahead of `latestSeq` came from an earlier
   * body process (seq restarts at 1), so it reads from the start.
   */
  since(since: number): EventPage {
    const latestSeq = this.latestSeq
    if (since > latestSeq) since = 0
    const oldest = this.events[0]?.seq ?? this.nextSeq
    return {
      events: this.events.filter((e) => e.seq > since),
      latestSeq,
      missed: Math.max(0, oldest - since - 1),
    }
  }

  /** The newest `limit` events that are at most `maxAgeMs` old, oldest first. */
  recent(limit: number, maxAgeMs: number, now = this.now()): BodyEvent[] {
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
