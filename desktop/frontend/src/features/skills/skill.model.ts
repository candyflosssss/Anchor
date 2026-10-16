import { bridge, call, DesktopProblem } from '../../desktop/transport';
import type { DesktopBridge, SkillAction, SkillDetail, SkillQuery, SkillWorkspace } from '../../desktop/contract.generated';
export type { Skill, SkillSource, SkillMount, SkillScope, SkillAction, SkillDetail, SkillWorkspace } from '../../desktop/contract.generated';
const bad = (): never => { throw new DesktopProblem('read_error'); };
const string = (v: unknown) => typeof v === 'string';
const number = (v: unknown) => Number.isSafeInteger(v) && Number(v) >= 0;
const files = (v: SkillDetail['files']) => Array.isArray(v) && v.every(f => f && string(f.path) && string(f.hash) && number(f.size) && typeof f.executable === 'boolean');
export function checkedSkillWorkspace(v: SkillWorkspace): SkillWorkspace {
  if (!v || !number(v.revision) || v.revision < 1 || !Array.isArray(v.library) || !Array.isArray(v.scopes) || !Array.isArray(v.sources) || !Array.isArray(v.mounts) || !Array.isArray(v.warnings) || !v.warnings.every(string)) return bad();
  if (!v.library.every(s => s && string(s.id) && string(s.name) && Array.isArray(s.versions) && s.versions.length > 0 && s.versions.every(n => n && number(n.number) && n.number > 0 && number(n.parent) && number(n.baseline) && [n.hash, n.created_at, n.origin].every(string) && files(n.files)))) return bad();
  if (!v.scopes.every(s => s && [s.id, s.agent, s.kind, s.root, s.project, s.origin].every(string))) return bad();
  if (!v.sources.every(s => s && [s.id, s.scope_id, s.path, s.name, s.hash, s.skill_id, s.status].every(string) && number(s.baseline))) return bad();
  if (!v.mounts.every(s => s && [s.id, s.skill_id, s.scope_id, s.path, s.target, s.status].every(string) && number(s.version) && s.version > 0)) return bad();
  return v;
}
export function checkedSkillDetail(v: SkillDetail): SkillDetail {
  if (!v || ![v.skill_id, v.source_id, v.hash, v.file, v.content].every(string) || !number(v.version) || typeof v.editable !== 'boolean' || !files(v.files) || !Array.isArray(v.differences) || !v.differences.every(d => d && [d.path, d.change, d.before, d.after].every(string) && typeof d.binary === 'boolean' && typeof d.truncated === 'boolean')) return bad();
  return v;
}
export function skillAction(action: string, fields: Partial<SkillAction> = {}): SkillAction {
  return { action, expected_revision: 0, approved: false, skill_id: '', source_id: '', scope_id: '', mount_id: '', name: '', version: 0, project_path: '', expected_hash: '', edits: [], ...fields };
}
export function skillQuery(fields: Partial<SkillQuery>): SkillQuery { return { skill_id: '', version: 0, source_id: '', against_version: 0, file: '', ...fields }; }
export function createSkillService(getBridge: () => DesktopBridge = bridge) { return {
  workspace: async () => checkedSkillWorkspace(await call(() => getBridge().SkillWorkspace())),
  action: async (input: SkillAction) => checkedSkillWorkspace(await call(() => getBridge().ManageSkill(input), true)),
  detail: async (input: SkillQuery) => checkedSkillDetail(await call(() => getBridge().SkillDetail(input))),
}; }
export const skillService = createSkillService();
export const skillStatus: Record<string, string> = {
  local: '本地原件', imported: '已导入', in_sync: '内容一致', source_changed: '来源已变化', library_changed: '库中有不同版本', diverged: '双方存在分歧',
  external_link: '外部链接 · 仅登记', unsafe: '路径或文件类型不支持', unavailable: '库版本不可用', limit: '超过扫描限制',
  linked: '固定版本链接', missing: '原位置已缺失', conflict: '原位置被外部修改', broken: '链接目标已缺失',
};
export function skillMessage(error: unknown): string {
  if (!(error instanceof DesktopProblem)) return '操作未完成，请重新读取。';
  const messages: Partial<Record<DesktopProblem['code'], string>> = {
    unknown: '写入结果未知。请重新读取后核对，不要直接重复提交。', conflict: '版本已变化或目标位置已存在，未覆盖原件。请重新读取。',
    skill_changed: '内容或链接已被修改，未覆盖外部变化。请重新读取并核对。', skill_unsafe: '路径、权限或文件类型不支持安全操作。',
    skill_limit: '超过文件数量、大小或库存上限。', skill_link_unavailable: '系统未能创建软链接，未自动改用复制模式。',
    referenced: '此 Skill 仍有挂载，请先解除挂载。', invalid: '输入未通过校验。', unsupported: '当前核心或桌面接口不支持 Skill 管理。',
    offline: '当前设备不可用。', unauthorized: '管理授权不可用。', read_error: 'Skill 数据无效或无法读取。',
  };
  return messages[error.code] ?? 'Skill 操作未完成，请核对设备状态。';
}
