import { useCallback, useEffect, useRef, useState } from 'react';
import type { ProblemCode, SessionInfo } from '../../desktop/contract.generated';
import { DesktopProblem } from '../../desktop/transport';
import { connections, emptySSH, passwordSaveWarning, type ConnectionProfile, type SSHConnectionInput } from './connection.model';

export function useConnectionController(onConnected: (info: SessionInfo) => void, onActivity: (busy: boolean) => void) {
  const [profiles, setProfiles] = useState<ConnectionProfile[]>([]);
  const [draft, setDraft] = useState<SSHConnectionInput>({ ...emptySSH });
  const [passwords, setPasswords] = useState<Record<string,string>>({});
  const [remember, setRemember] = useState<Record<string, boolean>>({});
  const [notice, setNotice] = useState<string | null>(null);
 const [usePassword, setUsePassword] = useState(false);
 const [loading, setLoading] = useState(true);
  const [pending, setPending] = useState<string | null>(null);
  const [error, setError] = useState<ProblemCode | null>(null);
  const lock = useRef(false), alive = useRef(true), epoch = useRef(0);
  const refresh = useCallback(async () => {
    const generation = ++epoch.current;
    setLoading(true);
    try { const rows = await connections.list(); if (alive.current && generation === epoch.current) setProfiles(rows); }
    catch (e) { if (alive.current && generation === epoch.current) setError(e instanceof DesktopProblem ? e.code : 'read_error'); }
    finally { if (alive.current && generation === epoch.current) setLoading(false); }
  }, []);
  useEffect(() => { alive.current = true; void refresh(); return () => { alive.current = false; }; }, [refresh]);
  useEffect(() => { onActivity(pending !== null); return () => onActivity(false); }, [pending, onActivity]);
  async function connect(name?: string) {
    if (lock.current) return;
    lock.current = true; setPending(name ?? 'new'); setError(null); setNotice(null); epoch.current++;
    const password = name ? (passwords[name] ?? '') : draft.password;
 if (name) setPasswords(p => ({ ...p, [name]: '' })); else setDraft(p => ({ ...p, password: '' }));
 try {
      const result = name ? await connections.use(name, password, !!remember[name] && !!password) : await connections.add({ ...draft, remember_password: usePassword && !!draft.remember_password, password: usePassword ? password : '', name: draft.name.trim(), target: draft.target.trim(), identity_file: draft.identity_file.trim(), remote_executable: draft.remote_executable.trim() });
      if (alive.current) { onConnected(result.info); setNotice(result.passwordWarning ? passwordSaveWarning : null); if (!name) setDraft({ ...emptySSH }); else setRemember(p => ({ ...p, [name]: false })); await refresh(); }
    } catch (e) { if (alive.current) setError(e instanceof DesktopProblem ? e.code : 'read_error'); }
    finally { lock.current = false; if (alive.current) setPending(null); }
  }
  async function forget(name: string) {
    if (lock.current) return;
    lock.current = true; epoch.current++; setPending(`forget:${name}`); setError(null); setNotice(null);
    try {
      const rows = await connections.forget(name);
      if (alive.current) { setProfiles(rows); setRemember(p => ({ ...p, [name]: false })); setNotice('已忘记保存的密码。当前连接保持不变，下次连接需重新输入密码。'); }
    } catch (e) { if (alive.current) setError(e instanceof DesktopProblem ? e.code : 'unknown'); }
    finally { lock.current = false; if (alive.current) setPending(null); }
  }
  return { passwords, setPasswords, remember, setRemember, notice, usePassword, setUsePassword, profiles, draft, setDraft, loading, pending, error, refresh, connect, forget };
}
