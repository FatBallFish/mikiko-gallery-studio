import { useEffect, useMemo, useState } from 'react'
import type { ReviewItem } from '../../../shared/api-types'
import { adminApi } from '../../../shared/admin-api'
import { cn } from '../../../shared/classnames'
import { Badge, ConfirmDrawer, EmptyBlock, ErrorBlock, LoadingBlock } from '../components'
import { adminButton, adminPage } from '../ui/classes'
import { reviewDefaultReason, reviewRowView, reviewStatusLabel } from './reviewRows'
import type { ReviewDecision } from './reviewRows'

type DrawerState = { item: ReviewItem; decision: ReviewDecision } | null
const primaryReviewTabs = ['pending_review', 'approved', 'rejected'] as const
const secondaryReviewTabs = ['unpublished', 'all'] as const
const allReviewTabs = [...primaryReviewTabs, ...secondaryReviewTabs] as const
const reviewClasses = {
  tabBar: 'flex flex-wrap items-center gap-4 rounded-2xl bg-white/5 p-1 w-fit',
  cardList: 'grid grid-cols-1 gap-4',
  card: 'group flex items-center gap-6 rounded-3xl border border-[var(--line)] bg-white/[0.02] p-6 transition-all hover:border-[var(--line-strong)] hover:bg-white/[0.04] max-[720px]:grid max-[720px]:grid-cols-1',
  imageWrap: 'relative size-24 shrink-0 overflow-hidden rounded-2xl border border-[var(--line)] bg-white/5',
  image: 'size-full object-cover transition-transform duration-300 group-hover:scale-105',
  content: 'min-w-0 flex-1',
  titleRow: 'mb-1 flex min-w-0 flex-wrap items-center gap-3',
  title: 'min-w-0 truncate text-lg font-bold text-[var(--text)]',
  meta: 'flex flex-wrap items-center gap-3 text-xs text-[var(--muted-strong)]',
  dot: 'size-1 rounded-full bg-white/10',
  actions: 'flex shrink-0 flex-wrap justify-end gap-3',
  actionPrimary: 'border-emerald-500 bg-emerald-500 px-6 py-2.5 text-white shadow-lg shadow-emerald-500/20 hover:scale-105',
  actionDanger: 'border-transparent bg-white/5 px-6 py-2.5 text-[var(--muted)] hover:bg-[var(--red)]/10 hover:text-[var(--red)]',
}

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
      <section className="grid min-h-0 gap-5">
        <section>
          <div className={reviewClasses.tabBar}>
            {allReviewTabs.map((tab) => (
              <button key={tab} type="button" className={cn(adminPage.microTab, filter === tab && adminPage.microTabActive)} onClick={() => setFilter(tab)}>
                {reviewTabLabel(tab)}{tab === 'pending_review' ? ` (${rows.filter((row) => row.status === 'pending_review' || row.status === 'pending').length})` : ''}
              </button>
            ))}
          </div>

          {!visibleRows.length ? <EmptyBlock title="没有匹配的审核项" detail="切换筛选或等待用户提交公开申请。" /> : (
            <div className={reviewClasses.cardList}>
              {visibleRows.map((rawRow) => {
                const row = reviewRowView(rawRow)
                return (
                <article key={row.raw.id} className={reviewClasses.card}>
                  <div className={reviewClasses.imageWrap}>
                    <img className={reviewClasses.image} src={adminApi.imageReviewUrl(row.imageID, accessToken)} alt={row.title} />
                    {row.statusTone === 'danger' ? <span className="absolute left-2 top-2 size-3 rounded-full bg-rose-500 shadow-[0_0_10px_rgba(244,63,94,0.5)]" /> : null}
                  </div>
                  <div className={reviewClasses.content}>
                    <div className={reviewClasses.titleRow}>
                      <h3 className={reviewClasses.title}>{row.title}</h3>
                      <Badge tone="primary">{row.taskTypeLabel}</Badge>
                      {filter !== 'pending_review' ? <Badge tone={row.statusTone}>{row.statusLabel}</Badge> : null}
                    </div>
                    <div className={reviewClasses.meta}>
                      <span>用户: <strong className="text-[var(--soft)]">{row.owner}</strong></span>
                      <span className={reviewClasses.dot} />
                      <span>位置: <strong className="text-[var(--soft)]">{row.context}</strong></span>
                      <span className={reviewClasses.dot} />
                      <span>提交于: {row.createdAtLabel}</span>
                    </div>
                  </div>
                  <div className={reviewClasses.actions}>
                    {row.actions.map((action) => (
                      <button key={action.decision} type="button" className={cn(adminButton.base, action.tone === 'primary' ? reviewClasses.actionPrimary : reviewClasses.actionDanger)} onClick={() => openDrawer(row.raw, action.decision)}>{action.label}</button>
                    ))}
                    {!row.actions.length ? <span className={adminPage.mutedAction}>{row.terminalActionLabel}</span> : null}
                  </div>
                </article>
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

function reviewTabLabel(tab: typeof allReviewTabs[number]) {
  if (tab === 'pending_review') return '待处理'
  if (tab === 'all') return '全部'
  return reviewStatusLabel(tab)
}
