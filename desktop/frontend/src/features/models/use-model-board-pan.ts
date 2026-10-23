import { useRef, useState, type KeyboardEvent, type PointerEvent } from 'react';

/** Local viewport movement only; route cards retain their own sorting gesture. */
export function useModelBoardPan() {
  const current = useRef<{ pointer: number; x: number; y: number; left: number } | null>(null);
  const [panning, setPanning] = useState(false);

  function end(e: PointerEvent<HTMLElement>) {
    if (current.current?.pointer !== e.pointerId) return;
    current.current = null;
    setPanning(false);
    if (e.currentTarget.hasPointerCapture(e.pointerId)) e.currentTarget.releasePointerCapture(e.pointerId);
  }

  return {
    panning,
    onPointerDown(e: PointerEvent<HTMLElement>) {
      if (e.button !== 0 || !e.isPrimary || current.current || e.currentTarget.scrollWidth <= e.currentTarget.clientWidth) return;
      if ((e.target as HTMLElement).closest('button, a, input, select, textarea, [role="dialog"], [role="alertdialog"], .mapping-key-card')) return;
      current.current = { pointer: e.pointerId, x: e.clientX, y: e.clientY, left: e.currentTarget.scrollLeft };
      e.currentTarget.setPointerCapture(e.pointerId);
      e.currentTarget.focus({ preventScroll: true });
    },
    onPointerMove(e: PointerEvent<HTMLElement>) {
      const start = current.current;
      if (!start || start.pointer !== e.pointerId) return;
      const dx = e.clientX - start.x, dy = e.clientY - start.y;
      if (!panning && Math.abs(dx) < 6) return;
      if (!panning && Math.abs(dy) > Math.abs(dx)) { end(e); return; }
      e.preventDefault();
      setPanning(true);
      e.currentTarget.scrollLeft = start.left - dx;
    },
    onPointerUp: end,
    onPointerCancel: end,
    onLostPointerCapture: end,
    onKeyDown(e: KeyboardEvent<HTMLElement>) {
      if (e.target !== e.currentTarget) return;
      const board = e.currentTarget;
      if (e.key === 'ArrowLeft' || e.key === 'ArrowRight') {
        e.preventDefault(); board.scrollLeft += e.key === 'ArrowLeft' ? -324 : 324;
      } else if (e.key === 'Home' || e.key === 'End') {
        e.preventDefault(); board.scrollLeft = e.key === 'Home' ? 0 : board.scrollWidth;
      }
    },
  };
}
