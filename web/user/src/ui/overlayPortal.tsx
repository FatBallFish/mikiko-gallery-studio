import type { ReactNode } from 'react'
import { createPortal } from 'react-dom'

export const overlayPortalTarget = 'document.body'

export function overlayPortalHost(documentLike: Pick<Document, 'body'> | null | undefined): HTMLElement | null {
  return documentLike?.body ?? null
}

export function OverlayPortal({ children }: { children: ReactNode }) {
  const host = overlayPortalHost(typeof document === 'undefined' ? null : document)
  return host ? createPortal(children, host) : null
}
