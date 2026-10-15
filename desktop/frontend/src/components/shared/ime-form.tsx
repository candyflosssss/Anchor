import type { ComponentProps } from 'react';
import { useImeGuard } from './use-ime-guard';

/** A native form that does not implicitly submit when Enter selects an IME candidate. */
export function ImeForm({ onCompositionStartCapture, onCompositionEndCapture, onBlurCapture, onKeyDownCapture, ...props }: ComponentProps<'form'>) {
  const ime = useImeGuard();
  return <form {...props}
    onCompositionStartCapture={event => { ime.onCompositionStartCapture(event); onCompositionStartCapture?.(event); }}
    onCompositionEndCapture={event => { ime.onCompositionEndCapture(event); onCompositionEndCapture?.(event); }}
    onBlurCapture={event => { ime.onBlurCapture(event); onBlurCapture?.(event); }}
    onKeyDownCapture={event => { ime.onKeyDownCapture(event); if (!event.defaultPrevented) onKeyDownCapture?.(event); }} />;
}
