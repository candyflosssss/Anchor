import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { identityQuery, initialFilters, safeSummary, type LogFilters, type RequestLog } from './log.model';
import { logService, requestLog, systemLog, type LogQuery, type LogSettings } from './log.service';
import type { Page } from '../../desktop/contract.generated';
export function useLogController() {
  const [tab, setTab] = useState<'requests' | 'system'>('requests');
  const [filters, setFilters] = useState(initialFilters), [level, setLevel] = useState('all');
  const [page, setPage] = useState<Page | null>(null), [cursors, setCursors] = useState<number[]>([0]);
  const [snapshot, setSnapshot] = useState(0), [paused, setPaused] = useState(false);
  const [selected, setSelected] = useState<RequestLog | null>(null), [loading, setLoading] = useState(false);
  const [error, setError] = useState(''), [notice, setNotice] = useState('');
  const [settings, showSettings] = useState(false), [config, setConfig] = useState<LogSettings | null>(null);
  const [revision, refresh] = useState(0), [exporting, setExporting] = useState(false), [saving, setSaving] = useState(false);
  const lifecycle = useRef({ request: 0, detail: 0, alive: true }).current;
  const query = useMemo<LogQuery>(() => ({ ...(tab === 'requests' ? identityQuery(filters) : {}), kind: tab, limit: 50, cursor: cursors.at(-1) ?? 0, until: snapshot,
    since_ms: Date.now() - Number(filters.window) * 60000, before_ms: 0, status: tab === 'requests' && filters.status !== 'all' ? filters.status : '',
    source: filters.source !== 'all' ? filters.source : '', provider: tab === 'requests' && filters.provider !== 'all' ? filters.provider : '',
    model: filters.model, key: filters.key, http: filters.http ? Number(filters.http) : null, code: filters.code, stage: filters.stage,
    level: level === 'all' ? '' : level, search: filters.query.trim() }), [tab, filters, level, cursors, snapshot]);
  const load = useCallback(async () => {
    const id = ++lifecycle.request; setLoading(true);
    try { const p = await logService.list({ ...query, since_ms: Date.now() - Number(filters.window) * 60000 }); if (lifecycle.alive && id === lifecycle.request) { setPage(p); setError(''); } }
    catch { if (lifecycle.alive && id === lifecycle.request) { setError('日志读取失败；保留上次读取结果。'); } }
    finally { if (lifecycle.alive && id === lifecycle.request) setLoading(false); }
  }, [query, filters.window, lifecycle]);
  useEffect(() => { lifecycle.alive = true; return () => { lifecycle.alive = false; lifecycle.request++; lifecycle.detail++; }; }, [lifecycle]);
  useEffect(() => { let stopped = false; let timer: ReturnType<typeof setTimeout>;
    async function poll() { await load(); if (!stopped && !paused && cursors.length === 1) timer = setTimeout(() => void poll(), 3000); }
    void poll(); return () => { stopped = true; clearTimeout(timer); lifecycle.request++; };
  }, [load, paused, cursors.length, revision, lifecycle]);
  useEffect(() => { let cancelled = false; void logService.settings().then(v => { if (!cancelled) setConfig(v); }).catch(() => {}); return () => { cancelled = true; }; }, []);
  function filter<K extends keyof LogFilters>(key: K, value: LogFilters[K]) { setFilters(current => ({ ...current, [key]: value })); setCursors([0]); setSnapshot(0); setPage(null); }
  function resetFilters() { setFilters(initialFilters); setLevel('all'); setCursors([0]); setSnapshot(0); setPage(null); }
  async function select(row: RequestLog | null) {
    const id = ++lifecycle.detail; if (!row) { setSelected(null); return; }
    setSelected(null); try { const v = await logService.detail(row.id); if (lifecycle.alive && id === lifecycle.detail) setSelected(v); }
    catch { if (lifecycle.alive && id === lifecycle.detail) setNotice('详情读取失败'); }
  }
  async function copy(row: RequestLog) { try { await navigator.clipboard.writeText(safeSummary(row)); setNotice('已复制'); } catch { setNotice('复制失败'); } }
  async function exportLogs() { if (exporting) return; setExporting(true); try { const v = await logService.export({ ...query, cursor: 0 }); if (lifecycle.alive) setNotice(v.accepted ? '已导出' : '已取消'); } catch { if (lifecycle.alive) setNotice('导出失败'); } finally { if (lifecycle.alive) setExporting(false); } }
  async function saveSettings() { if (!config || saving) return; setSaving(true); try { const v = await logService.configure(config); if (lifecycle.alive) { setConfig(v); showSettings(false); setNotice('已保存'); refresh(v => v + 1); } } catch { if (lifecycle.alive) setNotice('保存失败'); } finally { if (lifecycle.alive) setSaving(false); } }
  return { revision, tab, setTab, filters, filter, resetFilters, level, setLevel: (value: string) => { setLevel(value); setCursors([0]); setSnapshot(0); setPage(null); }, paused, setPaused, selected, select, loading, error, notice, copy, exportLogs,
    exporting, saving, settings, showSettings, config, setConfig, saveSettings, visible: (page?.requests ?? []).map(requestLog), systems: (page?.events ?? []).map(systemLog), health: page?.health,
    pageNumber: cursors.length, hasMore: page?.has_more ?? false, refresh: () => { setCursors([0]); setSnapshot(0); refresh(v => v + 1); },
    next: () => { if (page?.has_more) { setSnapshot(v => v || page.latest); setCursors(v => [...v, page.next_cursor]); setPage(null); } },
    previous: () => { setCursors(v => v.slice(0, -1)); setPage(null); },
    findRequest: async (id: string) => { const generation = ++lifecycle.detail; setSelected(null); try { const row = await logService.detail(id); if (lifecycle.alive && generation === lifecycle.detail) setSelected(row); } catch { if (lifecycle.alive && generation === lifecycle.detail) setNotice('关联请求不存在'); } },
  };
}
export type LogController = ReturnType<typeof useLogController>;
