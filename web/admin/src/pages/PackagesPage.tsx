import { Archive, Pause, Pencil, Play, RotateCcw } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import type { FormEvent } from 'react'
import type { CashierPlan, CashierPlanTransitionAction } from '../../../shared/api-types'
import { adminApi } from '../../../shared/admin-api'
import { cn } from '../../../shared/classnames'
import { Badge, EmptyBlock, ErrorBlock, InlineFeedback, LoadingBlock, Modal, PageHeader, RefreshIconButton, TooltipIconButton } from '../components'
import { adminButton, adminPage } from '../ui/classes'
import { DataTable, FilterToolbar, ListPage, Pager, type ColumnDef } from '../ui/dataTable'
import { FilterIcon } from '../ui/listIcons'
import { CashierPlanEditorDialog } from './CashierPlanEditorDialog'
import { cashierPlanDraftFromRow, cashierPlanEmptyDraft, cashierPlanPayloadFromDraft, type CashierPlanDraft } from './cashierPlanDraft'
import { cashierPlanActions, cashierPlanEmptyState, cashierPlanPurchaseBadge, type CashierPlanAction } from './cashierPlanPurchase'
import { cashierPlanStatusBadge, cashierPlanTypeLabel } from './cashierStatusRows'
import { createLatestListRequestGuard } from './listRefresh'

export function PackagesPage({ onFeedback }: { onFeedback: (title: string, detail?: string) => void }) {
  const [rows, setRows] = useState<CashierPlan[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [dialog, setDialog] = useState<CashierPlanDraft | null>(null)
  const [dialogError, setDialogError] = useState<string | null>(null)
  const [saving, setSaving] = useState(false)
  const [query, setQuery] = useState('')
  const [planType, setPlanType] = useState('')
  const [status, setStatus] = useState('')
  const [sortBy, setSortBy] = useState('sort_order')
  const [sortOrder, setSortOrder] = useState('asc')
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(20)
  const [total, setTotal] = useState(0)
  const [planTransition, setPlanTransition] = useState<{ plan: CashierPlan; action: CashierPlanAction } | null>(null)
  const requestGuard = useRef(createLatestListRequestGuard()).current

  const load = async (nextPage = page, nextPageSize = pageSize) => {
    const request = requestGuard.begin()
    setLoading(true)
    setError(null)
    try {
      const result = await adminApi.listCashierPlans({
        page: nextPage,
        page_size: nextPageSize,
        query: query.trim() || undefined,
        plan_type: planType || undefined,
        status: status || undefined,
        sort_by: sortBy,
        sort_order: sortOrder,
      })
      if (!requestGuard.isCurrent(request)) return
      setRows(result.items)
      setTotal(result.total)
      setPage(nextPage)
    } catch (caught) {
      if (!requestGuard.isCurrent(request)) return
      setError(caught instanceof Error ? caught.message : '套餐载入失败')
    } finally {
      if (!requestGuard.isCurrent(request)) return
      setLoading(false)
    }
  }

  useEffect(() => {
    void load(1)
    return () => requestGuard.invalidate()
  }, [])

  const openEditor = (draft: CashierPlanDraft) => {
    setDialogError(null)
    setDialog(draft)
  }

  const save = async () => {
    if (!dialog || saving) return
    setSaving(true)
    setDialogError(null)
    try {
      const payload = cashierPlanPayloadFromDraft(dialog)
      if (dialog.row) await adminApi.updateCashierPlan(dialog.row.id, payload)
      else await adminApi.createCashierPlan(payload)
      const savedName = dialog.plan_name
      setDialog(null)
      await load(page)
      onFeedback('充值套餐已保存', savedName)
    } catch (caught) {
      setDialogError(caught instanceof Error ? caught.message : '充值套餐保存失败')
    } finally {
      setSaving(false)
    }
  }

  const transition = async () => {
    if (!planTransition || saving) return
    setSaving(true)
    setError(null)
    try {
      await adminApi.transitionCashierPlan(planTransition.plan.id, planTransition.action.action)
      const feedback = planTransition.action.label
      const name = planTransition.plan.plan_name
      setPlanTransition(null)
      await load(page)
      onFeedback(feedback, name)
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : '套餐状态更新失败')
    } finally {
      setSaving(false)
    }
  }

  if (loading && !rows.length) return <LoadingBlock label="载入套餐列表" />
  if (error && !rows.length) return <ErrorBlock message={error} onRetry={() => void load()} />

  return (
    <section className={adminPage.stack}>
      <PageHeader
        title="套餐管理"
        description="维护价格、积分、上下架和展示顺序；支付通道配置已移至支付配置。"
        primaryAction={<button type="button" className={cn(adminButton.base, adminButton.primary)} onClick={() => openEditor(cashierPlanEmptyDraft())}>新增套餐</button>}
        secondaryActions={<RefreshIconButton label="刷新套餐列表" refreshing={loading} disabled={saving} onClick={() => void load(page)} />}
      />
      {error ? <InlineFeedback tone="danger" message={`套餐列表刷新失败：${error}`} /> : null}
      <ListPage
        filters={(
          <form onSubmit={(event: FormEvent) => { event.preventDefault(); void load(1) }}>
            <FilterToolbar
              fields={[
                { key: 'query', label: '关键词', primary: true, control: <input value={query} onChange={(event) => setQuery(event.target.value)} placeholder="套餐代码 / 名称" /> },
                { key: 'plan_type', label: '套餐类型', primary: true, control: <select value={planType} onChange={(event) => setPlanType(event.target.value)}><option value="">全部类型</option><option value="points_package">积分包</option><option value="subscription">订阅套餐</option></select> },
                { key: 'status', label: '套餐状态', primary: true, control: <select value={status} onChange={(event) => setStatus(event.target.value)}><option value="">未删除</option><option value="active">启用</option><option value="disabled">停用</option><option value="archived">已删除</option><option value="all">全部状态</option></select> },
                { key: 'sort', label: '排序', control: <select value={`${sortBy}:${sortOrder}`} onChange={(event) => { const [field, direction] = event.target.value.split(':'); setSortBy(field); setSortOrder(direction) }}><option value="sort_order:asc">排序值升序</option><option value="sort_order:desc">排序值降序</option><option value="price_cny:asc">价格升序</option><option value="price_cny:desc">价格降序</option><option value="points:asc">积分升序</option><option value="points:desc">积分降序</option></select> },
              ]}
              actions={<button type="submit" className={cn(adminButton.base, adminButton.primary, adminButton.small, 'gap-1.5')}><FilterIcon className="size-4" /><span>筛选</span></button>}
              resultSummary={`共 ${total} 个套餐 · 当前显示 ${rows.length} 个`}
            />
          </form>
        )}
        pagination={<Pager page={page} pageSize={pageSize} total={total} onChange={(next) => void load(next)} onPageSizeChange={(size) => { setPageSize(size); void load(1, size) }} />}
      >
        <DataTable
          columns={packageColumns((plan) => openEditor(cashierPlanDraftFromRow(plan)), (plan, action) => setPlanTransition({ plan, action }))}
          rows={rows}
          rowKey={(plan) => plan.id}
          empty={<EmptyBlock title={cashierPlanEmptyState.title} detail={cashierPlanEmptyState.detail} />}
        />
      </ListPage>
      {dialog ? <CashierPlanEditorDialog draft={dialog} saving={saving} error={dialogError} onChange={setDialog} onClose={() => setDialog(null)} onSave={() => void save()} /> : null}
      {planTransition ? <Modal
        title={planTransition.action.label}
        detail={planTransition.action.detail}
        onClose={() => setPlanTransition(null)}
        footer={<><button type="button" className={cn(adminButton.base, adminButton.ghost)} disabled={saving} onClick={() => setPlanTransition(null)}>取消</button><button type="button" className={cn(adminButton.base, planTransition.action.tone === 'danger' ? adminButton.danger : adminButton.primary)} disabled={saving} onClick={() => void transition()}>{saving ? '处理中...' : '确认'}</button></>}
      ><p className="text-sm leading-6 text-[var(--muted-strong)]">状态变更只影响后续展示和购买，不会修改历史订单、积分或账本记录。</p></Modal> : null}
    </section>
  )
}

function packageColumns(onEdit: (plan: CashierPlan) => void, onTransition: (plan: CashierPlan, action: CashierPlanAction) => void): ColumnDef<CashierPlan>[] {
  return [
    {
      key: 'plan',
      title: '套餐',
      width: 'minmax(220px,2fr)',
      render: (plan) => <span className="grid min-w-0 gap-1"><strong className="truncate text-[var(--text)]">{plan.plan_name}</strong><code className="truncate text-xs text-[var(--soft)]">{plan.plan_code}</code></span>,
    },
    {
      key: 'type',
      title: '类型',
      width: 'minmax(110px,1fr)',
      render: (plan) => cashierPlanTypeLabel(plan.plan_type),
    },
    {
      key: 'price',
      title: '价格',
      width: 'minmax(100px,.8fr)',
      kind: 'number',
      align: 'right',
      render: (plan) => <code className="font-semibold text-[var(--text)]">¥ {plan.price_cny}</code>,
    },
    {
      key: 'points',
      title: '积分 / 赠送',
      width: 'minmax(130px,1fr)',
      kind: 'number',
      align: 'right',
      render: (plan) => <span className="grid gap-1 text-right"><code className="font-semibold text-[var(--text)]">{plan.points}</code><span className="text-xs text-[var(--soft)]">赠送 {plan.bonus_points || '0'}</span></span>,
    },
    {
      key: 'order',
      title: '排序',
      width: 'minmax(70px,.6fr)',
      kind: 'number',
      align: 'right',
      render: (plan) => <code>{plan.sort_order ?? 0}</code>,
    },
    {
      key: 'status',
      title: '状态',
      width: 'minmax(150px,1.2fr)',
      render: (plan) => {
        const status = cashierPlanStatusBadge(plan.status)
        const purchase = cashierPlanPurchaseBadge(plan)
        return <span className="flex flex-wrap gap-1.5"><Badge tone={status.tone}>{status.label}</Badge><Badge tone={purchase.tone}>{purchase.label}</Badge></span>
      },
    },
    {
      key: 'actions',
      title: '操作',
      width: 'minmax(150px,1fr)',
      align: 'right',
      render: (plan) => <span className="flex justify-end gap-1.5">{plan.status !== 'archived' ? <TooltipIconButton label={`编辑 ${plan.plan_name}`} onClick={() => onEdit(plan)}><Pencil /></TooltipIconButton> : null}{cashierPlanActions(plan).map((action) => <TooltipIconButton key={action.action} label={`${action.label} ${plan.plan_name}`} className={action.tone === 'danger' ? 'text-[var(--red)]' : undefined} onClick={() => onTransition(plan, action)}><PlanActionIcon action={action.action} /></TooltipIconButton>)}</span>,
    },
  ]
}

function PlanActionIcon({ action }: { action: CashierPlanTransitionAction }) {
  if (action === 'enable') return <Play />
  if (action === 'disable') return <Pause />
  if (action === 'restore') return <RotateCcw />
  return <Archive />
}
