import { bridge, call, DesktopProblem } from '../../desktop/transport';
import type { CCImportReport, CCImportRequest } from '../../desktop/contract.generated';
import { isValidProtocol } from './provider.model';

export type { CCImportReport };
export interface CCImportTransport { run(input: CCImportRequest, signal: AbortSignal): Promise<CCImportReport> }
export const ccImportTransport: CCImportTransport = {
  async run(input, signal) {
    signal.throwIfAborted();
    const value = await call(() => {
      const api = bridge();
      if (typeof api.ImportCCSwitch !== 'function') throw new DesktopProblem('unsupported');
      return api.ImportCCSwitch(input);
    }, input.approved);
    signal.throwIfAborted();
    try { return checkedImportReport(value); }
    catch { throw new DesktopProblem(input.approved ? 'unknown' : 'read_error'); }
  },
};

export function checkedImportReport(value: CCImportReport): CCImportReport {
  const text = (v: unknown): v is string => typeof v === 'string';
  const strings = (v: unknown): v is string[] => Array.isArray(v) && v.every(text);
  if (!value || !text(value.database_path) || !text(value.fingerprint) || !['', 'source_missing', 'source_invalid', 'source_limit'].includes(value.failure) ||
    (!value.failure && !/^[a-f0-9]{64}$/.test(value.fingerprint)) || typeof value.applied !== 'boolean' ||
    ![value.imported_providers, value.imported_keys, value.imported_models].every(n => Number.isSafeInteger(n) && n >= 0) ||
    !Array.isArray(value.providers) || !Array.isArray(value.models) || !Array.isArray(value.issues)) throw new Error('invalid report');
  // Keep only the public contract so accidental extra response fields never
  // persist in frontend state, even when an instance runs a different version.
  const providers = value.providers.map(p => {
    if (!p || !text(p.id) || !strings(p.base_urls) || !Array.isArray(p.sources) || !Array.isArray(p.keys) || !['ready', 'exists'].includes(p.status)) throw new Error('invalid provider');
    const sources = p.sources.map(s => { if (!s || !text(s.platform) || !text(s.name)) throw new Error('invalid source'); return { platform: s.platform, name: s.name }; });
    const keys = p.keys.map(k => { if (!k || !text(k.id) || !isValidProtocol(k.protocol)) throw new Error('invalid key'); return { id: k.id, protocol: k.protocol }; });
    return { id: p.id, base_urls: [...p.base_urls], sources, keys, status: p.status };
  });
  const models = value.models.map(m => {
    if (!m || !text(m.id) || !text(m.provider_id) || !text(m.upstream) || !strings(m.key_ids) || !m.key_ids.length) throw new Error('invalid mapping');
    return { id: m.id, provider_id: m.provider_id, upstream: m.upstream, key_ids: [...m.key_ids] };
  });
  const issues = value.issues.map(i => { if (!i || !text(i.platform) || !text(i.name) || !text(i.reason)) throw new Error('invalid issue'); return { platform: i.platform, name: i.name, reason: i.reason }; });
  return { database_path: value.database_path, fingerprint: value.fingerprint, providers, models, issues, failure: value.failure, applied: value.applied,
    imported_providers: value.imported_providers, imported_keys: value.imported_keys, imported_models: value.imported_models };
}

export const importReason: Record<string, string> = {
  source_missing: '目标实例账户下未找到 CC Switch 数据库，请检查路径。',
  source_invalid: '数据库无法读取或格式不受支持。',
  source_limit: '数据库超出导入上限（4096 条供应商或 16 MiB 配置）。',
  invalid_config: '配置格式无效',
  managed_auth: '需要 OAuth 或客户端托管认证，无法转换为 Anchor Key',
  request_overrides: '依赖自定义请求头、请求体或完整端点，暂不能可靠转换',
  unsupported_protocol: '此配置的上游协议尚不受 Anchor 支持',
  unsupported_platform: '此平台的配置格式暂不受支持',
  mixed_model_protocol: '模型具有不同的协议覆盖，暂不能可靠转换',
  missing_or_referenced_key: '缺少密钥或使用环境变量／文件引用，需手动填写',
  invalid_endpoint: '请求地址不符合 Anchor 规则，或超过 16 个',
  models_not_configured: '供应商可导入；没有明确模型，导入后请获取目录并配置映射',
  invalid_model: '模型配置不符合 Anchor 规则',
  candidate_limit: '公开模型已有 16 个候选，未添加此来源',
  key_limit: '同一供应商超过 64 个 Key',
};

export function importCounts(report: CCImportReport) {
  const ready = report.providers.filter(p => p.status === 'ready');
  return { providers: ready.length, keys: ready.reduce((sum, p) => sum + p.keys.length, 0), mappings: report.models.length };
}
