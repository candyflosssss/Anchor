import { Button } from '../../components/ui/button';
import { Dialog, DialogContent, DialogTitle } from '../../components/ui/dialog';
import { routingRules } from '../models/model-routing.model';
import type { ProviderDialog } from './provider-summary.model';
import type { ProviderController } from './use-provider-controller';
import { useMappingBatchDialog } from './use-provider-mapping-batch';

const names = { test: '批量测试', edit: '批量编辑', delete: '批量删除' };
const statuses = { pending: '未执行', running: '执行中', success: '成功', failed: '失败', skipped: '跳过' };
export function ProviderMappingBatchDialog({ c, d }: { c: ProviderController; d: Extract<ProviderDialog, { kind: 'mapping-batch' }> }) {
  const batch = useMappingBatchDialog(c, d);
  const { patch, change, report } = batch;
  const done = report?.items.filter(item => !['pending', 'running'].includes(item.status)).length ?? 0;
  return <Dialog open onOpenChange={open => { if (!open) batch.close(); }}>
    <DialogContent aria-describedby="mapping-batch-description" className="action-dialog" closeLabel="关闭对话框" onRequestClose={batch.close}
      onInteractOutside={event => { event.preventDefault(); batch.close(); }} onEscapeKeyDown={event => { event.preventDefault(); batch.close(); }}>
      <header className="editor-header"><span className="eyebrow">{d.workspace.provider_id}{c.mock ? ' · 演示' : ''}</span>
        <DialogTitle className="text-xl font-semibold">{names[d.operation]} {d.ids.length} 个模型映射</DialogTitle></header>
      <form onSubmit={event => { event.preventDefault(); batch.start(); }}>
        <fieldset className="editor-fields" disabled={batch.busy || !!report}>
          <p id="mapping-batch-description" className={d.operation === 'edit' ? 'field-hint' : 'warning-copy'}>{d.operation === 'test'
            ? '将按每条映射保存的上游模型和来源 Key 依次发送一次推理请求，可能产生用量；没有可用 Key 的映射会跳过。'
            : d.operation === 'delete'
              ? '仅删除当前供应商的所选映射，保留其他供应商候选。移除最后一个候选会删除公开模型；Binding 引用保护仍由核心校验。此操作不可撤销。'
              : '仅应用主动修改的字段，其余保持各映射原值；不修改别名与上游模型。能力、调度策略和速度档位属于公开模型，会影响同名模型的其他供应商渠道。'}</p>
          <p className="field-hint">逐项执行，不是整体事务。遇到操作错误或版本冲突会停止，已完成项不会回滚，也不会自动重试。停止操作将在当前请求完成后生效。</p>
          <details><summary>查看所选映射（{d.ids.length}）</summary><ul className="max-h-32 overflow-auto text-sm">{d.ids.map(id => <li className="mono break-all" key={id}>{id}</li>)}</ul></details>
          {d.operation === 'edit' && <>
            {(['tools', 'streaming'] as const).map(key => <div className="field" key={key}><label htmlFor={`batch-${key}`}>{key === 'tools' ? '函数工具' : '流式输出'}</label>
              <select id={`batch-${key}`} className="select-field" value={patch[key] === undefined ? 'keep' : String(patch[key])} onChange={event => change(key, event.target.value === 'keep' ? undefined : event.target.value === 'true')}>
                <option value="keep">保持各自原值</option><option value="true">全部开启</option><option value="false">全部关闭</option>
              </select></div>)}
            <div className="field"><label htmlFor="batch-strategy">公开模型调度策略</label><select id="batch-strategy" className="select-field" value={patch.strategy ?? 'keep'} onChange={event => change('strategy', event.target.value === 'keep' ? undefined : event.target.value)}>
              <option value="keep">保持各自原值</option>{Object.entries(routingRules).map(([id, rule]) => <option key={id} value={id}>{rule.label}</option>)}
            </select></div>
            <div className="field"><label htmlFor="batch-tier">Anchor 固定速度档位</label><select id="batch-tier" className="select-field" value={patch.service_tier ?? 'keep'} onChange={event => change('service_tier', event.target.value === 'keep' ? undefined : event.target.value)}>
              <option value="keep">保持各自原值</option><option value="">不固定（沿用请求参数）</option><option value="priority">Fast（priority）</option><option value="ultrafast">Ultrafast</option>
            </select><p className="field-hint">固定档位需要上游支持，可能增加费用。</p></div>
            <div className="field"><label htmlFor="batch-keys">来源 Key</label><select id="batch-keys" className="select-field" value={patch.key_ids === undefined ? 'keep' : 'replace'} onChange={event => change('key_ids', event.target.value === 'keep' ? undefined : [])}>
              <option value="keep">保持各自绑定</option><option value="replace">统一设置来源 Key</option>
            </select>{patch.key_ids !== undefined && <>
              <p className="warning-copy">将替换所有所选映射的 Key 绑定；不勾选表示允许所有启用 Key。</p>
              {d.workspace.keys.map(key => <label key={key.id} className="checkbox-row"><input type="checkbox" checked={patch.key_ids!.includes(key.id)} onChange={event => change('key_ids', event.target.checked ? [...patch.key_ids!, key.id] : patch.key_ids!.filter(id => id !== key.id))} />{key.id}{!key.enabled && ' · 已停用'}</label>)}
            </>}</div>
          </>}
        </fieldset>
        {report && <section className="editor-fields" aria-label="批量操作结果">
          <p role="status">{report.status === 'running' ? batch.stopping ? '正在停止，等待当前请求完成' : '正在执行' : report.status === 'stopped' ? '已停止' : '已完成'} · {done} / {report.items.length} · 成功 {report.items.filter(item => item.status === 'success').length} · 失败 {report.items.filter(item => item.status === 'failed').length} · 跳过 {report.items.filter(item => item.status === 'skipped').length}</p>
          {report.needsRead && <p className="field-error" role="alert">{report.error} 请关闭弹窗并重新读取，核对结果后再选择剩余项目；不要直接重放整批操作。</p>}
          <ul className="max-h-64 overflow-auto space-y-2 text-sm">{report.items.map(item => <li key={item.id} className="break-all"><strong className="mono">{item.id}</strong> · {statuses[item.status]} · {item.message}{item.probe && <span> · {item.probe.model} · Key {item.probe.key_id} · HTTP {item.probe.http_status || '—'} · {item.probe.elapsed_ms} ms</span>}</li>)}</ul>
        </section>}
        <footer className="editor-footer"><div className="flex flex-wrap gap-2">
          <Button type="button" variant="outline" disabled={batch.busy} onClick={batch.close}>{report ? '关闭' : '取消'}</Button>
          {batch.busy && report?.status === 'running' ? <Button type="button" variant="outline" disabled={batch.stopping} onClick={batch.stop}>停止后续操作</Button>
            : !report && <Button variant={d.operation === 'delete' ? 'destructive' : 'default'} disabled={!batch.canStart}>{d.operation === 'test' ? '确认并开始测试' : d.operation === 'delete' ? '确认删除所选映射' : '保存批量修改'}</Button>}
        </div></footer>
      </form>
    </DialogContent>
  </Dialog>;
}
