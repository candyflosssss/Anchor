import * as Primitive from '@radix-ui/react-dialog';
import { X } from 'lucide-react';
import { useRef, type ComponentProps, type ReactNode } from 'react';
import { cn } from '../../lib/utils';
export const Dialog = Primitive.Root;
export const DialogTitle = Primitive.Title;
export const DialogDescription = Primitive.Description;
export function DialogContent({ children, className, onRequestClose, closeLabel = '关闭编辑面板', onCompositionStartCapture, onCompositionEndCapture, onKeyDownCapture, onEscapeKeyDown, ...props }: ComponentProps<typeof Primitive.Content> & { children: ReactNode; onRequestClose: () => void; closeLabel?: string }) {
  const composing = useRef(false);
  return <Primitive.Portal><Primitive.Overlay className="dialog-overlay" />
    <Primitive.Content className={cn('dialog-content rounded-lg border bg-background shadow-lg', className)} {...props}
      onEscapeKeyDown={event => {
        // Radix observes Escape on document capture, before the input handler.
        if (composing.current || event.isComposing || event.keyCode === 229) { event.preventDefault(); return; }
        onEscapeKeyDown?.(event);
      }}
      onCompositionStartCapture={event => { composing.current = true; onCompositionStartCapture?.(event); }}
      onCompositionEndCapture={event => { composing.current = false; onCompositionEndCapture?.(event); }}
      onKeyDownCapture={event => {
        // Enter accepts an IME candidate; Escape dismisses candidates, not the dialog.
        // keyCode 229 covers WebViews that clear isComposing before the final keydown.
        if ((event.key === 'Enter' || event.key === 'Escape') && (composing.current || event.nativeEvent.isComposing || event.keyCode === 229)) {
          event.preventDefault(); event.stopPropagation(); return;
        }
        onKeyDownCapture?.(event);
      }}>
      {children}
      <button className="dialog-close" aria-label={closeLabel} type="button" onClick={onRequestClose}><X size={18} /></button>
    </Primitive.Content>
  </Primitive.Portal>;
}
