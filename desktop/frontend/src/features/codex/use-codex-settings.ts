import { useEffect, useRef, useState } from 'react';
import type { CodexSettingsState } from '../../desktop/contract.generated';
import { DesktopProblem } from '../../desktop/transport';
import { codexSettingsError, codexSettingsNotice, codexSettingsService, parseCompactPercent, parseRetries } from './codex-settings.model';

export function useCodexSettings(enabled: boolean, onActivity: (busy: boolean) => void) {
  const [state, setState] = useState<CodexSettingsState | null>(null);
  const [draft, setDraft] = useState('5'), [busy, setBusy] = useState(false), [error, setError] = useState(''), [notice, setNotice] = useState(''), [needsRead, setNeedsRead] = useState(false);
  const [autoCompact, setAutoCompact] = useState(true), [compactPercent, setCompactPercent] = useState('90');
  const epoch = useRef(0), lock = useRef(false);
  useEffect(() => {
    const lifecycle = { epoch, lock };
    const at = ++epoch.current;
    setState(null); setError(''); setNotice(''); setNeedsRead(false);
    lock.current = enabled; setBusy(enabled);
    if (enabled) void codexSettingsService.read().then(out => {
      if (at === epoch.current) { setState(out); setDraft(String(out.settings.retries)); setAutoCompact(out.settings.auto_compact); setCompactPercent(String(out.settings.compact_percent)); }
    }).catch(e => { if (at === epoch.current) setError(codexSettingsError(e)); }).finally(() => {
      if (at === epoch.current) { lock.current = false; setBusy(false); }
    });
    return () => { lifecycle.epoch.current++; lifecycle.lock.current = false; };
  }, [enabled]);
  useEffect(() => { onActivity(busy); return () => onActivity(false); }, [busy, onActivity]);
  async function run(save: boolean) {
    const retries = parseRetries(draft), percent = parseCompactPercent(compactPercent);
    if (!enabled || lock.current || (save && (!state || needsRead || retries === null || percent === null))) return;
    const at = epoch.current;
    lock.current = true; setBusy(true); setError(''); setNotice('');
    try {
      const out = save ? await codexSettingsService.save({ retries, auto_compact: autoCompact, compact_percent: percent, expected_revision: state!.settings.revision }) : await codexSettingsService.read();
      if (at === epoch.current) {
        setState(out); setDraft(String(out.settings.retries)); setAutoCompact(out.settings.auto_compact); setCompactPercent(String(out.settings.compact_percent)); setNeedsRead(false);
        if (save) setNotice(codexSettingsNotice(out));
      }
    } catch (e) {
      if (at === epoch.current) {
        setError(codexSettingsError(e));
        if (save && e instanceof DesktopProblem && ['conflict', 'unknown'].includes(e.code)) setNeedsRead(true);
      }
    } finally { if (at === epoch.current) { lock.current = false; setBusy(false); } }
  }
  return { state, draft, setDraft, autoCompact, setAutoCompact, compactPercent, setCompactPercent, busy, error, notice, needsRead, valid: parseRetries(draft) !== null && parseCompactPercent(compactPercent) !== null, read: () => run(false), save: () => run(true) };
}
