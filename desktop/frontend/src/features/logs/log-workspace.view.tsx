import { ChevronLeft, ChevronRight, Columns3, Download, FileText, FlaskConical, Pause, Play, Search, Settings2, ShieldCheck, Terminal, TriangleAlert, X } from 'lucide-react';
import { Button } from '../../components/ui/button';
import { Input } from '../../components/ui/input';
import { Dialog, DialogContent, DialogDescription, DialogTitle } from '../../components/ui/dialog';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '../../components/ui/table';
import { RequestDetail } from './log-detail.view';
import { statusLabels } from './log.model';
import { useLogController, type LogController } from './use-log-controller';
import { RequestTable } from './log-request-table.view';
import { LogColumnSettings } from './log-columns.view';
import { useLogColumnsController } from './use-log-columns-controller';
import './logs.css';

function Filters({ c }: { c: LogController }) {
  const active = c.filters.query || c.filters.status !== 'all' || c.filters.source !== 'all' || c.filters.provider !== 'all' || c.filters.window !== '60' || c.level !== 'all';
  return <div className="log-filters">
    <label className="log-search"><Search size={15} /><Input aria-label="搜索日志" placeholder={c.tab === 'requests' ? '搜索模型、Key 或 request ID…' : '搜索事件、消息或 request ID…'} value={c.filters.query} onChange={event => c.filter('query', event.target.value)} />{c.filters.query && <button aria-label="清除日志搜索" onClick={() => c.filter('query', '')}><X size={13} /></button>}</label>
    <select aria-label="日志时间范围" className="select-field" value={c.filters.window} onChange={event => c.filter('window', event.target.value)}><option value="15">最近 15 分钟</option><option value="60">最近 1 小时</option><option value="1440">最近 24 小时</option><option value="10080">最近 7 天</option></select>
    {c.tab === 'requests' ? <>
      <select aria-label="请求结果" className="select-field" value={c.filters.status} onChange={event => c.filter('status', event.target.value)}><option value="all">全部结果</option>{Object.entries(statusLabels).map(([value, label]) => <option key={value} value={value}>{label}</option>)}</select>
      <select aria-label="请求来源" className="select-field" value={c.filters.source} onChange={event => c.filter('source', event.target.value)}><option value="all">全部来源</option><option value="proxy">代理请求</option><option value="probe">推理测试</option></select>
      <select aria-label="日志供应商" className="select-field" value={c.filters.provider} onChange={event => c.filter('provider', event.target.value)}><option value="all">全部供应商</option>{c.providers.map(provider => <option key={provider}>{provider}</option>)}</select>
    </> : <select aria-label="系统日志级别" className="select-field" value={c.level} onChange={event => c.setLevel(event.target.value)}><option value="all">全部级别</option><option value="info">信息</option><option value="warn">警告</option><option value="error">错误</option></select>}
    {active && <Button variant="ghost" size="sm" onClick={c.resetFilters}>清除筛选</Button>}
  </div>;
}
function EmptyState({ c }: { c: LogController }) {
  return <div className="log-empty"><FileText size={28} strokeWidth={1.3} /><h2>{c.scenario === 'offline' ? '核心暂不可用' : c.scenario === 'empty' ? '还没有请求记录' : '没有匹配的日志'}</h2><p>{c.scenario === 'offline' ? '这是离线状态的虚拟预览；真实日志接入后会保留上次结果并说明数据已过期。' : c.scenario === 'empty' ? '请求经过 Anchor 后会出现在这里。当前展示的是空状态设计。' : '试试调整时间范围、搜索词或筛选条件。'}</p><Button variant="outline" size="sm" onClick={() => { c.setScenario('normal'); c.resetFilters(); }}>{c.scenario === 'normal' ? '清除筛选' : '恢复有数据预览'}</Button></div>;
}
function SystemTable({ c }: { c: LogController }) {
  return <Table className="log-table"><TableHeader><TableRow><TableHead>时间</TableHead><TableHead>级别</TableHead><TableHead>来源</TableHead><TableHead>事件 / 消息</TableHead><TableHead>关联请求</TableHead></TableRow></TableHeader><TableBody>{c.systems.map(row => <TableRow key={row.id}><TableCell className="mono text-xs">{row.time}</TableCell><TableCell><span className={`log-level log-level-${row.level}`}><span className="mini-dot" />{row.level === 'warn' ? '警告' : row.level === 'error' ? '错误' : '信息'}</span></TableCell><TableCell className="mono text-xs">{row.source}</TableCell><TableCell><span className="mono text-xs">{row.event}</span><small className="log-subtext">{row.message}</small></TableCell><TableCell>{row.requestId ? <button className="mono text-xs underline decoration-border underline-offset-4" onClick={() => { c.select(c.findRequest(row.requestId ?? '')); }}> {row.requestId}</button> : <span className="text-muted-foreground">—</span>}</TableCell></TableRow>)}</TableBody></Table>;
}
export function LogsPreview() {
  const c = useLogController();
  const layout = useLogColumnsController();
  const count = c.tab === 'requests' ? c.visible.length : c.systems.length;
  const empty = c.scenario === 'empty' || c.scenario === 'offline' || count === 0;
  return <main className="workspace log-workspace">
    <header className="page-header log-page-header"><div><div className="flex items-center gap-2"><h1>日志</h1><span className="log-preview-tag"><FlaskConical size={12} />设计预览</span></div><p className="text-sm">从请求到交付，看清每一次调用。</p></div><div className="toolbar-actions"><Button variant="outline" size="sm" aria-pressed={c.paused} onClick={() => c.setPaused(!c.paused)}>{c.paused ? <Play size={14} /> : <Pause size={14} />}{c.paused ? '继续刷新' : '暂停刷新'}</Button><Button variant="outline" size="sm" onClick={c.exportPreview} disabled={empty}><Download size={14} />导出示例</Button><Button variant="ghost" size="icon" aria-label="日志保留设置" onClick={() => c.showSettings(true)}><Settings2 size={16} /></Button></div></header>
    <div className="log-preview-notice"><FlaskConical size={14} /><span>所有记录均为虚拟数据，用于预览页面与排障交互。</span><span className="ml-auto hidden text-xs sm:block">合成时间 · 2026-10-11</span></div>
    <div className="log-tabs-row"><div className="detail-tabs" role="group" aria-label="日志分类"><button aria-pressed={c.tab === 'requests'} className={c.tab === 'requests' ? 'is-active' : ''} onClick={() => { c.setTab('requests'); c.resetFilters(); }}><FileText size={15} />请求日志<span className="tab-count">{c.total}</span></button><button aria-pressed={c.tab === 'system'} className={c.tab === 'system' ? 'is-active' : ''} onClick={() => { c.setTab('system'); c.resetFilters(); }}><Terminal size={15} />系统日志</button></div><span className="hidden items-center gap-1.5 text-xs text-muted-foreground sm:flex"><ShieldCheck size={13} />仅记录元数据</span></div>
    <div className="log-filter-bar"><Filters c={c} />{c.tab === 'requests' && <Button variant="outline" size="sm" onClick={() => layout.setOpen(true)}><Columns3 size={14} />显示内容<span className="log-column-button-count">{layout.ids.length}</span></Button>}</div>
    {c.scenario === 'degraded' && <div role="status" className="log-degraded"><TriangleAlert size={15} /><div><strong>日志记录不完整</strong><p>模拟写入队列达到上限，3 条记录未能保存。请求转发继续运行。</p></div></div>}
    {c.notice && <p role="status" className="mb-3 text-xs text-muted-foreground">{c.notice}</p>}
    {c.pending && c.tab === 'requests' && <Button variant="outline" className="mb-3 w-full" onClick={() => { c.loadNew(); c.setPaused(false); }}>有 1 条新记录，点击加载</Button>}
    <section className="log-surface" aria-label={c.tab === 'requests' ? '请求日志列表' : '系统日志列表'}>
      {empty ? <EmptyState c={c} /> : c.tab === 'requests' ? <RequestTable c={c} layout={layout} /> : <SystemTable c={c} />}
      <footer className="log-table-footer"><span>{empty ? '0' : count} 条{c.tab === 'requests' ? '请求' : '系统事件'}<span className="mx-2">·</span>虚拟实例 <span className="mono">local-demo</span></span><div className="flex items-center gap-3"><span>第 1 页</span><Button variant="ghost" size="icon" aria-label="上一页日志" disabled><ChevronLeft size={14} /></Button><Button variant="ghost" size="icon" aria-label="下一页日志" disabled><ChevronRight size={14} /></Button></div></footer>
    </section>
    <div className="log-bottom-note"><ShieldCheck size={13} /><span>不记录对话正文与密钥</span><span className="mx-1">·</span><span>保留 {c.days} 天</span><span className="ml-auto">{c.paused ? '刷新已暂停 · 预览' : '等待新记录 · 预览'}</span></div>
    <footer className="prototype-controls log-prototype-controls"><div><FlaskConical size={14} /><label htmlFor="log-scenario">预览场景</label><select id="log-scenario" value={c.scenario} onChange={event => c.setScenario(event.target.value)}><option value="normal">正常记录</option><option value="degraded">记录缺口</option><option value="empty">空数据</option><option value="offline">核心离线</option></select><Button variant="ghost" size="sm" disabled={c.pending || c.received} onClick={() => c.setPending(true)}>模拟新记录</Button></div><span>虚拟记录 · 刷新重置记录</span></footer>
    <LogColumnSettings c={layout} />
    <RequestDetail notice={c.notice} row={c.selected} onClose={() => c.select(null)} onCopy={row => { void c.copy(row); }} />
    <Dialog open={c.settings} onOpenChange={c.showSettings}><DialogContent className="action-dialog" onRequestClose={() => c.showSettings(false)} closeLabel="关闭保留设置"><div className="p-6"><DialogTitle className="text-lg font-semibold">日志保留设置</DialogTitle><DialogDescription className="mt-2 text-sm text-muted-foreground">设计预览：仅调整内存中的显示，不修改真实实例。</DialogDescription><label htmlFor="log-retention" className="mb-2 mt-6 block text-sm">请求明细保留</label><select id="log-retention" className="select-field w-full" value={c.days} onChange={event => c.setDays(event.target.value)}><option value="1">1 天</option><option value="7">7 天（默认）</option><option value="14">14 天</option><option value="30">30 天</option></select><dl className="log-properties mt-5"><div><dt>最多请求</dt><dd>50,000 条</dd></div><div><dt>请求库容量目标</dt><dd>256 MiB</dd></div><div><dt>系统日志预算</dt><dd>32 MiB</dd></div><div><dt>正文捕获</dt><dd>关闭</dd></div></dl><p className="mt-5 text-xs leading-6 text-muted-foreground">日志清理与累计用量独立。达到容量上限时应提示记录缺口，不影响模型转发。</p><Button className="mt-6 w-full" onClick={() => c.showSettings(false)}>完成预览</Button></div></DialogContent></Dialog>
  </main>;
}
