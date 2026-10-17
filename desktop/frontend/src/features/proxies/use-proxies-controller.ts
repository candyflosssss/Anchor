import { useCallback, useEffect, useRef, useState } from 'react';
import { prototype } from '../../desktop/environment';
import { proxyError, proxyModel, type ProxyAction, type ProxyWorkspace } from './proxies.model';
export function useProxiesController(onActivity?: (busy: boolean) => void) {
  const [data, setData] = useState<ProxyWorkspace | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [needsRead, setNeedsRead] = useState(false);
  const generation = useRef(0);
  const active = useRef(false);
  const refresh = useCallback(async () => {
    if (active.current || prototype) return;
    active.current = true; const seq = ++generation.current; setBusy(true); onActivity?.(true);
    try { const result = await proxyModel.read(); if (seq === generation.current) { setData(result); setError(''); setNeedsRead(false); } }
    catch (e) { if (seq === generation.current) { setError(proxyError(e)); setNeedsRead(true); } }
    finally { if (seq === generation.current) { active.current = false; setBusy(false); onActivity?.(false); } }
  }, [onActivity]);
  const cancel = useCallback(() => {
    generation.current++;
    active.current = false;
    onActivity?.(false);
  }, [onActivity]);
  useEffect(() => { void refresh(); return cancel; }, [refresh, cancel]);
  const save = async (input: ProxyAction) => {
    if (active.current || prototype || needsRead) return false;
    active.current = true; const seq = ++generation.current; setBusy(true); onActivity?.(true);
    try { const result = await proxyModel.action(input); if (seq !== generation.current) return false; setData(result); setError(''); return true; }
    catch (e) { if (seq === generation.current) { setError(proxyError(e)); setNeedsRead(true); } return false; }
    finally { if (seq === generation.current) { active.current = false; setBusy(false); onActivity?.(false); } }
  };
  return { data, busy, error, needsRead, refresh, save, disabled: busy || needsRead || prototype || !data };
}
