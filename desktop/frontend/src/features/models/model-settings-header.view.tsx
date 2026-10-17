import { Button } from '../../components/ui/button';
import { DialogDescription, DialogTitle } from '../../components/ui/dialog';

export function ModelSettingsHeader({ alias, page, busy, switchPage }: {
  alias: string; page: 'general' | 'speed'; busy: boolean; switchPage?: () => void;
}) {
  return <div className="model-settings-heading">
    <header><DialogTitle>模型设置</DialogTitle><p className="model-settings-alias mono">{alias}</p>
      <DialogDescription className="sr-only">配置模型重试、上下文、能力与速度。切换分类前请保存，未保存的修改会丢弃。</DialogDescription>
    </header>
    {switchPage && <nav className="model-settings-tabs" aria-label="模型设置分类">
      <Button variant="ghost" aria-current={page === 'general' ? 'page' : undefined} disabled={busy} onClick={page === 'general' ? undefined : switchPage}>常规设置</Button>
      <Button variant="ghost" aria-current={page === 'speed' ? 'page' : undefined} disabled={busy} onClick={page === 'speed' ? undefined : switchPage}>速度与分身</Button>
    </nav>}
  </div>;
}
