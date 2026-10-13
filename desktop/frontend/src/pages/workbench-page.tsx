import { useEffect, useRef } from 'react';
import { SlidersHorizontal } from 'lucide-react';
import { ConfirmAction } from '../components/shared/confirm-action';
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
import { useProviderSummaries } from '../features/providers/use-provider-summaries';

/** Overview, providers and models share one controller so navigating between them keeps state and data. */
export function WorkbenchPage({ environment, page, navSeq, onNavigate, onActivity }: {
  environment: ProviderEnvironment; page: Exclude<Page, 'settings' | 'logs'>; navSeq: number; onNavigate: (page: Page) => void; onActivity: (busy: boolean) => void;
}) {
  const c = useProviderController(environment);
  const summaries = useProviderSummaries(c.providers, c.phase === 'ready', environment.transport, c.features.data);
  const balances = useProviderBalances(c.providers, c.phase === 'ready' && page === 'providers' && !c.selected, environment.transport);
  const rates = useMappingRates(c.providers, c.phase === 'ready' && page === 'models', environment.transport);
  const opener = useRef<HTMLElement | null>(null);
  const busy = rates.busy || c.busy || !!c.editor || !!c.toggle || !!c.dialog;
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
    {page === 'overview' ? <Overview summaries={summaries} loading={loading} disabled={c.blocked} onOpen={open} onCreate={() => { onNavigate('providers'); edit(); }} onShowProviders={() => onNavigate('providers')} onShowModels={() => onNavigate('models')}>{banner}</Overview>
      : page === 'models' ? <ModelsView rates={rates} summaries={summaries} loading={loading} disabled={c.blocked} onOpen={open} onShowProviders={() => onNavigate('providers')}>{banner}</ModelsView>
      : c.selected && !c.quickId ? <div className="workspace">{banner}<Notice text={c.features.notice || c.notice} /><ProviderDetail c={c} summary={summary} onEdit={() => edit(c.selected ?? undefined)} /></div>
      : <ProviderList c={c} summaries={summaries} balances={balances} onEdit={edit} onCreate={() => edit()}>{banner}<Notice text={c.notice} /></ProviderList>}
    {prototype && c.mock && <div className="workspace pt-0"><footer className="prototype-controls mt-4"><div><SlidersHorizontal size={14} /><label htmlFor="scenario">原型场景</label><select id="scenario" value={c.scenario} disabled={busy} onChange={event => c.changeScenario(event.target.value as Scenario)}>{Object.entries(scenarios).map(([value, label]) => <option key={value} value={value}>{label}</option>)}</select></div></footer></div>}
    {c.editor && <ProviderEditor c={c} restoreFocus={() => { requestAnimationFrame(() => { if (opener.current?.isConnected) opener.current.focus(); }); }} />}
    <ProviderDialogs c={c} />
    <ConfirmAction open={!!c.toggle} title={`${c.toggle?.enabled ? '停用' : '启用'} ${c.toggle?.id ?? ''}？`} busy={c.busy} onCancel={c.cancelToggle} onConfirm={c.confirmToggle}>
      <p>目标：<strong>{c.toggle?.id}</strong> · 当前 revision {c.toggle?.revision}</p><p>{c.toggle?.enabled ? '此供应商将不再参与新的候选路由选择。配置和已存密钥会保留；不保证终止已在途请求。' : '此供应商将允许被候选路由选用，但不保证上游可用。'}</p><p className="mt-2">{c.mock ? '本次为模拟操作，不会修改真实实例。' : '本次将修改本机实例的真实配置。'}</p>
    </ConfirmAction>
  </>;
}
