import { useRef } from 'react';
import { ArrowDown, ArrowUp, Columns3, RotateCcw } from 'lucide-react';
import { Button } from '../../components/ui/button';
import { Dialog, DialogContent, DialogDescription, DialogTitle } from '../../components/ui/dialog';
import { columnGroups, columnPresets, logColumns } from './log-columns.model';
import type { LogColumnsController } from './use-log-columns-controller';

export function LogColumnSettings({ c }: { c: LogColumnsController }) {
  const opener = useRef<HTMLElement | null>(null);
  return <Dialog open={c.open} onOpenChange={c.setOpen}>
    <DialogContent className="log-column-dialog" closeLabel="关闭显示内容" onRequestClose={() => c.setOpen(false)}
      onOpenAutoFocus={() => { opener.current = document.activeElement instanceof HTMLElement ? document.activeElement : null; }}
      onCloseAutoFocus={event => { event.preventDefault(); opener.current?.focus(); }}>
      <div className="log-column-heading">
        <DialogTitle className="flex items-center gap-2 text-lg font-semibold"><Columns3 size={19} />显示内容</DialogTitle>
        <DialogDescription className="mt-2 text-xs leading-6 text-muted-foreground">按需组合日志字段，勾选后立即显示。只保存本机列偏好，记录仍为虚拟数据。</DialogDescription>
        <div className="log-column-presets" aria-label="显示预设">
          {Object.entries(columnPresets).map(([key, preset]) => <Button key={key} variant="outline" size="sm" onClick={() => c.preset(key as keyof typeof columnPresets)}>{preset.label}</Button>)}
          <Button variant="ghost" size="sm" onClick={c.reset}><RotateCcw size={13} />恢复默认</Button>
        </div>
      </div>
      <div className="log-column-layout">
        <div className="log-column-catalog">
          {columnGroups.map(group => <fieldset key={group}><legend>{group}</legend>
            {logColumns.filter(column => column.group === group).map(column => <label key={column.id} className="log-column-option">
              <input type="checkbox" checked={c.ids.includes(column.id)} disabled={c.ids.length === 1 && c.ids.includes(column.id)} onChange={() => c.toggle(column.id)} />
              <span><strong>{column.label}</strong><small>{column.description}</small></span>
            </label>)}
          </fieldset>)}
        </div>
        <section className="log-column-order" aria-label="已显示字段顺序">
          <h3>已显示 <span>{c.ids.length} / {logColumns.length}</span></h3>
          <p>从左到右，至少保留一列。详情入口始终可用。</p>
          <ol>{c.columns.map((column, index) => <li key={column.id}>
            <span className="mono log-column-index">{index + 1}</span><span className="log-column-order-label">{column.label}</span>
            <Button variant="ghost" size="icon" disabled={index === 0} aria-label={`左移${column.label}`} onClick={() => c.move(column.id, -1)}><ArrowUp size={13} /></Button>
            <Button variant="ghost" size="icon" disabled={index === c.ids.length - 1} aria-label={`右移${column.label}`} onClick={() => c.move(column.id, 1)}><ArrowDown size={13} /></Button>
          </li>)}</ol>
        </section>
      </div>
      <footer className="log-column-dialog-footer">
        <p role="status">{c.storageError ? '偏好无法保存，当前页面仍可使用；刷新后可能重置。' : '列偏好保存在本机；缺失指标显示“未知”。'}</p>
        <Button size="sm" onClick={() => c.setOpen(false)}>完成</Button>
      </footer>
    </DialogContent>
  </Dialog>;
}
