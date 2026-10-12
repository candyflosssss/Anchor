import { bridge, call, DesktopProblem } from '../../desktop/transport';
import type { DesktopBridge, AgentWorkspace, AgentTarget, AgentModel, EnrollAgent, ManageAgent } from '../../desktop/contract.generated';
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
      if (!m || typeof m.id !== 'string' || typeof m.tools !== 'boolean' || typeof m.streaming !== 'boolean' || !Array.isArray(m.upstream_models) || m.upstream_models.some(v => typeof v !== 'string')) return bad();
      return { id: m.id, tools: m.tools, streaming: m.streaming, upstream_models: [...m.upstream_models] };
    }),
    bindings: value.bindings.map(b => {
      if (!b || ![b.id, b.config_root, b.model, b.model_scope, b.catalog_version, b.status, b.catalog_path].every(v => typeof v === 'string') || !Number.isSafeInteger(b.revision) || b.revision < 1 || typeof b.enabled !== 'boolean' || !Array.isArray(b.available_models) || b.available_models.some(v => typeof v !== 'string') || !b.model_templates || Array.isArray(b.model_templates) || typeof b.model_templates !== 'object' || Object.values(b.model_templates).some(v => typeof v !== 'string')) return bad();
      return { id: b.id, config_root: b.config_root, model: b.model, enabled: b.enabled, revision: b.revision, model_scope: b.model_scope, catalog_version: b.catalog_version, status: b.status, catalog_path: b.catalog_path, available_models: [...b.available_models], model_templates: { ...b.model_templates } };
    }),
  };
}

/** Exact identity only; conflicting upstream identities require an explicit choice. */
export function matchingTemplate(target: AgentTarget, model: AgentModel): string {
  const slugs = new Set(target.templates.map(t => t.slug));
  if (slugs.has(model.id)) return model.id;
  const sources = [...new Set(model.upstream_models.filter(v => slugs.has(v)))];
  return sources.length === 1 ? sources[0] : '';
}
export function createAgentService(getBridge: () => DesktopBridge = bridge) { return {
  workspace: async () => checkedAgentWorkspace(await call(() => getBridge().AgentWorkspace())),
  enroll: async (input: EnrollAgent) => checkedAgentWorkspace(await call(() => getBridge().EnrollAgent(input), true)),
  manage: async (input: ManageAgent) => checkedAgentWorkspace(await call(() => getBridge().ManageAgent(input), true)),
}; }
export const agentService = createAgentService();

export const targetStatus: Record<string, string> = {
  ready: '可托管', unsupported: '尚无适配器', unsafe: '目录权限或类型不支持', config_unsupported: '配置含暂不支持的覆盖或传输设置',
  catalog_unavailable: '缺少完整模型缓存：请先启动一次 Codex，再重新识别',
};
export const managementStatus: Record<string, string> = {
  applied: '配置已应用', pending: '等待应用', unmanaged: '尚未接管', restored: '配置已恢复', needs_sync: '目录或配置已变化，需核对同步',
  overridden: '托管字段已被外部覆盖，请核对', apply_failed: '应用失败', recovery_pending: '恢复待处理', conflict: '配置冲突，保留了第三方修改', prepared: '写入待完成',
};
export function agentMessage(error: unknown): string {
  const code = error instanceof DesktopProblem ? error.code : 'unknown';
  const messages: Record<string, string> = {
    invalid: '托管选择无效。请检查默认模型、模板、目录和版本，重新识别后再提交。',
    conflict: '目标状态已变化、存在配置冲突，或模型不满足托管条件。已重新读取，请核对目录、模板和供应商启用状态后再操作。',
    unknown: '操作结果未知。请重新读取托管状态后核对，不要重复提交。',
    unsupported: '当前核心未提供 Agent 托管接口，请更新后重新连接。',
    identity: '本机核心身份不匹配，已停止操作。', unauthorized: '没有本机核心管理权限。',
    offline: '本机核心连接中断，请重新连接。', read_error: '无法读取托管结果，请重新读取。',
  };
  return messages[code] ?? 'Agent 托管操作失败，请重新读取状态。';
}
