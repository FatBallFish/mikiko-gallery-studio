import { useEffect, useMemo, useState } from 'react'
import type { ReviewItem } from '../../../shared/api-types'
import { adminApi } from '../../../shared/admin-api'
import { cn } from '../../../shared/classnames'
import { AdminTabs, Badge, ConfirmDrawer, EmptyBlock, ErrorBlock, LoadingBlock, PageHeader } from '../components'
import { adminButton, adminPage } from '../ui/classes'
import { ListPage } from '../ui/dataTable'
import { reviewDefaultReason, reviewRowView, reviewStatusLabel } from './reviewRows'
import type { ReviewDecision } from './reviewRows'

type DrawerState = { item: ReviewItem; decision: ReviewDecision } | null
const primaryReviewTabs = ['pending_review', 'approved', 'rejected'] as const
const secondaryReviewTabs = ['unpublished', 'all'] as const
const allReviewTabs = [...primaryReviewTabs, ...secondaryReviewTabs] as const
const reviewClasses = {
  cardList: 'grid grid-cols-1 gap-4',
  card: 'group flex items-center gap-6 rounded-lg border border-[var(--border)] bg-[var(--surface)] p-6 transition-all hover:border-[var(--border-strong)] hover:bg-[var(--elevated)] max-[720px]:grid max-[720px]:grid-cols-1',
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
  workbench: 'grid min-h-[560px] grid-cols-[minmax(240px,.85fr)_minmax(320px,1.35fr)_minmax(240px,.8fr)] overflow-hidden rounded-lg bg-[var(--surface-solid)] max-[1100px]:grid-cols-1',
  queue: 'min-h-0 overflow-y-auto p-3 max-[1100px]:border-b',
  queueItem: 'grid w-full gap-2 rounded-lg border border-transparent p-3 text-left hover:bg-[var(--elevated)]',
  queueItemActive: 'border-[var(--accent)]/30 bg-[var(--accent)]/10',
  preview: 'grid min-h-0 grid-rows-[minmax(0,1fr)_auto]',
  previewImageWrap: 'grid min-h-[320px] place-items-center bg-[var(--canvas)] p-4',
  previewImage: 'max-h-[56vh] max-w-full rounded-lg object-contain',
  previewMeta: 'grid gap-2 border-t border-[var(--border)] p-4',
  actionPanel: 'grid content-start gap-3 p-4',
  reasonTemplates: 'flex flex-wrap gap-2',
}

export function ReviewPage({ accessToken, onFeedback }: { accessToken?: string; onFeedback: (title: string, detail?: string) => void }) {
  const [rows, setRows] = useState<ReviewItem[]>([])
  const [filter, setFilter] = useState<ReviewItem['status'] | 'all'>('pending_review')
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [drawer, setDrawer] = useState<DrawerState>(null)
  const [reason, setReason] = useState('')
  const [busy, setBusy] = useState(false)
  const [selectedId, setSelectedId] = useState<string>('')

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
  const selectedItem = useMemo(() => visibleRows.find((row) => String(row.id) === selectedId) ?? visibleRows[0] ?? null, [selectedId, visibleRows])

  useEffect(() => {
    if (selectedItem) setSelectedId(String(selectedItem.id))
  }, [selectedItem?.id])

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
      <PageHeader title="审核队列" description="队列、图片预览和审核动作并排处理，减少高频审核的来回跳转。" />
      <section className="grid min-h-0 gap-5">
        <ListPage
          filters={(
            <AdminTabs
              ariaLabel="审核状态筛选"
              items={allReviewTabs.map((tab) => ({
                id: tab,
                label: reviewTabLabel(tab),
                badge: tab === 'pending_review' ? <span>{rows.filter((row) => row.status === 'pending_review' || row.status === 'pending').length}</span> : undefined,
              }))}
              value={filter}
              onChange={setFilter}
            />
          )}
        >
          {!visibleRows.length ? <EmptyBlock title="没有匹配的审核项" detail="切换筛选或等待用户提交公开申请。" /> : (
            <ReviewWorkbench
              rows={visibleRows}
              selected={selectedItem}
              accessToken={accessToken}
              onSelect={(item) => setSelectedId(String(item.id))}
              onDecision={openDrawer}
            />
          )}
        </ListPage>
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

function ReviewWorkbench({
  rows,
  selected,
  accessToken,
  onSelect,
  onDecision,
}: {
  rows: ReviewItem[]
  selected: ReviewItem | null
  accessToken?: string
  onSelect: (item: ReviewItem) => void
  onDecision: (item: ReviewItem, decision: ReviewDecision) => void
}) {
  const selectedRow = selected ? reviewRowView(selected) : null
  return (
    <section className={reviewClasses.workbench}>
      <aside className={reviewClasses.queue} aria-label="审核队列">
        <div className="mb-3 text-xs font-bold text-[var(--muted)]">{rows.length} 个审核项</div>
        <div className="grid gap-1">
          {rows.map((item) => {
            const row = reviewRowView(item)
            const active = selected?.id === item.id
            return (
              <button key={item.id} type="button" className={cn(reviewClasses.queueItem, active && reviewClasses.queueItemActive)} onClick={() => onSelect(item)}>
                <strong className="truncate">{row.title}</strong>
                <span className="text-xs text-[var(--muted)]">{row.owner} · {row.createdAtLabel}</span>
                <span><Badge tone={row.statusTone}>{row.statusLabel}</Badge></span>
              </button>
            )
          })}
        </div>
      </aside>
      <section className={reviewClasses.preview} aria-label="审核预览">
        {selectedRow ? (
          <>
            <div className={reviewClasses.previewImageWrap}>
              <img className={reviewClasses.previewImage} src={adminApi.imageReviewUrl(selectedRow.imageID, accessToken)} alt={selectedRow.title} />
            </div>
            <div className={reviewClasses.previewMeta}>
              <div className="flex flex-wrap items-center gap-2">
                <strong>{selectedRow.title}</strong>
                <Badge tone="primary">{selectedRow.taskTypeLabel}</Badge>
              </div>
              <p>用户：{selectedRow.owner} · 位置：{selectedRow.context}</p>
            </div>
          </>
        ) : <EmptyBlock title="请选择审核项" detail="从左侧队列选择图片后预览和处理。" />}
      </section>
      <aside className={reviewClasses.actionPanel} aria-label="审核动作">
        {selectedRow ? (
          <>
            <strong>审核动作</strong>
            <p className="text-sm text-[var(--muted)]">拒绝原因可在提交前调整，结果会进入审计日志。</p>
            <div className={reviewClasses.reasonTemplates}>
              {['违规内容', '低质量图片', '版权风险'].map((reason) => <span key={reason} className="rounded-lg border border-[var(--border)] px-2 py-1 text-xs text-[var(--muted)]">{reason}</span>)}
            </div>
            <div className="grid gap-2">
              {selectedRow.actions.map((action) => (
                <button key={action.decision} type="button" className={cn(adminButton.base, action.tone === 'primary' ? adminButton.primary : adminButton.danger)} onClick={() => onDecision(selectedRow.raw, action.decision)}>{action.label}</button>
              ))}
              {!selectedRow.actions.length ? <span className={adminPage.mutedAction}>{selectedRow.terminalActionLabel}</span> : null}
            </div>
          </>
        ) : null}
      </aside>
    </section>
  )
}

function reviewTabLabel(tab: typeof allReviewTabs[number]) {
  if (tab === 'pending_review') return '待处理'
  if (tab === 'all') return '全部'
  return reviewStatusLabel(tab)
}
