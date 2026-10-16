import { bridge, call, DesktopProblem } from '../../desktop/transport';
import type { DesktopBridge, AgentWorkspace, AgentTarget, AgentModel, EnrollAgent, ManageAgent, ManagedAgent } from '../../desktop/contract.generated';
export type { AgentWorkspace, AgentTarget, AgentModel, ManagedAgent } from '../../desktop/contract.generated';

export function checkedAgentWorkspace(value: AgentWorkspace): AgentWorkspace {
  const bad = () => { throw new DesktopProblem('read_error'); };
  if (!value || !Array.isArray(value.targets) || !Array.isArray(value.bindings) || !Array.isArray(value.models)) return bad();
  return {
    targets: value.targets.map(t => {
      if (!t || ![t.id, t.agent, t.config_root, t.source, t.status, t.catalog_version].every(v => typeof v === 'string') || typeof t.supported !== 'boolean' || !Array.isArray(t.templates)) return bad();
      return { id: t.id, agent: t.agent, config_root: t.config_root, source: t.source, supported: t.supported, status: t.status, catalog_version: t.catalog_version,
        templates: t.templates.map(v => { if (!v || typeof v.slug !== 'string' || typeof v.display_name !== 'string') return bad(); return { slug: v.slug, display_name: v.display_name }; }) };
    }),
    models: value.models.map(m => {
      if (!m || typeof m.id !== 'string' || typeof m.tools !== 'boolean' || typeof m.streaming !== 'boolean' || !Array.isArray(m.upstream_models) || m.upstream_models.some(v => typeof v !== 'string') || !Array.isArray(m.available_targets) || m.available_targets.some(v => typeof v !== 'string')) return bad();
      return { id: m.id, tools: m.tools, streaming: m.streaming, upstream_models: [...m.upstream_models], available_targets: [...m.available_targets] };
    }),
    bindings: value.bindings.map(b => {
      if (!b || ![b.id, b.adapter_id, b.config_root, b.model, b.model_scope, b.catalog_version, b.status, b.catalog_path].every(v => typeof v === 'string') || !Number.isSafeInteger(b.revision) || b.revision < 1 || typeof b.enabled !== 'boolean' || !Array.isArray(b.available_models) || b.available_models.some(v => typeof v !== 'string') || !b.model_templates || Array.isArray(b.model_templates) || typeof b.model_templates !== 'object' || Object.values(b.model_templates).some(v => typeof v !== 'string')) return bad();
      return { id: b.id, adapter_id:b.adapter_id, config_root: b.config_root, model: b.model, enabled: b.enabled, revision: b.revision, model_scope: b.model_scope, catalog_version: b.catalog_version, status: b.status, catalog_path: b.catalog_path, available_models: [...b.available_models], model_templates: { ...b.model_templates } };
    }),
  };
}

/** The core resolves mapping metadata and provider availability for each target. */
export function availableAgentModels(data: AgentWorkspace, target: AgentTarget): AgentModel[] {
  return data.models.filter(m => m.available_targets.includes(target.id));
}
export function createAgentService(getBridge: () => DesktopBridge = bridge) { return {
  workspace: async () => checkedAgentWorkspace(await call(() => getBridge().AgentWorkspace())),
  enroll: async (input: EnrollAgent) => checkedAgentWorkspace(await call(() => getBridge().EnrollAgent(input), true)),
  manage: async (input: ManageAgent) => checkedAgentWorkspace(await call(() => getBridge().ManageAgent(input), true)),
}; }
export const agentService = createAgentService();

export const targetStatus: Record<string, string> = {
  ready: '可托管', unsupported: '尚无适配器', unsafe: '目录权限或类型不支持', config_unsupported: '配置含暂不支持的覆盖或传输设置',
  catalog_unavailable: '无法读取模型缓存。请核对该目录的 models_cache.json，重新启动不一定能解决',
  catalog_missing: '该目录没有 models_cache.json。请核对 Codex 使用的 CODEX_HOME；只有生成缓存后才能设置托管',
  catalog_unreadable: 'models_cache.json 无法读取或不是普通文件，请检查文件类型与访问权限',
  catalog_unsafe: '模型缓存路径不安全或包含链接，不能用于托管',
  catalog_invalid: 'models_cache.json 不是有效的模型缓存 JSON，重新启动不能修复格式不兼容',
  catalog_version_invalid: '模型缓存缺少有效的 client_version（X.Y.Z），不能确认模板版本',
  catalog_empty: '模型缓存中没有模型条目，暂时不能设置托管',
  catalog_too_large: '模型缓存超过 4 MiB 或 128 个模型的限制',
  catalog_metadata_invalid: '缓存中的模型元数据不完整或格式不兼容；请核对缓存，重复重启不能解决校验失败',
  catalog_duplicate: '模型缓存有重复的模型标识，无法确定应使用的模板',
};
export const managementStatus: Record<string, string> = {
  applied: '配置已应用', pending: '等待应用', unmanaged: '尚未接管', restored: '配置已恢复', needs_sync: '目录或配置已变化，需核对同步',
  overridden: '外部配置已变化，可重新接管', apply_failed: '应用失败', recovery_pending: '恢复待处理', conflict: '保留了外部修改，可重新接管', prepared: '写入待完成',
};
export function agentMessage(error: unknown): string {
  const code = error instanceof DesktopProblem ? error.code : 'unknown';
  const messages: Record<string, string> = {
    invalid: '托管选择无效。请检查默认模型、模板、目录和版本，重新识别后再提交。',
    conflict: '托管记录版本已变化，本次操作未完成。请核对重新读取的状态后再操作。',
    agent_config_conflict: '托管配置或模型目录已变化。请展开对应 Agent，确认“重新接管并同步”以恢复托管；也可以停用以保留外部修改。写入未完成时需先处理恢复记录。',
    agent_config_unsafe: '无法安全修改 Agent 配置。请检查配置结构、文件类型和目录权限；不要直接删除原配置。',
    agent_model_unavailable: '默认模型在当前托管范围内不可用。请检查模型映射和候选供应商的启用状态，或选择其他默认模型。',
    agent_catalog_invalid: '无法生成托管模型目录。请检查本地 Codex 模板和模型元数据；这不是托管记录版本冲突，重复重启不能修复。',
    agent_operation_failed: 'Agent 托管操作未完成，后台未提供更具体的安全诊断。请核对当前状态和本机恢复记录；不能据此判定为配置冲突。',
    unknown: '操作结果未知。请重新读取托管状态后核对，不要重复提交。',
    unsupported: '当前核心未提供 Agent 托管接口，请更新后重新连接。',
    identity: '本机核心身份不匹配，已停止操作。', unauthorized: '没有本机核心管理权限。',
    offline: '本机核心连接中断，请重新连接。', read_error: '无法读取托管结果，请重新读取。',
  };
  return messages[code] ?? 'Agent 托管操作失败，请重新读取状态。';
}

export interface AgentRow { id: string; agent: string; target?: AgentTarget; binding?: ManagedAgent }

/** Keep unavailable platforms visible, and retain recovery for undiscovered bindings. */
export function agentRows(data: AgentWorkspace): AgentRow[] {
  const rows: AgentRow[] = data.targets.map(target => ({ id: target.id, agent: target.agent, target,
    binding: data.bindings.find(b => b.adapter_id === target.agent && sameAgentRoot(b.config_root, target.config_root)) }));
  for (const binding of data.bindings) {
    if (!rows.some(row => row.binding?.id === binding.id)) rows.push({ id: `binding-${binding.id}`, agent: binding.adapter_id, binding });
  }
  const platforms = ['codex', 'claude', 'gemini', 'opencode', 'dsh', 'dsh-desktop'];
  for (const agent of platforms) if (!rows.some(row => row.agent === agent)) rows.push({ id: `platform-${agent}`, agent });
  return rows.sort((a, b) => (platforms.indexOf(a.agent) < 0 ? platforms.length : platforms.indexOf(a.agent)) -
    (platforms.indexOf(b.agent) < 0 ? platforms.length : platforms.indexOf(b.agent)));
}

export function sameAgentRoot(a: string, b: string) {
  return a === b || (/^[A-Za-z]:[\\/]/.test(a) && a.replaceAll('/', '\\').toLowerCase() === b.replaceAll('/', '\\').toLowerCase());
}
