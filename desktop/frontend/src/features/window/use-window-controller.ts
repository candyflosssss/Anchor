import { useEffect, useRef, useState, type MouseEvent } from 'react';
import { windowMessages, windowModel } from './window.model';

export function useWindowController(enabled: boolean) {
  const [maximised, setMaximised] = useState<boolean | null>(null);
  const [error, setError] = useState<string | null>(null);
  const scheduleRefresh = useRef<() => void>(() => {});
  const available = enabled && windowModel.available();

  useEffect(() => {
    if (!enabled) return;
    let disposed = false;
    let inFlight = false;
    let refreshAgain = false;
    let timer: ReturnType<typeof setTimeout> | undefined;
    async function refresh() {
      if (disposed || document.hidden) return;
      if (inFlight) { refreshAgain = true; return; }
      inFlight = true;
      try {
        const next = await windowModel.isMaximised();
        if (!disposed) { setMaximised(next); setError(null); }
      } catch {
        if (!disposed) { setMaximised(null); setError(windowMessages.failed); }
      } finally {
        inFlight = false;
        if (!disposed && refreshAgain) { refreshAgain = false; schedule(); }
      }
    }
    function schedule() {
      clearTimeout(timer);
      timer = setTimeout(() => { void refresh(); }, 120);
    }
    scheduleRefresh.current = schedule;
    void refresh();
    window.addEventListener('resize', schedule);
    window.addEventListener('focus', schedule);
    document.addEventListener('visibilitychange', schedule);
    return () => {
      disposed = true;
      clearTimeout(timer);
      scheduleRefresh.current = () => {};
      window.removeEventListener('resize', schedule);
      window.removeEventListener('focus', schedule);
      document.removeEventListener('visibilitychange', schedule);
    };
  }, [enabled]);

  function invoke(action: () => void) {
    if (!enabled) return;
    try { action(); scheduleRefresh.current(); }
    catch { setError(windowMessages.failed); }
  }
  function toggleMaximise() { invoke(windowModel.toggleMaximise); }
  function onDragDoubleClick(event: MouseEvent<HTMLElement>) {
    // CSS is also what Wails uses for hit testing; never toggle from a button.
    if (!enabled || !available || event.button !== 0 || !(event.target instanceof Element)) return;
    if (getComputedStyle(event.target).getPropertyValue('--wails-draggable').trim() !== 'drag') return;
    event.preventDefault();
    toggleMaximise();
  }
  return {
    available, maximised,
    error: enabled ? available ? error : windowMessages.unavailable : null,
    minimise: () => invoke(windowModel.minimise),
    toggleMaximise,
    close: () => invoke(windowModel.close),
    onDragDoubleClick,
  };
}

export type WindowController = ReturnType<typeof useWindowController>;
