import { useEffect, useMemo, useState } from 'react'
import type { ReviewItem } from '../../../shared/api-types'
import { adminApi } from '../../../shared/admin-api'
import { cn } from '../../../shared/classnames'
import { Badge, ConfirmDrawer, EmptyBlock, ErrorBlock, LoadingBlock, PageHeader } from '../components'
import { adminButton, adminPage } from '../ui/classes'
import { adminDataGrid, adminGridCols } from '../ui/dataGrid'
import { reviewDefaultReason, reviewRowView, reviewStatusLabel, reviewStatusTabs } from './reviewRows'
import type { ReviewDecision } from './reviewRows'

type DrawerState = { item: ReviewItem; decision: ReviewDecision } | null
const reviewImageCellClass = 'grid grid-cols-[54px_minmax(0,1fr)] items-center gap-3'
const reviewImageClass = 'size-[54px] rounded-lg border border-[var(--line)] object-cover'

export function ReviewPage({ accessToken, onFeedback }: { accessToken?: string; onFeedback: (title: string, detail?: string) => void }) {
  const [rows, setRows] = useState<ReviewItem[]>([])
  const [filter, setFilter] = useState<ReviewItem['status'] | 'all'>('pending_review')
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
    setReason(reviewDefaultReason(decision))
  }

  const submitDecision = async () => {
    if (!drawer) return
    setBusy(true)
    try {
      const updated = await adminApi.decideReview(drawer.item.image_id ?? drawer.item.id, drawer.decision, reason)
      setRows((current) => current.map((item) => item.id === updated.id ? updated : item))
      onFeedback('审核决策已提交', `${updated.title}: ${reviewStatusLabel(updated.status)}`)
      setDrawer(null)
    } finally {
      setBusy(false)
    }
  }

  if (loading) return <LoadingBlock label="载入审核队列" />
  if (error) return <ErrorBlock message={error} onRetry={load} />

  return (
    <section className={adminPage.stack}>
      <PageHeader eyebrow="Reviews" title="审核队列" detail="公开申请、驳回与下架都要求填写原因，并写入审计轨迹。" />
      <section className="grid min-h-0 grid-cols-[minmax(0,1fr)_320px] overflow-hidden rounded-[var(--pg-radius-sm)] border border-[var(--line)] bg-white max-[1260px]:grid-cols-1">
        <section className={adminPage.mainLane}>
          <div className={adminPage.microTabs}>
            {reviewStatusTabs.map((tab) => <button key={tab} type="button" className={cn(adminPage.microTab, filter === tab && adminPage.microTabActive)} onClick={() => setFilter(tab)}>{tab === 'all' ? '全部' : reviewStatusLabel(tab)}</button>)}
          </div>

          {!visibleRows.length ? <EmptyBlock title="没有匹配的审核项" detail="切换筛选或等待用户提交公开申请。" /> : (
            <div className={cn(adminDataGrid.root, adminGridCols.review)}>
              <div className={cn(adminDataGrid.head, adminGridCols.review)}><span>图片</span><span>用户</span><span>类型</span><span>上下文</span><span>状态</span><span>动作</span></div>
              {visibleRows.map((rawRow) => {
                const row = reviewRowView(rawRow)
                return (
                <div key={row.raw.id} className={cn(adminDataGrid.row, adminGridCols.review)}>
                  <div className={reviewImageCellClass}><img className={reviewImageClass} src={adminApi.imageReviewUrl(row.imageID, accessToken)} alt="" /><div className={adminDataGrid.stackCell}><strong>{row.title}</strong><p className={adminDataGrid.detail}>{row.createdAtLabel}</p></div></div>
                  <span>{row.owner}</span>
                  <span>{row.taskTypeLabel}</span>
                  <span>{row.context}</span>
                  <Badge tone={row.statusTone}>{row.statusLabel}</Badge>
                  <div className={adminDataGrid.actions}>
                    {row.actions.map((action) => (
                      <button key={action.decision} type="button" className={action.tone === 'primary' ? cn(adminButton.base, adminButton.primary, adminButton.small) : cn(adminButton.base, adminButton.ghost, adminButton.small, 'text-[var(--red)]')} onClick={() => openDrawer(row.raw, action.decision)}>{action.label}</button>
                    ))}
                    {!row.actions.length ? <span className={adminPage.mutedAction}>{row.terminalActionLabel}</span> : null}
                  </div>
                </div>
                )
              })}
            </div>
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
