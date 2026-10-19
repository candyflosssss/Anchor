import { useEffect, useRef, useState } from 'react';
import type { CodexSettingsState } from '../../desktop/contract.generated';
import { DesktopProblem } from '../../desktop/transport';
import { codexSettingsError, codexSettingsService, parseRetries } from './codex-settings.model';

export function useCodexSettings(enabled: boolean, onActivity: (busy: boolean) => void) {
  const [state, setState] = useState<CodexSettingsState | null>(null);
  const [draft, setDraft] = useState('5'), [busy, setBusy] = useState(false), [error, setError] = useState(''), [notice, setNotice] = useState(''), [needsRead, setNeedsRead] = useState(false);
  const epoch = useRef(0), lock = useRef(false);
  useEffect(() => {
    const lifecycle = { epoch, lock };
    const at = ++epoch.current;
    setState(null); setError(''); setNotice(''); setNeedsRead(false);
    lock.current = enabled; setBusy(enabled);
    if (enabled) void codexSettingsService.read().then(out => {
      if (at === epoch.current) { setState(out); setDraft(String(out.settings.retries)); }
    }).catch(e => { if (at === epoch.current) setError(codexSettingsError(e)); }).finally(() => {
      if (at === epoch.current) { lock.current = false; setBusy(false); }
    });
    return () => { lifecycle.epoch.current++; lifecycle.lock.current = false; };
  }, [enabled]);
  useEffect(() => { onActivity(busy); return () => onActivity(false); }, [busy, onActivity]);
  async function run(save: boolean) {
    const retries = parseRetries(draft);
    if (!enabled || lock.current || (save && (!state || needsRead || retries === null))) return;
    const at = epoch.current;
    lock.current = true; setBusy(true); setError(''); setNotice('');
    try {
      const out = save ? await codexSettingsService.save({ retries, expected_revision: state!.settings.revision }) : await codexSettingsService.read();
      if (at === epoch.current) {
        setState(out); setDraft(String(out.settings.retries)); setNeedsRead(false);
        if (save) setNotice(out.needs_sync.length ? '重试次数已保存，部分 Codex 配置尚未同步。' : '重试次数已保存并同步，请重启 Codex 后生效。');
      }
    } catch (e) {
      if (at === epoch.current) {
        setError(codexSettingsError(e));
        if (save && e instanceof DesktopProblem && ['conflict', 'unknown'].includes(e.code)) setNeedsRead(true);
      }
    } finally { if (at === epoch.current) { lock.current = false; setBusy(false); } }
  }
  return { state, draft, setDraft, busy, error, notice, needsRead, valid: parseRetries(draft) !== null, read: () => run(false), save: () => run(true) };
}
