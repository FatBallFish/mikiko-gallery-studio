import { useEffect, useState } from 'react'
import { Drawer } from '../../components'
import './media-redesign.css'
import { publicVideoURL, videoPublicationApi, type PublicVideo } from './videoPublicationApi'

export function PublicVideoDetailDialog({ video, onClose }: { video: PublicVideo; onClose: () => void }) {
  const [detail, setDetail] = useState<PublicVideo | null>(null)
  const [error, setError] = useState('')
  const [playbackError, setPlaybackError] = useState('')
  useEffect(() => {
    let alive = true
    setDetail(null); setError('')
    void videoPublicationApi.detail(video.id).then((item) => { if (alive) setDetail(item) }).catch((caught) => { if (alive) setError(caught instanceof Error ? caught.message : '视频已不可用') })
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

export function PublicVideoGallery({ pageSize = 12 }: { pageSize?: number }) {
  const [failedPosters, setFailedPosters] = useState<Set<string>>(new Set())
  const [items, setItems] = useState<PublicVideo[]>([])
  const [page, setPage] = useState(1)
  const [total, setTotal] = useState(0)
  const [selected, setSelected] = useState<PublicVideo | null>(null)
  const [error, setError] = useState('')
  const [reload, setReload] = useState(0)
  const [loading, setLoading] = useState(true)
  useEffect(() => {
    let alive = true; setLoading(true); setError('')
    void videoPublicationApi.list(page, pageSize).then((result) => { if (alive) { setItems(result.items); setTotal(result.total) } }).catch((caught) => { if (alive) { setItems([]); setError(caught instanceof Error ? caught.message : '视频加载失败') } }).finally(() => { if (alive) setLoading(false) })
    return () => { alive = false }
  }, [page, pageSize, reload])
  return <section aria-label="公开视频" style={{ marginBlock: 24 }}><h2>公开视频</h2>
    {loading ? <p role="status">正在载入视频…</p> : error ? <p role="alert">{error} <button type="button" onClick={() => setReload((value) => value + 1)}>重试</button></p> : !items.length ? <p>暂无公开视频</p> : <div className="u-grid u-grid--gallery">{items.map((video) => <button className="u-card" type="button" key={video.id} onClick={() => setSelected(video)}>{failedPosters.has(video.id) ? <span className="media-detail-message">封面暂不可用 · 点击播放视频</span> : <img src={publicVideoURL(video.poster_url)} alt={video.title} loading="lazy" onError={() => setFailedPosters((current) => new Set(current).add(video.id))} style={{ width: '100%', aspectRatio: '16/9', objectFit: 'cover' }} />}<span style={{ display: 'block', padding: 12, overflowWrap: 'anywhere' }}>{video.title} · 播放视频</span></button>)}</div>}
    {total > pageSize ? <nav aria-label="视频翻页"><button type="button" disabled={loading || page <= 1} onClick={() => setPage(page - 1)}>上一页</button><span> {page} / {Math.ceil(total / pageSize)} </span><button type="button" disabled={loading || page * pageSize >= total} onClick={() => setPage(page + 1)}>下一页</button></nav> : null}
    {selected ? <PublicVideoDetailDialog video={selected} onClose={() => setSelected(null)} /> : null}
  </section>
}
