import { useEffect, useRef, useState, type PointerEvent as ReactPointerEvent } from 'react';
import { moveRoute } from './model-routing.model';
type Drag = { id: string; pointer: number; from: number; to: number; x: number; y: number; startX: number; startY: number; offsetX: number; offsetY: number; width: number; targets: { index: number; x: number; y: number }[]; active: boolean; scroll: HTMLElement | null; scrollTop: number };
const scrollParent = (node: HTMLElement): HTMLElement | null => {
  for (let p = node.parentElement; p; p = p.parentElement) if (p.scrollHeight > p.clientHeight && /auto|scroll/.test(getComputedStyle(p).overflowY)) return p;
  return null;
};
/** Pointer capture works in WebViews and touch; no write occurs until release. */
export function useRouteDrag(ids: string[], enabled: boolean, moved: (from: number, to: number) => void) {
  const [drag, setDrag] = useState<Drag | null>(null), [announcement, setAnnouncement] = useState('');
  const current = useRef<Drag | null>(null), grip = useRef<HTMLElement | null>(null);
  const signature = JSON.stringify(ids);
  function cancel() { current.current = null; setDrag(null); }
  useEffect(() => {
    if (!enabled || (current.current && !ids.includes(current.current.id))) { current.current = null; setDrag(null); }
  }, [enabled, signature]); // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => {
    const ref = current;
    const escape = (e: KeyboardEvent) => { if (e.key === 'Escape' && ref.current) { ref.current = null; setDrag(null); setAnnouncement('已取消排序'); } };
    window.addEventListener('keydown', escape);
    return () => { ref.current = null; window.removeEventListener('keydown', escape); };
  }, []);
  function start(e: ReactPointerEvent<HTMLButtonElement>, id: string) {
    if (!enabled || e.button !== 0 || current.current) return;
    const row = e.currentTarget.closest<HTMLElement>('.mapping-key-card'), list = row?.closest<HTMLElement>('.candidate-list');
    if (!row || !list) return;
    const box = row.getBoundingClientRect(), scroll = scrollParent(row);
    const next: Drag = { id, pointer: e.pointerId, from: ids.indexOf(id), to: ids.indexOf(id), x: e.clientX, y: e.clientY, startX: e.clientX, startY: e.clientY, offsetX: e.clientX - box.left, offsetY: e.clientY - box.top, width: box.width, targets: Array.from(list.querySelectorAll<HTMLElement>('.mapping-key-card')).map(n => { const b = n.getBoundingClientRect(); return { index: ids.indexOf(n.closest<HTMLElement>('.mapping-candidate')!.dataset.routeId!), x: (b.left + b.right) / 2, y: (b.top + b.bottom) / 2 }; }), active: false, scroll, scrollTop: scroll?.scrollTop ?? 0 };
    list.setPointerCapture(e.pointerId); grip.current = e.currentTarget; current.current = next;
    e.currentTarget.focus(); e.preventDefault();
  }
  function move(e: ReactPointerEvent<HTMLElement>) {
    const v = current.current; if (!v || e.pointerId !== v.pointer) return;
    if (!v.active && Math.hypot(e.clientX - v.startX, e.clientY - v.startY) < 6) return;
    if (v.scroll) {
      const b = v.scroll.getBoundingClientRect();
      if (e.clientY < b.top + 48) v.scroll.scrollTop -= 14;
      else if (e.clientY > b.bottom - 48) v.scroll.scrollTop += 14;
    }
    const y = e.clientY + (v.scroll ? v.scroll.scrollTop - v.scrollTop : 0);
    // Cards wrap across columns; any Key card targets its containing route.
    const to = v.targets.reduce((best, target) => Math.hypot(target.x - e.clientX, target.y - y) < Math.hypot(best.x - e.clientX, best.y - y) ? target : best).index;
    const next = { ...v, x: e.clientX, y: e.clientY, to, active: true }; current.current = next; setDrag(next);
  }
  function end(e: ReactPointerEvent<HTMLElement>) {
    const v = current.current; if (!v || e.pointerId !== v.pointer) return;
    cancel();
    if (v.active && v.from !== v.to && enabled) { moved(v.from, v.to); setAnnouncement(`已移至第 ${v.to + 1} 位，正在保存`); }
    if (e.currentTarget.hasPointerCapture(e.pointerId)) e.currentTarget.releasePointerCapture(e.pointerId);
    grip.current?.focus();
  }
  return { drag, announcement, order: drag?.active ? moveRoute(ids, drag.from, drag.to) : ids, start, move, end, cancel };
}
