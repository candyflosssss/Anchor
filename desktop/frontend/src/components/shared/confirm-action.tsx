import * as Alert from '@radix-ui/react-alert-dialog';
import { Button } from '../ui/button';
export function ConfirmAction({ open, title, children, busy, onCancel, onConfirm }: { open: boolean; title: string; children: React.ReactNode; busy: boolean; onCancel: () => void; onConfirm: () => void }) {
  return <Alert.Root open={open} onOpenChange={value => { if (!value && !busy) onCancel(); }}>
    <Alert.Portal><Alert.Overlay className="dialog-overlay" /><Alert.Content className="dialog-content confirm-content">
      <Alert.Title className="text-lg font-semibold">{title}</Alert.Title>
      <Alert.Description asChild><div className="mt-4 text-sm leading-6 text-muted-foreground">{children}</div></Alert.Description>
      <div className="mt-6 flex justify-end gap-2">
        <Alert.Cancel asChild><Button variant="outline" disabled={busy}>取消</Button></Alert.Cancel>
        <Button variant="destructive" disabled={busy} onClick={onConfirm}>{busy ? '正在提交…' : '确认变更'}</Button>
      </div>
    </Alert.Content></Alert.Portal>
  </Alert.Root>;
}
