import { Archive, Pause, Play, RotateCcw } from 'lucide-react'
import { FormEvent, useEffect, useRef, useState } from 'react'
import type { CashierPlan, CashierPlanTransitionAction } from '../../../shared/api-types'
import { adminApi } from '../../../shared/admin-api'
import { cn } from '../../../shared/classnames'
import { Badge, Drawer, EmptyBlock, ErrorBlock, Field, InlineFeedback, LoadingBlock, MetricStrip, Modal, PageHeader, RefreshIconButton, TooltipIconButton } from '../components'
import { DataTable, FilterToolbar, ListPage, Pager, type ColumnDef } from '../ui/dataTable'
import { cashierPlanDraftFromRow, cashierPlanDraftCanSave, cashierPlanEmptyDraft, cashierPlanPayloadFromDraft, type CashierPlanDraft } from './cashierPlanDraft'
import { cashierPlanActions, cashierPlanEmptyState, cashierPlanPurchaseBadge, type CashierPlanAction } from './cashierPlanPurchase'
import { cashierPlanStatusBadge, cashierPlanTypeLabel } from './cashierStatusRows'
import { createLatestListRequestGuard } from './listRefresh'
import './list-redesign.css'

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
    const target = planTransition.plan
    try {
      await adminApi.transitionCashierPlan(target.id, planTransition.action.action)
      const feedback = planTransition.action.label
      const name = target.plan_name
      setPlanTransition(null)
      setDialog((current) => current?.row && current.row.id === target.id ? null : current)
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

  const enabled = rows.filter((plan) => plan.status === 'active').length
  const purchasable = rows.filter((plan) => plan.purchase_enabled).length
  const averagePrice = rows.length ? rows.reduce((sum, plan) => sum + numeric(plan.price_cny), 0) / rows.length : 0
  const metrics = [
    { label: '套餐总数', value: String(total), trend: `当前页 ${rows.length} 个`, tone: 'neutral' as const },
    { label: '当前页启用', value: String(enabled), trend: `${purchasable} 个开放购买`, tone: 'good' as const },
    { label: '当前页停用 / 归档', value: String(rows.length - enabled), trend: '不影响历史订单', tone: rows.length - enabled ? 'warn' as const : 'neutral' as const },
    { label: '平均售价', value: `¥ ${averagePrice.toFixed(2)}`, trend: '当前页价格均值', tone: 'neutral' as const },
  ]

  return (
    <section className="a-page-stack">
      <PageHeader
        eyebrow="Packages"
        title="套餐管理"
        description="积分包与订阅套餐。售价、积分、赠送与有效期在抽屉里一次性编辑，列表只保留决策所需字段。"
        primaryAction={<button className="a-btn a-btn--sm a-btn--primary" type="button" onClick={() => openEditor(cashierPlanEmptyDraft())}>新增套餐</button>}
        secondaryActions={<RefreshIconButton label="刷新套餐列表" refreshing={loading} disabled={saving} onClick={() => void load(page)} />}
      />
      {error ? <InlineFeedback tone="danger" message={`套餐列表刷新失败：${error}`} /> : null}
      {loading && rows.length ? <InlineFeedback tone="neutral" message="正在刷新套餐，当前数据会保留到新结果返回。" /> : null}
      <MetricStrip metrics={metrics} />
      <ListPage
        resultSummary={<><h2>套餐列表</h2><p>售价、积分、赠送与有效期在编辑抽屉里维护。</p></>}
        actions={<span className="a-hint">共 {total} 个套餐</span>}
        filters={(
          <form onSubmit={(event: FormEvent) => { event.preventDefault(); void load(1) }}>
            <FilterToolbar
              fields={[
                { key: 'query', label: '搜索套餐', primary: true, control: <input className="a-input" value={query} onChange={(event) => setQuery(event.target.value)} placeholder="套餐代码或名称" /> },
                { key: 'plan_type', label: '套餐类型', primary: true, control: <select className="a-select" value={planType} onChange={(event) => setPlanType(event.target.value)}><option value="">全部类型</option><option value="points_package">积分包</option><option value="subscription">订阅套餐</option></select> },
                { key: 'status', label: '套餐状态', primary: true, control: <select className="a-select" value={status} onChange={(event) => setStatus(event.target.value)}><option value="">未删除</option><option value="active">启用</option><option value="disabled">停用</option><option value="archived">已归档</option><option value="all">全部状态</option></select> },
                { key: 'sort', label: '排序', control: <select className="a-select" value={`${sortBy}:${sortOrder}`} onChange={(event) => { const [field, direction] = event.target.value.split(':'); setSortBy(field); setSortOrder(direction) }}><option value="sort_order:asc">排序值升序</option><option value="sort_order:desc">排序值降序</option><option value="price_cny:asc">价格升序</option><option value="price_cny:desc">价格降序</option><option value="points:asc">积分升序</option><option value="points:desc">积分降序</option></select> },
              ]}
              actions={<button type="submit" className="a-btn a-btn--sm a-btn--primary">筛选</button>}
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
          minWidth={1040}
        />
      </ListPage>

      {dialog ? (
        <Drawer
          title={dialog.row ? `编辑套餐 · ${dialog.row.plan_name}` : '新增套餐'}
          description={dialog.row ? `${dialog.row.plan_code} · 排序 ${dialog.row.sort_order ?? 0}` : '代码创建后不可修改，历史订单仍引用旧代码'}
          headerAside={dialog.row ? <Badge tone={cashierPlanStatusBadge(dialog.row.status).tone}>{cashierPlanStatusBadge(dialog.row.status).label}</Badge> : undefined}
          onClose={() => { if (!planTransition) { setDialog(null); setDialogError(null) } }}
          footer={(
            <>
              {dialog.row && dialog.row.status !== 'archived' ? <button className="a-btn a-btn--sm a-btn--danger" type="button" disabled={saving} onClick={() => setPlanTransition({ plan: dialog.row!, action: { action: 'archive', label: '归档套餐', detail: `归档 ${dialog.row!.plan_name} 后不再出现在收银台，历史订单不受影响。`, tone: 'danger' } })}>归档</button> : null}
              <div className="lr-spacer" />
              <button className="a-btn a-btn--sm a-btn--quiet" type="button" disabled={saving} onClick={() => { setDialog(null); setDialogError(null) }}>取消</button>
              <button className="a-btn a-btn--sm a-btn--primary" type="button" disabled={saving || !cashierPlanDraftCanSave(dialog)} onClick={() => void save()}>{saving ? '保存中...' : '保存套餐'}</button>
            </>
          )}
        >
          {dialogError ? <InlineFeedback tone="danger" message={dialogError} /> : null}
          <PackageForm draft={dialog} onChange={setDialog} />
        </Drawer>
      ) : null}

      {planTransition ? (
        <Modal
          title={planTransition.action.label}
          detail={planTransition.action.detail}
          onClose={() => setPlanTransition(null)}
          footer={<><button type="button" className="a-btn a-btn--sm a-btn--quiet" disabled={saving} onClick={() => setPlanTransition(null)}>取消</button><button type="button" className={cn('a-btn a-btn--sm', planTransition.action.tone === 'danger' ? 'a-btn--danger' : 'a-btn--primary')} disabled={saving} onClick={() => void transition()}>{saving ? '处理中...' : '确认'}</button></>}
        >
          {error ? <InlineFeedback tone="danger" message={error} /> : null}
          <p className="a-hint">状态变更只影响后续展示和购买，不会修改历史订单、积分或账本记录。</p>
        </Modal>
      ) : null}
    </section>
  )
}

function PackageForm({ draft, onChange }: { draft: CashierPlanDraft; onChange: (draft: CashierPlanDraft) => void }) {
  return (
    <form className="a-form" onSubmit={(event: FormEvent) => event.preventDefault()}>
      <section className="a-form__section">
        <div className="a-form__head"><h3><span className="a-form__num">1</span>套餐信息</h3><p>代码创建后不可修改，历史订单仍引用旧代码。</p></div>
        <Field label="套餐代码"><input className="a-input" value={draft.plan_code} disabled={Boolean(draft.row)} onChange={(event) => onChange({ ...draft, plan_code: event.target.value })} placeholder="points_5200" style={{ fontFamily: 'var(--font-admin-mono)' }} /></Field>
        <Field label="套餐名称"><input className="a-input" value={draft.plan_name} onChange={(event) => onChange({ ...draft, plan_name: event.target.value })} placeholder="创作者包" /></Field>
        <Field label="套餐类型"><select className="a-select" value={draft.plan_type} onChange={(event) => onChange({ ...draft, plan_type: event.target.value, purchase_enabled: event.target.value === 'subscription' ? false : draft.purchase_enabled })}><option value="points_package">积分包</option><option value="subscription">订阅套餐</option></select></Field>
        <Field label="描述"><textarea className="a-textarea" value={draft.description} onChange={(event) => onChange({ ...draft, description: event.target.value })} placeholder="适合持续创作的个人创作者，约 208 张标准图片。" /></Field>
      </section>
      <section className="a-form__section">
        <div className="a-form__head"><h3><span className="a-form__num">2</span>价格与积分</h3></div>
        <div className="lr-form-pair">
          <Field label="售价（CNY）"><input className="a-input" value={draft.price_cny} onChange={(event) => onChange({ ...draft, price_cny: event.target.value })} inputMode="decimal" placeholder="50.00" /></Field>
          <Field label="基础积分"><input className="a-input" value={draft.points} onChange={(event) => onChange({ ...draft, points: event.target.value })} inputMode="decimal" placeholder="5000" /></Field>
        </div>
        <div className="lr-form-pair">
          <Field label="赠送积分" hint={`到账合计 ${numeric(draft.points) + numeric(draft.bonus_points)}。`}><input className="a-input" value={draft.bonus_points} onChange={(event) => onChange({ ...draft, bonus_points: event.target.value })} inputMode="decimal" placeholder="200" /></Field>
          <Field label="排序值"><input className="a-input" type="number" value={draft.sort_order} onChange={(event) => onChange({ ...draft, sort_order: event.target.value })} /></Field>
        </div>
      </section>
      <section className="a-form__section">
        <div className="a-form__head"><h3><span className="a-form__num">3</span>有效期</h3></div>
        <label className="a-switch">
          <input type="checkbox" checked={draft.credit_expiry_enabled} onChange={(event) => onChange({ ...draft, credit_expiry_enabled: event.target.checked, duration_days: event.target.checked ? draft.duration_days || '30' : '' })} />
          <span className="a-switch__track" />
          <span>积分有有效期</span>
        </label>
        {draft.credit_expiry_enabled ? <Field label="有效天数" hint="从积分到账时间开始计算。"><input className="a-input" type="number" min="1" value={draft.duration_days} onChange={(event) => onChange({ ...draft, duration_days: event.target.value })} /></Field> : null}
      </section>
      <section className="a-form__section">
        <div className="a-form__head"><h3><span className="a-form__num">4</span>状态</h3></div>
        <label className="a-switch">
          <input type="checkbox" checked={draft.status !== 'disabled' && draft.status !== 'archived'} onChange={(event) => onChange({ ...draft, status: event.target.checked ? 'active' : 'disabled' })} />
          <span className="a-switch__track" />
          <span>启用套餐</span>
        </label>
        <label className="a-switch">
          <input type="checkbox" checked={draft.purchase_enabled} disabled={draft.plan_type === 'subscription'} onChange={(event) => onChange({ ...draft, purchase_enabled: event.target.checked })} />
          <span className="a-switch__track" />
          <span>在收银台开放购买</span>
        </label>
      </section>
      <div className="a-banner a-banner--warn"><span>!</span><div><strong style={{ fontSize: 13 }}>修改不会影响历史订单</strong><div className="a-hint">已购买用户的积分与订单金额保持不变。</div></div></div>
    </form>
  )
}

function packageColumns(onEdit: (plan: CashierPlan) => void, onTransition: (plan: CashierPlan, action: CashierPlanAction) => void): ColumnDef<CashierPlan>[] {
  return [
    {
      key: 'plan',
      title: '套餐',
      width: 'minmax(220px,2fr)',
      render: (plan) => <span className="lr-cell"><strong>{plan.plan_name}</strong><span>{plan.plan_code}</span></span>,
    },
    {
      key: 'type',
      title: '类型',
      width: 'minmax(110px,1fr)',
      render: (plan) => cashierPlanTypeLabel(plan.plan_type),
    },
    {
      key: 'price',
      title: '售价',
      width: 'minmax(100px,.8fr)',
      align: 'right',
      kind: 'number',
      render: (plan) => <span className="lr-num">¥ {plan.price_cny}</span>,
    },
    {
      key: 'points',
      title: '积分 / 赠送',
      width: 'minmax(140px,1.1fr)',
      align: 'right',
      kind: 'number',
      render: (plan) => <span className="lr-cell" style={{ justifyItems: 'end' }}><strong className="lr-num">{plan.points}</strong><span>赠送 {plan.bonus_points || '0'}</span></span>,
    },
    {
      key: 'validity',
      title: '有效期',
      width: 'minmax(100px,.8fr)',
      align: 'right',
      kind: 'number',
      render: (plan) => <span className="lr-num">{plan.credit_expiry_enabled === false ? '长期有效' : `${plan.duration_days ?? '—'} 天`}</span>,
    },
    {
      key: 'order',
      title: '排序',
      width: 'minmax(70px,.6fr)',
      align: 'right',
      kind: 'number',
      render: (plan) => <span className="lr-num">{plan.sort_order ?? 0}</span>,
    },
    {
      key: 'status',
      title: '状态',
      width: 'minmax(160px,1.2fr)',
      render: (plan) => {
        const status = cashierPlanStatusBadge(plan.status)
        const purchase = cashierPlanPurchaseBadge(plan)
        return <span className="lr-row"><Badge tone={status.tone}>{status.label}</Badge><Badge tone={purchase.tone}>{purchase.label}</Badge></span>
      },
    },
    {
      key: 'actions',
      title: '操作',
      width: 'minmax(180px,1.2fr)',
      align: 'right',
      render: (plan) => (
        <div className="a-row-actions">
          {plan.status !== 'archived' ? <button className="a-row-btn a-row-btn--primary" type="button" onClick={() => onEdit(plan)}>编辑</button> : null}
          {cashierPlanActions(plan).map((action) => (
            <TooltipIconButton key={action.action} label={`${action.label} ${plan.plan_name}`} className={action.tone === 'danger' ? 'text-[var(--red)]' : undefined} onClick={() => onTransition(plan, action)}>
              <PlanActionIcon action={action.action} />
            </TooltipIconButton>
          ))}
        </div>
      ),
    },
  ]
}

function PlanActionIcon({ action }: { action: CashierPlanTransitionAction }) {
  if (action === 'enable') return <Play />
  if (action === 'disable') return <Pause />
  if (action === 'restore') return <RotateCcw />
  return <Archive />
}

function numeric(value?: string | null) {
  const parsed = Number(value)
  return Number.isFinite(parsed) ? parsed : 0
}