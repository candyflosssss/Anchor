import { createContext, useCallback, useContext, useEffect, useLayoutEffect, useRef, useState, type KeyboardEvent, type ReactNode } from 'react';
import { createPortal } from 'react-dom';
import { cn } from '../../lib/utils';

interface PopoverApi { close: (refocus?: boolean) => void; menu: boolean }
const PopoverContext = createContext<PopoverApi | null>(null);

/**
 * Small anchored popover. `menu` mode adds roving focus over MenuItem children (WAI-ARIA menu button);
 * otherwise it behaves like a non-modal dialog. Closes on outside press, Esc, scroll and resize.
 */
export function Popover({ trigger, label, menu = false, className, children, disabled }: {
  trigger: (props: { ref: (node: HTMLButtonElement | null) => void; onClick: () => void; 'aria-expanded': boolean; 'aria-haspopup': 'menu' | 'dialog'; disabled?: boolean }) => ReactNode;
  label: string; menu?: boolean; className?: string; disabled?: boolean;
  children: ReactNode | ((close: () => void) => ReactNode);
}) {
  const [open, setOpen] = useState(false);
  const anchor = useRef<HTMLButtonElement | null>(null);
  const panel = useRef<HTMLDivElement | null>(null);
  // Focus returns synchronously so a dialog opened by a menu item records the trigger as its restore target.
  const close = useCallback((refocus = true) => { setOpen(false); if (refocus) anchor.current?.focus(); }, []);
  useLayoutEffect(() => {
    const node = panel.current; const a = anchor.current;
    if (!open || !node || !a) return;
    const r = a.getBoundingClientRect(); const w = node.offsetWidth; const h = node.offsetHeight;
    const left = Math.min(Math.max(8, r.right - w), window.innerWidth - w - 8);
    const below = r.bottom + 6; const top = below + h > window.innerHeight - 8 && r.top - h - 6 > 8 ? r.top - h - 6 : below;
    node.style.left = `${left}px`; node.style.top = `${top}px`; node.style.opacity = '1';
    if (menu) node.querySelector<HTMLElement>('[role=menuitem]:not([disabled])')?.focus();
    else node.focus();
  }, [open, menu]);
  useEffect(() => {
    if (!open) return;
    const press = (e: PointerEvent) => { const t = e.target as Node; if (!panel.current?.contains(t) && !anchor.current?.contains(t)) close(false); };
    const scroll = (e: Event) => { if (!panel.current?.contains(e.target as Node)) close(false); };
    const resize = () => close(false);
    document.addEventListener('pointerdown', press, true); window.addEventListener('scroll', scroll, true); window.addEventListener('resize', resize);
    return () => { document.removeEventListener('pointerdown', press, true); window.removeEventListener('scroll', scroll, true); window.removeEventListener('resize', resize); };
  }, [open, close]);
  function keys(e: KeyboardEvent<HTMLDivElement>) {
    if (e.key === 'Escape') { e.stopPropagation(); e.preventDefault(); close(); return; }
    if (!menu) return;
    if (e.key === 'Tab') { close(false); return; }
    const items = [...(panel.current?.querySelectorAll<HTMLElement>('[role=menuitem]:not([disabled])') ?? [])];
    const at = items.indexOf(document.activeElement as HTMLElement);
    const next = e.key === 'ArrowDown' ? (at + 1) % items.length : e.key === 'ArrowUp' ? (at - 1 + items.length) % items.length : e.key === 'Home' ? 0 : e.key === 'End' ? items.length - 1 : -1;
    if (next >= 0 && items.length) { e.preventDefault(); items[next].focus(); }
  }
  return <>
    {trigger({ ref: node => { anchor.current = node; }, onClick: () => setOpen(v => !v), 'aria-expanded': open, 'aria-haspopup': menu ? 'menu' : 'dialog', disabled })}
    {open && createPortal(<PopoverContext.Provider value={{ close, menu }}>
      <div ref={panel} role={menu ? 'menu' : 'dialog'} aria-label={label} tabIndex={-1} onKeyDown={keys} className={cn('popover-panel', menu && 'popover-menu', className)} style={{ position: 'fixed', zIndex: 30, opacity: 0, top: 0, left: 0 }}>
        {typeof children === 'function' ? children(() => close()) : children}
      </div>
    </PopoverContext.Provider>, document.body)}
  </>;
}

export function MenuItem({ children, onSelect, disabled, destructive }: { children: ReactNode; onSelect: () => void; disabled?: boolean; destructive?: boolean }) {
  const api = useContext(PopoverContext);
  return <button type="button" role="menuitem" tabIndex={-1} disabled={disabled} className={cn('menu-item', destructive && 'menu-item-danger')}
    onClick={() => { api?.close(); onSelect(); }}>{children}</button>;
}
