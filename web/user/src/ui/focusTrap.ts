export const focusableSelector = [
  'a[href]',
  'button:not([disabled])',
  'input:not([disabled])',
  'select:not([disabled])',
  'textarea:not([disabled])',
  '[tabindex]:not([tabindex="-1"])',
].join(',')

export function focusTrapTargetIndex(currentIndex: number, focusableCount: number, shiftKey: boolean): number | null {
  if (focusableCount < 1) return null
  if (currentIndex < 0) return shiftKey ? focusableCount - 1 : 0
  if (shiftKey && currentIndex === 0) return focusableCount - 1
  if (!shiftKey && currentIndex === focusableCount - 1) return 0
  return null
}

export function focusableElements(container: HTMLElement): HTMLElement[] {
  return Array.from(container.querySelectorAll<HTMLElement>(focusableSelector)).filter((element) => (
    !element.hidden
    && element.getAttribute('aria-hidden') !== 'true'
    && element.style.display !== 'none'
    && element.style.visibility !== 'hidden'
  ))
}
