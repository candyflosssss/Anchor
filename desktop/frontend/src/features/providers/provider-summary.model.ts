import type { MappingBatchOperation } from './provider-mapping-batch.model';
import { channelAvailability, type ChannelAvailability } from './channel-availability.model';
import type { Probe, Workspace } from './provider-features.model';
import type { KeyStatus } from '../../desktop/contract.generated';
import type { Provider } from './provider.model';

/** Pure view-model derivations shared by overview, list, detail and model pages. */
export type Tone = 'ok' | 'warn' | 'bad' | 'muted' | 'unknown';
export type DetailTab = 'overview' | 'keys' | 'models' | 'usage' | 'account';
export type ProviderDialog =
  | { kind: 'mapping-batch'; operation: MappingBatchOperation; workspace: Workspace; ids: string[] }
  | { kind: 'test'; model?: string; keyId?: string; keyIds?: string[] }
  | { kind: 'rename' } | { kind: 'copy' } | { kind: 'delete' }
  | { kind: 'key'; keyId?: string }
  | { kind: 'mapping'; alias?: string; upstream?: string; tools?: boolean; streaming?: boolean };
export type HealthCode = 'loading' | 'error' | 'disabled' | 'no-enabled-key' | 'auth' | 'degraded' | 'unmapped' | 'anonymous' | 'test-failed' | 'untested' | 'ready';
export interface ProviderSummary {
  provider: Provider;
  /** undefined = still reading, null = read failed. */
  workspace: Workspace | null | undefined;
  code: HealthCode; tone: Tone; label: string; hint: string;
  keyCount: number; enabledKeys: number; modelCount: number;
  lastTest: Probe | null; requests: number; failures: number;
  usd: number; usdPartial: boolean;
}
export const toneLabel: Record<Tone, string> = { ok: '正常', warn: '需留意', bad: '不可用', muted: '已停用', unknown: '未知' };
export { isValidId } from '../../lib/identifier';

export function latestProbe(w: Workspace | null | undefined): Probe | null {
  if (!w) return null;
  return [w.probe, ...w.keys.map(k => k.test)].reduce<Probe | null>((best, p) => p && (!best || p.at > best.at) ? p : best, null);
}

export function summarize(provider: Provider, workspace: Workspace | null | undefined): ProviderSummary {
  const w = workspace;
  const keys = w?.keys ?? [];
  const enabled = keys.filter(k => k.enabled);
  const lastTest = latestProbe(w);
  const requests = w?.usage.reduce((n, u) => n + u.requests, 0) ?? 0;
  const failures = w?.usage.reduce((n, u) => n + u.failures, 0) ?? 0;
  const known = w?.estimates.filter(e => e.usd !== null) ?? [];
  const usd = known.reduce((n, e) => n + (e.usd ?? 0), 0);
  const usdPartial = !!w && (w.estimates.some(e => e.usd === null) || w.usage.some(u => u.known < u.requests || !w.estimates.some(e => e.model === u.model)));
  const base = { provider, workspace, keyCount: keys.length, enabledKeys: enabled.length, modelCount: w?.models.length ?? 0, lastTest, requests, failures, usd, usdPartial };
  const out = (code: HealthCode, tone: Tone, label: string, hint: string): ProviderSummary => ({ ...base, code, tone, label, hint });
  if (w === undefined) return out('loading', 'unknown', '读取中', '正在读取 Key、模型与用量');
  if (w === null) return out('error', 'unknown', '状态未知', '未能读取详情，可进入详情重试');
  if (!provider.enabled) return out('disabled', 'muted', '已停用', '不参与路由；配置和密钥均保留');
  if (keys.length && !enabled.length) return out('no-enabled-key', 'bad', '无可用 Key', '所有 Key 都已停用');
  if (enabled.length && enabled.every(k => k.state === 'invalid')) return out('auth', 'bad', '认证失效', '所有启用的 Key 都被上游拒绝');
  const degraded = enabled.filter(k => ['cooling', 'half-open', 'invalid'].includes(k.state));
  if (degraded.length) return out('degraded', 'warn', '部分 Key 冷却/失效', `${degraded.length} / ${enabled.length} 个 Key 暂不可调度`);
  const downweighted = enabled.filter(k => k.state === 'degraded');
  if (downweighted.length) return out('degraded', 'warn', 'Key 已降权', `${downweighted.length} / ${enabled.length} 个 Key 已降权，仍可调度`);
  if (!w.models.length) return out('unmapped', 'warn', '未映射模型', 'Agent 还不能通过此供应商使用任何模型');
  if (!keys.length) return out('anonymous', 'warn', '匿名请求', '没有 Key，将以匿名方式请求上游');
  if (lastTest?.failure) return out('test-failed', 'warn', '最近测试失败', `${lastTest.model} · HTTP ${lastTest.http_status || '—'}`);
  if (!lastTest) return out('untested', 'ok', '可调度', '尚未测试推理；可调度只代表配置完整');
  return out('ready', 'ok', '可调度', `最近测试成功 · ${lastTest.elapsed_ms} ms`);
}

export interface AttentionItem { id: string; code: HealthCode; tone: Tone; title: string; detail: string; tab: DetailTab; action: string; dialog?: ProviderDialog }
const attentionPlan: Partial<Record<HealthCode, { title: string; tab: DetailTab; action: string; dialog?: ProviderDialog }>> = {
  error: { title: '详情读取失败', tab: 'overview', action: '查看详情' },
  'no-enabled-key': { title: '没有启用的 Key', tab: 'keys', action: '管理 Key' },
  auth: { title: 'Key 认证失效', tab: 'keys', action: '替换 Key' },
  degraded: { title: 'Key 调度状态需留意', tab: 'keys', action: '查看 Key' },
  unmapped: { title: '还没有映射模型', tab: 'models', action: '映射模型' },
  anonymous: { title: '没有配置 Key', tab: 'keys', action: '添加 Key', dialog: { kind: 'key' } },
  'test-failed': { title: '最近一次测试失败', tab: 'overview', action: '重新测试', dialog: { kind: 'test' } },
  untested: { title: '尚未测试推理', tab: 'overview', action: '测试连通', dialog: { kind: 'test' } },
};
/** Problems first (bad, warn), then low-priority "never tested" reminders. Disabled providers are skipped. */
export function attention(summaries: ProviderSummary[]): AttentionItem[] {
  const rank: Record<Tone, number> = { bad: 0, warn: 1, unknown: 2, ok: 3, muted: 4 };
  return summaries.flatMap(s => {
    const plan = attentionPlan[s.code];
    return plan ? [{ id: s.provider.id, code: s.code, tone: s.tone, title: plan.title, detail: s.hint, tab: plan.tab, action: plan.action, dialog: plan.dialog }] : [];
  }).sort((a, b) => rank[a.tone] - rank[b.tone] || a.id.localeCompare(b.id, 'zh-CN'));
}

export function overviewTotals(summaries: ProviderSummary[]) {
  const loaded = summaries.filter(s => s.workspace);
  return {
    providers: summaries.length,
    enabled: summaries.filter(s => s.provider.enabled).length,
    aliases: aggregateModels(summaries).length,
    routes: loaded.reduce((n, s) => n + s.modelCount, 0),
    requests: loaded.reduce((n, s) => n + s.requests, 0),
    failures: loaded.reduce((n, s) => n + s.failures, 0),
    usd: loaded.reduce((n, s) => n + s.usd, 0),
    usdPartial: loaded.some(s => s.usdPartial) || loaded.length < summaries.length,
    pending: summaries.some(s => s.workspace === undefined),
  };
}

export interface ModelCandidate { service_tier?: string; provider_enabled: boolean; provider_revision: number; availability: ChannelAvailability; provider_id: string; upstream: string; tools: boolean; streaming: boolean; cost_multiplier: number; multiplier: number; strategy: string; revision: number; candidate_index: number; candidate_count: number; key_ids: string[]; keys: KeyStatus[]; tone: Tone; label: string }
export interface ModelGroup { alias: string; revision: number; strategy: string; complete: boolean; candidates: ModelCandidate[] }
export function aggregateModels(summaries: ProviderSummary[]): ModelGroup[] {
  const groups = new Map<string, ModelCandidate[]>();
  for (const s of summaries) for (const m of s.workspace?.models ?? []) {
    const list = groups.get(m.id) ?? [];
    list.push({ service_tier: m.service_tier, provider_enabled: s.provider.enabled, provider_revision: s.provider.revision, availability: channelAvailability(s.provider.enabled, m.key_ids, s.workspace?.keys ?? []), cost_multiplier: s.provider.cost_multiplier, provider_id: s.provider.id, upstream: m.upstream, tools: m.tools, streaming: m.streaming, multiplier: m.multiplier, strategy: m.strategy, revision: m.revision, candidate_index: m.candidate_index, candidate_count: m.candidate_count, key_ids: m.key_ids, keys: s.workspace?.keys ?? [], tone: s.tone, label: s.label });
    groups.set(m.id, list);
  }
  return [...groups].map(([alias, candidates]) => {
    candidates.sort((a, b) => a.candidate_index - b.candidate_index);
    const first = candidates[0];
    const complete = candidates.length === first.candidate_count && candidates.every((c, i) => c.revision === first.revision && c.strategy === first.strategy && c.candidate_index === i);
    return { alias, revision: first.revision, strategy: first.strategy, complete, candidates };
  }).sort((a, b) => a.alias.localeCompare(b.alias, 'zh-CN'));
}

export function formatAgo(at: number, now = Date.now() / 1000): string {
  const diff = Math.max(0, Math.floor(now - at));
  if (diff < 60) return '刚刚';
  if (diff < 3600) return `${Math.floor(diff / 60)} 分钟前`;
  if (diff < 86400) return `${Math.floor(diff / 3600)} 小时前`;
  if (diff < 86400 * 30) return `${Math.floor(diff / 86400)} 天前`;
  return new Date(at * 1000).toLocaleDateString();
}
