import { useMemo, useState } from 'react';
import { filterRequests, initialFilters, safeSummary, type LogFilters, type RequestLog } from './log.model';
import { incomingFixture, requestFixtures, systemFixtures } from './log.mock';
export function useLogController() {
  const [tab, setTab] = useState<'requests' | 'system'>('requests');
  const [filters, setFilters] = useState(initialFilters);
  const [rows, setRows] = useState(requestFixtures);
  const [selected, select] = useState<RequestLog | null>(null);
  const [level, setLevel] = useState('all');
  const [paused, setPaused] = useState(false);
  const [pending, setPending] = useState(false);
  const [received, setReceived] = useState(false);
  const [scenario, setScenario] = useState('normal');
  const [notice, setNotice] = useState('');
  const [settings, showSettings] = useState(false);
  const [days, setDays] = useState('7');
  const filtered = useMemo(() => filterRequests(rows, filters), [rows, filters]);
  const systems = useMemo(() => systemFixtures.filter(row => row.ageMinutes <= Number(filters.window)
    && (level === 'all' || row.level === level)
    && `${row.event} ${row.message} ${row.source} ${row.requestId ?? ''}`.toLowerCase().includes(filters.query.trim().toLowerCase())), [filters, level]);
  function filter<K extends keyof LogFilters>(key: K, value: LogFilters[K]) { setFilters(current => ({ ...current, [key]: value })); }
  function resetFilters() { setFilters(initialFilters); setLevel('all'); }
  async function copy(row: RequestLog) {
    try { await navigator.clipboard.writeText(safeSummary(row)); setNotice('已复制虚拟数据的安全排障摘要'); }
    catch { setNotice('无法访问剪贴板，请从详情中手动选择文本。'); }
  }
  function exportPreview() {
    const data = tab === 'requests' ? filtered : systems;
    const blob = new Blob([JSON.stringify({ preview: true, schema_version: 1, data }, null, 2)], { type: 'application/json' });
    const url = URL.createObjectURL(blob);
    const link = document.createElement('a'); link.href = url; link.download = `anchor-${tab}-demo.json`; link.click();
    setTimeout(() => URL.revokeObjectURL(url), 1000);
    setNotice(`已导出 ${data.length} 条虚拟记录，不包含真实请求。`);
  }
  function loadNew() { setRows(current => [incomingFixture, ...current]); setPending(false); setReceived(true); }
  return { tab, setTab, filters, filter, resetFilters, filtered, systems, selected, select, level, setLevel, paused, setPaused,
    pending, setPending, received, loadNew, total: rows.length, findRequest: (id: string) => rows.find(row => row.id === id) ?? null, scenario, setScenario, notice, copy, exportPreview, settings, showSettings, days, setDays,
    providers: [...new Set(requestFixtures.flatMap(row => row.attempts.map(attempt => attempt.provider)))],
    visible: scenario === 'empty' || scenario === 'offline' ? [] : filtered,
  };
}
export type LogController = ReturnType<typeof useLogController>;
