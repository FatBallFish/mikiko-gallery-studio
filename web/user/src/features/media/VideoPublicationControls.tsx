import { useEffect, useState } from 'react'
import { videoPublicationApi, videoPublicationLabel, type VideoPublication } from './videoPublicationApi'

export function VideoPublicationControls({ assetID, onChanged }: { assetID: string; onChanged: () => void }) {
  const [publication, setPublication] = useState<VideoPublication | null>(null)
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  useEffect(() => {
    let alive = true
    void videoPublicationApi.owner(assetID).then((value) => { if (alive) setPublication(value) }).catch((caught) => { if (alive) setError(caught instanceof Error ? caught.message : '公开状态加载失败') })
    return () => { alive = false }
  }, [assetID])
  const change = async (withdraw: boolean) => {
    setBusy(true); setError('')
    try { setPublication(await (withdraw ? videoPublicationApi.withdraw(assetID) : videoPublicationApi.submit(assetID))); onChanged() }
    catch (caught) { setError(caught instanceof Error ? caught.message : '公开状态更新失败') }
    finally { setBusy(false) }
  }
  const statusTone = publication?.status === 'approved' ? 'u-status--ok' : publication?.status === 'pending_review' ? 'u-status--warn' : publication?.status === 'rejected' ? 'u-status--bad' : ''
  return (
    <section className="video-publication" aria-label="视频公开设置" aria-busy={busy || (!publication && !error)}>
      <div className="video-publication__head">
        <span className={`u-status video-publication__status ${statusTone}`} role="status">
          <span className="u-dot" aria-hidden="true" />
          {publication ? videoPublicationLabel(publication.status) : error ? '公开状态加载失败' : '正在读取公开状态…'}
        </span>
        {publication ? (
          <button className="u-btn u-btn--sm video-publication__action" type="button" disabled={busy} onClick={() => void change(publication.status === 'approved' || publication.status === 'pending_review')}>
            {busy ? '正在更新…' : publication.status === 'approved' ? '撤回公开' : publication.status === 'pending_review' ? '取消公开申请' : '申请公开视频'}
          </button>
        ) : null}
      </div>
      {publication?.review_reason ? <p className="video-publication__reason">{publication.review_reason}</p> : null}
      {publication?.status === 'private' || publication?.status === 'rejected' ? <p className="video-publication__hint">视频和提示词将在审核通过后公开；上传和生成的视频均需审核。</p> : null}
      {error ? <p className="video-publication__error" role="alert">{error}</p> : null}
    </section>
  )
}
