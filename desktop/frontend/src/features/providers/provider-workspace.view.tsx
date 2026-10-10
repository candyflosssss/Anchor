import { ArrowRight, Check, CircleHelp, KeyRound, LoaderCircle, Pencil, Plus, RefreshCw, Search, Server, SlidersHorizontal, Unplug } from 'lucide-react';
import { useRef } from 'react';
import { Button } from '../../components/ui/button';
import { Input } from '../../components/ui/input';
import { Table, TableBody, TableCaption, TableCell, TableHead, TableHeader, TableRow } from '../../components/ui/table';
import { ConfirmAction } from '../../components/shared/confirm-action';
import { StatusBadge } from '../../components/shared/status-badge';
import { ProviderEditor } from './provider-editor.view';
import { ProviderDetail } from './provider-detail.view';
import { scenarios, type Scenario } from './provider.mock';
import { prototype } from '../../desktop/environment';
import type { ProviderController } from './use-provider-controller';
import type { Provider } from './provider.model';
export function ProviderWorkspace({ c }: { c: ProviderController }) {
  const opener = useRef<HTMLElement | null>(null);
  const open = (source?: Provider) => { opener.current = document.activeElement as HTMLElement; c.openEditor(source); };
  const enabled = c.providers.filter(p => p.enabled).length;
  return <div className="workspace">
    <header className="page-header"><div><div className="eyebrow">配置管理</div><h1>供应商</h1><p>管理上游服务、API 地址与访问凭据。</p></div><Button disabled={c.blocked} onClick={() => open()}><Plus />添加供应商</Button></header>
    <div className="scope-note"><Server size={15} /><span><strong>本机实例</strong><span className="note-separator">/</span>{c.mock ? "示例配置，仅在本次预览内生效" : "配置保存到本机核心，CLI 与 GUI 共用"}</span><span className="prototype-pill">{c.mock ? "Mock 交互原型" : "本机连接"}</span></div>
    {c.notice && <div className="feedback success-feedback" role="status"><Check size={16} /><span>{c.notice}</span></div>}
    {c.problem && <div className="feedback error-feedback" role="alert"><CircleHelp size={17} /><div><strong>{c.problem === 'unknown' ? '写入结果未知' : c.problem === 'conflict' ? '配置版本冲突' : c.problem === 'offline' ? '实例不可用' : c.problem === 'unauthorized' ? '管理授权不可用' : c.problem === 'read_error' ? '读取供应商失败' : '配置未通过校验'}</strong><p>{c.problemMessage}</p></div><Button variant="outline" disabled={c.busy || !!c.editor} onClick={() => void c.load()}>重新读取配置</Button></div>}
    <div className="list-toolbar"><div className="filter-tabs" role="group" aria-label="供应商状态筛选">{([['all', '全部', c.providers.length], ['enabled', '已启用', enabled], ['disabled', '已停用', c.providers.length - enabled]] as const).map(([value, label, count]) => <button key={value} aria-pressed={c.filter === value} onClick={() => c.setFilter(value)}>{label}<span>{c.phase === 'ready' ? count : '—'}</span></button>)}</div>
      <div className="toolbar-actions"><div className="search-input"><Search size={15} /><Input aria-label="搜索供应商名称或地址" placeholder="搜索名称或地址…" value={c.search} onChange={event => c.setSearch(event.target.value)} /></div><Button variant="outline" size="icon" disabled={c.busy || !!c.editor || c.phase === 'loading'} onClick={() => void c.load()} aria-label="刷新供应商列表"><RefreshCw /></Button></div>
    </div>
    <section className={`provider-surface ${c.selected ? 'has-detail' : ''}`} aria-label="供应商列表与详情">
      <div className="list-region">
        <div className="table-scroll"><Table><TableCaption className="sr-only">{c.mock ? "模拟供应商配置列表；不含真实密钥或上游健康数据" : "供应商配置列表；不回传密钥，不含上游健康数据"}</TableCaption><TableHeader><TableRow><TableHead scope="col">供应商 / API 地址</TableHead><TableHead scope="col">协议</TableHead><TableHead scope="col">凭据</TableHead><TableHead scope="col">状态</TableHead><TableHead scope="col" className="text-right">操作</TableHead></TableRow></TableHeader><TableBody>
          {c.phase === 'ready' && c.visible.map(p => <TableRow key={p.id} className={c.selected?.id === p.id ? 'row-selected' : ''}>
            <TableCell><div className="provider-cell"><span className="provider-avatar" aria-hidden="true">{Array.from(p.id)[0]}</span><div><button className="provider-name" onClick={() => c.select(p.id)} aria-expanded={c.selected?.id === p.id}>{p.id}<ArrowRight size={13} /></button><span className="provider-url mono" title={p.base_url}>{p.base_url}</span></div></div></TableCell>
            <TableCell><span className="protocol-tag">Responses</span></TableCell><TableCell><span className="secret-status"><KeyRound size={14} />{p.has_secret ? '已配置' : '未配置'}</span></TableCell><TableCell><StatusBadge enabled={p.enabled} /></TableCell>
            <TableCell className="text-right"><Button variant="ghost" size="icon" disabled={c.blocked} onClick={() => open(p)} aria-label={`编辑 ${p.id}`}><Pencil /></Button></TableCell>
          </TableRow>)}
        </TableBody></Table></div>
        {c.phase === 'loading' && <div className="empty-state" role="status"><LoaderCircle className="loading-icon" size={28} /><h2>正在读取供应商</h2><p>加载当前实例的配置…</p></div>}
        {c.phase === 'error' && <div className="empty-state"><Unplug size={32} /><h2>暂时无法展示供应商</h2><p>数据不可用不代表没有配置。请检查实例状态后重新读取。</p></div>}
        {c.phase === 'ready' && !c.visible.length && <div className="empty-state"><Server size={32} /><h2>{c.providers.length ? '没有匹配的供应商' : '添加你的第一个供应商'}</h2><p>{c.providers.length ? '尝试其他关键词，或重置状态筛选。' : '从一个 API 根地址开始，统一管理你的上游服务。'}</p><Button variant={c.providers.length ? 'outline' : 'default'} onClick={() => { if (c.providers.length) { c.setSearch(''); c.setFilter('all'); } else open(); }}>{c.providers.length ? '清除筛选' : '添加供应商'}</Button></div>}
        {c.phase === 'ready' && c.providers.length > 0 && <footer className="table-footer"><span>显示 {c.visible.length} / {c.providers.length} 个供应商</span><span>仅配置数据 · 无健康探测</span></footer>}
      </div>
      {c.selected && <ProviderDetail provider={c.selected} blocked={c.blocked} onClose={() => c.select(null)} onEdit={() => open(c.selected ?? undefined)} onToggle={() => c.selected && c.requestToggle(c.selected)} />}
    </section>
    <div className="capability-note"><CircleHelp size={16} /><p>当前版本仅支持 <strong>Responses</strong> 协议。停用请先选择供应商查看详情；删除、余额与连通性测试不在本版范围内。</p></div>
    {prototype && c.mock && <footer className="prototype-controls"><div><SlidersHorizontal size={14} /><label htmlFor="scenario">原型场景</label><select id="scenario" value={c.scenario} disabled={!!c.editor || c.busy || !!c.toggle} onChange={event => c.changeScenario(event.target.value as Scenario)}>{Object.entries(scenarios).map(([value, label]) => <option key={value} value={value}>{label}</option>)}</select></div><span>所有数据均为模拟 · 刷新页面将重置</span></footer>}
    {c.editor && <ProviderEditor c={c} restoreFocus={() => { requestAnimationFrame(() => { if (opener.current?.isConnected) opener.current.focus(); }); }} />}
    <ConfirmAction open={!!c.toggle} title={`${c.toggle?.enabled ? '停用' : '启用'} ${c.toggle?.id ?? ''}？`} busy={c.busy} onCancel={c.cancelToggle} onConfirm={c.confirmToggle}>
      <p>目标：<strong>{c.toggle?.id}</strong> · 当前 revision {c.toggle?.revision}</p><p>{c.toggle?.enabled ? '此供应商将不再参与新的候选路由选择。配置和已存密钥会保留；不保证终止已在途请求。' : '此供应商将允许被候选路由选用，但不保证上游可用。'}</p><p className="mt-2">{c.mock ? "本次为模拟操作，不会修改真实实例。" : "本次将修改本机实例的真实配置。"}</p>
    </ConfirmAction>
  </div>;
}
