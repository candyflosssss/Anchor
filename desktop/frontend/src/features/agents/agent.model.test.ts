import { describe, expect, it, vi } from 'vitest';
import fixture from '../../../../contract/agents.json';
import { CONTRACT, type AgentWorkspace, type DesktopBridge } from '../../desktop/contract.generated';
import { checkedAgentWorkspace, createAgentService, matchingTemplate } from './agent.model';

function api(overrides: Partial<DesktopBridge> = {}): DesktopBridge {
  const result = { contract: CONTRACT, data: fixture.workspace, problem: null };
  return { AgentWorkspace: async () => result, EnrollAgent: async () => result, ManageAgent: async () => result, ...overrides } as DesktopBridge;
}
describe('Agent contract and controlled transport', () => {
  it('uses the shared DTO and removes unknown data', () => {
    expect(fixture.contract).toBe(CONTRACT);
    expect(checkedAgentWorkspace({ ...fixture.workspace, token: 'private' } as AgentWorkspace)).toEqual(fixture.workspace);
  });
  it('matches exact identities and rejects conflicting upstream identities', () => {
    const t = fixture.workspace.targets[0], m = fixture.workspace.models[0];
    expect(matchingTemplate(t, m)).toBe('fixture-source');
    expect(matchingTemplate(t, { ...m, upstream_models: ['unmatched'] })).toBe('');
    const multi = { ...t, templates: [...t.templates, { slug: 'second', display_name: 'Second' }] };
    expect(matchingTemplate(multi, { ...m, upstream_models: ['fixture-source', 'second'] })).toBe('');
    expect(matchingTemplate(multi, { ...m, id: 'fixture-source', upstream_models: ['second'] })).toBe('fixture-source');
  });
  it('passes approval and expected revision without generic file or command access', async () => {
    const enroll = vi.fn(api().EnrollAgent), manage = vi.fn(api().ManageAgent);
    const service = createAgentService(() => api({ EnrollAgent: enroll, ManageAgent: manage }));
    expect(await service.workspace()).toEqual(fixture.workspace);
    await service.enroll(fixture.enroll); await service.manage(fixture.sync);
    expect(enroll).toHaveBeenCalledExactlyOnceWith(fixture.enroll);
    expect(manage).toHaveBeenCalledExactlyOnceWith(fixture.sync);
  });
  it.each([
    { bindings: [{ ...fixture.workspace.bindings[0], revision: Number.MAX_SAFE_INTEGER + 1 }] },
    { bindings: [{ ...fixture.workspace.bindings[0], model_templates: { public: 42 } }] },
    { targets: [{ ...fixture.workspace.targets[0], templates: null }] },
    { models: [{ ...fixture.workspace.models[0], upstream_models: [null] }] },
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
