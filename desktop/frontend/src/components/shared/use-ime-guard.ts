import { useRef, type CompositionEvent, type FocusEvent, type KeyboardEvent } from 'react';

type ImeKey = Pick<globalThis.KeyboardEvent, 'isComposing' | 'keyCode' | 'target'>;

/** Share candidate-key handling between modal dialogs, inline forms and popovers. */
export function useImeGuard() {
  const target = useRef<EventTarget | null>(null);
  const isComposing = (event: ImeKey) => event.isComposing || event.keyCode === 229 || (target.current !== null && target.current === event.target);
  return {
    isComposing,
    onCompositionStartCapture: (event: CompositionEvent) => { target.current = event.target; },
    onCompositionEndCapture: (event: CompositionEvent) => { if (target.current === event.target) target.current = null; },
    // Some WebViews omit compositionend when an input loses focus or is removed.
    onBlurCapture: (event: FocusEvent) => { if (target.current === event.target) target.current = null; },
    onKeyDownCapture: (event: KeyboardEvent) => {
      if ((event.key === 'Enter' || event.key === 'Escape') && isComposing(event.nativeEvent)) {
        event.preventDefault(); event.stopPropagation();
      }
    },
  };
}
