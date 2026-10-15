import { actionFor, type Action, type Probe, type Workspace } from './provider-features.model';
import { keyForMappingTest, probeFailureText } from './provider-test.model';
import { problemText, ProviderProblem, type ProviderTransport } from './provider.model';

export type MappingBatchOperation = 'test' | 'edit' | 'delete';
/** Undefined means preserve each mapping's own value, including explicit false/empty values. */
export type MappingBatchPatch = Partial<Pick<Workspace['models'][number], 'tools' | 'streaming' | 'strategy' | 'service_tier' | 'key_ids'>>;
export interface MappingBatchItem {
  id: string;
  status: 'pending' | 'running' | 'success' | 'failed' | 'skipped';
  message: string;
  probe?: Probe;
}
export interface MappingBatchReport {
  status: 'running' | 'completed' | 'stopped';
  items: MappingBatchItem[];
  needsRead: boolean;
  error: string;
}

/** Freeze revisions at confirmation; never adopt newer configuration mid-batch. */
export function mappingBatchAction(workspace: Workspace, id: string, operation: MappingBatchOperation, patch: MappingBatchPatch = {}): Action | null {
  const mapping = workspace.models.find(model => model.id === id);
  if (!mapping) throw new ProviderProblem('conflict');
  const provider = { id: workspace.provider_id, revision: workspace.revision };
  if (operation === 'test') {
    const key = keyForMappingTest(workspace, mapping.key_ids);
    return key ? actionFor(provider, 'test', { target: key, model: mapping.upstream, approved: true }) : null;
  }
  if (operation === 'delete') return actionFor(provider, 'model-delete', { target: id, model_revision: mapping.revision, approved: true });
  return actionFor(provider, 'model-put', {
    target: id, model: mapping.upstream, model_revision: mapping.revision,
    tools: patch.tools ?? mapping.tools, streaming: patch.streaming ?? mapping.streaming,
    strategy: patch.strategy ?? mapping.strategy, service_tier: patch.service_tier ?? mapping.service_tier ?? '',
    key_ids: patch.key_ids ?? [...mapping.key_ids], approved: true,
  });
}

/** Existing typed actions, serial execution, no retry and no cross-item transaction. */
export async function executeMappingBatch(options: {
  workspace: Workspace; ids: string[]; operation: MappingBatchOperation; patch?: MappingBatchPatch;
  transport: ProviderTransport; signal: AbortSignal; stopping: () => boolean;
  progress: (report: MappingBatchReport) => void; saved: (workspace: Workspace) => void;
}): Promise<MappingBatchReport> {
  const { workspace, operation, transport, signal, stopping, progress, saved } = options;
  const report: MappingBatchReport = { status: 'running', items: [...new Set(options.ids)].map(id => ({ id, status: 'pending', message: '等待执行' })), needsRead: false, error: '' };
  const publish = () => progress({ ...report, items: report.items.map(item => ({ ...item })) });
  publish();
  for (const item of report.items) {
    if (signal.aborted || stopping()) { report.status = 'stopped'; break; }
    item.status = 'running'; item.message = '正在执行'; publish();
    try {
      if (!transport.action) throw new ProviderProblem('invalid');
      const input = mappingBatchAction(workspace, item.id, operation, options.patch);
      if (!input) {
        item.status = 'skipped'; item.message = '没有符合映射绑定的启用 Key，未发送请求'; publish(); continue;
      }
      const result = await transport.action(input, signal);
      if (signal.aborted) throw new ProviderProblem('unknown');
      if (result.provider_id !== workspace.provider_id || result.deleted) throw new ProviderProblem('unknown');
      saved(result);
      if (operation === 'test') {
        const probe = result.probe;
        if (!probe || probe.revision !== input.expected_revision || probe.model !== input.model || probe.key_id !== input.target) throw new ProviderProblem('unknown');
        item.probe = probe;
        item.status = probe.failure ? 'failed' : 'success';
        item.message = probe.failure ? probeFailureText(probe.failure) : '测试成功';
      } else {
        item.status = 'success'; item.message = operation === 'delete' ? '已删除映射' : '已保存映射';
      }
    } catch (error) {
      // A transport failure may hide a successful write: stop, reread, never replay.
      item.status = 'failed';
      item.message = signal.aborted ? '操作中断，结果需重新读取核对' : problemText[error instanceof ProviderProblem ? error.code : 'unknown'];
      report.error = item.message; report.needsRead = true; report.status = 'stopped';
      publish(); break;
    }
    publish();
  }
  if (report.status === 'running') report.status = 'completed';
  publish();
  return report;
}
