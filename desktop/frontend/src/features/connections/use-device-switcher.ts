import { useCallback, useEffect, useRef, useState } from 'react';
import type { ProblemCode, SessionInfo } from '../../desktop/contract.generated';
import { DesktopProblem } from '../../desktop/transport';
import { connections, type ConnectionProfile } from './connection.model';

/** Lives outside the popover so dismissing it cannot discard a successful switch. */
export function useDeviceSwitcher(onConnected: (info: SessionInfo) => void, onActivity: (busy: boolean) => void) {
  const [profiles, setProfiles] = useState<ConnectionProfile[]>([]);
  const [loading, setLoading] = useState(false);
  const [pending, setPending] = useState<string | null>(null);
  const [error, setError] = useState<ProblemCode | null>(null);
  const alive = useRef(true), lock = useRef(false), generation = useRef(0);
  useEffect(() => {
    alive.current = true;
    return () => { alive.current = false; };
  }, []);
  const refresh = useCallback(async () => {
    const epoch = ++generation.current;
    setLoading(true); setError(null);
    try {
      const rows = await connections.list();
      if (alive.current && epoch === generation.current) setProfiles(rows);
    } catch (e) {
      if (alive.current && epoch === generation.current) setError(e instanceof DesktopProblem ? e.code : 'read_error');
    } finally {
      if (alive.current && epoch === generation.current) setLoading(false);
    }
  }, []);
  async function connect(name: string, password = '') {
    if (lock.current) return false;
    lock.current = true; generation.current++;
    setLoading(false); setPending(name); setError(null); onActivity(true);
    try {
      const info = await connections.use(name, password);
      if (!alive.current) return false;
      onConnected(info);
      return true;
    } catch (e) {
      if (alive.current) setError(e instanceof DesktopProblem ? e.code : 'read_error');
      return false;
    } finally {
      lock.current = false;
      if (alive.current) { setPending(null); onActivity(false); }
    }
  }
  return { profiles, loading, pending, error, refresh, connect };
}
