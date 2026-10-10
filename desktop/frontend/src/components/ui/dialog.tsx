import * as Primitive from '@radix-ui/react-dialog';
import { X } from 'lucide-react';
import type { ComponentProps, ReactNode } from 'react';
import { cn } from '../../lib/utils';
export const Dialog = Primitive.Root;
export const DialogTitle = Primitive.Title;
export const DialogDescription = Primitive.Description;
export function DialogContent({ children, className, onRequestClose, ...props }: ComponentProps<typeof Primitive.Content> & { children: ReactNode; onRequestClose: () => void }) {
  return <Primitive.Portal><Primitive.Overlay className="dialog-overlay" />
    <Primitive.Content className={cn('dialog-content rounded-lg border bg-background shadow-lg', className)} {...props}>
      {children}
      <button className="dialog-close" aria-label="关闭编辑面板" type="button" onClick={onRequestClose}><X size={18} /></button>
    </Primitive.Content>
  </Primitive.Portal>;
}
