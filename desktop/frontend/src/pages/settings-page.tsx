import { useCallback, useRef } from 'react';
import { KeyCooldownSettings } from '../features/key-cooldown/key-cooldown.view';
import { CodexSettings } from '../features/codex/codex-settings.view';
import type { Appearance } from '../app/use-appearance';
import { AppearanceControl } from '../components/shared/appearance-control';
import type { SessionInfo } from '../desktop/contract.generated';
import { RemoteUpdateSettings } from '../features/updates/remote-update-settings.view';
import { UpdateSettings } from '../features/updates/update-settings.view';

export function SettingsPage({ appearance, setAppearance, coreReady, session, onActivity }: {
  session?: SessionInfo | null; coreReady: boolean; appearance: Appearance; setAppearance: (value: Appearance) => void; onActivity: (busy: boolean) => void;
}) {
  const activity = useRef({ cooldown: false, codex: false, updates: false, remoteUpdates: false });
  const report = useCallback((key: keyof typeof activity.current, busy: boolean) => {
    activity.current[key] = busy;
    onActivity(Object.values(activity.current).some(Boolean));
  }, [onActivity]);
  const cooldownActivity = useCallback((busy: boolean) => report('cooldown', busy), [report]);
  const codexActivity = useCallback((busy: boolean) => report('codex', busy), [report]);
  const updateActivity = useCallback((busy: boolean) => report('updates', busy), [report]);
  const remoteUpdateActivity = useCallback((busy: boolean) => report('remoteUpdates', busy), [report]);
  return <div className="workspace settings-workspace">
    <header className="page-header"><h1>设置</h1></header>
    <section className="detail-card" aria-labelledby="appearance-title">
      <header className="card-toolbar"><h2 id="appearance-title">外观</h2></header>
      <AppearanceControl labelled value={appearance} onChange={setAppearance} />
    </section>
    <KeyCooldownSettings coreReady={coreReady} onActivity={cooldownActivity} />
    <CodexSettings coreReady={coreReady} onActivity={codexActivity} />
    <UpdateSettings onActivity={updateActivity} />
    {session?.transport === 'ssh' && <RemoteUpdateSettings key={session.instance_id} session={session} onActivity={remoteUpdateActivity} />}
  </div>;
}
