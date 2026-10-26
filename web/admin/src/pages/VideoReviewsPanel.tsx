import { useEffect, useState } from 'react'
import { getDefaultBaseUrl, sharedApiClient } from '../../../shared/http-client'
import { AdminTabs, EmptyBlock, InlineFeedback, PageHeader } from '../components'
import { Pager } from '../ui/dataTable'

type VideoReview = { id: string; title: string; status: string; prompt?: string; review_reason?: string }
type ReviewPage = { items: VideoReview[]; total: number }
const labels: Record<string, string> = { pending_review: '待审核', approved: '已公开', rejected: '未通过', private: '未公开', all: '全部' }
export function VideoReviewsPanel({ accessToken, onFeedback }: { accessToken?: string; onFeedback: (title: string, detail?: string) => void }) {
  const [status, setStatus] = useState('pending_review')
  const [page, setPage] = useState(1)
  const [items, setItems] = useState<VideoReview[]>([])
  const [total, setTotal] = useState(0)
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const [reload, setReload] = useState(0)
  const [reasons, setReasons] = useState<Record<string, string>>({})
  useEffect(() => {
    let alive = true; setError(''); setBusy(true)
    void sharedApiClient.request<ReviewPage>('/api/ops/admin/v1/video-reviews', { query: { status, page, page_size: 10 } }).then((result) => { if (alive) { setItems(result.items); setTotal(result.total) } }).catch((caught) => { if (alive) setError(caught instanceof Error ? caught.message : '视频审核加载失败') }).finally(() => { if (alive) setBusy(false) })
    return () => { alive = false }
  }, [status, page, reload])
  const mediaURL = (id: string, purpose: string) => {
    const url = new URL(`/api/ops/admin/v1/video-reviews/${encodeURIComponent(id)}/${purpose}`, `${getDefaultBaseUrl() || window.location.origin}/`)
    if (accessToken) url.searchParams.set('access_token', accessToken)
    return url.toString()
  }
  const decide = async (item: VideoReview, action: 'approve' | 'reject' | 'unpublish') => {
    if (action === 'reject' && !reasons[item.id]?.trim()) { setError('请填写不通过原因'); return }
    setBusy(true); setError('')
    try {
      await sharedApiClient.request(`/api/ops/admin/v1/video-reviews/${encodeURIComponent(item.id)}:${action}`, { method: 'POST', body: { reason: reasons[item.id] ?? '' } })
      onFeedback('视频审核已更新', item.title); setReload((value) => value + 1)
    } catch (caught) { setError(caught instanceof Error ? caught.message : '视频审核失败'); setBusy(false) }
  }
  return <section className="a-page-stack">
    <PageHeader title="视频公开审核" description="审核实际视频、封面和公开提示词。通过后才可匿名播放，撤回后立即停止公开访问。" secondaryActions={<button className="a-btn" disabled={busy} onClick={() => setReload((value) => value + 1)}>刷新</button>} />
    <AdminTabs ariaLabel="视频审核状态" items={Object.entries(labels).map(([id, label]) => ({ id, label }))} value={status} onChange={(value) => { setStatus(value); setPage(1) }} />
    {error ? <InlineFeedback tone="danger" message={error} /> : null}
    {busy ? <p role="status">正在处理视频审核…</p> : null}
    {!busy && !items.length ? <EmptyBlock title="没有匹配的视频" detail="等待用户提交公开视频申请。" /> : null}
    {items.map((item) => <article className="a-panel" key={item.id} style={{ padding: 20 }}>
      <h3>{item.title} · {labels[item.status] ?? item.status}</h3>
      <video key={`${item.id}:${item.status}`} controls playsInline preload="none" src={mediaURL(item.id, 'video')} poster={mediaURL(item.id, 'poster')} style={{ width: '100%', maxHeight: 380, background: '#111' }} />
      {item.prompt ? <p style={{ whiteSpace: 'pre-wrap', overflowWrap: 'anywhere' }}>{item.prompt}</p> : null}
      {item.review_reason ? <p>审核原因：{item.review_reason}</p> : null}
      {item.status === 'pending_review' || item.status === 'approved' ? <><label>审核原因<textarea className="a-input" aria-label={`${item.title}审核原因`} value={reasons[item.id] ?? ''} onChange={(event) => setReasons((current) => ({ ...current, [item.id]: event.target.value }))} /></label>
        {item.status === 'pending_review' ? <><button className="a-btn a-btn--primary" disabled={busy} onClick={() => void decide(item, 'approve')}>通过</button><button className="a-btn" disabled={busy} onClick={() => void decide(item, 'reject')}>不通过</button></> : <button className="a-btn" disabled={busy} onClick={() => void decide(item, 'unpublish')}>下架</button>}
      </> : null}
    </article>)}
    <Pager page={page} pageSize={10} total={total} onChange={setPage} />
  </section>
}
