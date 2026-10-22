import { describe, expect, it, vi } from 'vitest';
import { DesktopProblem } from '../../desktop/transport';
import fixture from '../../../../contract/agents.json';
import { CONTRACT, type AgentWorkspace, type DesktopBridge } from '../../desktop/contract.generated';
import { checkedAgentWorkspace, createAgentService, availableAgentModels, targetStatus, agentRows, agentMessage } from './agent.model';

function api(overrides: Partial<DesktopBridge> = {}): DesktopBridge {
  const result = { contract: CONTRACT, data: fixture.workspace, problem: null };
  return { AgentWorkspace: async () => result, EnrollAgent: async () => result, ManageAgent: async () => result, ...overrides } as DesktopBridge;
}
describe('Agent contract and controlled transport', () => {
  it('distinguishes safe failure categories without asserting a reread has succeeded', async () => {
    const codes = ['conflict', 'agent_operation_failed', 'agent_model_unavailable', 'agent_catalog_invalid', 'agent_config_conflict', 'agent_config_unsafe'] as const;
    expect(fixture.failure_codes).toEqual(codes.slice(1));
    const messages = codes.map(code => agentMessage(new DesktopProblem(code)));
    expect(new Set(messages).size).toBe(codes.length);
    expect(messages.every(message => !message.includes('已重新读取'))).toBe(true);
    expect(agentMessage(new DesktopProblem('agent_operation_failed'))).toContain('不能据此判定为配置冲突');
    for (const code of codes.slice(1)) {
      const service = createAgentService(() => api({ EnrollAgent: async () => ({ contract: CONTRACT, data: null, problem: { code } }) }));
      await expect(service.enroll(fixture.enroll)).rejects.toMatchObject({ code });
    }
  });
  it('keeps seven platforms and matches Windows binding paths without duplicate targets', () => {
    const data = { ...fixture.workspace, bindings: [{ ...fixture.workspace.bindings[0], config_root: 'c:/users/fixture/.codex' }] };
    const rows = agentRows(data);
    expect(rows.map(row => row.agent)).toEqual(['codex', 'claude', 'claude-desktop', 'gemini', 'opencode', 'dsh', 'dsh-desktop']);
    expect(rows[0].binding?.id).toBe('codex-fixture');
    expect(rows[4].target).toBeUndefined();
  });
  it('matches DSH Web and Desktop bindings by adapter as well as their shared home', () => {
    const target = fixture.workspace.targets[0], binding = fixture.workspace.bindings[0];
    const data = { ...fixture.workspace,
      targets: ['dsh', 'dsh-desktop'].map(agent => ({ ...target, id: agent, agent, config_root: 'C:\\Users\\fixture\\.dsh' })),
      bindings: ['dsh-desktop', 'dsh'].map(adapter_id => ({ ...binding, id: `bound-${adapter_id}`, adapter_id, config_root: 'c:/users/fixture/.dsh' })),
    };
    const rows = agentRows(data).filter(row => row.agent.startsWith('dsh'));
    expect(rows).toHaveLength(2);
    expect(rows.map(row => [row.agent, row.binding?.id])).toEqual([['dsh', 'bound-dsh'], ['dsh-desktop', 'bound-dsh-desktop']]);
    const missingDesktop = agentRows({ ...data, targets: data.targets.slice(0, 1) });
    expect(missingDesktop.find(row => row.agent === 'dsh-desktop')?.binding?.id).toBe('bound-dsh-desktop');
    expect(missingDesktop.find(row => row.agent === 'dsh-desktop')?.target).toBeUndefined();
  });
  it('keeps Claude CLI and Desktop independent, including recovery of missing Desktop roots', () => {
    const target = fixture.workspace.targets[0], binding = fixture.workspace.bindings[0];
    const data = { ...fixture.workspace,
      targets: ['claude', 'claude-desktop'].map(agent => ({ ...target, id: agent, agent, config_root: `/fixture/${agent}` })),
      bindings: ['claude-desktop', 'claude'].map(adapter_id => ({ ...binding, id: `bound-${adapter_id}`, adapter_id, config_root: `/fixture/${adapter_id}` })),
    };
    const rows = agentRows(data).filter(row => row.agent.startsWith('claude'));
    expect(rows.map(row => row.binding?.id)).toEqual(['bound-claude', 'bound-claude-desktop']);
    const missing = agentRows({ ...data, targets: data.targets.slice(0, 1) }).find(row => row.agent === 'claude-desktop');
    expect(missing?.binding?.id).toBe('bound-claude-desktop');
    expect(missing?.target).toBeUndefined();
  });
  it('retains multiple roots and recovery when a bound directory disappears', () => {
    const data = { ...fixture.workspace, targets: [{ ...fixture.workspace.targets[0], id: 'another', config_root: '/isolated/codex' }] };
    const rows = agentRows(data);
    expect(rows.filter(row => row.agent === 'codex')).toHaveLength(2);
    expect(rows.find(row => row.binding)?.target).toBeUndefined();
    expect(rows.find(row => row.binding)?.binding?.id).toBe('codex-fixture');
  });
  it('distinguishes absent caches from incompatible caches without a restart loop', () => {
    const categories = ['catalog_missing', 'catalog_unreadable', 'catalog_unsafe', 'catalog_invalid', 'catalog_version_invalid', 'catalog_empty', 'catalog_too_large', 'catalog_metadata_invalid', 'catalog_duplicate'];
    expect(new Set(categories.map(code => targetStatus[code])).size).toBe(categories.length);
    expect(categories.every(code => !!targetStatus[code])).toBe(true);
    expect(targetStatus.catalog_metadata_invalid).toContain('重复重启不能解决');
    expect(targetStatus.catalog_missing).toContain('CODEX_HOME');
  });
  it('uses the shared DTO and removes unknown data', () => {
    expect(fixture.contract).toBe(CONTRACT);
    expect(checkedAgentWorkspace({ ...fixture.workspace, token: 'private' } as AgentWorkspace)).toEqual(fixture.workspace);
  });
  it('uses core availability even when shared metadata has no matching cache slug', () => {
    const target = fixture.workspace.targets[0], model = fixture.workspace.models[0];
    const data = { ...fixture.workspace, models: [
      { ...model, upstream_models: ['upstream-with-shared-metadata'] },
      { ...model, id: 'disabled-provider', available_targets: [] },
      { ...model, id: 'different-target', available_targets: ['other-codex'] },
    ] };
    expect(availableAgentModels(data, target).map(m => m.id)).toEqual(['public']);
  });
  it('passes approval and expected revision without generic file or command access', async () => {
    const enroll = vi.fn(api().EnrollAgent), manage = vi.fn(api().ManageAgent);
    const service = createAgentService(() => api({ EnrollAgent: enroll, ManageAgent: manage }));
    expect(await service.workspace()).toEqual(fixture.workspace);
    await service.enroll(fixture.enroll); await service.manage(fixture.sync);
    expect(enroll).toHaveBeenCalledExactlyOnceWith(fixture.enroll);
    expect(manage).toHaveBeenCalledExactlyOnceWith(fixture.sync);
  });
  it('uses a single enrollment request for an approved configuration update', async () => {
    const enroll = vi.fn(api().EnrollAgent), manage = vi.fn(api().ManageAgent);
    const service = createAgentService(() => api({ EnrollAgent: enroll, ManageAgent: manage }));
    await service.enroll(fixture.update);
    expect(enroll).toHaveBeenCalledExactlyOnceWith(fixture.update);
    expect(manage).not.toHaveBeenCalled();
  });
  it.each([
    { bindings: [{ ...fixture.workspace.bindings[0], revision: Number.MAX_SAFE_INTEGER + 1 }] },
    { bindings: [{ ...fixture.workspace.bindings[0], model_templates: { public: 42 } }] },
    { targets: [{ ...fixture.workspace.targets[0], templates: null }] },
    { models: [{ ...fixture.workspace.models[0], upstream_models: [null] }] },
    { models: [{ ...fixture.workspace.models[0], available_targets: null }] },
    { models: [{ ...fixture.workspace.models[0], available_targets: [42] }] },
  ])('rejects malformed host data %j', changed => {
    expect(() => checkedAgentWorkspace({ ...fixture.workspace, ...changed } as unknown as AgentWorkspace)).toThrow('read_error');
  });
  it('does not replay failed writes or expose parser diagnostics', async () => {
    const enroll = vi.fn(async () => { throw new Error('private config value'); });
    const service = createAgentService(() => api({ EnrollAgent: enroll }));
    await expect(service.enroll(fixture.enroll)).rejects.toMatchObject({ code: 'unknown' });
    expect(enroll).toHaveBeenCalledTimes(1);
  });
});
