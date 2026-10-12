import { checkedWorkspace } from '../features/providers/provider-features.model';
import { CONTRACT, PROBLEM_CODES, type DesktopBridge, type ProblemCode, type Provider as DesktopProvider, type SessionInfo } from './contract.generated';
import { ProviderProblem, checkedCatalog, type DiscoverProviderModels, type ProviderModels, type Provider, type ProviderTransport, type SaveProvider } from '../features/providers/provider.model';

declare global { interface Window { go?: { desktop?: { Bridge?: DesktopBridge } } } }
export class DesktopProblem extends Error { constructor(readonly code: ProblemCode) { super(code); } }
export function bridge(): DesktopBridge {
  const value = window.go?.desktop?.Bridge;
  if (!value) throw new DesktopProblem('unsupported');
  return value;
}
export async function call<T>(operation: () => Promise<{ contract: string; data: T; problem: { code: ProblemCode } | null }>, writing = false): Promise<NonNullable<T>> {
  try {
    const result = await operation();
    if (result.contract !== CONTRACT) throw new DesktopProblem(writing ? 'unknown' : 'unsupported');
    if (result.problem) {
      if (!PROBLEM_CODES.includes(result.problem.code)) throw new DesktopProblem(writing ? 'unknown' : 'read_error');
      throw new DesktopProblem(result.problem.code);
    }
    if (result.data === null || result.data === undefined) throw new DesktopProblem(writing ? 'unknown' : 'read_error');
    return result.data;
  } catch (error) {
    if (error instanceof DesktopProblem) throw error;
    throw new DesktopProblem(writing ? 'unknown' : 'offline');
  }
}
export function checkedProvider(p: DesktopProvider): Provider {
  if (!p || typeof p.id !== 'string' || typeof p.homepage_url !== 'string' || !Array.isArray(p.base_urls) || p.base_urls.length < 1 || p.base_urls.length > 16 ||
    p.base_urls.some(u => typeof u !== 'string' || !u) ||
    typeof p.enabled !== 'boolean' || typeof p.has_secret !== 'boolean' ||
    !Number.isSafeInteger(p.revision) || p.revision < 1 ||
    !Number.isInteger(p.timeout_seconds) || p.timeout_seconds < 1 || p.timeout_seconds > 3600 ||
    !Number.isInteger(p.max_concurrent) || p.max_concurrent < 1 || p.max_concurrent > 64) throw new ProviderProblem('read_error');
  // Pick only public DTO fields; unexpected response properties never reach state.
  return { id: p.id, homepage_url: p.homepage_url, base_urls: [...p.base_urls], enabled: p.enabled,
    has_secret: p.has_secret, revision: p.revision, timeout_seconds: p.timeout_seconds, max_concurrent: p.max_concurrent };
}
function providerError(error: unknown, writing: boolean): never {
  if (error instanceof DesktopProblem) {
    const code = error.code;
    if (code === 'referenced' || code === 'conflict' || code === 'unauthorized' || code === 'unknown' || code === 'invalid' || code === 'offline') throw new ProviderProblem(code);
  }
  throw new ProviderProblem(writing ? 'unknown' : 'read_error');
}
export class DesktopProviderTransport implements ProviderTransport {
  constructor(private readonly getBridge = bridge) {}
  async workspace(id: string, signal: AbortSignal) {
    signal.throwIfAborted();
    try { const result = await call(() => this.getBridge().ProviderWorkspace({ provider_id: id })); signal.throwIfAborted(); return checkedWorkspace(result); }
    catch (error) { return providerError(error, false); }
  }
  async action(input: import('./contract.generated').Action, signal: AbortSignal) {
    signal.throwIfAborted();
    try { const result = await call(() => this.getBridge().ProviderAction(input), true); signal.throwIfAborted(); return checkedWorkspace(result); }
    catch (error) { return providerError(error, true); }
  }
  async list(signal: AbortSignal): Promise<Provider[]> {
    signal.throwIfAborted();
    try {
      const rows = await call(() => this.getBridge().Providers());
      signal.throwIfAborted();
      if (!Array.isArray(rows)) throw new ProviderProblem('read_error');
      return rows.map(checkedProvider);
    } catch (error) { return providerError(error, false); }
  }
  async discover(input: DiscoverProviderModels, signal: AbortSignal): Promise<ProviderModels> {
    signal.throwIfAborted();
    try {
      const result = await call(() => this.getBridge().DiscoverProviderModels(input));
      signal.throwIfAborted();
      return checkedCatalog(result, input);
    } catch (error) {
      if (error instanceof DesktopProblem && error.code === 'unknown') throw new ProviderProblem('read_error');
      return providerError(error, false);
    }
  }
  async save(input: SaveProvider, signal: AbortSignal): Promise<Provider> {
    signal.throwIfAborted();
    try {
      const row = await call(() => this.getBridge().SaveProvider(input), true);
      signal.throwIfAborted();
      return checkedProvider(row);
    } catch (error) { return providerError(error, true); }
  }
}
export function checkedSession(info: SessionInfo): SessionInfo {
  if (!info || typeof info.instance_id !== 'string' || !info.instance_id || info.phase !== 'ready' ||
    !['started', 'reused'].includes(info.source) || typeof info.can_stop !== 'boolean') throw new DesktopProblem('read_error');
  return { instance_id: info.instance_id, phase: info.phase, source: info.source, can_stop: info.can_stop };
}
export const desktopSession = {
  start: async () => checkedSession(await call(() => bridge().Start())),
  status: async () => checkedSession(await call(() => bridge().Status())),
  stop: async (info: SessionInfo) => {
    const result = await call(() => bridge().StopCore({ instance_id: info.instance_id, approved: true }), true);
    if (result.accepted !== true) throw new DesktopProblem('unknown');
    return result;
  },
};
