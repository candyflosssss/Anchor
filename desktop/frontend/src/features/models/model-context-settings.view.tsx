import type { CatalogPricing, ModelConfiguration, ModelSettingsState } from '../../desktop/contract.generated';
import { Button } from '../../components/ui/button';
import { applyContextWindow, crossesPriceBand } from './model-settings.model';

const tokens = (n: number) => n.toLocaleString('en-US');

export function ContextPriceNotice({ state, acknowledged, acknowledge, change }: {
  state: ModelSettingsState;
  acknowledged: boolean;
  acknowledge: (value: boolean) => void;
  change: (configuration: ModelConfiguration) => void;
}) {
  const policy = state.context_policy, threshold = policy?.price_threshold;
  const suggested = policy?.recommended_context, long = crossesPriceBand(state);
  return <div className="context-price-notice">
    <p className="card-hint">模型规格上限：{policy?.model_context == null ? '未知' : `${tokens(policy.model_context)} tokens`}。上方是导出给 Agent 的运行窗口，不是自动压缩阈值，也不是上游费用硬限额。</p>
    {threshold != null ? <>
      <p>基础价输入分界：<strong>{tokens(threshold)} tokens</strong>。单次输入超过此值（含缓存读取与写入）后，整次请求使用对应长上下文档单价，不是只对超出部分加价。</p>
      <p className="card-hint">设置窗口本身不收费；实际输入量决定价格档位。输出不参与输入分界判断，但跨档后输出单价也可能变化。</p>
      {suggested != null && state.configuration.limits.context !== suggested && <Button type="button" size="sm" variant="outline" onClick={() => change(applyContextWindow(state.configuration, suggested))}>使用基础价参考窗口（{tokens(suggested)}）</Button>}
      {long && <div className="context-price-warning" role="alert">
        <p>当前运行窗口 {tokens(state.configuration.limits.context!)} 超过基础价分界，允许请求进入长上下文价格档。已有设置未被自动改小。</p>
        <label><input type="checkbox" checked={acknowledged} onChange={e => acknowledge(e.target.checked)} />我已了解跨档费用，保留此窗口</label>
      </div>}
      {!state.reference && <p className="card-hint">此分界仅来自匹配的基础模型目录；未应用标准档价格快照，不能据此确认 Fast / Ultrafast 或中转渠道的单价。</p>}
    </> : <p className="card-hint">当前没有可用的上下文价格分档信息；不代表长上下文不加价，请核对实际渠道。</p>}
    <p className="card-hint">参考建议不验证渠道能力或保证账单；客户端仍可能因工具结果等超过预算，实际收费以上游规则和账单为准。</p>
  </div>;
}

export function ContextPriceTable({ pricing }: { pricing: CatalogPricing }) {
  if (!pricing.tiers.length) return null;
  const bands = [{ threshold: null, rates: pricing.rates }, ...pricing.tiers.map(tier => ({ threshold: tier.above_tokens, rates: tier.rates }))];
  return <div className="context-price-table"><table>
    <caption>按单次输入量选择整次请求单价 · USD / 百万 tokens</caption>
    <thead><tr><th>输入范围（含缓存）</th><th>输入</th><th>输出</th><th>缓存读取</th><th>缓存写入</th><th>思考</th></tr></thead>
    <tbody>{bands.map((band, i) => <tr key={band.threshold ?? 'base'}>
      <th>{band.threshold === null ? `≤ ${tokens(pricing.tiers[0].above_tokens)}` : `> ${tokens(band.threshold)}${pricing.tiers[i] ? ` 且 ≤ ${tokens(pricing.tiers[i].above_tokens)}` : ''}`}</th>
      <td>{band.rates.input ?? '未知'}</td><td>{band.rates.output ?? '未知'}</td><td>{band.rates.cache_read ?? '未知'}</td><td>{band.rates.cache_write ?? '未知'}</td><td>{band.rates.reasoning ?? '按输出'}</td>
    </tr>)}</tbody>
  </table></div>;
}
