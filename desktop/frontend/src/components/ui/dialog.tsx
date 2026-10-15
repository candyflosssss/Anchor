import * as Primitive from '@radix-ui/react-dialog';
import { X } from 'lucide-react';
import type { ComponentProps, ReactNode } from 'react';
import { useImeGuard } from '../shared/use-ime-guard';
import { cn } from '../../lib/utils';
export const Dialog = Primitive.Root;
export const DialogTitle = Primitive.Title;
export const DialogDescription = Primitive.Description;
export function DialogContent({ children, className, onRequestClose, closeLabel = '关闭编辑面板', onCompositionStartCapture, onCompositionEndCapture, onBlurCapture, onKeyDownCapture, onEscapeKeyDown, ...props }: ComponentProps<typeof Primitive.Content> & { children: ReactNode; onRequestClose: () => void; closeLabel?: string }) {
  const ime = useImeGuard();
  return <Primitive.Portal><Primitive.Overlay className="dialog-overlay" />
    <Primitive.Content className={cn('dialog-content rounded-lg border bg-background shadow-lg', className)} {...props}
      onEscapeKeyDown={event => {
        // Radix observes Escape on document capture, before the input handler.
        if (ime.isComposing(event)) { event.preventDefault(); return; }
        onEscapeKeyDown?.(event);
      }}
      onCompositionStartCapture={event => { ime.onCompositionStartCapture(event); onCompositionStartCapture?.(event); }}
      onCompositionEndCapture={event => { ime.onCompositionEndCapture(event); onCompositionEndCapture?.(event); }}
      onBlurCapture={event => { ime.onBlurCapture(event); onBlurCapture?.(event); }}
      onKeyDownCapture={event => { ime.onKeyDownCapture(event); if (!event.defaultPrevented) onKeyDownCapture?.(event); }}>
      {children}
      <button className="dialog-close" aria-label={closeLabel} type="button" onClick={onRequestClose}><X size={18} /></button>
    </Primitive.Content>
  </Primitive.Portal>;
}
