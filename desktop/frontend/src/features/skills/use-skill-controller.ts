import { useCallback, useEffect, useRef, useState } from 'react';
import { skillAction, skillMessage, skillQuery, skillService, type Skill, type SkillAction, type SkillDetail, type SkillSource, type SkillWorkspace } from './skill.model';

export function useSkillController(onActivity?: (busy: boolean) => void) {
  const [data, setData] = useState<SkillWorkspace | null>(null);
  const [detail, setDetail] = useState<SkillDetail | null>(null);
  const [selected, setSelected] = useState('');
  const [tab, setTab] = useState<'library' | 'sources' | 'mounts'>('library');
  const [loading, setLoading] = useState(true);
  const [writing, setWriting] = useState(false);
  const [readingDetail, setReadingDetail] = useState(false);
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  const [dialog, setDialog] = useState<'create' | 'project' | 'link' | null>(null);
  const [name, setName] = useState('');
  const [content, setContent] = useState('');
  const [filePath, setFilePath] = useState('SKILL.md');
  const [scope, setScope] = useState('');
  const [project, setProject] = useState('');
  const [editing, setEditing] = useState(false);
  const [confirmation, setConfirmation] = useState<{ title: string; impact: string; input: SkillAction } | null>(null);
  const alive = useRef(false), lock = useRef(false), detailEpoch = useRef(0), readEpoch = useRef(0);
  const read = useCallback(async () => {
    const epoch = ++readEpoch.current; setLoading(true); setError('');
    try { const value = await skillService.workspace(); if (alive.current && epoch === readEpoch.current) setData(value); }
    catch (e) { if (alive.current && epoch === readEpoch.current) setError(skillMessage(e)); }
    finally { if (alive.current && epoch === readEpoch.current) setLoading(false); }
  }, []);
  useEffect(() => {
    const details = detailEpoch, reads = readEpoch;
    alive.current = true; void read();
    return () => { alive.current = false; details.current++; reads.current++; };
  }, [read]);
  useEffect(() => { onActivity?.(writing); return () => onActivity?.(false); }, [onActivity, writing]);
  const query = async (fields: Parameters<typeof skillQuery>[0]) => {
    const epoch = ++detailEpoch.current; setReadingDetail(true); setDetail(null); setEditing(false); setError('');
    try { const value = await skillService.detail(skillQuery(fields)); if (alive.current && epoch === detailEpoch.current) { setDetail(value); setContent(value.content); setFilePath(value.file || 'SKILL.md'); } }
    catch (e) { if (alive.current && epoch === detailEpoch.current) setError(skillMessage(e)); }
    finally { if (alive.current && epoch === detailEpoch.current) setReadingDetail(false); }
  };
  const select = (s: Skill, version = s.versions.at(-1)!.number, file = 'SKILL.md') => {
    setSelected(s.id); const record = s.versions.find(v => v.number === version)!;
    void query({ skill_id: s.id, version, against_version: record.parent, file });
  };
  const request = (input: SkillAction, title: string, impact: string) => {
    if (!data || lock.current || loading) return;
    setConfirmation({ title, impact, input: { ...input, approved: true, expected_revision: data.revision } });
  };
  const confirm = async () => {
    if (!confirmation || lock.current) return;
    lock.current = true; readEpoch.current++; detailEpoch.current++; setLoading(false); setReadingDetail(false); setWriting(true); setError(''); setNotice('');
    try {
      const value = await skillService.action(confirmation.input);
      if (alive.current) { setData(value); setDialog(null); setEditing(false); setDetail(null); detailEpoch.current++; setNotice('操作已完成。'); }
    } catch (e) {
      if (alive.current) { setError(skillMessage(e)); setDetail(null); setEditing(false); detailEpoch.current++; }
      // Reread is safe; never automatically retry a write, including unknown results.
      try { const value = await skillService.workspace(); if (alive.current) setData(value); }
      catch { if (alive.current) setError(`${skillMessage(e)} 当前状态尚未确认。`); }
    } finally { lock.current = false; if (alive.current) { setWriting(false); setConfirmation(null); } }
  };
  const inspectSource = (source: SkillSource, latest = false) => {
    setSelected(''); const s = data?.library.find(s => s.id === source.skill_id);
    void query({ source_id: source.id, against_version: latest ? s?.versions.at(-1)?.number ?? 0 : 0 });
  };
  const open = (value: typeof dialog) => { setName(''); setContent(''); setFilePath('SKILL.md'); setDialog(value); setScope(data?.scopes[0]?.id ?? ''); };
  const current = data?.library.find(s => s.id === selected);
  return { data, detail, current, selected, tab, setTab, loading, writing, readingDetail, error, notice, dialog, setDialog, name, setName, content, setContent, filePath, setFilePath, scope, setScope, project, setProject, editing, setEditing, confirmation, setConfirmation, read, select, request, confirm, inspectSource, open,
    save: () => detail && request(skillAction('save', { skill_id: detail.skill_id, version: detail.version, edits: [{ path: filePath, content, executable: detail.files.find(f => f.path === filePath)?.executable ?? false, delete: false }] }), '保存为新版本？', '内容有变化时保存新版本；与已有版本相同时复用该版本。原版本、项目原件和现有挂载保持不变。'),
    removeFile: () => detail && request(skillAction('save', { skill_id: detail.skill_id, version: detail.version, edits: [{ path: filePath, content: '', executable: false, delete: true }] }), `在新版本中删除 ${filePath}？`, '原版本与项目原件保持不变。'),
    create: () => request(skillAction('create', { name, edits: [{ path: 'SKILL.md', content, executable: false, delete: false }] }), `创建 ${name}？`, '只创建中心库版本，不自动安装到任何项目。'),
    register: () => request(skillAction('register-project', { project_path: project }), '登记项目？', `${project}；自动收录项目里的 Skill，保留原件。`),
    cleanup: (source: SkillSource) => request(skillAction('cleanup', { source_id: source.id, expected_hash: source.hash }), `清理 ${source.name} 的原件？`, `${source.path}；先把原件当前内容完整复制到 Anchor 库并核对，再将原位置换成指向该版本的软链接。原位置若是软链接，只替换链接，保留其目标文件。不清理也能继续使用。`),
    link: () => current && detail && request(skillAction('link', { skill_id: current.id, version: detail.version, scope_id: scope, name }), `安装 ${current.name} v${detail.version}？`, `${data?.scopes.find(s => s.id === scope)?.root}；只在空位置创建软链接，已有内容不覆盖。`),
  };
}
