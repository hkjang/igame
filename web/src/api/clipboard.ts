/**
 * Copies text and reports honestly whether it worked.
 *
 * `navigator.clipboard` exists only in a secure context, so it is simply absent
 * on an intranet deployment served over plain HTTP. The values copied here —
 * an API key shown exactly once — cannot be recovered, so a silent failure or a
 * false "copied" confirmation is worse than telling the user to copy manually.
 */
export async function copyText(value: string): Promise<boolean> {
  if (!value) return false;
  if (navigator.clipboard?.writeText) {
    try {
      await navigator.clipboard.writeText(value);
      return true;
    } catch {
      // Denied by permissions policy or the user; fall through to the legacy path.
    }
  }
  return legacyCopy(value);
}

/** execCommand is deprecated but remains the only path outside a secure context. */
function legacyCopy(value: string): boolean {
  if (typeof document === 'undefined' || typeof document.execCommand !== 'function') return false;
  const field = document.createElement('textarea');
  field.value = value;
  field.setAttribute('readonly', '');
  field.setAttribute('aria-hidden', 'true');
  field.style.position = 'fixed';
  field.style.top = '-1000px';
  field.style.opacity = '0';
  document.body.appendChild(field);
  try {
    field.select();
    field.setSelectionRange(0, value.length);
    return document.execCommand('copy');
  } catch {
    return false;
  } finally {
    document.body.removeChild(field);
  }
}
