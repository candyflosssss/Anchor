import { useEffect, useRef, useState } from 'react';
import type { AutostartState } from '../../desktop/contract.generated';
import { DesktopProblem } from '../../desktop/transport';
import { autostartError, autostartService } from './autostart.model';

export function useAutostart(enabled: boolean, onActivity: (busy: boolean) => void) {
  const [state, setState] = useState<AutostartState | null>(null);
  const [pending, setPending] = useState<boolean | null>(null);
  const [busy, setBusy] = useState(false), [needsRead, setNeedsRead] = useState(false);
  const [unsupported, setUnsupported] = useState(false);
  const [error, setError] = useState(''), [notice, setNotice] = useState('');
  const epoch = useRef(0), lock = useRef(false);
  useEffect(() => {
    const lifecycle = { epoch, lock };
    const at = ++epoch.current;
    lock.current = enabled; setBusy(enabled); setState(null); setPending(null); setNeedsRead(false); setUnsupported(false); setError(''); setNotice('');
    if (enabled) void autostartService.read().then(out => { if (at === epoch.current) setState(out); }).catch(e => {
      if (at === epoch.current) {
        if (e instanceof DesktopProblem && e.code === 'unsupported') setUnsupported(true);
        else setError(autostartError(e));
      }
    }).finally(() => {
      if (at === epoch.current) { lock.current = false; setBusy(false); }
    });
    return () => { lifecycle.epoch.current++; lifecycle.lock.current = false; };
  }, [enabled]);
  useEffect(() => { onActivity(busy); return () => onActivity(false); }, [busy, onActivity]);
  async function run(save: boolean) {
    if (!enabled || lock.current || (save && (!state?.supported || unsupported || needsRead || pending === null || (pending && !state.can_enable)))) return;
    const at = epoch.current;
    lock.current = true; setBusy(true); setUnsupported(false); setError(''); setNotice('');
    try {
      const out = save ? await autostartService.save({ enabled: pending, approved: true, expected_revision: state!.revision }) : await autostartService.read();
      if (at === epoch.current) { setState(out); setNeedsRead(false); if (save) setNotice(out.enabled ? '登录项已保存' : '登录项已移除'); }
    } catch (e) {
      if (at === epoch.current) {
        if (e instanceof DesktopProblem && e.code === 'unsupported') setUnsupported(true);
        else setError(autostartError(e));
        if (save && e instanceof DesktopProblem && ['conflict', 'unknown'].includes(e.code)) setNeedsRead(true);
      }
    } finally { if (at === epoch.current) { lock.current = false; setBusy(false); setPending(null); } }
  }
  return { state, pending, setPending, busy, needsRead, unsupported, error, notice, read: () => run(false), save: () => run(true) };
}
