import workspaceFixture from '../../../../contract/provider-workspace.json';
import { checkedWorkspace, type Action, type Workspace } from './provider-features.model';
import catalogFixture from '../../../../contract/provider-models.json';
import fixture from '../../../../contract/providers.json';
import { checkedProvider } from '../../desktop/transport';
import { ProviderProblem, type Provider, type ProviderTransport, type DiscoverProviderModels, type ProviderModels, type SaveProvider } from './provider.model';
export const scenarios = {
  normal: '正常列表', empty: '空数据', offline: '实例离线', unauthorized: '授权不可用',
  read_error: '读取错误', conflict: '保存冲突', unknown: '写入结果未知',
} as const;
export type Scenario = keyof typeof scenarios;
// Entirely synthetic. All endpoints, timestamps and health are synthetic; no real credentials.
const examples = (): Provider[] => fixture.providers.map(checkedProvider);
const fixtureError = (code: keyof typeof fixture.errors) => new ProviderProblem(fixture.errors[code].code as ProviderProblem['code']);
const copy = (p: Provider) => ({ ...p });
async function delay(signal: AbortSignal) {
  if (signal.aborted) throw new DOMException('Aborted', 'AbortError');
  await new Promise<void>((resolve, reject) => {
    const abort = () => { clearTimeout(timer); reject(new DOMException('Aborted', 'AbortError')); };
    const timer = setTimeout(() => { signal.removeEventListener('abort', abort); resolve(); }, 240);
    signal.addEventListener('abort', abort, { once: true });
  });
}
/** Synthetic first-read state so overview, attention and model pages have something to show. */
function seed(id: string, w: Workspace) {
  const now = Math.floor(Date.now() / 1000);
  const mapping = (upstream: string) => ({ id: 'coding-main', display_name: 'coding-main', upstream, tools: true, streaming: true, revision: 1, candidate_count: 2 });
  if (id === '摸鱼AI') w.models = [mapping('demo-model-mini')];
  if (id === '备用供应商') {
    w.keys = [
      { id: 'backup', protocol: 'chat-completions', model_protocols: [], enabled: true, state: 'ready', failures: 0, retry_at: 0, test: { revision: w.revision, key_id: 'backup', model: 'backup-model', at: now - 7200, http_status: 200, elapsed_ms: 860, failure: '' } },
      { id: 'primary', protocol: 'responses', model_protocols: [], enabled: true, state: 'cooling', failures: 3, retry_at: now + 600, test: null },
    ];
    w.models = [mapping('backup-model')];
    w.usage = [{ provider_id: id, model: 'coding-main', requests: 42, failures: 1, elapsed_ms: 50400, known: 40, input_tokens: 120000, output_tokens: 30000, cache_read: 20000, cache_write: 0 }];
    w.estimates = [{ model: 'coding-main', usd: 0.0123 }];
  }
}
/** Simulates revision/secret semantics only. Secrets are discarded, never stored. */
export class MockProviderTransport implements ProviderTransport {
  private rows = examples().map(copy);
  private workspaces = new Map<string, Workspace>();
  private seeded = new Set<string>();
  private scenario: Scenario = 'normal';
  reset(scenario: Scenario) {
    this.scenario = scenario;
    this.workspaces.clear(); this.seeded.clear();
    this.rows = scenario === 'empty' ? fixture.empty.map(checkedProvider) : examples().map(copy);
  }
  async list(signal: AbortSignal): Promise<Provider[]> {
    await delay(signal);
    if (this.scenario === 'offline' || this.scenario === 'unauthorized' || this.scenario === 'read_error') throw fixtureError(this.scenario);
    return this.rows.map(copy);
  }
  async discover(input: DiscoverProviderModels, signal: AbortSignal): Promise<ProviderModels> {
    await delay(signal);
    if (this.scenario === 'offline' || this.scenario === 'unauthorized') throw fixtureError(this.scenario);
    const current = this.rows.find(p => p.id === input.provider_id);
    if (current?.revision !== input.expected_revision) throw fixtureError('conflict');
    const w = this.detail(current);
    const key = input.key_id ? w.keys.find(k => k.id === input.key_id) : w.keys.find(k => k.enabled);
    if (input.key_id && !key) throw new ProviderProblem('invalid');
    if (w.keys.length && !key) return { ...catalogFixture.success, provider_id: current.id, revision: current.revision, key_id: '', failure: 'authentication', http_status: 0, models: [] };
    return { ...catalogFixture.success, key_id: key?.id ?? 'anonymous', provider_id: current.id, revision: current.revision, models: [...catalogFixture.success.models] };
  }
  private detail(p: Provider): Workspace {
    let w = this.workspaces.get(p.id);
    if (!w) {
      w = checkedWorkspace({ ...structuredClone(workspaceFixture), provider_id: p.id, revision: p.revision, keys: p.has_secret ? structuredClone(workspaceFixture.keys) : [] });
      if (!this.seeded.has(p.id)) { this.seeded.add(p.id); seed(p.id, w); }
      this.workspaces.set(p.id, w);
    }
    w.revision = p.revision; return w;
  }
  async workspace(id: string, signal: AbortSignal): Promise<Workspace> {
    await delay(signal);
    if (this.scenario === 'offline' || this.scenario === 'unauthorized' || this.scenario === 'read_error') throw fixtureError(this.scenario);
    const p = this.rows.find(p => p.id === id); if (!p) throw new ProviderProblem('read_error');
    return structuredClone(this.detail(p));
  }
  async action(a: Action, signal: AbortSignal): Promise<Workspace> {
    await delay(signal);
    if (this.scenario === 'offline' || this.scenario === 'unauthorized') throw fixtureError(this.scenario);
    const p = this.rows.find(p => p.id === a.provider_id); if (!p) throw new ProviderProblem('read_error');
    if (this.scenario === 'conflict' || p.revision !== a.expected_revision) { this.scenario = 'normal'; throw fixtureError('conflict'); }
    const w = this.detail(p);
    switch (a.action) {
      case 'delete': if (!a.approved) throw fixtureError('invalid'); if (w.models.length) throw new ProviderProblem('referenced'); this.rows = this.rows.filter(row => row !== p); this.workspaces.delete(p.id); return { ...structuredClone(w), deleted: true };
      case 'rename': case 'copy': {
        if (!a.target || this.rows.some(p => p.id === a.target)) throw fixtureError('conflict');
        if (a.action === 'rename') { if (!a.approved) throw fixtureError('invalid'); this.workspaces.delete(p.id); p.id = a.target; p.revision++; w.provider_id = a.target; w.revision = p.revision; this.workspaces.set(p.id, w); }
        else { const copy = { ...p, id: a.target, enabled: false, has_secret: a.copy_keys && p.has_secret, revision: 1 }; this.rows.push(copy); const next = this.detail(copy); next.keys = a.copy_keys ? structuredClone(w.keys) : []; next.strategy = w.strategy; return structuredClone(next); }
        break;
      }
      case 'key-put': {
        const k = w.keys.find(k => k.id === a.target); if (!a.target || (!k && !a.secret)) throw fixtureError('invalid');
        if (a.protocol && !['responses', 'chat-completions', 'anthropic'].includes(a.protocol)) throw fixtureError('invalid');
        if (k) { k.protocol = a.protocol || k.protocol; k.enabled = a.enabled; k.state = a.enabled ? 'ready' : 'disabled'; }
        else w.keys.push({ id: a.target, protocol: a.protocol || 'responses', model_protocols: [], enabled: a.enabled, state: a.enabled ? 'ready' : 'disabled', failures: 0, retry_at: 0, test: null }); p.has_secret = !!w.keys.length; p.revision++; break;
      }
      case 'key-model-protocol': {
        const k = w.keys.find(k => k.id === a.target);
        if (!k || !a.model || (a.protocol && a.protocol !== 'inherit' && !['responses', 'chat-completions', 'anthropic'].includes(a.protocol))) throw fixtureError('invalid');
        k.model_protocols = k.model_protocols.filter(m => m.model !== a.model);
        if (a.protocol && a.protocol !== 'inherit') k.model_protocols.push({ model: a.model, protocol: a.protocol });
        p.revision++; break;
      }
      case 'key-delete': if (!a.approved) throw fixtureError('invalid'); w.keys = w.keys.filter(k => k.id !== a.target); p.has_secret = !!w.keys.length; p.revision++; break;
      case 'key-reset': { const k = w.keys.find(k => k.id === a.target); if (!a.approved || !k) throw fixtureError('invalid'); k.failures = 0; k.retry_at = 0; k.state = k.enabled ? 'ready' : 'disabled'; break; }
      case 'strategy': w.strategy = a.strategy; p.revision++; break;
      case 'model-put': { const old = w.models.find(m => m.id === a.target); if (!a.target || !a.model || (old?.revision ?? 0) !== a.model_revision) throw fixtureError('conflict'); w.models = w.models.filter(m => m.id !== a.target); w.models.push({ id: a.target, display_name: a.target, upstream: a.model, tools: a.tools, streaming: a.streaming, revision: a.model_revision + 1, candidate_count: 1 }); break; }
      case 'model-delete': if (!a.approved) throw fixtureError('invalid'); w.models = w.models.filter(m => m.id !== a.target); break;
      case 'test': if (!a.approved) throw fixtureError('invalid'); w.probe = { revision: p.revision, key_id: a.target, model: a.model, at: Math.floor(Date.now() / 1000), http_status: 200, elapsed_ms: 120, failure: '' }; { const k = w.keys.find(k => k.id === a.target); if (k) k.test = { ...w.probe }; } break;
      case 'price-import': w.imported_price = { model: a.target, input_usd: 1, output_usd: 3, cache_read_usd: null, cache_write_usd: null, source: 'prototype/synthetic', updated_at: 1791648000 }; break;
      case 'price-put': if (!a.price) throw fixtureError('invalid'); w.prices = [...w.prices.filter(p => p.model !== a.target), { ...a.price, model: a.target }]; w.imported_price = null; p.revision++; break;
      default: throw fixtureError('invalid');
    }
    w.revision = p.revision;
    if (this.scenario === 'unknown') { this.scenario = 'normal'; throw fixtureError('unknown'); }
    return structuredClone(w);
  }
  async save(input: SaveProvider, signal: AbortSignal): Promise<Provider> {
    await delay(signal);
    if (this.scenario === 'offline' || this.scenario === 'unauthorized') throw fixtureError(this.scenario);
    const current = this.rows.find(p => p.id === input.provider.id);
    if (this.scenario === 'conflict') {
      // One external edit, then permit a read/review/fresh revision cycle.
      if (current) { current.revision++; current.timeout_seconds = 180; }
      this.scenario = 'normal';
      throw fixtureError('conflict');
    }
    if ((input.expected_revision === 0 && current) || (input.expected_revision > 0 && current?.revision !== input.expected_revision)) throw fixtureError('conflict');
    if (!input.provider.base_urls.length || input.provider.base_urls.length > 16) throw fixtureError('invalid');
    const saved: Provider = { ...input.provider, base_urls: [...input.provider.base_urls], revision: input.expected_revision + 1,
      has_secret: input.secret === null ? current?.has_secret ?? false : !!input.secret };
    if (current) {
      const w = this.detail(current);
      if (input.secret !== null) {
        const k = w.keys.find(k => k.id === 'default');
        if (!input.secret) w.keys = w.keys.filter(k => k.id !== 'default');
        else if (k) { k.enabled = true; k.state = 'ready'; }
        else w.keys.push({ id: 'default', protocol: 'responses', model_protocols: [], enabled: true, state: 'ready', failures: 0, retry_at: 0, test: null });
      }
      saved.has_secret = w.keys.length > 0;
    }
    this.rows = [...this.rows.filter(p => p.id !== saved.id), saved].sort((a, b) => a.id.localeCompare(b.id, 'zh-CN'));
    if (this.scenario === 'unknown') { this.scenario = 'normal'; throw fixtureError('unknown'); }
    return copy(saved);
  }
}
