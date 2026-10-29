import { validRoutingRule, checkedRouting, type ModelRouting } from '../models/model-routing.model';
import type { ProbeProgress, ProbeRequest } from '../../desktop/contract.generated';
import accountFixture from '../../../../contract/provider-account.json';
import type { AccountSettings, SaveAccountSettings, AccountQuery, AccountObservation, AccountRateObservation } from './provider-account.model';
import mappingFixture from '../../../../contract/provider-model-mapping.json';
import workspaceFixture from '../../../../contract/provider-workspace.json';
import { isValidModelAlias, checkedWorkspace, type Action, type Workspace } from './provider-features.model';
import catalogFixture from '../../../../contract/provider-models.json';
import fixture from '../../../../contract/providers.json';
import { checkedProvider } from '../../desktop/transport';
import { newDraft, validateDraft, ProviderProblem, type Provider, type ProviderTransport, type DiscoverProviderModels, type ProviderModels, type SaveProvider } from './provider.model';
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
  const mapping = (upstream: string) => ({ ...mappingFixture, upstream });
  if (id === '摸鱼AI') w.models = [mapping('demo-model-mini')];
  if (id === '备用供应商') {
    w.keys = [
      { rate: null, effective_multiplier: null, id: 'backup', protocol: 'chat-completions', model_protocols: [], enabled: true, state: 'ready', failures: 0, retry_at: 0, test: { revision: w.revision, key_id: 'backup', model: 'backup-model', at: now - 7200, http_status: 200, elapsed_ms: 860, failure: '' } },
      { rate: null, effective_multiplier: null, id: 'primary', protocol: 'responses', model_protocols: [], enabled: true, state: 'degraded', failures: 3, retry_at: now + 600, test: null },
    ];
    w.models = [{ ...mapping('backup-model'), candidate_index: 1 }];
    w.usage = [{ provider_id: id, model: 'coding-main', requests: 42, failures: 1, elapsed_ms: 50400, known: 40, input_tokens: 120000, output_tokens: 30000, cache_read: 20000, cache_write: 0 }];
    w.estimates = [{ model: 'coding-main', usd: 0.0123 }];
  }
}
/** Simulates revision/secret semantics only. Secrets are discarded, never stored. */
export class MockProviderTransport implements ProviderTransport {
  private rows = examples().map(copy);
  private workspaces = new Map<string, Workspace>();
  private accounts = new Map<string, AccountSettings>();
  private seeded = new Set<string>();
  private probes = new Map<string, { value: ProbeProgress; started: number }>();
  private scenario: Scenario = 'normal';
  reset(scenario: Scenario) {
    this.scenario = scenario;
    this.probes.clear(); this.accounts.clear(); this.workspaces.clear(); this.seeded.clear();
    this.rows = scenario === 'empty' ? fixture.empty.map(checkedProvider) : examples().map(copy);
  }
  async accountSettings(id: string, signal: AbortSignal): Promise<AccountSettings> {
    await delay(signal);
    const p = this.rows.find(p => p.id === id); if (!p) throw new ProviderProblem('invalid');
    const saved = this.accounts.get(id) ?? { ...accountFixture.settings, platform: 'auto', provider_id: id };
    return { ...saved, revision: p.revision };
  }
  async detectPlatform(q: AccountQuery, signal: AbortSignal): Promise<AccountSettings> {
    const settings = await this.accountSettings(q.provider_id, signal);
    if (settings.revision !== q.expected_revision) throw new ProviderProblem('conflict');
    const value = { ...settings, platform: 'auto', detected_platform: 'sub2api', detection_failure: '' };
    this.accounts.set(q.provider_id, value);
    return value;
  }
  async saveAccount(input: SaveAccountSettings, signal: AbortSignal): Promise<AccountSettings> {
    await delay(signal);
    const p = this.rows.find(p => p.id === input.provider_id);
    if (!p || p.revision !== input.expected_revision || this.scenario === 'conflict') { this.scenario = 'normal'; throw new ProviderProblem('conflict'); }
    const old = this.accounts.get(p.id);
    const sameTarget = old && old.platform === input.platform && old.site_url === input.site_url && old.user_id === input.user_id;
    const v: AccountSettings = { provider_id: p.id, revision: ++p.revision, platform: input.platform, detected_platform: old?.detected_platform ?? 'sub2api', detection_failure: old?.detection_failure ?? '', site_url: input.site_url, user_id: input.user_id,
      has_user_token: input.user_token === null ? !!(sameTarget && old.has_user_token) : !!input.user_token };
    this.accounts.set(p.id, v);
    if (this.scenario === 'unknown') { this.scenario = 'normal'; throw new ProviderProblem('unknown'); }
    return { ...v };
  }
  async rate(q: AccountQuery, signal: AbortSignal): Promise<AccountRateObservation> {
    await delay(signal);
    const settings = await this.accountSettings(q.provider_id, signal);
    if (settings.revision !== q.expected_revision) throw new ProviderProblem('conflict');
    const platform = settings.platform && settings.platform !== 'auto' ? settings.platform : 'sub2api';
    const failure = platform === 'oneapi' ? 'unsupported' : !settings.has_user_token ? 'auth_required' : '';
    const p = this.rows.find(p => p.id === q.provider_id)!;
    const key = this.detail(p).keys.find(k => k.id === (q.key_id || 'default'));
    if (key && q.remote_key_id === 0) {
      const now = new Date().toISOString();
      if (failure) { key.rate = { multiplier: null, group: '', group_name: '', basis: '', observed_at: '', ...key.rate, attempted_at: now, failure }; }
      else key.rate = { multiplier: Number(accountFixture.rate.rate.multiplier), group: accountFixture.rate.rate.group, group_name: accountFixture.rate.rate.group_name, basis: accountFixture.rate.rate.basis, observed_at: now, attempted_at: now, failure: '' };
    }
    return { ...accountFixture.rate, provider_id: q.provider_id, revision: q.expected_revision, platform,
      key_id: q.key_id || 'default', remote_key_id: q.remote_key_id, observed_at: new Date().toISOString(), failure,
      rate: failure ? null : { ...accountFixture.rate.rate } };
  }
  async allowance(q: AccountQuery, signal: AbortSignal): Promise<AccountObservation> {
    await delay(signal);
    const p = this.rows.find(p => p.id === q.provider_id); if (!p || p.revision !== q.expected_revision) throw new ProviderProblem('conflict');
    const saved = this.accounts.get(p.id) ?? accountFixture.settings;
    const settings = { ...saved, platform: !saved.platform || saved.platform === 'auto' ? 'sub2api' : saved.platform };
    const w = this.detail(p), key = q.key_id ? w.keys.find(k => k.id === q.key_id) : w.keys.find(k => k.id === 'default' && k.enabled) ?? w.keys.find(k => k.enabled);
    if (q.key_id && !key) throw new ProviderProblem('invalid');
    const v = structuredClone(q.operation === 'balance-summary' ? accountFixture.summary : q.operation === 'balance' ? accountFixture.balance : accountFixture.quota);
    Object.assign(v, { provider_id: p.id, revision: p.revision, platform: settings.platform, operation: q.operation, key_id: key?.id ?? 'anonymous', remote_key_id: q.remote_key_id, observed_at: new Date().toISOString() });
    let failure = '';
    if (q.operation === 'balance-summary' && (!key || settings.platform === 'oneapi')) failure = 'auth_required';
    else if (settings.platform !== 'sub2api' && !settings.has_user_token && (q.operation === 'balance' || settings.platform === 'oneapi')) failure = 'auth_required';
    else if (settings.platform === 'newapi' && settings.has_user_token && !settings.user_id && q.operation === 'balance') failure = 'auth_required';
    else if (settings.platform === 'oneapi' && q.operation === 'key-quota' && !q.remote_key_id) failure = 'remote_key_required';
    else if ((settings.platform === 'sub2api' || settings.platform === 'newapi' && q.operation === 'key-quota') && !key && !(q.operation === 'balance' && settings.has_user_token)) failure = 'auth_required';
    const out: AccountObservation = { ...v, failure, allowance: failure ? null : v.allowance };
    if (out.allowance && settings.platform !== 'sub2api') {
      out.allowance = { scope: q.operation === 'balance' ? 'account' : 'key', remaining: { value: '1250000', unit: 'quota' }, used: { value: '250000', unit: 'quota' }, limit: null, unlimited: null, windows: [] };
    }
    return out;
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
    const savedKeys = w.keys;
    const selected = input.key_id ? savedKeys.filter(k => k.id === input.key_id) : savedKeys;
    if (input.key_id && !selected.length && !(input.key_id === 'anonymous' && !savedKeys.length)) throw new ProviderProblem('invalid');
    const keys = selected.filter(k => k.enabled);
    if (!keys.length && savedKeys.length) {
      return { ...catalogFixture.success, key_id: input.key_id, provider_id: current.id, revision: current.revision, http_status: 0, models: [], entries: [], keys: [], failure: 'no_enabled_keys', partial: false };
    }
    const observations = (keys.length ? keys : [{ id: 'anonymous', enabled: true, protocol: 'responses' }]).map((k, i) => ({
      ...catalogFixture.success.keys[0], id: k.id, enabled: k.enabled, protocol: k.protocol,
      ...(i ? { group: 'premium', group_name: '高级组', multiplier: '1.5' } : {}),
    }));
    for (const observation of observations) {
      const key = w.keys.find(k => k.id === observation.id);
      if (key && observation.multiplier != null) { const now = new Date().toISOString(); key.rate = { multiplier: Number(observation.multiplier), group: observation.group, group_name: observation.group_name, basis: observation.rate_basis, observed_at: now, attempted_at: now, failure: '' }; }
    }
    return { ...catalogFixture.success, key_id: input.key_id || (keys.length ? '' : 'anonymous'), provider_id: current.id, revision: current.revision,
      models: [...catalogFixture.success.models], entries: catalogFixture.success.models.map(id => ({ id, key_ids: observations.map(k => k.id) })), keys: observations };

  }
  private detail(p: Provider): Workspace {
    let w = this.workspaces.get(p.id);
    if (!w) {
      w = checkedWorkspace({ ...structuredClone(workspaceFixture), provider_id: p.id, revision: p.revision, keys: p.has_secret ? structuredClone(workspaceFixture.keys).map(k => ({ ...k, protocol: p.protocol || k.protocol })) : [] });
      if (!this.seeded.has(p.id)) { this.seeded.add(p.id); seed(p.id, w); }
      this.workspaces.set(p.id, w);
    }
    for (const key of w.keys) key.effective_multiplier = key.rate?.multiplier == null ? null : p.cost_multiplier * key.rate.multiplier;
    w.revision = p.revision; return w;
  }
  async probe(input: ProbeRequest, signal: AbortSignal): Promise<ProbeProgress> {
    await delay(signal);
    const p = this.rows.find(p => p.id === input.provider_id);
    if (!p || p.revision !== input.expected_revision) throw fixtureError('conflict');
    const w = this.detail(p);
    if (input.action === 'start') {
      if (!input.approved || !input.model || (w.keys.length && !w.keys.some(k => k.id === input.key_id && k.enabled))) throw fixtureError('invalid');
      const value: ProbeProgress = { provider_id: p.id, revision: p.revision, run_id: `synthetic-${Date.now()}`, key_id: input.key_id, model: input.model,
        status: 'running', phase: 'connecting', text: '', elapsed_ms: 0, first_byte_ms: 0, result: null };
      this.probes.set(value.run_id, { value, started: Date.now() }); return structuredClone(value);
    }
    const run = this.probes.get(input.run_id);
    if (!run || run.value.provider_id !== p.id) throw fixtureError('invalid');
    const value = run.value; value.elapsed_ms = Date.now() - run.started;
    if (input.action === 'cancel') value.status = 'cancelled';
    if (value.status === 'running') {
      value.phase = 'receiving'; value.text = value.elapsed_ms > 700 ? 'OK' : 'O';
      if (!value.first_byte_ms) value.first_byte_ms = Math.max(1, value.elapsed_ms);
      if (value.elapsed_ms > 1100) value.status = 'completed';
    }
    if (value.status !== 'running') {
      value.phase = 'complete';
      value.result = { revision: p.revision, key_id: value.key_id, model: value.model, at: Math.floor(Date.now() / 1000), http_status: 200,
        elapsed_ms: value.elapsed_ms, failure: value.status === 'cancelled' ? 'network' : '' };
      const k = w.keys.find(k => k.id === value.key_id); if (k) k.test = value.result; w.probe = value.result;
    }
    return structuredClone(value);
  }
  async workspace(id: string, signal: AbortSignal): Promise<Workspace> {
    await delay(signal);
    if (this.scenario === 'offline' || this.scenario === 'unauthorized' || this.scenario === 'read_error') throw fixtureError(this.scenario);
    const p = this.rows.find(p => p.id === id); if (!p) throw new ProviderProblem('read_error');
    return structuredClone(this.detail(p));
  }
  async routing(input: ModelRouting, signal: AbortSignal) {
    await delay(signal);
    const candidates = this.rows.flatMap(p => this.detail(p).models.filter(m => m.id === input.model_id).map(m => ({ p, m })));
    if (!candidates.length || candidates.some(c => c.m.revision !== input.expected_revision) || this.scenario === 'conflict') { this.scenario = 'normal'; throw fixtureError('conflict'); }
    if (!validRoutingRule(input.strategy) || candidates.length !== input.order.length) throw fixtureError('invalid');
    const ordered = input.order.map(ref => candidates.find(c => c.p.id === ref.provider_id && c.m.upstream === ref.model));
    if (ordered.some(c => !c) || new Set(ordered).size !== candidates.length) throw fixtureError('invalid');
    if (input.source_order) {
      const sources = candidates.flatMap(c => {
        const keys = this.detail(c.p).keys;
        const ids = c.m.key_ids.length ? c.m.key_ids : keys.length ? keys.map(k => k.id) : ['anonymous'];
        return ids.map(key_id => JSON.stringify([c.p.id, c.m.upstream, key_id]));
      });
      const refs = input.source_order.map(c => JSON.stringify([c.provider_id,c.model,c.key_id]));
      if (refs.length !== sources.length || new Set(refs).size !== refs.length || refs.some(ref => !sources.includes(ref))) throw fixtureError('invalid');
    }
    ordered.forEach((c, i) => Object.assign(c!.m, { source_order: input.source_order, candidate_index: i, strategy: input.strategy, revision: input.expected_revision + 1 }));
    if (this.scenario === 'unknown') throw fixtureError('unknown');
    return checkedRouting({ model_id: input.model_id, revision: input.expected_revision + 1, strategy: input.strategy, order: input.order, ...(input.source_order ? { source_order: input.source_order } : {}) }, input);
  }
  private deleteKey(w: Workspace, keyID: string) {
    if (!w.keys.some(k => k.id === keyID)) throw fixtureError('invalid');
    const all = this.rows.map(p => this.detail(p));
    w.keys = w.keys.filter(k => k.id !== keyID);
    const affected = new Set<string>();
    w.models = w.models.filter(m => {
      if (m.key_ids.length ? !m.key_ids.includes(keyID) : w.keys.length > 0) return true;
      affected.add(m.id);
      m.key_ids = m.key_ids.filter(id => id !== keyID);
      return m.key_ids.length > 0;
    });
    this.pruneDeletedMappings(all, affected);
  }
  private pruneDeletedMappings(all: Workspace[], affected: Set<string>) {
    for (const id of affected) {
      const remaining = all.flatMap(w => w.models.filter(m => m.id === id));
      remaining.sort((a, b) => a.candidate_index - b.candidate_index).forEach((m, i) => {
        m.revision++; m.candidate_count = remaining.length; m.candidate_index = i;
      });
      for (const w of all) if (!w.models.some(m => m.id === id)) w.prices = w.prices.filter(p => p.model !== id);
    }
  }
  async action(a: Action, signal: AbortSignal): Promise<Workspace> {
    await delay(signal);
    if (this.scenario === 'offline' || this.scenario === 'unauthorized') throw fixtureError(this.scenario);
    const p = this.rows.find(p => p.id === a.provider_id); if (!p) throw new ProviderProblem('read_error');
    if (this.scenario === 'conflict' || p.revision !== a.expected_revision) { this.scenario = 'normal'; throw fixtureError('conflict'); }
    const w = this.detail(p);
    switch (a.action) {
      case 'delete': {
        if (!a.approved) throw fixtureError('invalid');
        const all = this.rows.map(p => this.detail(p)), affected = new Set(w.models.map(m => m.id));
        w.models = []; w.keys = []; w.prices = [];
        this.pruneDeletedMappings(all, affected);
        this.rows = this.rows.filter(row => row !== p); this.workspaces.delete(p.id); this.accounts.delete(p.id);
        return { ...structuredClone(w), deleted: true };
      }
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
        else w.keys.push({ rate: null, effective_multiplier: null, id: a.target, protocol: a.protocol || 'responses', model_protocols: [], enabled: a.enabled, state: a.enabled ? 'ready' : 'disabled', failures: 0, retry_at: 0, test: null }); p.has_secret = !!w.keys.length; p.revision++; break;
      }
      case 'key-model-protocol': {
        const k = w.keys.find(k => k.id === a.target);
        if (!k || !a.model || (a.protocol && a.protocol !== 'inherit' && !['responses', 'chat-completions', 'anthropic'].includes(a.protocol))) throw fixtureError('invalid');
        k.model_protocols = k.model_protocols.filter(m => m.model !== a.model);
        if (a.protocol && a.protocol !== 'inherit') k.model_protocols.push({ model: a.model, protocol: a.protocol });
        p.revision++; break;
      }
      case 'key-delete': if (!a.approved) throw fixtureError('invalid'); this.deleteKey(w, a.target); p.has_secret = !!w.keys.length; p.revision++; break;
      case 'key-reset': case 'key-reset-all': {
        const keys = a.action === 'key-reset-all' ? w.keys : w.keys.filter(k => k.id === a.target);
        if (!a.approved || (a.action === 'key-reset-all' ? !!a.target : !keys.length)) throw fixtureError('invalid');
        for (const k of keys) { k.failures = 0; if (k.error_score != null) k.error_score = 0; k.retry_at = 0; k.test = null; k.state = k.enabled ? 'ready' : 'disabled'; }
        break;
      }
      case 'strategy': w.strategy = a.strategy; p.revision++; break;
      case 'model-put': {
        const speeds = a.speed_variants ?? [];
        const allModels = this.rows.flatMap(p => this.detail(p).models);
        const previous = allModels.find(m => m.id === a.target);
        const serviceTier = a.service_tier ?? previous?.service_tier ?? '';
        if (!['', 'priority', 'ultrafast'].includes(serviceTier) || speeds.length > 2 || new Set(speeds).size !== speeds.length || speeds.some(speed => !['fast', 'ultrafast'].includes(speed) || serviceTier || !isValidModelAlias(`${a.target}(${speed})`))) throw fixtureError('invalid');
        if (speeds.some(speed => allModels.some(m => m.id === `${a.target}(${speed})`))) throw fixtureError('conflict');

        const all = this.rows.map(p => this.detail(p));
        const mappings = all.flatMap(w => w.models.filter(m => m.id === a.target));
        const local = w.models.find(m => m.id === a.target);
        const shared = mappings[0];
        if (!a.target || !a.model) throw fixtureError('invalid');
        if (a.model_revision === 0 ? !!local : !local || local.revision !== a.model_revision) throw fixtureError('conflict');
        const multiplier = a.multiplier || local?.multiplier || 1;
        if (!Number.isFinite(multiplier) || multiplier < 0.01 || multiplier > 1000) throw fixtureError('invalid');
        const strategy = a.model_revision > 0 || !shared ? a.strategy || shared?.strategy || 'round-robin' : shared.strategy;
        if (!validRoutingRule(strategy)) throw fixtureError('invalid');
        if (shared && !a.model_revision && a.service_tier != null && serviceTier !== (shared.service_tier ?? '')) throw fixtureError('conflict');
        const revision = (shared?.revision ?? 0) + 1;
        const candidate_count = mappings.length + (local ? 0 : 1);
        const tools = a.model_revision > 0 || !shared ? a.tools : shared.tools;
        const streaming = a.model_revision > 0 || !shared ? a.streaming : shared.streaming;
        for (const m of mappings) Object.assign(m, { revision, candidate_count, strategy, tools, streaming, ...(serviceTier || a.service_tier != null ? { service_tier: serviceTier } : {}) });
        if (local) Object.assign(local, { upstream: a.model, multiplier, key_ids: a.key_ids ?? local.key_ids });
        else w.models.push({ key_ids: a.key_ids ?? [], candidate_index: mappings.length, id: a.target, display_name: shared?.display_name ?? a.target, upstream: a.model, multiplier, revision, candidate_count, strategy, tools, streaming });
        const saved = w.models.find(m => m.id === a.target)!;
        if (serviceTier || a.service_tier != null) saved.service_tier = serviceTier;
        for (const speed of speeds) w.models.push({ ...structuredClone(saved), id: `${a.target}(${speed})`, display_name: `${saved.display_name}(${speed})`, service_tier: speed === 'fast' ? 'priority' : 'ultrafast', revision: 1, candidate_count: 1, candidate_index: 0 });
        break;
      }
      case 'model-delete': {
        if (!a.approved) throw fixtureError('invalid');
        const all = this.rows.map(p => this.detail(p));
        const local = w.models.find(m => m.id === a.target);
        if (!local || local.revision !== a.model_revision) throw fixtureError('conflict');
        w.models = w.models.filter(m => m !== local);
        w.prices = w.prices.filter(p => p.model !== a.target);
        const remaining = all.flatMap(w => w.models.filter(m => m.id === a.target));
        remaining.sort((a,b) => a.candidate_index-b.candidate_index).forEach((m,i) => { m.revision++; m.candidate_count = remaining.length; m.candidate_index = i; });
        break;
      }
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
    if (input.provider.protocol && !['responses', 'chat-completions', 'anthropic'].includes(input.provider.protocol)) throw fixtureError('invalid');
    const initialKeys = input.initial_keys ?? [];
    if (initialKeys.length && (input.expected_revision !== 0 || Object.keys(validateDraft({ ...newDraft(input.provider), secret: input.secret ?? '', initialKeys })).length)) throw fixtureError('invalid');
    const saved: Provider = { ...input.provider, protocol: input.provider.protocol || current?.protocol || 'responses', base_urls: [...input.provider.base_urls], revision: input.expected_revision + 1,
      has_secret: !!initialKeys.length || (input.secret === null ? current?.has_secret ?? false : !!input.secret) };
    if (current) {
      const w = this.detail(current);
      if (input.secret !== null) {
        const k = w.keys.find(k => k.id === 'default');
        if (!input.secret) { if (k) this.deleteKey(w, 'default'); }
        else if (k) { k.enabled = true; k.state = 'ready'; }
        else w.keys.push({ rate: null, effective_multiplier: null, id: 'default', protocol: saved.protocol!, model_protocols: [], enabled: true, state: 'ready', failures: 0, retry_at: 0, test: null });
      }
      if (input.provider.protocol) {
        const key = w.keys.find(k => k.id === 'default');
        if (key) key.protocol = input.provider.protocol;
      }
      saved.has_secret = w.keys.length > 0;
    }
    if (!current && initialKeys.length) {
      const w = this.detail(saved);
      w.keys = [...(input.secret ? [{ id: 'default', protocol: saved.protocol! }] : []), ...initialKeys].map(key => ({
        id: key.id, protocol: key.protocol, enabled: true, model_protocols: [], state: 'ready', failures: 0, retry_at: 0, test: null, rate: null, effective_multiplier: null,
      }));
    }
    this.rows = current ? this.rows.map(p => p.id === saved.id ? saved : p) : [...this.rows, saved].sort((a, b) => a.id.localeCompare(b.id, 'zh-CN'));
    if (this.scenario === 'unknown') { this.scenario = 'normal'; throw fixtureError('unknown'); }
    return copy(saved);
  }
}
