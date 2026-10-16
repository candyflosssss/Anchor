import { ChevronLeft, ChevronRight, Columns3, Download, FileText, Pause, Play, RefreshCw, Settings2, Terminal } from 'lucide-react';
import { Button } from '../../components/ui/button';
import { Input } from '../../components/ui/input';
import { Dialog, DialogContent, DialogTitle } from '../../components/ui/dialog';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '../../components/ui/table';
import { RequestDetail } from './log-detail.view';
import { dimensionLabels, type LogDimension, statusLabels } from './log.model';
import { useLogController, type LogController } from './use-log-controller';
import { RequestTable } from './log-request-table.view';
import { LogColumnSettings } from './log-columns.view';
import { useLogColumnsController } from './use-log-columns-controller';
import { LogFilterInput } from './log-filter-input.view';
import { LogStatsView } from './log-stats.view';
import './logs.css';
function Filters({ c }: { c: LogController }) {
  return <div className="log-filters">
    <LogFilterInput label="搜索日志" placeholder="搜索" field="search" kind={c.tab} window={c.filters.window} revision={c.revision} value={c.filters.query} onChange={value => c.filter('query', value)} />
    <select aria-label="日志时间范围" className="select-field" value={c.filters.window} onChange={e => c.filter('window', e.target.value)}>
      <option value="15">最近 15 分钟</option><option value="60">最近 1 小时</option><option value="1440">最近 24 小时</option><option value="10080">最近 7 天</option><option value="43200">最近 30 天</option><option value="129600">最近 90 天</option>
    </select>
    {c.tab === 'requests' ? <>
      {(Object.keys(dimensionLabels) as LogDimension[]).map(key => <LogFilterInput key={key} label={`${dimensionLabels[key]}筛选`} placeholder={c.filters[key] === '' ? `${dimensionLabels[key]}：未识别` : dimensionLabels[key]} field={key} kind={c.tab} window={c.filters.window} revision={c.revision} value={c.filters[key] ?? ''} onChange={value => c.filter(key, value || null)} />)}
      <select aria-label="请求结果" className="select-field" value={c.filters.status} onChange={e => c.filter('status', e.target.value)}><option value="all">全部结果</option>{Object.entries(statusLabels).map(([k,v]) => <option key={k} value={k}>{v}</option>)}</select>
      <select aria-label="日志来源" className="select-field" value={c.filters.source} onChange={e => c.filter('source', e.target.value)}><option value="all">全部来源</option><option value="proxy">代理请求</option><option value="probe">推理测试</option></select>
      <LogFilterInput label="供应商筛选" placeholder="供应商" field="provider" kind={c.tab} window={c.filters.window} revision={c.revision} value={c.filters.provider === 'all' ? '' : c.filters.provider} onChange={value => c.filter('provider', value || 'all')} />
      {(['model','key','http','code','stage'] as const).map(k => <LogFilterInput key={k} label={`${k}筛选`} placeholder={{ model:'模型', key:'Key', http:'HTTP', code:'错误码', stage:'阶段' }[k]} field={k} kind={c.tab} window={c.filters.window} revision={c.revision} value={c.filters[k]} onChange={value => c.filter(k,value)} />)}
    </> : <select aria-label="日志级别" className="select-field" value={c.level} onChange={e => { c.setLevel(e.target.value); c.refresh(); }}><option value="all">全部级别</option><option value="info">INFO</option><option value="warn">WARN</option><option value="error">ERROR</option></select>}
    {c.tab === 'requests' && (Object.keys(dimensionLabels) as LogDimension[]).filter(key => c.filters[key] != null).map(key => <Button key={key} variant="outline" size="sm" aria-label={`清除${dimensionLabels[key]}筛选`} onClick={() => c.filter(key, null)}>{dimensionLabels[key]}：{c.filters[key] || '未识别'} ×</Button>)}
    <Button variant="ghost" size="sm" onClick={c.resetFilters}>重置</Button>
  </div>;
}
function SystemTable({ c }: { c: LogController }) {
  return <Table className="log-table"><TableHeader><TableRow>{['时间','级别','来源','事件','数据','请求'].map(k => <TableHead key={k}>{k}</TableHead>)}</TableRow></TableHeader>
    <TableBody>{c.systems.map(row => <TableRow key={row.id}><TableCell className="mono">{row.time}</TableCell><TableCell>{row.level.toUpperCase()}</TableCell><TableCell>{row.source}</TableCell><TableCell className="mono">{row.event}</TableCell><TableCell>{row.message || '—'}</TableCell><TableCell>{row.requestId ? <Button variant="link" onClick={() => void c.findRequest(row.requestId!)}>{row.requestId}</Button> : '—'}</TableCell></TableRow>)}</TableBody></Table>;
}
export function LogsPreview() {
  const c = useLogController(), layout = useLogColumnsController();
  const count = c.tab === 'requests' ? c.visible.length : c.systems.length;
  return <main className="log-workspace">
    <header className="log-page-header"><h1 className="text-2xl font-semibold">日志</h1><div className="flex gap-2">
      <Button variant="outline" size="sm" onClick={() => c.setPaused(v => !v)}>{c.paused ? <Play size={14} /> : <Pause size={14} />}{c.paused ? '继续' : '暂停'}</Button>
      <Button variant="outline" size="sm" onClick={c.refresh}><RefreshCw size={14} />刷新</Button>
      <Button variant="outline" size="sm" disabled={c.exporting} onClick={() => void c.exportLogs()}><Download size={14} />导出</Button>
      <Button variant="outline" size="icon" aria-label="日志保留设置" onClick={() => c.showSettings(true)}><Settings2 size={16} /></Button>
    </div></header>
    <div className="log-tabs-row"><div className="detail-tabs" role="group" aria-label="日志分类">
      <button aria-pressed={c.tab === 'requests'} className={c.tab === 'requests' ? 'is-active' : ''} onClick={() => { c.setTab('requests'); c.resetFilters(); }}><FileText size={15} />请求日志</button>
      <button aria-pressed={c.tab === 'system'} className={c.tab === 'system' ? 'is-active' : ''} onClick={() => { c.setTab('system'); c.resetFilters(); }}><Terminal size={15} />系统日志</button>
    </div>{c.tab === 'requests' && <Button variant="outline" size="sm" onClick={() => layout.setOpen(true)}><Columns3 size={14} />显示内容<span>{layout.ids.length}</span></Button>}</div>
    <Filters c={c} />
    {c.tab === 'requests' && <LogStatsView filters={c.filters} paused={c.paused} revision={c.revision} onGroupSelect={(dimension, value) => c.filter(dimension, value)} />}
    {c.health && (c.health.dropped > 0 || c.health.write_failures > 0 || c.health.error) && <div role="status" className="log-degraded">丢弃 {c.health.dropped} · 写入失败 {c.health.write_failures} · {c.health.error || '—'}</div>}
    {(c.notice || c.error) && <p role="status" className="my-3 text-sm text-destructive">{c.error || c.notice}</p>}
    <section className="log-surface" aria-label={c.tab === 'requests' ? '请求日志列表' : '系统日志列表'} aria-busy={c.loading}>
      {count === 0 ? <div className="p-12 text-center text-sm text-muted-foreground">{c.loading ? '加载中' : c.error ? '日志不可用' : '暂无日志'}</div> : c.tab === 'requests' ? <RequestTable c={c} layout={layout} /> : <SystemTable c={c} />}
      <footer className="log-table-footer"><span>{count} 条</span><div className="flex items-center gap-3"><span>第 {c.pageNumber} 页</span><Button variant="ghost" size="icon" aria-label="上一页日志" disabled={c.pageNumber === 1 || c.loading} onClick={c.previous}><ChevronLeft size={14} /></Button><Button variant="ghost" size="icon" aria-label="下一页日志" disabled={!c.hasMore || c.loading} onClick={c.next}><ChevronRight size={14} /></Button></div></footer>
    </section>
    <LogColumnSettings c={layout} />
    <RequestDetail notice={c.notice} row={c.selected} onClose={() => void c.select(null)} onCopy={row => { void c.copy(row); }} />
    <Dialog open={c.settings} onOpenChange={c.showSettings}><DialogContent aria-describedby={undefined} className="action-dialog" onRequestClose={() => c.showSettings(false)} closeLabel="关闭保留设置"><div className="p-6">
      <DialogTitle className="text-lg font-semibold">日志保留设置</DialogTitle>
      {c.config ? <div className="space-y-4 mt-6">
        {(['days','system_days','max_requests','request_bytes','system_bytes'] as const).map(k => <label key={k} className="block text-sm">{{ days:'请求保留天数', system_days:'系统保留天数', max_requests:'最多请求', request_bytes:'请求容量 / MiB', system_bytes:'系统容量 / MiB' }[k]}
          <Input type="number" min="1" value={k.endsWith('bytes') ? c.config![k] / 1048576 : c.config![k]} onChange={e => c.setConfig(v => v ? { ...v, [k]: Number(e.target.value) * (k.endsWith('bytes') ? 1048576 : 1) } : null)} />
        </label>)}
        {c.notice && <p role="status">{c.notice}</p>}<Button className="w-full" disabled={c.saving} onClick={() => void c.saveSettings()}>保存</Button>
      </div> : <p role="status" className="mt-6">设置不可用</p>}
    </div></DialogContent></Dialog>
  </main>;
}
