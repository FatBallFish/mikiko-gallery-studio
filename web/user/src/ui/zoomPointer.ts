type ZoomPointerCandidate = {
  button: number
  target: { closest?: (selector: string) => unknown } | null
}

const interactiveSelector = 'button,a,input,textarea,select,[role="button"],[data-no-zoom-drag]'

export function shouldStartZoomDrag(candidate: ZoomPointerCandidate) {
  if (candidate.button !== 0) return false
  return !candidate.target?.closest?.(interactiveSelector)
}
