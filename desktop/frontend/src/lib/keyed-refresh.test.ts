import { describe, expect, it } from 'vitest';
import { KeyedRefresh, reconcileEntities, type RefreshTarget } from './keyed-refresh';

const tick = async () => { for (let i = 0; i < 8; i++) await Promise.resolve(); };
function harness() {
  const calls: { target: RefreshTarget; signal: AbortSignal; finish: () => void }[] = [];
  const queue = new KeyedRefresh<RefreshTarget>((target, signal) => new Promise<void>(finish => { calls.push({ target, signal, finish }); }));
  return { queue, calls };
}
const targets = ['a', 'b', 'c'].map(id => ({ id, revision: 1 }));
describe('keyed background refresh', () => {
  it('bounds concurrency and deduplicates unchanged entities, including reads still in flight', async () => {
    const { queue, calls } = harness();
    queue.update(targets); await tick();
    expect(calls.map(c => c.target.id)).toEqual(['a', 'b']);
    queue.update(targets.map(t => ({ ...t }))); queue.refresh('a'); await tick();
    expect(calls).toHaveLength(2);
    calls[0].finish(); await tick();
    expect(calls.map(c => c.target.id)).toEqual(['a', 'b', 'c']);
    calls[1].finish(); calls[2].finish(); await tick();
    queue.update(targets); await tick(); expect(calls).toHaveLength(3);
    queue.dispose();
  });
  it('invalidates only the changed revision and rejects its old result without restarting neighbors', async () => {
    const { queue, calls } = harness();
    queue.update(targets); await tick();
    queue.update([{ id: 'a', revision: 2 }, targets[1]]); await tick();
    expect(calls[0].signal.aborted).toBe(true); expect(calls[1].signal.aborted).toBe(false);
    expect(calls).toHaveLength(2); // A non-abortable host call still occupies a slot.
    calls[0].finish(); await tick();
    expect(calls[2].target).toEqual({ id: 'a', revision: 2 });
    queue.dispose(); calls[1].finish(); calls[2].finish(); await tick();
    expect(calls).toHaveLength(3); // Removed c never starts.
  });
  it('supports per-row retry and explicit whole-list refresh without replaying writes', async () => {
    const { queue, calls } = harness();
    queue.update(targets.slice(0, 2)); await tick();
    calls.forEach(c => c.finish()); await tick();
    queue.refresh('b'); await tick(); expect(calls[2].target.id).toBe('b');
    calls[2].finish(); await tick();
    queue.update(targets.slice(0, 2), 1); await tick();
    expect(calls.slice(3).map(c => c.target.id)).toEqual(['a', 'b']);
    queue.dispose(); calls.slice(3).forEach(c => c.finish());
  });
  it('aborts owned reads and never starts queued work after disposal', async () => {
    const { queue, calls } = harness();
    queue.update(targets); await tick(); queue.dispose();
    expect(calls.every(c => c.signal.aborted)).toBe(true);
    calls.forEach(c => c.finish()); await tick(); expect(calls).toHaveLength(2);
  });
});

describe('snapshot reconciliation', () => {
  it('reuses unchanged row and array identities but applies metadata, removals and authoritative ordering', () => {
    expect(reconcileEntities(targets, targets.map(t => ({ ...t })))).toBe(targets);
    const updated = reconcileEntities(targets, [{ id: 'b', revision: 1 }, { id: 'a', revision: 2 }]);
    expect(updated[0]).toBe(targets[1]); expect(updated[1]).not.toBe(targets[0]);
    expect(updated.map(t => t.id)).toEqual(['b', 'a']);
    const original = [{ id: 'a', revision: 1, enabled: true }];
    expect(reconcileEntities(original, [{ ...original[0], enabled: false }])[0].enabled).toBe(false);
  });
});
