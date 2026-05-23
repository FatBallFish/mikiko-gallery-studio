import { useEffect, useMemo, useState } from 'react'
import type { ReviewItem } from '../../../shared/api-types'
import { adminApi } from '../../../shared/admin-api'
import { Badge, ConfirmDrawer, EmptyBlock, ErrorBlock, LoadingBlock, PageHeader } from '../components'

type ReviewDecision = 'approve' | 'reject' | 'unpublish'

type DrawerState = { item: ReviewItem; decision: ReviewDecision } | null

const statusLabel: Record<ReviewItem['status'], string> = {
  pending: '待审核',
  pending_review: '待审核',
  approved: '已通过',
  rejected: '已驳回',
  unpublished: '已下架',
}

export function ReviewPage({ onFeedback }: { onFeedback: (title: string, detail?: string) => void }) {
  const [rows, setRows] = useState<ReviewItem[]>([])
  const [filter, setFilter] = useState<ReviewItem['status'] | 'all'>('pending')
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [drawer, setDrawer] = useState<DrawerState>(null)
  const [reason, setReason] = useState('')
  const [busy, setBusy] = useState(false)

  const load = async () => {
    setLoading(true)
    setError(null)
    try {
      setRows(await adminApi.listReviews())
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : '审核队列载入失败')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    void load()
  }, [])

  const visibleRows = useMemo(() => filter === 'all' ? rows : rows.filter((row) => row.status === filter), [filter, rows])

  const openDrawer = (item: ReviewItem, decision: ReviewDecision) => {
    setDrawer({ item, decision })
    setReason(decision === 'approve' ? '内容质量稳定，准许公开展示。' : decision === 'reject' ? '内容不符合公开展示规范。' : '运营复核后下架公开展示。')
  }

  const submitDecision = async () => {
    if (!drawer) return
    setBusy(true)
    try {
      const updated = await adminApi.decideReview(drawer.item.image_id ?? drawer.item.id, drawer.decision, reason)
      setRows((current) => current.map((item) => item.id === updated.id ? updated : item))
      onFeedback('审核决策已提交', `${updated.title}: ${statusLabel[updated.status]}`)
      setDrawer(null)
    } finally {
      setBusy(false)
    }
  }

  if (loading) return <LoadingBlock label="载入审核队列" />
  if (error) return <ErrorBlock message={error} onRetry={load} />

  return (
    <section className="page-stack">
      <PageHeader eyebrow="Reviews" title="审核队列" detail="公开申请、驳回与下架都要求填写原因，并写入审计轨迹。" />
      <section className="pg-admin-card ops-surface full-main review-workspace">
        <section className="main-lane table-lane no-divider">
          <div className="micro-tabs as-buttons">
            {(['pending', 'approved', 'rejected', 'unpublished', 'all'] as const).map((tab) => <button key={tab} type="button" className={filter === tab ? 'active' : ''} onClick={() => setFilter(tab)}>{tab === 'all' ? '全部' : statusLabel[tab]}</button>)}
          </div>

          {!visibleRows.length ? <EmptyBlock title="没有匹配的审核项" detail="切换筛选或等待用户提交公开申请。" /> : (
            <>
              <div className="table-head review-grid"><span>图片</span><span>用户</span><span>类型</span><span>上下文</span><span>状态</span><span>动作</span></div>
              {visibleRows.map((row) => (
                <div key={row.id} className="table-row review-grid">
                  <div className="review-image-cell"><img src={row.image_url} alt="" /><div><strong>{row.title}</strong><p>{row.created_at}</p></div></div>
                  <span>{row.owner}</span>
                  <span>{row.task_type}</span>
                  <span>{row.reason}</span>
                  <Badge tone={row.status === 'approved' ? 'success' : row.status === 'pending' ? 'warning' : 'danger'}>{statusLabel[row.status]}</Badge>
                  <div className="row-actions buttons">
                    <button type="button" className="btn small" onClick={() => openDrawer(row, 'approve')} disabled={row.status === 'approved'}>通过</button>
                    <button type="button" className="ghost small" onClick={() => openDrawer(row, 'reject')} disabled={row.status === 'rejected'}>驳回</button>
                    <button type="button" className="ghost small danger-text" onClick={() => openDrawer(row, 'unpublish')} disabled={row.status === 'unpublished'}>下架</button>
                  </div>
                </div>
              ))}
            </>
          )}
        </section>
        {drawer ? (
          <ConfirmDrawer
            title={`${drawer.item.title} · ${drawer.decision === 'approve' ? '通过' : drawer.decision === 'reject' ? '驳回' : '下架'}`}
            detail="原因会显示在审核上下文并进入审计日志。"
            value={reason}
            decisionLabel="提交决策"
            tone={drawer.decision === 'approve' ? 'success' : 'danger'}
            busy={busy}
            onChange={setReason}
            onCancel={() => setDrawer(null)}
            onConfirm={() => void submitDecision()}
          />
        ) : null}
      </section>
    </section>
  )
}
