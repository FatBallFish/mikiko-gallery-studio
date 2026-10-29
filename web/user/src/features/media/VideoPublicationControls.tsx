import { useMemo, type ReactNode } from 'react'
import { videoPublicationApi } from './videoPublicationApi'
import { loadVideoPublication, PublicationControls } from './PublicationControls'

export function VideoPublicationControls({ assetID, onChanged, children }: { assetID: string; onChanged: () => void; children?: ReactNode }) {
  const load = useMemo(() => loadVideoPublication(assetID), [assetID])
  return <PublicationControls mediaType="video" load={load} onChanged={onChanged} change={async (withdraw) => {
    const value = await (withdraw ? videoPublicationApi.withdraw(assetID) : videoPublicationApi.submit(assetID))
    return { status: value.status, reason: value.review_reason }
  }}>{children}</PublicationControls>
}
