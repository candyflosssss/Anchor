import { CircleHelp, LoaderCircle, List, KeyRound, Pencil, ShieldCheck, X } from 'lucide-react';
import { Button } from '../../components/ui/button';
import { StatusBadge } from '../../components/shared/status-badge';
import { catalogFailureText, catalogProblemText, type Provider, type ProviderModels, type ProblemCode } from './provider.model';
export function ProviderDetail({ provider: p, blocked, models, modelsBusy, modelsProblem, onDiscover, onClose, onEdit, onToggle }: { provider: Provider; blocked: boolean; models: ProviderModels | null; modelsBusy: boolean; modelsProblem: ProblemCode | null; onDiscover: () => void; onClose: () => void; onEdit: () => void; onToggle: () => void }) {
  return <aside className="provider-detail" aria-label={`${p.id} 的详情`}>
    <header><span className="eyebrow">供应商详情</span><Button variant="ghost" size="icon" disabled={modelsBusy} onClick={onClose} aria-label="收起供应商详情"><X /></Button></header>
    <div className="detail-identity"><span className="provider-avatar large">{Array.from(p.id)[0]}</span><h2>{p.id}</h2><StatusBadge enabled={p.enabled} /></div>
    <dl className="detail-properties">
      <dt>API 根地址</dt><dd className="mono break-all">{p.base_url}</dd>
      <dt>协议</dt><dd>Responses</dd>
      <dt>凭据</dt><dd className="flex items-center gap-2"><KeyRound size={14} />{p.has_secret ? '已配置 · 密钥不可回显' : '未配置密钥'}</dd>
      <dt>请求超时</dt><dd>{p.timeout_seconds} 秒</dd>
      <dt>最大并发</dt><dd>{p.max_concurrent} 个请求</dd>
      <dt>配置版本</dt><dd className="mono">revision {p.revision}</dd>
    </dl>
    <section className="model-directory" aria-label="上游模型目录" aria-busy={modelsBusy}>
      <h3>上游模型目录</h3>
      <p className="detail-footnote">手动读取已存地址的 /models；不生成内容、不导入映射。停用供应商也可查询。</p>
      <Button variant="outline" disabled={blocked} onClick={onDiscover}>{modelsBusy ? <LoaderCircle className="loading-icon" /> : <List />}{modelsBusy ? '正在获取目录…' : '获取模型目录'}</Button>
      {modelsProblem && <p className="catalog-feedback" role="alert">{catalogProblemText(modelsProblem)}</p>}
      {models && <div className="catalog-feedback">
        <p role="status">{models.failure ? '目录获取失败' : `已获取 ${models.models.length} 个模型`} · HTTP {models.http_status || '—'} · {models.elapsed_ms} ms · revision {models.revision}</p>
        {models.failure ? <p role="alert">{catalogFailureText[models.failure]}</p> : models.models.length === 0 ? <p>上游返回空目录；未推断其推理能力。</p> : <ul className="catalog-list" tabIndex={0} aria-label="上游模型 ID">{models.models.map(id => <li className="mono break-all" key={id}>{id}</li>)}</ul>}
      </div>}
    </section>
    <div className="detail-note"><CircleHelp size={16} /><div><strong>模型推理尚未测试</strong><p>目录成功不代表模型生成、工具调用、流式输出、余额或持续健康可用。</p></div></div>
    <div className="detail-actions"><Button variant="outline" disabled={blocked} onClick={onEdit}><Pencil />编辑配置</Button><Button variant="ghost" disabled={blocked} onClick={onToggle}>{p.enabled ? '停用供应商' : '启用供应商'}</Button></div>
    <p className="detail-footnote"><ShieldCheck size={14} />停用不会删除配置或已存密钥。</p>
  </aside>;
}
