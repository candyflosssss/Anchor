import { describe, expect, it, vi } from 'vitest';
import fixture from '../../../../contract/skills.json';
import { CONTRACT, type DesktopBridge } from '../../desktop/contract.generated';
import { checkedSkillDetail, checkedSkillWorkspace, createSkillService, skillAction, skillMessage, skillQuery, skillWarning } from './skill.model';
import { DesktopProblem } from '../../desktop/transport';

describe('Skill contract and controlled transport', () => {
  it('explains incomplete automatic project discovery without claiming a complete scan', () => {
    expect(skillWarning('project_limit')).toContain('部分项目尚未检查');
    expect(skillWarning('project_search_limit')).toContain('补充项目');
    expect(skillWarning('unreadable_project_search:search-example')).toContain('其他项目仍会继续扫描');
    expect(skillWarning('pending_transaction')).toContain('原件备份会保留');
    expect(skillWarning('collect_failed:source-example')).toContain('部分 Skill 尚未收录');
  });
  it('accepts the same fixture as Go and preserves version pinning and source baseline', () => {
    const ws = checkedSkillWorkspace(fixture.workspace);
    expect(ws.mounts[0].version).toBe(1);
    expect(ws.sources[0].baseline).toBe(1);
    expect(checkedSkillDetail(fixture.detail).content).toBe('# Example\n');
    expect(skillAction('create', { ...fixture.create })).toEqual(fixture.create);
    expect(skillAction('cleanup', { ...fixture.cleanup })).toEqual(fixture.cleanup);
  });
  it('rejects incomplete DTOs, malformed nested snapshots and unsafe revision numbers', () => {
    for (const value of [null, {}, { ...fixture.workspace, revision: Number.MAX_SAFE_INTEGER + 1 }, { ...fixture.workspace, library: [{ ...fixture.workspace.library[0], versions: null }] }]) {
      expect(() => checkedSkillWorkspace(value as typeof fixture.workspace)).toThrow();
    }
    expect(() => checkedSkillDetail({ ...fixture.detail, differences: [{ ...fixture.detail.differences[0], binary: undefined }] } as unknown as typeof fixture.detail)).toThrow();
  });
  it('uses typed methods, preserves approvals/hashes and never retries unknown writes', async () => {
    const write = vi.fn(async () => ({ contract: CONTRACT, data: null, problem: { code: 'unknown' as const } }));
    const read = vi.fn(async () => ({ contract: CONTRACT, data: fixture.workspace, problem: null }));
    const detail = vi.fn(async () => ({ contract: CONTRACT, data: fixture.detail, problem: null }));
    const service = createSkillService(() => ({ SkillWorkspace: read, ManageSkill: write, SkillDetail: detail }) as unknown as DesktopBridge);
    expect(await service.workspace()).toEqual(fixture.workspace);
    await expect(service.action(fixture.create)).rejects.toMatchObject({ code: 'unknown' });
    expect(write).toHaveBeenCalledTimes(1);
    expect(write).toHaveBeenCalledWith(fixture.create);
    expect(await service.detail(skillQuery({ skill_id: 'skill-fixture', version: 1 }))).toEqual(fixture.detail);
  });
  it('keeps unsupported bridges unavailable instead of returning mock success', async () => {
    const service = createSkillService(() => { throw new DesktopProblem('unsupported'); });
    await expect(service.workspace()).rejects.toMatchObject({ code: 'unsupported' });
    expect(skillMessage(new DesktopProblem('unknown'))).toContain('不要直接重复提交');
    expect(skillMessage(new DesktopProblem('skill_link_unavailable'))).toContain('未自动改用复制');
  });
});
