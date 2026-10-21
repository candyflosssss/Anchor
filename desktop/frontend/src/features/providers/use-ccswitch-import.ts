import { useEffect, useRef, useState } from 'react';
import { DesktopProblem } from '../../desktop/transport';
import { ccImportTransport, importCounts, importReason, type CCImportReport, type CCImportTransport } from './ccswitch-import.model';

export function useCCSwitchImport(onImported: () => Promise<unknown>, transport: CCImportTransport = ccImportTransport) {
  const [open, setOpen] = useState(false);
  const [path, setPath] = useState('');
  const [report, setReport] = useState<CCImportReport | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const locked = useRef(false);
  const active = useRef<AbortController | null>(null);
  useEffect(() => () => active.current?.abort(), []);
  function close() { if (!locked.current) { setOpen(false); setReport(null); setError(''); } }
  async function run(approved = false) {
    if (locked.current || (approved && (!report || report.applied || report.failure))) return;
    locked.current = true; setBusy(true); setError('');
    const abort = new AbortController(); active.current = abort;
    const snapshot = report;
    try {
      const result = await transport.run({ database_path: approved ? snapshot!.database_path : path, fingerprint: approved ? snapshot!.fingerprint : '', approved }, abort.signal);
      if (abort.signal.aborted) return;
      setReport(result);
      if (result.applied) await onImported();
    } catch (e) {
      if (abort.signal.aborted) return;
      // Any failed write discards the executable plan. A fresh preview shows
      // existing IDs without silently replaying an uncertain write.
      if (approved) setReport(null);
      const code = e instanceof DesktopProblem ? e.code : approved ? 'unknown' : 'read_error';
      setError(code === 'conflict' ? '来源或 Anchor 配置已变化，请重新预览。'
        : code === 'unknown' ? '导入结果未知，请重新读取预览并核对已有供应商，勿直接重试写入。'
        : code === 'unsupported' ? '此实例或桌面版本不支持导入，请升级 Anchor。'
        : '无法读取或导入 CC Switch，请检查目标实例和数据库路径。');
    } finally { active.current = null; locked.current = false; if (!abort.signal.aborted) setBusy(false); }
  }
  return { open, busy, path, report, error: error || (report?.failure ? importReason[report.failure] ?? '无法读取来源' : ''),
    counts: report ? importCounts(report) : null,
    show: () => { setReport(null); setError(''); setOpen(true); void run(); }, close,
    changePath: (value: string) => { if (!locked.current) { setPath(value); setReport(null); setError(''); } },
    preview: () => run(), apply: () => run(true) };
}
export type CCImportController = ReturnType<typeof useCCSwitchImport>;
