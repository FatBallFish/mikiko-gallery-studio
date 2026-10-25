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
  return <section aria-label="视频公开设置" style={{ padding: 12 }}>
    <span>{publication ? videoPublicationLabel(publication.status) : '正在读取公开状态…'}</span>
    {publication?.review_reason ? <p>{publication.review_reason}</p> : null}
    {publication ? <button type="button" disabled={busy} onClick={() => void change(publication.status === 'approved' || publication.status === 'pending_review')}>
      {busy ? '正在更新…' : publication.status === 'approved' ? '撤回公开' : publication.status === 'pending_review' ? '取消公开申请' : '申请公开视频'}
    </button> : null}
    {publication?.status === 'private' || publication?.status === 'rejected' ? <p>视频和提示词将在审核通过后公开；上传和生成的视频均需审核。</p> : null}
    {error ? <p role="alert">{error}</p> : null}
  </section>
}
