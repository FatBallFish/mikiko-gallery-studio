import React, { useEffect, useState, type ReactNode } from 'react'
import type { MediaAsset } from '../../../../shared/api-types'
import { userApi } from '../../../../shared/user-api'
import { videoPublicationApi } from './videoPublicationApi'

export function publicationState(status: string) {
  if (status === 'approved' || status === 'public') return 'public'
  if (['pending', 'pending_review', 'reviewing'].includes(status)) return 'pending'
  if (status === 'unpublished' || status === 'offline') return 'offline'
  if (status === 'rejected') return 'rejected'
  return 'private'
}

type Publication = { status: string; reason?: string }
type Props = {
  mediaType: 'image' | 'video'
  initialStatus?: string
  load?: () => Promise<Publication>
  change: (withdraw: boolean) => Promise<Publication>
  restriction?: string
  onChanged: () => void
  children?: ReactNode
}

export function PublicationControls({ mediaType, initialStatus = 'private', load, change, restriction, onChanged, children }: Props) {
  const [publication, setPublication] = useState<Publication>({ status: initialStatus })
  const [loading, setLoading] = useState(Boolean(load))
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [retry, setRetry] = useState(0)
  useEffect(() => {
    let alive = true
    setPublication({ status: initialStatus }); setError(''); setLoading(Boolean(load))
    if (load) void load().then((value) => { if (alive) setPublication(value) }).catch((caught) => {
      if (alive) setError(caught instanceof Error ? caught.message : '公开状态加载失败')
    }).finally(() => { if (alive) setLoading(false) })
    return () => { alive = false }
  }, [load, initialStatus, retry])
  const state = publicationState(publication.status)
  const withdraw = state === 'public' || state === 'pending'
  const update = async () => {
    setBusy(true); setError('')
    try { setPublication(await change(withdraw)); onChanged() }
    catch (caught) { setError(caught instanceof Error ? caught.message : '公开状态更新失败') }
    finally { setBusy(false) }
  }
  return <section className="video-publication media-publication" data-media-selection-control aria-label={`${mediaType === 'image' ? '图片' : '视频'}公开设置`} aria-busy={busy || loading}>
    <div className="video-publication__head">
      <span className={`u-status video-publication__status ${state === 'public' ? 'u-status--ok' : state === 'pending' ? 'u-status--warn' : state === 'rejected' ? 'u-status--bad' : ''}`} role="status">
        <span className="u-dot" aria-hidden="true" />
        {loading ? '正在读取公开状态…' : ({ private: '私有', public: '已公开', pending: '待审核', rejected: '未通过', offline: '已下架' })[state]}
      </span>
      {!restriction ? <button className="u-btn u-btn--sm video-publication__action" type="button" disabled={busy || loading || Boolean(load && error)} onClick={() => void update()}>
        {busy ? '正在更新…' : state === 'public' ? '撤回公开' : state === 'pending' ? '取消公开申请' : '申请公开'}
      </button> : null}
    </div>
    {children}
    {restriction ? <p className="video-publication__hint">{restriction}</p> : null}
    {publication.reason ? <p className="video-publication__reason">{publication.reason}</p> : null}
    {!restriction && (state === 'private' || state === 'rejected' || state === 'offline') ? <p className="video-publication__hint">媒体和提示词将在审核通过后公开。</p> : null}
    {error ? <p className="video-publication__error" role="alert">{error}{load ? <button className="u-btn u-btn--sm" type="button" disabled={busy || loading} onClick={() => setRetry((value) => value + 1)}>重新读取</button> : null}</p> : null}
  </section>
}

export function ImagePublicationControls({ asset, onChanged, children }: { asset: MediaAsset; onChanged: () => void; children?: ReactNode }) {
  const restriction = !asset.legacy_image_id ? '此图片来源尚未关联可公开的历史图片记录，暂不支持申请公开。' : asset.status !== 'ready' ? '图片处理完成后才可申请公开。' : undefined
  return <PublicationControls mediaType="image" initialStatus={asset.visibility_status} restriction={restriction} onChanged={onChanged} change={async (withdraw) => {
    if (!asset.legacy_image_id) throw new Error('缺少可公开的历史图片记录')
    const image = await (withdraw ? userApi.cancelImagePublish(asset.legacy_image_id) : userApi.publishImage(asset.legacy_image_id))
    return { status: image.visibility_status, reason: image.review_reason }
  }}>{children}</PublicationControls>
}

export const loadVideoPublication = (assetID: string) => async (): Promise<Publication> => {
  const value = await videoPublicationApi.owner(assetID)
  return { status: value.status, reason: value.review_reason }
}
