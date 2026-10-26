import { useEffect, useState } from 'react'
import { Drawer, useApp } from '../../components'
import { sharedApiClient } from '../../../../shared/http-client'
import './media-redesign.css'
import { publicVideoURL, videoPublicationApi, type PublicVideo } from './videoPublicationApi'

export type InteractivePublicVideo = PublicVideo & {
  like_count?: number; favorite_count?: number; liked_by_viewer?: boolean; favorited_by_viewer?: boolean; download_url?: string
}

function VideoActions({ video, onChange }: { video: InteractivePublicVideo; onChange: (video: InteractivePublicVideo) => void }) {
  const app = useApp()
  const [busy, setBusy] = useState(false)
  async function act(kind: 'like' | 'favorite' | 'download') {
    const token = app.session?.token
    if (!token) { app.notify('warn', '请先登录后再点赞、收藏或下载'); app.navigate('login', { returnTo: 'public-gallery' }); return }
    if (busy) return
    setBusy(true)
    try {
      const path = `/api/public/v1/videos/${encodeURIComponent(video.id)}`
      if (kind === 'download') {
        const response = await fetch(publicVideoURL(`${path}/download`), { headers: { Authorization: `Bearer ${token}` } })
        if (!response.ok) throw new Error(response.status === 404 ? '视频已不可用' : '下载失败，请重试')
        const disposition = response.headers.get('Content-Disposition') ?? ''
        const encodedName = disposition.match(/filename\*=UTF-8''([^;]+)/i)?.[1]
        const plainName = disposition.match(/filename="([^"]+)"|filename=([^;]+)/i)
        const name = encodedName ? decodeURIComponent(encodedName) : plainName?.[1] ?? plainName?.[2] ?? 'video.mp4'
        const url = URL.createObjectURL(await response.blob())
        const link = document.createElement('a'); link.href = url; link.download = name; document.body.append(link); link.click(); link.remove()
        setTimeout(() => URL.revokeObjectURL(url), 30_000)
        app.notify('success', '视频下载已开始')
      } else {
        const active = kind === 'like' ? !video.liked_by_viewer : !video.favorited_by_viewer
        const updated = await sharedApiClient.request<InteractivePublicVideo>(`${path}/${kind}`, { method: 'POST', body: { active } })
        onChange(updated)
      }
    } catch (caught) { app.notify('error', caught instanceof Error ? caught.message : '操作失败，请重试') }
    finally { setBusy(false) }
  }
  return <div className="u-card__meta" aria-label="视频互动" onClick={(event) => event.stopPropagation()}>
    <button type="button" className="u-btn" disabled={busy} aria-pressed={!!video.liked_by_viewer} onClick={() => void act('like')}>点赞 {video.like_count ?? 0}</button>
    <button type="button" className="u-btn" disabled={busy} aria-pressed={!!video.favorited_by_viewer} onClick={() => void act('favorite')}>收藏 {video.favorite_count ?? 0}</button>
    <details><summary aria-label="视频更多操作" style={{ cursor: 'pointer', padding: 10 }}>更多</summary><button type="button" className="u-btn" disabled={busy} onClick={() => void act('download')}>下载视频</button></details>
  </div>
}

export function PublicVideoDetailDialog({ video, onClose, onChange }: { video: InteractivePublicVideo; onClose: () => void; onChange?: (video: InteractivePublicVideo) => void }) {
  const [detail, setDetail] = useState<InteractivePublicVideo | null>(null)
  const [error, setError] = useState('')
  const [playbackError, setPlaybackError] = useState('')
  useEffect(() => {
    let alive = true
    setDetail(null); setError(''); setPlaybackError('')
    void sharedApiClient.request<InteractivePublicVideo>(`/api/public/v1/videos/${encodeURIComponent(video.id)}`).then((item) => { if (alive) setDetail(item) }).catch((caught) => { if (alive) setError(caught instanceof Error ? caught.message : '视频已不可用') })
    return () => { alive = false }
  }, [video.id])
  return <Drawer open title={detail?.title ?? video.title} onClose={onClose} className="media-detail-drawer">
    <div className="u-detail">
      <section className="u-detail__stage-col"><div className="u-detail__stage">
        <span className="u-detail__stage-label">公开视频</span>
        {error ? <p className="media-detail-message" role="alert">{error}</p> : detail ? <>
          <video className="media-detail-media" controls playsInline preload="metadata" poster={publicVideoURL(detail.poster_url)} src={publicVideoURL(detail.video_url)} onError={() => setPlaybackError('视频已不可用或暂时无法播放，请关闭后重试。')} />
          {playbackError ? <p className="media-detail-message" role="alert">{playbackError}</p> : null}
        </> : <p className="media-detail-message" role="status">正在载入视频…</p>}
      </div></section>
      <section className="u-detail__info"><div className="u-detail__info-scroll">
        <h2 className="u-detail__title">{detail?.title ?? video.title}</h2>
        <span className="u-status u-status--ok">已公开</span>
        {detail ? <>
          <VideoActions video={detail} onChange={(updated) => { setDetail(updated); onChange?.(updated) }} />
          <div className="u-detail__section"><h4>Prompt</h4><div className="u-prompt" tabIndex={0}>{detail.prompt || '未记录 Prompt'}</div></div>
          <div className="u-detail__section"><h4>视频信息</h4><div className="media-detail-fields">
            {detail.width && detail.height ? <p>尺寸：{detail.width} × {detail.height}</p> : null}
            {detail.duration_ms ? <p>时长：{(detail.duration_ms / 1000).toFixed(1)} 秒</p> : null}
            {detail.published_at ? <p>公开时间：{new Date(detail.published_at).toLocaleString()}</p> : null}
          </div></div>
        </> : null}
      </div><div className="u-detail__foot"><button className="u-btn" type="button" onClick={onClose}>关闭</button></div></section>
    </div>
  </Drawer>
}

export function PublicVideoCard({ video, onChange }: { video: InteractivePublicVideo; onChange?: (video: InteractivePublicVideo) => void }) {
  const [current, setCurrent] = useState(video)
  useEffect(() => { setCurrent(video) }, [video])
  function update(updated: InteractivePublicVideo) { setCurrent(updated); onChange?.(updated) }
  const [failed, setFailed] = useState(false)
  const [open, setOpen] = useState(false)
  return <article className="u-card pg-card">
    <button type="button" className="u-card__media" onClick={() => setOpen(true)} aria-label={`播放视频 ${video.title}`} style={{ width: '100%', border: 0, padding: 0, position: 'relative' }}>
      {failed ? <span className="media-detail-message">封面暂不可用 · 点击播放视频</span> : <img src={publicVideoURL(video.poster_url)} alt={video.title} loading="lazy" onError={() => setFailed(true)} style={{ width: '100%', aspectRatio: '4/3', objectFit: 'cover' }} />}
      <span className="u-card__tag">视频 · 播放</span>
    </button>
    <div className="u-card__body"><div className="u-card__title" style={{ minHeight: '2.8em', lineHeight: 1.4, display: '-webkit-box', WebkitLineClamp: 2, WebkitBoxOrient: 'vertical', overflow: 'hidden' }}>{video.title || '公开视频'}</div><div className="u-card__meta"><span>{video.duration_ms ? `${(video.duration_ms / 1000).toFixed(1)} 秒` : '视频'}</span><span>{video.width && video.height ? `${video.width}×${video.height}` : ''}</span></div></div>
    <VideoActions video={current} onChange={update} />
    {open ? <PublicVideoDetailDialog video={current} onChange={update} onClose={() => setOpen(false)} /> : null}
  </article>
}

export function PublicVideoGallery({ pageSize = 12 }: { pageSize?: number }) {
  const [items, setItems] = useState<PublicVideo[]>([])
  const [page, setPage] = useState(1)
  const [total, setTotal] = useState(0)
  const [error, setError] = useState('')
  const [reload, setReload] = useState(0)
  const [loading, setLoading] = useState(true)
  useEffect(() => {
    let alive = true; setLoading(true); setError('')
    void videoPublicationApi.list(page, pageSize).then((result) => { if (alive) { setItems(result.items); setTotal(result.total) } }).catch((caught) => { if (alive) { setItems([]); setError(caught instanceof Error ? caught.message : '视频加载失败') } }).finally(() => { if (alive) setLoading(false) })
    return () => { alive = false }
  }, [page, pageSize, reload])
  if (!loading && !error && !items.length) return null
  return <section aria-label="公开视频" style={{ marginBlock: 24 }}>
    {loading ? <p role="status">正在载入视频…</p> : error ? <p role="alert">{error} <button type="button" onClick={() => setReload((value) => value + 1)}>重试</button></p> : !items.length ? <p>暂无公开视频</p> : <div className="u-grid u-grid--gallery">{items.map((video) => <PublicVideoCard key={video.id} video={video} onChange={(updated) => setItems((current) => current.map((item) => item.id === updated.id ? updated : item))} />)}</div>}
    {total > pageSize ? <nav aria-label="视频翻页"><button type="button" disabled={loading || page <= 1} onClick={() => setPage(page - 1)}>上一页</button><span> {page} / {Math.ceil(total / pageSize)} </span><button type="button" disabled={loading || page * pageSize >= total} onClick={() => setPage(page + 1)}>下一页</button></nav> : null}
  </section>
}
