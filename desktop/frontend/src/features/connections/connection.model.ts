import { bridge, call, checkedSession, DesktopProblem } from '../../desktop/transport';
import type { ConnectionProfile, SSHConnectionInput } from '../../desktop/contract.generated';

export type { ConnectionProfile, SSHConnectionInput };
export function checkedProfiles(value: ConnectionProfile[]): ConnectionProfile[] {
  if (!Array.isArray(value)) throw new DesktopProblem('read_error');
  const names = new Set<string>();
  return value.map(row => {
    if (!row || typeof row.name !== 'string' || !row.name || names.has(row.name) || !['local', 'ssh'].includes(row.transport) || typeof row.target !== 'string' || typeof row.instance_id !== 'string' || typeof row.active !== 'boolean' || (row.selected !== undefined && typeof row.selected !== 'boolean') || !['owner', 'key', 'password'].includes(row.authentication)) throw new DesktopProblem('read_error');
    names.add(row.name);
    return { authentication: row.authentication, name: row.name, transport: row.transport, target: row.target, instance_id: row.instance_id, active: row.active, ...(row.selected !== undefined ? { selected: row.selected } : {}) };
  });
}
export const connections = {
  list: async () => checkedProfiles(await call(() => bridge().Connections())),
  add: async (input: SSHConnectionInput) => checkedSession(await call(() => bridge().ConnectSSH(input))),
  use: async (name: string, password = '') => checkedSession(await call(() => bridge().UseConnection({ name, password }))),
};
export const emptySSH: SSHConnectionInput = { password: '', name: '', target: '', port: 0, identity_file: '', remote_executable: 'anchor', remote_control_port: 17861, trust_new_host: false };
