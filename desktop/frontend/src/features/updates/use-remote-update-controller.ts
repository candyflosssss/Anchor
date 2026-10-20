import { useEffect, useRef, useState } from 'react';
import type { UpdateInfo } from '../../desktop/contract.generated';
import { updates } from './update.model';
import { message } from './use-update-controller';

export function useRemoteUpdateController(instance: string, onActivity: (busy: boolean) => void) {
  const [info, setInfo] = useState<UpdateInfo | null>(null);
  const [operation, setOperation] = useState('');
  const [error, setError] = useState('');
  const [confirm, setConfirm] = useState(false);
  const alive = useRef(true);
  const latch = useRef(false);
  useEffect(() => { alive.current = true; return () => { alive.current = false; }; }, []);
  const busy = operation !== '';
  useEffect(() => { onActivity(busy); return () => onActivity(false); }, [busy, onActivity]);
  async function run(name: string, action: () => Promise<UpdateInfo>) {
    if (latch.current) return;
    latch.current = true; setOperation(name); setError('');
    try { const result = await action(); if (alive.current) setInfo(result); }
    catch (e) { if (alive.current) { setError(message(e)); setInfo(null); } }
    finally { latch.current = false; if (alive.current) { setOperation(''); setConfirm(false); } }
  }
  return { info, busy, error, operation, confirm, setConfirm,
    check: () => run('checking', () => updates.remoteCheck(instance)),
    install: () => info && run('installing', () => updates.remoteInstall(instance, info.latest_version)) };
}
