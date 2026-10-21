import { useEffect, useState } from 'react';
import type { Stats } from '../../desktop/contract.generated';
import type { LogFilters, LogTimeRange } from './log.model';
import { errorStatsQuery } from './log-stats.model';
import { logService } from './log.service';

export function useLogRangeController(filters: LogFilters, range: LogTimeRange | null, revision: number, paused: boolean) {
  const key = JSON.stringify([filters, range]);
  const [state, setState] = useState<{ key: string; data: Stats | null; error: string; loading: boolean }>({ key, data: null, error: '', loading: true });
  useEffect(() => {
    if (!range) return;
    let cancelled = false; let timer: ReturnType<typeof setTimeout>;
    setState({ key, data: null, error: '', loading: true });
    async function load() {
      try {
        const data = await logService.stats(errorStatsQuery(filters, range!));
        if (!cancelled) setState({ key, data, error: data.error_summary == null ? '当前核心不支持区间错误汇总，请更新核心。' : '', loading: false });
      } catch {
        if (!cancelled) setState({ key, data: null, error: '区间错误汇总读取失败，请刷新重试。', loading: false });
      }
      if (!cancelled && !paused) timer = setTimeout(() => void load(), 15000);
    }
    void load();
    return () => { cancelled = true; clearTimeout(timer); };
  }, [filters, range, revision, paused, key]);
  return state.key === key ? state : { data: null, error: '', loading: true };
}
