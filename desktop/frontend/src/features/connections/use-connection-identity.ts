import { useEffect, useState } from 'react';
import type { SessionInfo } from '../../desktop/contract.generated';
import { prototype } from '../../desktop/environment';
import { connections, type ConnectionProfile } from './connection.model';

export type ConnectionIdentity = Pick<ConnectionProfile, 'name' | 'transport' | 'target'>;

/** A saved target identifies the device; it never authorizes lifecycle actions. */
export function useConnectionIdentity(session: SessionInfo | null, phase: string): ConnectionIdentity | null {
  const [selected, setSelected] = useState<ConnectionIdentity | null>(null);
  useEffect(() => {
    if (prototype || session || phase === 'starting') return;
    let canceled = false;
    void connections.list().then(rows => {
      // Older hosts do not expose selected; do not guess from the first row or active.
      const targets = rows.filter(row => row.selected === true);
      if (!canceled) setSelected(targets.length === 1 ? targets[0] : null);
    }).catch(() => { if (!canceled) setSelected(null); });
    return () => { canceled = true; };
  }, [session, phase]);
  return session ? { name: session.profile_name, transport: session.transport, target: session.target } : selected;
}
