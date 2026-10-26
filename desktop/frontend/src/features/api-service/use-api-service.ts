import { useEffect, useRef, useState } from 'react';
import type { APIServiceWorkspace, IssuedAPIKey } from '../../desktop/contract.generated';
import { apiError, apiInteger, apiService } from './api-service.model';

export function useAPIService(enabled: boolean, onActivity: (busy: boolean) => void) {
  const [data, setData] = useState<APIServiceWorkspace | null>(null);
  const [automatic, setAutomatic] = useState(false), [port, setPort] = useState('17862'), [lan, setLAN] = useState(false);
  const [label, setLabel] = useState(''), [days, setDays] = useState('30');
  const [issueOpen, setIssueOpen] = useState(false), [copyNotice, setCopyNotice] = useState('');
  const copyEpoch = useRef(0);
  const [issued, setIssued] = useState<IssuedAPIKey | null>(null), [revokeTarget, setRevokeTarget] = useState<string | null>(null);
  const [busy, setBusy] = useState(false), [needsRead, setNeedsRead] = useState(false), [error, setError] = useState(''), [notice, setNotice] = useState('');
  const epoch = useRef(0), lock = useRef(false);
  function accept(value: APIServiceWorkspace) { setData(value); setAutomatic(value.settings.enabled); setPort(String(value.settings.port)); setLAN(value.settings.allow_lan); }
  useEffect(() => {
    const lifecycle = { epoch, lock, copyEpoch }, at = ++epoch.current;
    setData(null); setIssueOpen(false); setCopyNotice(''); copyEpoch.current++; setIssued(null); setLabel(''); setRevokeTarget(null); setError(''); setNeedsRead(false); setNotice('');
    lock.current = enabled; setBusy(enabled);
    if (enabled) void apiService.read().then(value => { if (epoch.current === at) accept(value); }).catch(e => { if (epoch.current === at) setError(apiError(e)); }).finally(() => { if (epoch.current === at) { lock.current = false; setBusy(false); } });
    return () => { lifecycle.epoch.current++; lifecycle.copyEpoch.current++; lifecycle.lock.current = false; };
  }, [enabled]);
  useEffect(() => { onActivity(busy); return () => onActivity(false); }, [busy, onActivity]);
  const numberPort = apiInteger(port, 65535, 1), numberDays = apiInteger(days, 3650);
  const validLabel = !!label.trim() && new TextEncoder().encode(label.trim()).length <= 128 && Array.from(label).every(char => { const code = char.charCodeAt(0); return code > 31 && (code < 127 || code > 159); });
  async function run(action: 'read' | 'save' | 'issue' | 'revoke') {
    if (!enabled || lock.current || (action !== 'read' && (!data || needsRead)) || (action === 'save' && numberPort === null) || (action === 'issue' && (!validLabel || numberDays === null)) || (action === 'revoke' && !revokeTarget)) return;
    copyEpoch.current++; setCopyNotice('');
    const at = epoch.current; lock.current = true; setBusy(true); setError(''); setNotice(''); setIssued(null);
    try {
      if (action === 'issue') {
        const out = await apiService.issue({ label: label.trim(), ttl_seconds: numberDays! * 86400 });
        if (at === epoch.current) { setIssued(out); setLabel(''); setData(current => current ? { ...current, keys: [...current.keys, out.access] } : null); }
      } else {
        const out = action === 'save' ? await apiService.save({ enabled: automatic, port: numberPort!, allow_lan: lan, expected_revision: data!.settings.revision }) : action === 'revoke' ? await apiService.revoke(revokeTarget!) : await apiService.read();
        if (at === epoch.current) { accept(out); setNeedsRead(false); setRevokeTarget(null); if (action !== 'read') setNotice(action === 'save' ? '设置已保存。' : 'Key 已撤销。'); }
      }
    } catch (e) { if (at === epoch.current) { setError(apiError(e)); if (action !== 'read') setNeedsRead(true); if (action === 'revoke') setRevokeTarget(null); } }
    finally { if (at === epoch.current) { lock.current = false; setBusy(false); } }
  }
  function openIssue() {
    if (lock.current || !data || needsRead) return;
    setLabel(''); setDays('30'); setIssued(null); setCopyNotice(''); setError(''); setNotice(''); setIssueOpen(true);
  }
  function closeIssue() {
    if (lock.current) return;
    copyEpoch.current++; setIssueOpen(false); setIssued(null); setLabel(''); setDays('30'); setCopyNotice('');
  }
  async function copy(value: string) {
    const at = ++copyEpoch.current;
    try { await navigator.clipboard.writeText(value); if (at === copyEpoch.current) setCopyNotice('已复制'); }
    catch { if (at === copyEpoch.current) setCopyNotice('复制失败，请选中文本手动复制。'); }
  }
  const dirty = !!data && (automatic !== data.settings.enabled || port !== String(data.settings.port) || lan !== data.settings.allow_lan);
  return { data, automatic, setAutomatic, port, setPort, lan, setLAN, label, setLabel, days, setDays, issued, issueOpen, openIssue, closeIssue, copy, copyNotice, dirty, revokeTarget, setRevokeTarget, busy, needsRead, error, notice, validPort: numberPort !== null, validIssue: validLabel && numberDays !== null, read: () => run('read'), save: () => run('save'), issue: () => run('issue'), revoke: () => run('revoke') };
}
