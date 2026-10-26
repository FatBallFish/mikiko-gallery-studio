import { useEffect, useMemo, useRef, useState, type FormEvent, type KeyboardEvent } from 'react'
import type { ReviewItem } from '../../../shared/api-types'
import { adminApi } from '../../../shared/admin-api'
import { cn } from '../../../shared/classnames'
import { AdminTabs, Badge, EmptyBlock, ErrorBlock, InlineFeedback, LoadingBlock, PageHeader, RefreshIconButton } from '../components'
import { useAdminPreviewMotion } from '../ui/adminMotion'
import { FilterToolbar, Pager } from '../ui/dataTable'
import { FilterIcon, XIcon } from '../ui/listIcons'
import { RefreshableMediaImage } from '../ui/mediaRefresh'
import { adminTaskTypeOptions } from './adminTaskTypes'
import { reviewDefaultReason, reviewListQuery, reviewRowView, reviewStatusLabel } from './reviewRows'
import type { ReviewDecision, ReviewListFilters } from './reviewRows'
import { VideoReviewsPanel } from './VideoReviewsPanel'
import './list-redesign.css'

type DrawerState = { item: ReviewItem; decision: ReviewDecision } | null
const primaryReviewTabs = ['pending_review', 'approved', 'rejected'] as const
const secondaryReviewTabs = ['unpublished', 'all'] as const
const allReviewTabs = [...primaryReviewTabs, ...secondaryReviewTabs] as const
const reasonPresets = ['违规内容', '低质量图片', '版权风险'] as const
const initialFilters: ReviewListFilters = {
  user: '', prompt: '', model: '', taskType: '', baseResolution: '', requestedSize: '', width: '', height: '', aspectRatio: '',
  createdFrom: '', createdTo: '', publishedFrom: '', publishedTo: '',
}

export function ReviewPage({ accessToken, onFeedback }: { accessToken?: string; onFeedback: (title: string, detail?: string) => void }) {
  const [mediaKind, setMediaKind] = useState('image')
  return <section className="a-page-stack">
    <AdminTabs ariaLabel="审核媒体类型" items={[{ id: 'image', label: '图片' }, { id: 'video', label: '视频' }]} value={mediaKind} onChange={setMediaKind} />
    {mediaKind === 'video' ? <VideoReviewsPanel accessToken={accessToken} onFeedback={onFeedback} /> : <ImageReviewPage accessToken={accessToken} onFeedback={onFeedback} />}
  </section>
}

function ImageReviewPage({ accessToken, onFeedback }: { accessToken?: string; onFeedback: (title: string, detail?: string) => void }) {
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
  const [reloadKey, setReloadKey] = useState(0)

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

  useEffect(() => { void load() }, [appliedFilters, filter, page, pageSize, reloadKey])

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
    if (reason.trim().length < 3) {
      setMutationError('审核原因至少需要 3 个字符。')
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
    setReloadKey((value) => value + 1)
  }

  const resetFilters = () => {
    setFilters(initialFilters)
    setAppliedFilters(initialFilters)
    setPage(1)
  }

  if (loading && !rows.length) return <LoadingBlock label="载入审核队列" />
  if (error && !rows.length) return <ErrorBlock message={error} onRetry={load} />

  return (
    <section className="a-page-stack">
      <PageHeader
        eyebrow="Public Review"
        title="审核队列"
        description="左侧选队列、中间看大图、右侧做决策。审核原因写入审计日志，键盘可用上下方向键切换。"
        secondaryActions={<RefreshIconButton label="刷新审核队列" refreshing={refreshing} disabled={Boolean(drawer) || busy} onClick={() => void load()} />}
      />
      {refreshing ? <InlineFeedback tone="neutral" message="正在刷新审核队列，当前预览与选择会保留。" /> : null}
      {error && rows.length ? <InlineFeedback tone="danger" message={error} /> : null}
      <form onSubmit={submitFilters}>
        <FilterToolbar
          fields={[
            { key: 'user', label: '用户', primary: true, control: <input className="a-input" value={filters.user} onChange={(event) => setFilters({ ...filters, user: event.target.value })} placeholder="用户 ID、邮箱或名称" /> },
            { key: 'prompt', label: '提示词', primary: true, control: <input className="a-input" value={filters.prompt} onChange={(event) => setFilters({ ...filters, prompt: event.target.value })} placeholder="提示词关键词" /> },
            { key: 'model', label: '模型', primary: true, control: <input className="a-input" value={filters.model} onChange={(event) => setFilters({ ...filters, model: event.target.value })} placeholder="抽象模型或路由模型" /> },
            { key: 'taskType', label: '任务类型', control: <select className="a-select" value={filters.taskType} onChange={(event) => setFilters({ ...filters, taskType: event.target.value })}><option value="">全部类型</option>{adminTaskTypeOptions.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}</select> },
            { key: 'baseResolution', label: '基础分辨率', control: <input className="a-input" value={filters.baseResolution} onChange={(event) => setFilters({ ...filters, baseResolution: event.target.value })} placeholder="例如 2k" /> },
            { key: 'requestedSize', label: '请求尺寸', control: <input className="a-input" value={filters.requestedSize} onChange={(event) => setFilters({ ...filters, requestedSize: event.target.value })} placeholder="例如 1536x1024" /> },
            { key: 'width', label: '实际宽度', control: <input className="a-input" type="number" min="1" value={filters.width} onChange={(event) => setFilters({ ...filters, width: event.target.value })} /> },
            { key: 'height', label: '实际高度', control: <input className="a-input" type="number" min="1" value={filters.height} onChange={(event) => setFilters({ ...filters, height: event.target.value })} /> },
            { key: 'aspectRatio', label: '宽高比', control: <input className="a-input" value={filters.aspectRatio} onChange={(event) => setFilters({ ...filters, aspectRatio: event.target.value })} placeholder="例如 3:2" /> },
            { key: 'createdFrom', label: '创建时间从', control: <input className="a-input" type="date" value={filters.createdFrom} onChange={(event) => setFilters({ ...filters, createdFrom: event.target.value })} /> },
            { key: 'createdTo', label: '创建时间至', control: <input className="a-input" type="date" value={filters.createdTo} onChange={(event) => setFilters({ ...filters, createdTo: event.target.value })} /> },
            { key: 'publishedFrom', label: '公开时间从', control: <input className="a-input" type="date" value={filters.publishedFrom} onChange={(event) => setFilters({ ...filters, publishedFrom: event.target.value })} /> },
            { key: 'publishedTo', label: '公开时间至', control: <input className="a-input" type="date" value={filters.publishedTo} onChange={(event) => setFilters({ ...filters, publishedTo: event.target.value })} /> },
          ]}
          actions={<><button className="a-btn a-btn--sm a-btn--primary" type="submit"><FilterIcon className="size-4" /><span>筛选</span></button><button className="a-btn a-btn--sm a-btn--quiet" type="button" onClick={resetFilters}><XIcon className="size-4" /><span>清空</span></button></>}
          resultSummary={`共 ${total} 张图片 · 当前显示 ${rows.length} 张`}
        />
      </form>
      <section className="a-panel a-panel--flush">
        <div style={{ padding: '0 16px' }}>
          <AdminTabs
            ariaLabel="审核状态筛选"
            className="lr-drawer-tabs"
            items={allReviewTabs.map((tab) => ({
              id: tab,
              label: reviewTabLabel(tab),
              badge: tab === 'pending_review' ? <span>{rows.filter((row) => row.status === 'pending_review' || row.status === 'pending').length}</span> : undefined,
            }))}
            value={filter}
            onChange={(value) => { setFilter(value); setPage(1) }}
          />
        </div>
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
            onMediaRefresh={() => void load()}
            onSelect={selectItem}
            onDecision={openDrawer}
            onReasonChange={setReason}
            onCancelDecision={() => { setDrawer(null); setMutationError('') }}
            onConfirmDecision={() => void submitDecision()}
          />
        )}
        <footer className="a-pager"><Pager page={page} pageSize={pageSize} total={total} onChange={setPage} onPageSizeChange={(size) => { setPageSize(size); setPage(1) }} /></footer>
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
  onMediaRefresh,
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
  onMediaRefresh: () => void | Promise<void>
  onSelect: (item: ReviewItem) => void
  onDecision: (item: ReviewItem, decision: ReviewDecision, explanation?: string) => void
  onReasonChange: (value: string) => void
  onCancelDecision: () => void
  onConfirmDecision: () => void
}) {
  const selectedRow = selected ? reviewRowView(selected) : null
  const previewRef = useRef<HTMLElement | null>(null)
  const queueRef = useRef<HTMLElement | null>(null)
  const decisionPanelRef = useRef<HTMLDivElement | null>(null)
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
    <section className="a-review lr-review">
      <section ref={queueRef} className="a-review__col" data-review-region="queue" aria-label="审核队列">
        <div className="lr-review__queue-head">
          <span>{rows.length} 个审核项</span>
          <span>{selectedRow ? selectedRow.statusLabel : '未选择'}</span>
        </div>
        <div className="a-review__list" role="listbox" aria-label="审核队列" onKeyDown={handleQueueKeyDown}>
          {rows.map((item) => {
            const row = reviewRowView(item)
            const active = selected?.id === item.id
            return (
              <button
                key={item.id}
                id={`review-queue-item-${item.id}`}
                data-review-queue-item
                type="button"
                role="option"
                aria-selected={active}
                tabIndex={active ? 0 : -1}
                className={cn('lr-review__item focus-visible:outline-[var(--focus-ring)]', active && 'is-active')}
                onClick={() => onSelect(item)}
              >
                <strong>{row.title}</strong>
                <span>{row.owner} · {row.createdAtLabel} · {row.taskTypeLabel}</span>
                <span><Badge tone={row.statusTone}>{row.statusLabel}</Badge></span>
              </button>
            )
          })}
        </div>
      </section>

      <section ref={previewRef} className="a-review__col" data-review-region="preview" aria-label="审核预览">
        <div className="a-review__col-head">预览</div>
        {selectedRow ? (
          <>
            <div className="a-review__canvas lr-review__canvas">
              <figure>
                <RefreshableMediaImage src={adminApi.imageReviewUrl(selectedRow.imageID, accessToken, selectedRow.imageURL)} alt={selectedRow.title} onMediaRefresh={onMediaRefresh} />
                <figcaption className="a-hint" style={{ fontFamily: 'var(--font-admin-mono)', fontSize: 11 }}>{selectedRow.title} · {selectedRow.taskTypeLabel}</figcaption>
              </figure>
            </div>
            <div className="lr-review__meta">
              <div className="lr-row">
                <strong>{selectedRow.title}</strong>
                <Badge tone={selectedRow.statusTone}>{selectedRow.statusLabel}</Badge>
                <Badge tone="primary">{selectedRow.taskTypeLabel}</Badge>
              </div>
              <div className="lr-review__prompt">{selectedRow.context}</div>
              <div className="a-hint">用户：{selectedRow.owner} · 提交于 {selectedRow.createdAtLabel}</div>
            </div>
          </>
        ) : <EmptyBlock title="请选择审核项" detail="从左侧队列选择图片后预览和处理。" />}
      </section>

      <aside className="a-review__col a-review__col--side" data-review-region="action" aria-label="审核动作">
        <div className="a-review__col-head">审核动作</div>
        <div className="a-review__side">
          {selectedRow ? (
            <>
              <div className="lr-review__owner">
                <span className="a-cell-entity__avatar">{(selectedRow.owner || '?').slice(0, 1).toUpperCase()}</span>
                <div>
                  <strong>{selectedRow.owner}</strong>
                  <span>{selectedRow.context}</span>
                </div>
              </div>
              {drawer ? (
                <div ref={decisionPanelRef} className="lr-review__confirm">
                  <strong>{drawer.decision === 'approve' ? '确认通过并公开' : drawer.decision === 'reject' ? '确认驳回' : '确认下架'}</strong>
                  <p>原因会写入审计日志，用户可在公开状态中看到结果。</p>
                  {mutationError ? <InlineFeedback tone="danger" message={mutationError} /> : null}
                  <label className="a-field">
                    <span>审核原因 <span style={{ color: 'var(--a-danger)' }}>*</span></span>
                    <textarea className="a-textarea" value={reason} onChange={(event) => onReasonChange(event.target.value)} rows={4} placeholder="写明通过、驳回或下架理由" />
                  </label>
                  <div className="lr-row lr-row--end">
                    <button className="a-btn a-btn--sm a-btn--quiet" type="button" disabled={busy} onClick={onCancelDecision}>取消</button>
                    <button className={cn('a-btn a-btn--sm', drawer.decision === 'approve' ? 'a-btn--primary' : 'a-btn--danger')} type="button" disabled={busy || reason.trim().length < 3} onClick={onConfirmDecision}>{busy ? '提交中...' : '确认提交'}</button>
                  </div>
                </div>
              ) : (
                <>
                  {selectedRow.actions.some((action) => action.decision === 'reject') ? (
                    <>
                      <div className="lr-review__presets" aria-label="驳回原因预设">
                        {reasonPresets.map((preset) => <button key={preset} type="button" className="lr-review__preset" aria-pressed={draftReason === preset} onClick={() => setDraftReason(preset)}>{preset}</button>)}
                      </div>
                      <label className="a-field">
                        <span>补充说明（可选）</span>
                        <textarea className="a-textarea" value={draftReason} onChange={(event) => setDraftReason(event.target.value)} rows={4} placeholder="输入更具体的驳回说明" />
                      </label>
                    </>
                  ) : null}
                  <div className="lr-stack--sm">
                    {selectedRow.actions.map((action) => (
                      <button
                        key={action.decision}
                        type="button"
                        disabled={interactionLocked}
                        className={cn('a-btn a-btn--sm', action.tone === 'primary' ? 'a-btn--primary' : 'a-btn--danger')}
                        onClick={(event) => {
                          decisionTriggerRef.current = event.currentTarget
                          onDecision(selectedRow.raw, action.decision, action.decision === 'reject' ? draftReason : '')
                        }}
                      >
                        {action.label}
                      </button>
                    ))}
                    {!selectedRow.actions.length ? <span className="a-hint">{selectedRow.terminalActionLabel}</span> : null}
                  </div>
                </>
              )}
              <div className="a-banner a-banner--info" style={{ fontSize: 12 }}><span>i</span><div>快捷原因：违规内容、低质量图片、版权风险。</div></div>
            </>
          ) : <p className="a-hint">从左侧选择审核项后在这里处理。</p>}
        </div>
      </aside>
    </section>
  )
}

function reviewTabLabel(tab: typeof allReviewTabs[number]) {
  if (tab === 'pending_review') return '待处理'
  if (tab === 'approved') return '已公开'
  if (tab === 'all') return '全部'
  return reviewStatusLabel(tab)
}