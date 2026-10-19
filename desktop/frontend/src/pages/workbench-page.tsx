import { useModelRouting } from '../features/models/use-model-routing';
import { aggregateModels } from '../features/providers/provider-summary.model';
import { useEffect, useRef } from 'react';
import { SlidersHorizontal } from 'lucide-react';
import type { Page } from '../components/shared/app-sidebar';
import type { ProviderEnvironment } from '../desktop/environment';
import { prototype } from '../desktop/environment';
import { useMappingRates } from '../features/models/use-mapping-rates';
import { ModelsView } from '../features/models/models.view';
import { Overview } from '../features/overview/overview.view';
import { ProviderDetail } from '../features/providers/provider-detail.view';
import { ProviderDialogs } from '../features/providers/provider-dialogs.view';
import { ProviderEditor } from '../features/providers/provider-editor.view';
import { Notice, ProblemBanner } from '../features/providers/provider-feedback.view';
import { ProviderList } from '../features/providers/provider-workspace.view';
import { scenarios, type Scenario } from '../features/providers/provider.mock';
import type { Provider } from '../features/providers/provider.model';
import type { DetailTab, ProviderDialog } from '../features/providers/provider-summary.model';
import { useProviderController } from '../features/providers/use-provider-controller';
import { useProviderBalances } from '../features/providers/use-provider-balances';
import type { ProviderBalanceCache } from '../features/providers/provider-balance.model';
import { useProviderSummaries } from '../features/providers/use-provider-summaries';

/** Overview, providers and models share one controller so navigating between them keeps state and data. */
export function WorkbenchPage({ environment, balanceCache, page, navSeq, onNavigate, onActivity }: {
  environment: ProviderEnvironment; balanceCache: ProviderBalanceCache; page: Exclude<Page, 'settings' | 'logs'>; navSeq: number; onNavigate: (page: Page) => void; onActivity: (busy: boolean) => void;
}) {
  const c = useProviderController(environment);
  const summaries = useProviderSummaries(c.providers, c.phase === 'ready', environment.transport, c.features.data);
  const balances = useProviderBalances(c.providers, c.phase === 'ready' && page === 'providers' && !c.selected, environment.transport, balanceCache);
  const rates = useMappingRates(c.providers, c.phase === 'ready' && page === 'models', environment.transport);
  const routing = useModelRouting(aggregateModels(summaries), page === 'models', environment.transport, async () => { if (!await c.load()) throw new Error('配置读取失败'); });
  const opener = useRef<HTMLElement | null>(null);
  const busy = routing.busy || rates.busy || c.busy || !!c.editor || !!c.dialog;
  useEffect(() => { onActivity(busy); }, [busy, onActivity]);
  useEffect(() => () => onActivity(false), [onActivity]);
  const selectRef = useRef(c.select);
  selectRef.current = c.select;
  // Sidebar navigation always lands on the list, never a stale detail page.
  useEffect(() => { if (navSeq) selectRef.current(null); }, [navSeq]);
  const edit = (source?: Provider) => { opener.current = document.activeElement as HTMLElement; c.openEditor(source); };
  const open = (id: string, tab?: DetailTab, dialog?: ProviderDialog) => { onNavigate('providers'); c.openProvider(id, tab, dialog); };
  const summary = summaries.find(s => s.provider.id === c.selected?.id);
  const loading = c.phase === 'loading';
  const banner = c.problem && !c.dialog ? <ProblemBanner code={c.problem} message={c.problemMessage} disabled={c.busy || !!c.editor} onReload={() => void c.load()} /> : null;
  return <>
    {page === 'overview' ? <Overview summaries={summaries} loading={loading} readFailed={c.phase === 'error'} disabled={c.blocked} onOpen={open} onCreate={() => { onNavigate('providers'); edit(); }} onShowProviders={() => onNavigate('providers')} onShowModels={() => onNavigate('models')}>{banner}</Overview>
      : page === 'models' ? <ModelsView transport={environment.transport} routing={routing} rates={rates} summaries={summaries} loading={loading} disabled={c.blocked} onOpen={open} onShowProviders={() => onNavigate('providers')}>{banner}</ModelsView>
      : c.selected && !c.quickId ? <div className="workspace">{banner}<Notice text={c.features.notice || c.notice} /><ProviderDetail c={c} summary={summary} onEdit={() => edit(c.selected ?? undefined)} /></div>
      : <ProviderList c={c} summaries={summaries} balances={balances.entries} onRefreshBalance={balances.refresh} onEdit={edit} onCreate={() => edit()}>{banner}<Notice text={c.notice} /></ProviderList>}
    {prototype && c.mock && <div className="workspace pt-0"><footer className="prototype-controls mt-4"><div><SlidersHorizontal size={14} /><label htmlFor="scenario">原型场景</label><select id="scenario" value={c.scenario} disabled={busy} onChange={event => c.changeScenario(event.target.value as Scenario)}>{Object.entries(scenarios).map(([value, label]) => <option key={value} value={value}>{label}</option>)}</select></div></footer></div>}
    {c.editor && <ProviderEditor c={c} restoreFocus={() => { requestAnimationFrame(() => { if (opener.current?.isConnected) opener.current.focus(); }); }} />}
    <ProviderDialogs c={c} />
  </>;
}
