import { useEffect, useRef } from 'react';

/**
 * Keeps keyboard focus from falling to the top of the document when a control
 * disables itself while its request is in flight.
 *
 * Disabling a focused button blurs it, and the browser has nowhere to put focus
 * but the body. Someone who saved the profile form with Enter was returned to
 * the start of the page: the result was announced, their place was not.
 *
 * The blur is the signal. Asking whether the button had focus once `busy` is
 * true is too late — React has already committed the `disabled` attribute and
 * the browser has already moved focus away. A blur that arrives while the
 * element is disabled is one the user did not ask for, and that is the one
 * worth undoing.
 *
 * Focus is only restored if nothing else has claimed it, so a form that
 * deliberately sends focus to a field needing correction keeps that.
 */
export function useRetainFocus<T extends HTMLElement>(busy: boolean) {
  const ref = useRef<T | null>(null);
  const takenAway = useRef(false);

  useEffect(() => {
    const element = ref.current;
    if (!element) return;
    const onBlur = () => { takenAway.current = element.hasAttribute('disabled'); };
    const onFocus = () => { takenAway.current = false; };
    element.addEventListener('blur', onBlur);
    element.addEventListener('focus', onFocus);
    return () => {
      element.removeEventListener('blur', onBlur);
      element.removeEventListener('focus', onFocus);
    };
  }, []);

  useEffect(() => {
    if (busy || !takenAway.current) return;
    const element = ref.current;
    if (!element || element.hasAttribute('disabled')) return;
    const active = document.activeElement;
    if (active && active !== document.body) return;
    takenAway.current = false;
    element.focus();
  }, [busy]);

  return ref;
}
