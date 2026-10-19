import { useEffect, useRef, useState } from 'react';
import type { KeyCooldownSettings } from '../../desktop/contract.generated';
import { DesktopProblem } from '../../desktop/transport';
import { keyCooldownError, keyCooldownService, parseCooldownInteger } from './key-cooldown.model';

export function useKeyCooldown(enabled: boolean, onActivity: (busy: boolean) => void) {
  const [state, setState] = useState<KeyCooldownSettings | null>(null);
  const [weight, setWeight] = useState('10');
  const [automatic, setAutomatic] = useState(true), [threshold, setThreshold] = useState('3'), [seconds, setSeconds] = useState('30'), [rateSeconds, setRateSeconds] = useState('30');
  const [busy, setBusy] = useState(false), [needsRead, setNeedsRead] = useState(false), [error, setError] = useState(''), [notice, setNotice] = useState('');
  const epoch = useRef(0), lock = useRef(false);
  function accept(out: KeyCooldownSettings) {
    setWeight(String(out.weight_percent)); setState(out); setAutomatic(out.enabled); setThreshold(String(out.failure_threshold)); setSeconds(String(out.cooldown_seconds)); setRateSeconds(String(out.rate_limit_seconds));
  }
  useEffect(() => {
    const lifecycle = { epoch, lock };
    const at = ++epoch.current;
    setState(null); setError(''); setNotice(''); setNeedsRead(false);
    lock.current = enabled; setBusy(enabled);
    if (enabled) void keyCooldownService.read().then(out => { if (at === epoch.current) accept(out); }).catch(e => { if (at === epoch.current) setError(keyCooldownError(e)); }).finally(() => {
      if (at === epoch.current) { lock.current = false; setBusy(false); }
    });
    return () => { lifecycle.epoch.current++; lifecycle.lock.current = false; };
  }, [enabled]);
  useEffect(() => { onActivity(busy); return () => onActivity(false); }, [busy, onActivity]);
  const weightPercent = parseCooldownInteger(weight, 100);
  const failureThreshold = parseCooldownInteger(threshold, 100), cooldownSeconds = parseCooldownInteger(seconds, 3600), rateLimitSeconds = parseCooldownInteger(rateSeconds, 3600);
  const valid = weightPercent !== null && failureThreshold !== null && cooldownSeconds !== null && rateLimitSeconds !== null;
  async function run(save: boolean) {
    if (!enabled || lock.current || (save && (!state || needsRead || !valid))) return;
    const at = epoch.current;
    lock.current = true; setBusy(true); setError(''); setNotice('');
    try {
      const out = save ? await keyCooldownService.save({ weight_percent: weightPercent, enabled: automatic, failure_threshold: failureThreshold, cooldown_seconds: cooldownSeconds, rate_limit_seconds: rateLimitSeconds, expected_revision: state!.revision }) : await keyCooldownService.read();
      if (at === epoch.current) { accept(out); setNeedsRead(false); if (save) setNotice('已保存'); }
    } catch (e) {
      if (at === epoch.current) {
        setError(keyCooldownError(e));
        if (save && e instanceof DesktopProblem && ['conflict', 'unknown'].includes(e.code)) setNeedsRead(true);
      }
    } finally { if (at === epoch.current) { lock.current = false; setBusy(false); } }
  }
  return { weight, setWeight, state, automatic, setAutomatic, threshold, setThreshold, seconds, setSeconds, rateSeconds, setRateSeconds, busy, needsRead, error, notice, valid, read: () => run(false), save: () => run(true) };
}
