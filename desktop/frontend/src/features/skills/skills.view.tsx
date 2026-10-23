import { useState } from 'react';
import { FileCode2, FolderPlus, Link2, Plus, RefreshCw, Trash2 } from 'lucide-react';
import { Button } from '../../components/ui/button';
import { Input } from '../../components/ui/input';
import { Dialog, DialogContent, DialogTitle } from '../../components/ui/dialog';
import { ConfirmAction } from '../../components/shared/confirm-action';
import { skillAction, skillLocationLabel, skillLocations, skillStatus, skillWarning, type SkillDetail, type SkillMount, type SkillSource, type SkillWorkspace } from './skill.model';
import { useSkillController } from './use-skill-controller';

const selectClass = 'h-9 min-w-0 rounded-md border bg-background px-2 text-sm';
const textareaClass = 'min-h-64 w-full rounded-md border bg-background p-3 font-mono text-sm';
function Differences({ detail }: { detail: SkillDetail }) {
  return <section className="grid gap-3" aria-label="文件差异"><h3 className="font-medium">文件差异 · {detail.differences.length}</h3>{detail.differences.length === 0 && <p className="text-sm text-muted-foreground">没有内容差异</p>}{detail.differences.map(d => <details key={d.path} className="min-w-0 rounded-md border p-3"><summary className="cursor-pointer break-all text-sm">{({ added: '新增', deleted: '删除', modified: '修改' } as Record<string, string>)[d.change] ?? d.change} · {d.path}{d.binary && ' · 二进制内容'}{d.truncated && ' · 预览已截断'}</summary>{!d.binary && <div className="mt-3 grid min-w-0 gap-3 lg:grid-cols-2"><div><h4 className="text-xs text-muted-foreground">变更前</h4><pre className="max-h-80 overflow-auto whitespace-pre-wrap break-all rounded bg-muted/50 p-2 text-xs">{d.before || '（空）'}</pre></div><div><h4 className="text-xs text-muted-foreground">变更后</h4><pre className="max-h-80 overflow-auto whitespace-pre-wrap break-all rounded bg-muted/50 p-2 text-xs">{d.after || '（空）'}</pre></div></div>}</details>)}</section>;
}
function MountRow({ mount, data, busy, request }: { mount: SkillMount; data: SkillWorkspace; busy: boolean; request: ReturnType<typeof useSkillController>['request'] }) {
  const s = data.library.find(s => s.id === mount.skill_id);
  const [version, setVersion] = useState(mount.version);
  return <li className="grid gap-3 border-b p-3 last:border-b-0">
    <div className="min-w-0"><div className="flex flex-wrap items-center justify-between gap-2"><span className="font-medium">{s?.name ?? mount.skill_id}</span><span className="rounded border px-2 py-0.5 text-xs text-muted-foreground">Anchor 快捷方式</span></div><p className="mt-1 break-all text-xs text-muted-foreground">{skillLocationLabel(data, mount.scope_id)}</p><p className="mt-1 break-all text-xs text-muted-foreground">{mount.path}</p></div>
    <p className="text-sm"><span>{skillStatus[mount.status] ?? '状态未识别'}</span><span className="ml-2 text-xs text-muted-foreground">指定版本 v{mount.version}</span></p>
    <div className="flex flex-wrap gap-2">
      <select aria-label={`${s?.name} 在 ${mount.path} 使用的版本`} className={selectClass} value={version} disabled={busy} onChange={e => setVersion(Number(e.target.value))}>{s?.versions.map(v => <option key={v.number} value={v.number}>v{v.number}</option>)}</select>
      <Button size="sm" variant="outline" disabled={busy || version === mount.version || mount.status !== 'linked'} onClick={() => request(skillAction('update', { mount_id: mount.id, version }), `将此处改为使用 v${version}？`, `${mount.path}；只更换此处快捷方式指向的版本，其他位置不变。`)}>更换版本</Button>
      <Button size="sm" variant="outline" disabled={busy || !['linked', 'missing', 'broken'].includes(mount.status)} onClick={() => request(skillAction('unlink', { mount_id: mount.id }), '移除此处的快捷方式？', `${mount.path}；移除快捷方式或清除已缺失位置的记录，不恢复独立文件。该位置将不能再通过此快捷方式读取 Skill；Anchor 保存的内容和其他位置不变。`)}>移除快捷方式</Button>
    </div>
  </li>;
}
type SkillController = ReturnType<typeof useSkillController>;
function SourceRow({ source, data, busy, controller: c }: { source: SkillSource; data: SkillWorkspace; busy: boolean; controller: SkillController }) {
  const skill = data.library.find(s => s.id === source.skill_id);
  return <li className="grid gap-3 border-b p-3 last:border-b-0">
    <div className="min-w-0"><div className="flex flex-wrap items-center justify-between gap-2"><span className="font-medium">{skill?.name ?? source.name}</span><span className="rounded border px-2 py-0.5 text-xs text-muted-foreground">原位置文件</span></div><p className="mt-1 break-all text-xs text-muted-foreground">{skillLocationLabel(data, source.scope_id)}</p><p className="mt-1 break-all text-xs text-muted-foreground">{source.path}</p></div>
    <p className="text-sm"><span>{skillStatus[source.status] ?? '状态未识别'}</span>{source.baseline > 0 && <span className="ml-2 text-xs text-muted-foreground">Anchor 已保存 v{source.baseline}</span>}</p>
    <div className="flex flex-wrap gap-2">
      <Button size="sm" variant="outline" disabled={busy || c.readingDetail || !source.hash} onClick={() => c.inspectSource(source)}>查看改动</Button>
      {!['in_sync', 'imported'].includes(source.status) && <Button size="sm" disabled={busy || !source.hash} onClick={() => c.request(skillAction('import', { source_id: source.id, expected_hash: source.hash }), source.skill_id ? '把此处改动保存到 Anchor？' : `将 ${source.name} 保存到 Anchor？`, `${source.path}；只复制保存到 Anchor，不修改此处文件或任何快捷方式。${source.status === 'diverged' ? '两边都有改动；将此处内容另存为一个版本，不自动合并。' : ''}`)}>{source.skill_id ? '保存此处改动' : '保存到 Anchor'}</Button>}
      <Button size="sm" variant="outline" disabled={busy || !source.hash} onClick={() => c.cleanup(source)}>改用 Anchor 副本</Button>
    </div>
  </li>;
}
function Locations({ data, skillID = '', controller: c }: { data: SkillWorkspace; skillID?: string; controller: SkillController }) {
  const locations = skillLocations(data, skillID), busy = c.loading || c.writing || c.editing;
  return <div className="min-w-0 rounded-lg border">
    <ul aria-label="文件位置列表">
      {locations.sources.map(s => <SourceRow key={s.id} source={s} data={data} busy={busy} controller={c} />)}
      {locations.mounts.map(m => <MountRow key={`${m.id}-${m.version}`} mount={m} data={data} busy={busy} request={c.request} />)}
    </ul>
    {locations.total === 0 && <p className="p-4 text-sm text-muted-foreground">尚未发现{skillID ? '这个 Skill 的' : ''}文件位置</p>}
  </div>;
}
export function SkillsView({ onActivity }: { onActivity?: (busy: boolean) => void }) {
  const c = useSkillController(onActivity), busy = c.loading || c.writing;
  const data = c.data;
  return <main className="flex min-w-0 flex-1 flex-col gap-5 p-4 md:p-6">
    <header className="flex flex-wrap items-center justify-between gap-3"><h1 className="text-xl font-semibold">Skills</h1><div className="flex flex-wrap gap-2"><Button variant="outline" disabled={busy} onClick={() => void c.read()}><RefreshCw className="size-4" />重新扫描</Button><Button variant="outline" disabled={busy || !data} onClick={() => c.open('project')}><FolderPlus className="size-4" />补充项目</Button><Button disabled={busy || !data} onClick={() => c.open('create')}><Plus className="size-4" />新建 Skill</Button></div></header>
    <details className="text-sm text-muted-foreground"><summary className="w-fit cursor-pointer">使用说明</summary><div className="mt-2 grid gap-2"><p>常见项目目录会自动识别，发现的 Skill 会复制保存到 Anchor，原文件不动。同一个 Skill 出现在多个位置时，“我的 Skill”只列一次，“文件位置”按路径计数。</p><p>“改用 Anchor 副本”会先保存当前文件，再把原位置换成指向已保存版本的快捷方式。不操作也能照常使用原文件。这里显示文件是否存在，不代表 Agent 已加载。</p></div></details>
    {c.error && <p role="alert" className="rounded-md border border-destructive/40 p-3 text-sm text-destructive">{c.error}</p>}
    {c.notice && <p role="status" className="text-sm">{c.notice}</p>}
    {c.loading && <p role="status" className="text-sm text-muted-foreground">正在查找 Skill 和文件位置…</p>}
    {data && <>
      {data.warnings.length > 0 && <div role="status" className="rounded-md border p-3 text-sm">{data.warnings.map((w, i) => <p key={`${w}-${i}`}>{skillWarning(w)}</p>)}</div>}
      <nav className="flex flex-wrap gap-2" aria-label="Skill 页面">{([['library', `我的 Skill · ${data.library.length} 个`], ['locations', `文件位置 · ${skillLocations(data).total} 处`]] as const).map(([tab, title]) => <Button key={tab} variant={c.tab === tab ? 'default' : 'outline'} disabled={c.writing || c.editing} aria-pressed={c.tab === tab} onClick={() => c.setTab(tab)}>{title}</Button>)}</nav>
      {c.tab === 'library' && <div className="grid min-w-0 gap-5 xl:grid-cols-[240px_minmax(0,1fr)]"><section className="grid content-start gap-2" aria-label="我的 Skill">{data.library.length === 0 && <p className="text-sm text-muted-foreground">Anchor 尚未保存 Skill</p>}{data.library.map(s => <button key={s.id} aria-pressed={c.selected === s.id} disabled={busy || c.readingDetail || c.editing} className={`rounded-lg border p-3 text-left ${c.selected === s.id ? 'border-primary bg-accent' : 'hover:bg-accent/50'}`} onClick={() => c.select(s)}><span className="flex items-center gap-2 font-medium"><FileCode2 className="size-4" />{s.name}</span><span className="mt-1 block text-xs text-muted-foreground">{s.versions.length} 个已保存版本 · {skillLocations(data, s.id).total} 处文件位置</span></button>)}</section>
        <section className="grid min-w-0 content-start gap-4 rounded-lg border p-4" aria-label="Skill 详情">
          {c.readingDetail ? <p role="status">正在读取版本…</p> : c.current && c.detail?.skill_id === c.current.id ? <>
            <header className="flex flex-wrap items-center justify-between gap-3"><h2 className="text-lg font-semibold">{c.current.name}</h2><div className="flex flex-wrap items-center gap-2"><select aria-label="Skill 版本" className={selectClass} disabled={busy || c.editing} value={c.detail.version} onChange={e => c.current && c.select(c.current, Number(e.target.value))}>{c.current.versions.map(v => <option key={v.number} value={v.number}>v{v.number} · {v.created_at}</option>)}</select><Button size="sm" variant="outline" disabled={busy || c.editing} onClick={() => c.open('link')}><Link2 className="size-4" />添加到 Agent / 项目</Button><Button size="icon" variant="outline" aria-label="从 Anchor 移除此 Skill" disabled={busy || c.editing || data.mounts.some(m => m.skill_id === c.current?.id)} onClick={() => c.request(skillAction('delete', { skill_id: c.current!.id }), `从 Anchor 的列表移除 ${c.current!.name}？`, '不删除其他位置的文件，并暂停自动保存这些位置；可以在“文件位置”中重新保存到 Anchor。Anchor 已保存的文件仍作为恢复备份保留。')}><Trash2 className="size-4" /></Button></div></header>
            <section className="grid min-w-0 gap-3" aria-label="此 Skill 的文件位置"><h3 className="font-medium">文件位置 · {skillLocations(data, c.current.id).total} 处</h3><Locations data={data} skillID={c.current.id} controller={c} /></section>
            <div className="flex flex-wrap gap-2"><select aria-label="Skill 文件" className={`${selectClass} flex-1`} disabled={busy || c.editing} value={c.detail.file} onChange={e => c.current && c.select(c.current, c.detail!.version, e.target.value)}>{c.detail.files.map(f => <option key={f.path} value={f.path}>{f.path} · {f.size} B{f.executable ? ' · 可执行' : ''}</option>)}</select>{!c.editing && c.detail.version === c.current.versions.at(-1)?.number && <><Button size="sm" variant="outline" disabled={busy || !c.detail.editable} onClick={() => c.setEditing(true)}>编辑文件</Button><Button size="sm" variant="outline" disabled={busy} onClick={() => { c.setFilePath(''); c.setContent(''); c.setEditing(true); }}>新增文本文件</Button></>}</div>
            {c.editing ? <div className="grid gap-3"><label className="grid gap-1 text-sm">相对文件路径<Input disabled={busy} value={c.filePath} onChange={e => c.setFilePath(e.target.value)} /></label><label className="grid gap-1 text-sm">文件内容<textarea className={textareaClass} disabled={busy} value={c.content} onChange={e => c.setContent(e.target.value)} /></label><div className="flex flex-wrap gap-2"><Button disabled={busy || !c.filePath} onClick={c.save}>保存为新版本</Button><Button variant="outline" disabled={busy} onClick={() => { c.setEditing(false); c.setFilePath(c.detail!.file); c.setContent(c.detail!.content); }}>取消</Button>{c.filePath !== 'SKILL.md' && c.detail.files.some(f => f.path === c.filePath) && <Button variant="outline" disabled={busy} onClick={c.removeFile}>在新版本中删除文件</Button>}</div></div> : c.detail.editable ? <pre className="max-h-96 overflow-auto whitespace-pre-wrap break-all rounded-md bg-muted/40 p-3 text-sm">{c.detail.content || '（空文件）'}</pre> : <p className="text-sm text-muted-foreground">此文件为二进制或超过文本编辑上限</p>}
            <Differences detail={c.detail} />
          </> : <p className="text-sm text-muted-foreground">选择 Skill 查看版本和内容</p>}
        </section>
      </div>}
      {c.tab === 'locations' && <section className="grid min-w-0 gap-4" aria-label="文件位置">
        <div className="flex flex-wrap items-center justify-between gap-3"><label className="flex items-center gap-2 text-sm">筛选 Skill<select className={selectClass} value={c.locationSkill} disabled={busy || c.readingDetail} onChange={e => c.filterLocations(e.target.value)}><option value="">全部 Skill</option>{data.library.map(s => <option key={s.id} value={s.id}>{s.name}</option>)}</select></label><p className="text-sm text-muted-foreground">原位置文件 {skillLocations(data, c.locationSkill).sources.length} 处 · Anchor 快捷方式 {skillLocations(data, c.locationSkill).mounts.length} 处</p></div>
        <Locations data={data} skillID={c.locationSkill} controller={c} />
        {c.readingDetail && <p role="status">正在对比文件…</p>}
        {c.detail?.source_id && <section className="grid gap-3 rounded-lg border p-4" aria-label="此处文件的改动"><h2 className="font-semibold">此处文件的改动 · {c.detail.version ? `对比 Anchor 已保存的 v${c.detail.version}` : '尚未保存到 Anchor'}</h2>{data.sources.find(s => s.id === c.detail?.source_id)?.skill_id && <Button className="w-fit" size="sm" variant="outline" disabled={busy || c.readingDetail} onClick={() => { const source = data.sources.find(s => s.id === c.detail?.source_id); if (source) c.inspectSource(source, true); }}>对比 Anchor 最新版</Button>}<Differences detail={c.detail} /></section>}
      </section>}
    </>}
    <Dialog open={c.dialog !== null} onOpenChange={open => { if (!open && !c.writing) c.setDialog(null); }}><DialogContent onRequestClose={() => { if (!c.writing) c.setDialog(null); }} aria-describedby={undefined} className="max-h-[85vh] overflow-auto sm:max-w-2xl"><div><DialogTitle>{c.dialog === 'create' ? '新建 Skill' : c.dialog === 'project' ? '补充项目' : '添加到 Agent / 项目'}</DialogTitle></div>
      {c.dialog === 'create' ? <div className="grid gap-4"><label className="grid gap-1 text-sm">名称<Input value={c.name} disabled={busy} onChange={e => c.setName(e.target.value)} /></label><label className="grid gap-1 text-sm">SKILL.md<textarea className={textareaClass} disabled={busy} value={c.content} onChange={e => c.setContent(e.target.value)} /></label><Button disabled={busy || !c.name || !c.content} onClick={c.create}>保存到 Anchor</Button></div> : c.dialog === 'project' ? <div className="grid gap-4"><p className="text-sm text-muted-foreground">常见项目目录已自动扫描；仅在项目位于非常用位置时补充路径。</p><label className="grid gap-1 text-sm">当前设备上的项目绝对路径<Input value={c.project} disabled={busy} onChange={e => c.setProject(e.target.value)} /></label><Button disabled={busy || !c.project} onClick={c.register}>登记并扫描</Button></div> : <div className="grid gap-4"><label className="grid gap-1 text-sm">目标 Agent / 项目<select className={selectClass} disabled={busy} value={c.scope} onChange={e => c.setScope(e.target.value)}>{data?.scopes.map(s => <option key={s.id} value={s.id}>{skillLocationLabel(data!, s.id)} · {s.root}</option>)}</select></label><label className="grid gap-1 text-sm">目标目录名（留空使用 Skill 名称）<Input value={c.name} disabled={busy} onChange={e => c.setName(e.target.value)} /></label><Button disabled={busy || !c.scope} onClick={c.link}>确认添加位置…</Button></div>}
    </DialogContent></Dialog>
    <ConfirmAction open={!!c.confirmation} title={c.confirmation?.title ?? ''} busy={c.writing} onCancel={() => c.setConfirmation(null)} onConfirm={() => void c.confirm()}><p className="break-all">{c.confirmation?.impact}</p></ConfirmAction>
  </main>;
}
