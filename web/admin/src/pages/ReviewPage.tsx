import { useEffect, useMemo, useRef, useState, type FormEvent, type KeyboardEvent } from 'react'
import type { ReviewItem } from '../../../shared/api-types'
import { adminApi } from '../../../shared/admin-api'
import { cn } from '../../../shared/classnames'
import { AdminTabs, Badge, ConfirmDrawer, EmptyBlock, ErrorBlock, InlineFeedback, LoadingBlock, PageHeader } from '../components'
import { adminButton, adminPage } from '../ui/classes'
import { useAdminPreviewMotion } from '../ui/adminMotion'
import { FilterToolbar, ListPage, Pager } from '../ui/dataTable'
import { FilterIcon, XIcon } from '../ui/listIcons'
import { adminTaskTypeOptions } from './adminTaskTypes'
import { reviewDefaultReason, reviewListQuery, reviewRowView, reviewStatusLabel } from './reviewRows'
import type { ReviewDecision, ReviewListFilters } from './reviewRows'

type DrawerState = { item: ReviewItem; decision: ReviewDecision } | null
const primaryReviewTabs = ['pending_review', 'approved', 'rejected'] as const
const secondaryReviewTabs = ['unpublished', 'all'] as const
const allReviewTabs = [...primaryReviewTabs, ...secondaryReviewTabs] as const
const reasonPresets = ['违规内容', '低质量图片', '版权风险'] as const
const initialFilters: ReviewListFilters = {
  user: '', prompt: '', model: '', taskType: '', baseResolution: '', requestedSize: '', width: '', height: '', aspectRatio: '',
  createdFrom: '', createdTo: '', publishedFrom: '', publishedTo: '',
}
const reviewClasses = {
  workbench: 'grid min-h-[560px] grid-cols-[minmax(240px,.85fr)_minmax(320px,1.35fr)_minmax(240px,.8fr)] overflow-hidden rounded-lg bg-[var(--surface-solid)] max-[1100px]:grid-cols-1',
  queue: 'min-h-0 overflow-y-auto border-r border-[var(--border)] p-3 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-[-2px] focus-visible:outline-[var(--focus-ring)] max-[1100px]:border-b max-[1100px]:border-r-0',
  queueItem: 'grid w-full gap-2 rounded-lg border border-transparent p-3 text-left transition-colors duration-[var(--admin-motion-fast)] hover:bg-[var(--elevated)] focus-visible:outline focus-visible:outline-2 focus-visible:outline-[var(--focus-ring)]',
  queueItemActive: 'border-[var(--accent)]/30 bg-[var(--accent)]/10',
  preview: 'grid min-h-0 grid-rows-[minmax(0,1fr)_auto]',
  previewImageWrap: 'grid min-h-[320px] place-items-center bg-[var(--canvas)] p-4',
  previewImage: 'max-h-[56vh] max-w-full rounded-lg object-contain',
  previewMeta: 'grid gap-2 border-t border-[var(--border)] p-4',
  actionPanel: 'grid min-h-0 content-start gap-3 border-l border-[var(--border)] p-4 max-[1100px]:border-l-0 max-[1100px]:border-t',
  decisionPanel: 'grid min-h-0 grid-rows-[auto_minmax(0,1fr)] [&_aside]:min-h-0 max-[1100px]:[&_aside]:border-l-0 max-[1100px]:[&_aside]:border-t',
  reasonTemplates: 'flex flex-wrap gap-2',
  reasonPreset: 'min-h-8 rounded-md border border-[var(--border)] bg-transparent px-2.5 text-xs font-semibold text-[var(--muted)] transition-colors hover:border-[var(--accent)] hover:text-[var(--text)] focus-visible:outline focus-visible:outline-2 focus-visible:outline-[var(--focus-ring)]',
  reasonInput: 'min-h-24 resize-y rounded-lg border border-[var(--border)] bg-[var(--canvas)] p-3 text-sm text-[var(--text)] focus:border-[var(--accent)] focus:outline-none',
}

export function ReviewPage({ accessToken, onFeedback }: { accessToken?: string; onFeedback: (title: string, detail?: string) => void }) {
  const [rows, setRows] = useState<ReviewItem[]>([])
  const [filter, setFilter] = useState<ReviewItem['status'] | 'all'>('pending_review')
  const [filters, setFilters] = useState<ReviewListFilters>(initialFilters)
  const [appliedFilters, setAppliedFilters] = useState<ReviewListFilters>(initialFilters)
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(20)
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(true)
  const [refreshing, setRefreshing] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [drawer, setDrawer] = useState<DrawerState>(null)
  const [reason, setReason] = useState('')
  const [mutationError, setMutationError] = useState('')
  const [busy, setBusy] = useState(false)
  const [selectedId, setSelectedId] = useState<string>('')
  const requestGenerationRef = useRef(0)

  const load = async () => {
    const requestGeneration = ++requestGenerationRef.current
    const initialLoad = !rows.length
    if (initialLoad) setLoading(true)
    else setRefreshing(true)
    setError(null)
    try {
      const result = await adminApi.listReviews(reviewListQuery(appliedFilters, filter, page, pageSize))
      if (requestGeneration !== requestGenerationRef.current) return
      setRows(result.items)
      setTotal(result.total)
    } catch (caught) {
      if (requestGeneration !== requestGenerationRef.current) return
      setError(caught instanceof Error ? caught.message : '审核队列载入失败')
    } finally {
      if (requestGeneration !== requestGenerationRef.current) return
      setLoading(false)
      setRefreshing(false)
    }
  }

  useEffect(() => { void load() }, [appliedFilters, filter, page, pageSize])

  const visibleRows = rows
  const selectedItem = useMemo(() => visibleRows.find((row) => String(row.id) === selectedId) ?? visibleRows[0] ?? null, [selectedId, visibleRows])

  useEffect(() => {
    if (selectedItem) setSelectedId(String(selectedItem.id))
  }, [selectedItem?.id])

  const selectItem = (item: ReviewItem) => {
    setSelectedId(String(item.id))
    setDrawer(null)
    setMutationError('')
  }

  const openDrawer = (item: ReviewItem, decision: ReviewDecision, explanation = '') => {
    if (refreshing) return
    requestGenerationRef.current += 1
    setRefreshing(false)
    setDrawer({ item, decision })
    setReason(explanation.trim() || reviewDefaultReason(decision))
    setMutationError('')
  }

  const submitDecision = async () => {
    if (!drawer) return
    if (drawer.decision === 'unpublish' && !reason.trim()) {
      setMutationError('下架原因不能为空')
      return
    }
    requestGenerationRef.current += 1
    setRefreshing(false)
    setBusy(true)
    setMutationError('')
    try {
      const updated = await adminApi.decideReview(drawer.item.image_id ?? drawer.item.id, drawer.decision, reason)
      setRows((current) => filter === 'all' ? current.map((item) => item.id === updated.id ? updated : item) : current.filter((item) => item.id !== updated.id))
      if (filter !== 'all') setTotal((current) => Math.max(0, current - 1))
      onFeedback('审核决策已提交', `${updated.title}: ${reviewStatusLabel(updated.status)}`)
      setDrawer(null)
    } catch (caught) {
      setMutationError(caught instanceof Error ? caught.message : '审核决策提交失败')
    } finally {
      setBusy(false)
    }
  }

  const submitFilters = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    setPage(1)
    setAppliedFilters(filters)
  }

  const resetFilters = () => {
    setFilters(initialFilters)
    setAppliedFilters(initialFilters)
    setPage(1)
  }

  if (loading && !rows.length) return <LoadingBlock label="载入审核队列" />
  if (error && !rows.length) return <ErrorBlock message={error} onRetry={load} />

  return (
    <section className={adminPage.stack}>
      <PageHeader
        title="公开图片管理"
        description="集中处理公开申请、已公开图片、已下架和已驳回内容。"
        secondaryActions={<button className={cn(adminButton.base, adminButton.ghost)} type="button" disabled={refreshing || Boolean(drawer) || busy} onClick={() => void load()}>{refreshing ? '刷新中...' : '刷新队列'}</button>}
      />
      {refreshing ? <InlineFeedback tone="neutral" message="正在刷新审核队列，当前预览与选择会保留。" /> : null}
      {error && rows.length ? <InlineFeedback tone="danger" message={error} /> : null}
      <form onSubmit={submitFilters}>
        <FilterToolbar
          fields={[
            { key: 'user', label: '用户', primary: true, control: <input value={filters.user} onChange={(event) => setFilters({ ...filters, user: event.target.value })} placeholder="用户 ID、邮箱或名称" /> },
            { key: 'prompt', label: '提示词', primary: true, control: <input value={filters.prompt} onChange={(event) => setFilters({ ...filters, prompt: event.target.value })} placeholder="提示词关键词" /> },
            { key: 'model', label: '模型', primary: true, control: <input value={filters.model} onChange={(event) => setFilters({ ...filters, model: event.target.value })} placeholder="抽象模型或路由模型" /> },
            { key: 'taskType', label: '任务类型', control: <select value={filters.taskType} onChange={(event) => setFilters({ ...filters, taskType: event.target.value })}><option value="">全部类型</option>{adminTaskTypeOptions.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}</select> },
            { key: 'baseResolution', label: '基础分辨率', control: <input value={filters.baseResolution} onChange={(event) => setFilters({ ...filters, baseResolution: event.target.value })} placeholder="例如 2k" /> },
            { key: 'requestedSize', label: '请求尺寸', control: <input value={filters.requestedSize} onChange={(event) => setFilters({ ...filters, requestedSize: event.target.value })} placeholder="例如 1536x1024" /> },
            { key: 'width', label: '实际宽度', control: <input type="number" min="1" value={filters.width} onChange={(event) => setFilters({ ...filters, width: event.target.value })} /> },
            { key: 'height', label: '实际高度', control: <input type="number" min="1" value={filters.height} onChange={(event) => setFilters({ ...filters, height: event.target.value })} /> },
            { key: 'aspectRatio', label: '宽高比', control: <input value={filters.aspectRatio} onChange={(event) => setFilters({ ...filters, aspectRatio: event.target.value })} placeholder="例如 3:2" /> },
            { key: 'createdFrom', label: '创建时间从', control: <input type="date" value={filters.createdFrom} onChange={(event) => setFilters({ ...filters, createdFrom: event.target.value })} /> },
            { key: 'createdTo', label: '创建时间至', control: <input type="date" value={filters.createdTo} onChange={(event) => setFilters({ ...filters, createdTo: event.target.value })} /> },
            { key: 'publishedFrom', label: '公开时间从', control: <input type="date" value={filters.publishedFrom} onChange={(event) => setFilters({ ...filters, publishedFrom: event.target.value })} /> },
            { key: 'publishedTo', label: '公开时间至', control: <input type="date" value={filters.publishedTo} onChange={(event) => setFilters({ ...filters, publishedTo: event.target.value })} /> },
          ]}
          actions={<><button className={cn(adminButton.base, adminButton.primary, adminButton.small)} type="submit"><FilterIcon className="size-4" /><span>筛选</span></button><button className={cn(adminButton.base, adminButton.ghost, adminButton.small)} type="button" onClick={resetFilters}><XIcon className="size-4" /><span>清空</span></button></>}
          resultSummary={`共 ${total} 张图片 · 当前显示 ${rows.length} 张`}
        />
      </form>
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
              onChange={(value) => { setFilter(value); setPage(1) }}
            />
          )}
          pagination={<Pager page={page} pageSize={pageSize} total={total} onChange={setPage} onPageSizeChange={(size) => { setPageSize(size); setPage(1) }} />}
        >
          {!visibleRows.length ? <EmptyBlock title="没有匹配的审核项" detail="切换筛选或等待用户提交公开申请。" /> : (
            <ReviewWorkbench
              rows={visibleRows}
              selected={selectedItem}
              accessToken={accessToken}
              drawer={drawer}
              reason={reason}
              busy={busy}
              mutationError={mutationError}
              interactionLocked={refreshing}
              onSelect={selectItem}
              onDecision={openDrawer}
              onReasonChange={setReason}
              onCancelDecision={() => { setDrawer(null); setMutationError('') }}
              onConfirmDecision={() => void submitDecision()}
            />
          )}
        </ListPage>
      </section>
    </section>
  )
}

function ReviewWorkbench({
  rows,
  selected,
  accessToken,
  drawer,
  reason,
  busy,
  mutationError,
  interactionLocked,
  onSelect,
  onDecision,
  onReasonChange,
  onCancelDecision,
  onConfirmDecision,
}: {
  rows: ReviewItem[]
  selected: ReviewItem | null
  accessToken?: string
  drawer: DrawerState
  reason: string
  busy: boolean
  mutationError: string
  interactionLocked: boolean
  onSelect: (item: ReviewItem) => void
  onDecision: (item: ReviewItem, decision: ReviewDecision, explanation?: string) => void
  onReasonChange: (value: string) => void
  onCancelDecision: () => void
  onConfirmDecision: () => void
}) {
  const selectedRow = selected ? reviewRowView(selected) : null
  const previewRef = useRef<HTMLElement | null>(null)
  const queueRef = useRef<HTMLElement | null>(null)
  const decisionPanelRef = useRef<HTMLElement | null>(null)
  const decisionTriggerRef = useRef<HTMLElement | null>(null)
  const decisionWasOpenRef = useRef(false)
  const [draftReason, setDraftReason] = useState('')
  useAdminPreviewMotion(previewRef, String(selected?.id ?? 'empty'))

  useEffect(() => {
    setDraftReason('')
  }, [selected?.id])

  useEffect(() => {
    if (drawer) {
      decisionWasOpenRef.current = true
      window.requestAnimationFrame(() => decisionPanelRef.current?.querySelector<HTMLElement>('textarea, button')?.focus())
      return
    }
    if (!decisionWasOpenRef.current) return
    decisionWasOpenRef.current = false
    if (decisionTriggerRef.current?.isConnected) decisionTriggerRef.current?.focus()
    else queueRef.current?.querySelector<HTMLButtonElement>('[aria-selected="true"]')?.focus()
  }, [drawer])

  const handleQueueKeyDown = (event: KeyboardEvent<HTMLElement>) => {
    if (event.key !== 'ArrowDown' && event.key !== 'ArrowUp') return
    event.preventDefault()
    const currentIndex = Math.max(0, rows.findIndex((item) => String(item.id) === String(selected?.id)))
    const direction = event.key === 'ArrowDown' ? 1 : -1
    const nextIndex = (currentIndex + direction + rows.length) % rows.length
    const nextItem = rows[nextIndex]
    if (!nextItem) return
    onSelect(nextItem)
    window.requestAnimationFrame(() => {
      queueRef.current?.querySelectorAll<HTMLButtonElement>('[data-review-queue-item]')[nextIndex]?.focus()
    })
  }

  return (
    <section className={reviewClasses.workbench}>
      <aside ref={queueRef} className={reviewClasses.queue} data-review-region="queue" aria-label="审核队列" role="listbox" onKeyDown={handleQueueKeyDown}>
        <div className="mb-3 text-xs font-bold text-[var(--muted)]">{rows.length} 个审核项</div>
        <div className="grid gap-1">
          {rows.map((item) => {
            const row = reviewRowView(item)
            const active = selected?.id === item.id
            return (
              <button key={item.id} id={`review-queue-item-${item.id}`} data-review-queue-item type="button" role="option" aria-selected={active} tabIndex={active ? 0 : -1} className={cn(reviewClasses.queueItem, active && reviewClasses.queueItemActive)} onClick={() => onSelect(item)}>
                <strong className="truncate">{row.title}</strong>
                <span className="text-xs text-[var(--muted)]">{row.owner} · {row.createdAtLabel}</span>
                <span><Badge tone={row.statusTone}>{row.statusLabel}</Badge></span>
              </button>
            )
          })}
        </div>
      </aside>
      <section ref={previewRef} className={reviewClasses.preview} data-review-region="preview" aria-label="审核预览">
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
      {drawer ? (
        <section ref={decisionPanelRef} className={reviewClasses.decisionPanel} data-review-region="action" aria-label="审核决策确认">
          {mutationError ? <InlineFeedback tone="danger" message={mutationError} /> : null}
          <ConfirmDrawer
            title={`${drawer.item.title} · ${drawer.decision === 'approve' ? '通过' : drawer.decision === 'reject' ? '驳回' : '下架'}`}
            detail="原因会显示在审核上下文并进入审计日志。"
            value={reason}
            decisionLabel="提交决策"
            tone={drawer.decision === 'approve' ? 'success' : 'danger'}
            busy={busy}
            onChange={onReasonChange}
            onCancel={onCancelDecision}
            onConfirm={onConfirmDecision}
          />
        </section>
      ) : (
        <aside className={reviewClasses.actionPanel} data-review-region="action" aria-label="审核动作">
          {selectedRow ? (
            <>
              <strong>审核动作</strong>
              <p className="text-sm text-[var(--muted)]">选择驳回原因或补充说明，结果会进入审计日志。</p>
              {selectedRow.actions.some((action) => action.decision === 'reject') ? (
                <>
                  <div className={reviewClasses.reasonTemplates} aria-label="驳回原因预设">
                    {reasonPresets.map((preset) => <button key={preset} type="button" className={reviewClasses.reasonPreset} aria-pressed={draftReason === preset} onClick={() => setDraftReason(preset)}>{preset}</button>)}
                  </div>
                  <label className="grid gap-1.5 text-xs font-semibold text-[var(--muted)]">
                    <span>补充说明（可选）</span>
                    <textarea className={reviewClasses.reasonInput} value={draftReason} onChange={(event) => setDraftReason(event.target.value)} rows={4} placeholder="输入更具体的驳回说明" />
                  </label>
                </>
              ) : null}
              <div className="grid gap-2">
                {selectedRow.actions.map((action) => (
                  <button key={action.decision} type="button" disabled={interactionLocked} className={cn(adminButton.base, action.tone === 'primary' ? adminButton.primary : adminButton.danger)} onClick={(event) => { decisionTriggerRef.current = event.currentTarget; onDecision(selectedRow.raw, action.decision, action.decision === 'reject' ? draftReason : '') }}>{action.label}</button>
                ))}
                {!selectedRow.actions.length ? <span className={adminPage.mutedAction}>{selectedRow.terminalActionLabel}</span> : null}
              </div>
            </>
          ) : null}
        </aside>
      )}
    </section>
  )
}

function reviewTabLabel(tab: typeof allReviewTabs[number]) {
  if (tab === 'pending_review') return '待处理'
  if (tab === 'approved') return '已公开'
  if (tab === 'all') return '全部'
  return reviewStatusLabel(tab)
}
