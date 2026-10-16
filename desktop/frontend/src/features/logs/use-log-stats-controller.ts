import { useEffect, useState } from 'react';
import type { Stats } from '../../desktop/contract.generated';
import type { LogDimension, LogFilters } from './log.model';
import { logService } from './log.service';
import { statsQuery, validInterval, type MetricID } from './log-stats.model';
export function useLogStatsController(filters: LogFilters, paused: boolean, revision: number) {
  const [groupBy, setGroupBy] = useState<LogDimension | ''>('');
  const [metric, setMetric] = useState<MetricID>('tokens');
  const [requestedInterval, setInterval] = useState(0);
  const interval = validInterval(filters.window, requestedInterval) ? requestedInterval : 0;
  const [state, setState] = useState<{ data: Stats | null; error: string; loading: boolean }>({ data: null, error: '', loading: true });
  useEffect(() => {
    let stopped = false; let timer: ReturnType<typeof setTimeout>;
    setState({ data: null, error: '', loading: true });
    async function load() {
      try {
        const data = await logService.stats(statsQuery(filters, interval, Date.now(), groupBy));
        if (!stopped) setState({ data, error: '', loading: false });
      } catch {
        if (!stopped) setState({ data: null, error: groupBy ? '分类统计读取失败，请确认核心版本支持' : '趋势读取失败', loading: false });
      }
      if (!stopped && !paused) timer = setTimeout(() => void load(), 15000);
    }
    void load();
    return () => { stopped = true; clearTimeout(timer); };
  }, [filters, interval, paused, revision, groupBy]);
  return { ...state, groupBy, setGroupBy, metric, setMetric, interval, setInterval };
}
