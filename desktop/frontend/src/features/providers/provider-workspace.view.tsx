import { ProviderBalanceCell } from './provider-balance.view';
import type { ProviderBalance } from './use-provider-balances';
import { ChevronRight, Copy, KeyRound, LoaderCircle, MoreHorizontal, Pencil, Play, Plus, Power, RefreshCw, Search, Server, TextCursorInput, Trash2, Unplug } from 'lucide-react';
import type { MouseEvent, ReactNode } from 'react';
import { Button } from '../../components/ui/button';
import { Switch } from '../../components/ui/switch';
import { Input } from '../../components/ui/input';
import { Table, TableBody, TableCaption, TableCell, TableHead, TableHeader, TableRow } from '../../components/ui/table';
import { StatusBadge } from '../../components/shared/status-badge';
import { MenuItem, Popover } from '../../components/shared/popover-menu';
import { HealthPill } from './provider-detail.view';
import { type Provider } from './provider.model';
import { formatAgo, type ProviderSummary } from './provider-summary.model';
import type { ProviderController } from './use-provider-controller';

/** Provider list: health, keys, models and the last test are visible per row; one click opens the full detail page. */
export function ProviderList({ c, summaries, balances, onEdit, onCreate, children }: { c: ProviderController; summaries: ProviderSummary[]; balances: Record<string, ProviderBalance>; onEdit: (p: Provider) => void; onCreate: () => void; children?: ReactNode }) {
  const enabled = c.providers.filter(p => p.enabled).length;
  const byId = new Map(summaries.map(s => [s.provider.id, s]));
  const rowClick = (e: MouseEvent, id: string) => { if (!(e.target as HTMLElement).closest('button, input, a, select, label')) c.openProvider(id); };
  return <div className="workspace">
    <header className="page-header"><div><div className="eyebrow">配置管理</div><h1>供应商</h1></div><Button disabled={c.blocked} onClick={onCreate}><Plus />添加供应商</Button></header>
    {children}
    <div className="list-toolbar"><div className="filter-tabs" role="group" aria-label="供应商状态筛选">{([['all', '全部', c.providers.length], ['enabled', '已启用', enabled], ['disabled', '已停用', c.providers.length - enabled]] as const).map(([value, label, count]) => <button key={value} aria-pressed={c.filter === value} disabled={c.busy} onClick={() => c.setFilter(value)}>{label}<span>{c.phase === 'ready' ? count : '—'}</span></button>)}</div>
      <div className="toolbar-actions"><div className="search-input"><Search size={15} /><Input disabled={c.busy} aria-label="搜索供应商名称或地址" placeholder="搜索名称或地址…" value={c.search} onChange={event => c.setSearch(event.target.value)} /></div><Button variant="outline" size="icon" disabled={c.busy || !!c.editor || c.phase === 'loading'} onClick={() => void c.load()} aria-label="刷新供应商列表"><RefreshCw /></Button></div>
    </div>
    <section className="provider-surface" aria-label="供应商列表">
      <div className="table-scroll"><Table><TableCaption className="sr-only">{c.mock ? '模拟供应商列表；健康状态来自本地 Key 与测试记录' : '供应商列表；不回传密钥，健康状态来自本地 Key 与测试记录'}</TableCaption><TableHeader><TableRow>
        <TableHead scope="col">供应商 / 请求地址</TableHead><TableHead scope="col">启用状态</TableHead><TableHead scope="col">健康</TableHead><TableHead scope="col">余额 / 额度</TableHead><TableHead scope="col">Key</TableHead><TableHead scope="col">模型</TableHead><TableHead scope="col" className="col-wide">最近测试</TableHead><TableHead scope="col" className="text-right">操作</TableHead>
      </TableRow></TableHeader><TableBody>
        {c.phase === 'ready' && c.visible.map(p => { const s = byId.get(p.id); const loaded = !!s?.workspace; return <TableRow key={p.id} className="clickable-row" onClick={e => rowClick(e, p.id)}>
          <TableCell><div className="provider-cell"><span className="provider-avatar" aria-hidden="true">{Array.from(p.id)[0]}</span><div><button className="provider-name" disabled={c.blocked} onClick={() => c.openProvider(p.id)}>{p.id}</button><span className="provider-url mono" title={p.base_urls.join('\n')}>{p.base_urls[0]}{p.base_urls.length > 1 ? ` +${p.base_urls.length - 1}` : ''}</span></div></div></TableCell>
          <TableCell><label className="key-toggle" data-state={p.enabled ? 'checked' : 'unchecked'}><Switch className="key-toggle-switch" checked={p.enabled} disabled={c.blocked || !!c.toggle || !!c.dialog || !!c.editor} aria-label={`供应商 ${p.id} 启用状态`} onCheckedChange={() => c.requestToggle(p)} /><span>{p.enabled ? '已启用' : '已停用'}</span></label></TableCell>
          <TableCell>{!p.enabled ? <StatusBadge enabled={false} /> : s ? <HealthPill tone={s.tone} label={s.label} /> : '—'}</TableCell>
          <TableCell><ProviderBalanceCell value={balances[p.id]} /></TableCell>
          <TableCell><span className="secret-status"><KeyRound size={14} />{!loaded ? (p.has_secret ? '已配置' : '—') : s!.keyCount ? `${s!.enabledKeys} / ${s!.keyCount}` : '匿名'}</span></TableCell>
          <TableCell className="text-sm">{loaded ? s!.modelCount || <span className="cell-hint">未映射</span> : '—'}</TableCell>
          <TableCell className="col-wide">{s?.lastTest ? <span className={s.lastTest.failure ? 'cell-hint text-destructive' : 'cell-hint'}>{s.lastTest.failure ? '失败 · ' : ''}{formatAgo(s.lastTest.at)}</span> : <span className="cell-hint">{loaded ? '未测试' : '—'}</span>}</TableCell>
          <TableCell className="text-right"><div className="row-actions">
            <Button variant="ghost" size="icon" disabled={c.blocked} title="测试连通" aria-label={`测试 ${p.id}`} onClick={() => c.openQuickTest(p.id)}><Play /></Button>
            <Button variant="ghost" size="icon" disabled={c.blocked} title="编辑配置" aria-label={`编辑 ${p.id}`} onClick={() => onEdit(p)}><Pencil /></Button>
            <Popover menu label={`${p.id} 的更多操作`} disabled={c.blocked} trigger={props => <Button variant="ghost" size="icon" title="更多操作" aria-label={`更多操作 ${p.id}`} {...props}><MoreHorizontal /></Button>}>
              <MenuItem onSelect={() => c.requestToggle(p)}><Power size={14} />{p.enabled ? '停用' : '启用'}</MenuItem>
              <MenuItem onSelect={() => c.openProvider(p.id, 'overview', { kind: 'copy' })}><Copy size={14} />复制为新供应商</MenuItem>
              <MenuItem onSelect={() => c.openProvider(p.id, 'overview', { kind: 'rename' })}><TextCursorInput size={14} />改名</MenuItem>
              <MenuItem destructive onSelect={() => c.openProvider(p.id, 'overview', { kind: 'delete' })}><Trash2 size={14} />删除</MenuItem>
            </Popover>
            <ChevronRight className="row-chevron" size={16} aria-hidden="true" />
          </div></TableCell>
        </TableRow>; })}
      </TableBody></Table></div>
      {c.phase === 'loading' && <div className="empty-state" role="status"><LoaderCircle className="loading-icon" size={28} /><h2>正在读取供应商</h2><p>加载当前实例的配置…</p></div>}
      {c.phase === 'error' && <div className="empty-state"><Unplug size={32} /><h2>暂时无法展示供应商</h2></div>}
      {c.phase === 'ready' && !c.visible.length && <div className="empty-state"><Server size={32} /><h2>{c.providers.length ? '没有匹配的供应商' : '添加你的第一个供应商'}</h2><Button variant="outline" disabled={c.blocked} onClick={() => { if (c.providers.length) { c.setSearch(''); c.setFilter('all'); } else onCreate(); }}>{c.providers.length ? '清除筛选' : '添加供应商'}</Button></div>}
      {c.phase === 'ready' && c.providers.length > 0 && <footer className="table-footer"><span>显示 {c.visible.length} / {c.providers.length} 个供应商</span></footer>}
    </section>
  </div>;
}
