import { bridge, call, checkedSession, DesktopProblem } from '../../desktop/transport';
import type { ConnectionProfile, SSHConnectionInput, SessionInfo, SessionResult } from '../../desktop/contract.generated';

export type { ConnectionProfile, SSHConnectionInput };
export function checkedProfiles(value: ConnectionProfile[]): ConnectionProfile[] {
  if (!Array.isArray(value)) throw new DesktopProblem('read_error');
  const names = new Set<string>();
  return value.map(row => {
    if (!row || typeof row.name !== 'string' || !row.name || names.has(row.name) || !['local', 'ssh'].includes(row.transport) || typeof row.target !== 'string' || typeof row.instance_id !== 'string' || typeof row.active !== 'boolean' || (row.selected !== undefined && typeof row.selected !== 'boolean') || !['owner', 'key', 'password'].includes(row.authentication)) throw new DesktopProblem('read_error');
    if (row.password_saved != null && typeof row.password_saved !== 'boolean') throw new DesktopProblem('read_error');
    names.add(row.name);
    return { authentication: row.authentication, name: row.name, transport: row.transport, target: row.target, instance_id: row.instance_id, active: row.active, ...(row.selected !== undefined ? { selected: row.selected } : {}), ...(row.password_saved != null ? { password_saved: row.password_saved } : {}) };
  });
}
export interface ConnectionOutcome { info: SessionInfo; passwordWarning: boolean }
async function connect(operation: () => Promise<SessionResult>): Promise<ConnectionOutcome> {
  let passwordWarning = false;
  const info = checkedSession(await call(async () => {
    const result = await operation();
    passwordWarning = !!result.warning;
    return result;
  }));
  return { info, passwordWarning };
}
export const connections = {
  list: async () => checkedProfiles(await call(() => bridge().Connections())),
  add: async (input: SSHConnectionInput) => connect(() => bridge().ConnectSSH(input)),
  use: async (name: string, password = '', remember = false) => connect(() => bridge().UseConnection({ name, password, ...(remember ? { remember_password: true } : {}) })),
  forget: async (name: string) => checkedProfiles(await call(() => bridge().ForgetConnectionPassword({ name, password: '' }), true)),
};
export const passwordSaveWarning = '设备已连接，但未能确认本次密码已保存。请检查系统凭据库后重新输入并保存。';
export const emptySSH: SSHConnectionInput = { password: '', name: '', target: '', port: 0, identity_file: '', remote_executable: 'anchor', remote_control_port: 17861, trust_new_host: false };
