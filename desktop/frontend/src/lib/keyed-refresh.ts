export interface RefreshTarget { id: string; revision: number }
type Job<T> = { target: T; version: number; abort: AbortController; pending: boolean };

/** Bounded, per-entity reads. Reconciliation never restarts an unrelated in-flight read. */
export class KeyedRefresh<T extends RefreshTarget> {
  private jobs = new Map<string, Job<T>>();
  private queued: Job<T>[] = [];
  private running = 0;
  private disposed = false;
  constructor(private readonly read: (target: T, signal: AbortSignal) => Promise<void>, private readonly concurrency = 2, private readonly onPending?: (target: T) => void) {}

  update(targets: T[], version = 0) {
    if (this.disposed) return;
    const ids = new Set(targets.map(target => target.id));
    for (const [id, job] of this.jobs) if (!ids.has(id)) { job.abort.abort(); this.jobs.delete(id); }
    for (const target of targets) {
      const old = this.jobs.get(target.id);
      if (old?.target.revision === target.revision && old.version === version) continue;
      old?.abort.abort();
      this.enqueue(target, version);
    }
    this.drain();
  }

  refresh(id: string) {
    const job = this.jobs.get(id);
    if (!job || job.pending || this.disposed) return;
    this.enqueue(job.target, job.version);
    this.drain();
  }

  dispose() {
    this.disposed = true;
    for (const job of this.jobs.values()) job.abort.abort();
    this.jobs.clear(); this.queued = [];
  }

  private enqueue(target: T, version: number) {
    const job = { target, version, abort: new AbortController(), pending: true };
    this.jobs.set(target.id, job); this.queued.push(job); this.onPending?.(target);
  }

  private drain() {
    while (!this.disposed && this.running < this.concurrency && this.queued.length) {
      const job = this.queued.shift()!;
      if (job.abort.signal.aborted) continue;
      this.running++;
      // Keep the slot occupied until even a non-abortable bridge read settles.
      void Promise.resolve().then(() => job.abort.signal.aborted ? undefined : this.read(job.target, job.abort.signal))
        .catch(() => { /* The reader owns error presentation. */ })
        .finally(() => { job.pending = false; this.running--; this.drain(); });
    }
  }
}

/** Reuse unchanged public DTOs and the list itself when a reread changes nothing. */
export function reconcileEntities<T extends RefreshTarget>(previous: T[], next: T[]): T[] {
  const byID = new Map(previous.map(item => [item.id, item]));
  const merged = next.map(item => {
    const old = byID.get(item.id);
    return old && JSON.stringify(old) === JSON.stringify(item) ? old : item;
  });
  return merged.length === previous.length && merged.every((item, i) => item === previous[i]) ? previous : merged;
}
