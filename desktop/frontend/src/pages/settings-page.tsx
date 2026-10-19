import { CodexSettings } from '../features/codex/codex-settings.view';
import type { Appearance } from '../app/use-appearance';
import { AppearanceControl } from '../components/shared/appearance-control';
import { UpdateSettings } from '../features/updates/update-settings.view';

export function SettingsPage({ appearance, setAppearance, coreReady, onActivity }: {
  coreReady: boolean; appearance: Appearance; setAppearance: (value: Appearance) => void; onActivity: (busy: boolean) => void;
}) {
  return <div className="workspace settings-workspace">
    <header className="page-header"><h1>设置</h1></header>
    <section className="detail-card" aria-labelledby="appearance-title">
      <header className="card-toolbar"><h2 id="appearance-title">外观</h2></header>
      <AppearanceControl labelled value={appearance} onChange={setAppearance} />
    </section>
    <CodexSettings coreReady={coreReady} onActivity={onActivity} />
    <UpdateSettings onActivity={onActivity} />
  </div>;
}
