import fixture from '../../../../contract/providers.json';
import { checkedProvider } from '../../desktop/transport';
import { ProviderProblem, type Provider, type ProviderTransport, type SaveProvider } from './provider.model';
export const scenarios = {
  normal: '正常列表', empty: '空数据', offline: '实例离线', unauthorized: '授权不可用',
  read_error: '读取错误', conflict: '保存冲突', unknown: '写入结果未知',
} as const;
export type Scenario = keyof typeof scenarios;
// Entirely synthetic. No real endpoints, credentials, timestamps or health claims.
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
/** Simulates revision/secret semantics only. Secrets are discarded, never stored. */
export class MockProviderTransport implements ProviderTransport {
  private rows = examples().map(copy);
  private scenario: Scenario = 'normal';
  reset(scenario: Scenario) {
    this.scenario = scenario;
    this.rows = scenario === 'empty' ? fixture.empty.map(checkedProvider) : examples().map(copy);
  }
  async list(signal: AbortSignal): Promise<Provider[]> {
    await delay(signal);
    if (this.scenario === 'offline' || this.scenario === 'unauthorized' || this.scenario === 'read_error') throw fixtureError(this.scenario);
    return this.rows.map(copy);
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
    if (input.provider.protocol !== 'responses') throw fixtureError('invalid');
    const saved: Provider = { ...input.provider, protocol: 'responses', revision: input.expected_revision + 1,
      has_secret: input.secret === null ? current?.has_secret ?? false : !!input.secret };
    this.rows = [...this.rows.filter(p => p.id !== saved.id), saved].sort((a, b) => a.id.localeCompare(b.id, 'zh-CN'));
    if (this.scenario === 'unknown') { this.scenario = 'normal'; throw fixtureError('unknown'); }
    return copy(saved);
  }
}
