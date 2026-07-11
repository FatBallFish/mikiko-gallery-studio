import { useEffect, useState } from 'react'
import type { CashierPlan } from '../../../shared/api-types'
import { adminApi } from '../../../shared/admin-api'
import { cn } from '../../../shared/classnames'
import { Badge, EmptyBlock, ErrorBlock, LoadingBlock, PageHeader } from '../components'
import { adminButton, adminPage } from '../ui/classes'
import { DataTable, FilterToolbar, ListPage, type ColumnDef } from '../ui/dataTable'
import { cashierPlanEmptyState, cashierPlanPurchaseBadge } from './cashierPlanPurchase'
import { cashierPlanStatusBadge, cashierPlanTypeLabel } from './cashierStatusRows'

export function PackagesPage({ onFeedback }: { onFeedback: (title: string, detail?: string) => void }) {
  const [rows, setRows] = useState<CashierPlan[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const load = async () => {
    setLoading(true)
    setError(null)
    try {
      const result = await adminApi.listCashierPlans({ page: 1, page_size: 100 })
      setRows(result.items)
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : '套餐载入失败')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    void load()
  }, [])

  if (loading) return <LoadingBlock label="载入套餐列表" />
  if (error) return <ErrorBlock message={error} onRetry={() => void load()} />

  return (
    <section className={adminPage.stack}>
      <PageHeader
        title="套餐管理"
        description="维护价格、积分、上下架和展示顺序；支付通道配置已移至支付配置。"
        primaryAction={<button type="button" className={cn(adminButton.base, adminButton.primary)} onClick={() => onFeedback('套餐编辑入口', '新增/编辑表单将在侧栏中完成。')}>新增套餐</button>}
        secondaryActions={<button type="button" className={cn(adminButton.base, adminButton.ghost)} onClick={() => void load()}>刷新</button>}
      />
      <ListPage
        filters={<FilterToolbar fields={[]} resultSummary={`共 ${rows.length} 个套餐`} />}
      >
        <DataTable
          columns={packageColumns(onFeedback)}
          rows={rows}
          rowKey={(plan) => plan.id}
          empty={<EmptyBlock title={cashierPlanEmptyState.title} detail={cashierPlanEmptyState.detail} />}
        />
      </ListPage>
    </section>
  )
}

function packageColumns(onFeedback: (title: string, detail?: string) => void): ColumnDef<CashierPlan>[] {
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
      width: 'minmax(90px,.7fr)',
      align: 'right',
      render: (plan) => <button type="button" className={cn(adminButton.base, adminButton.primary, adminButton.small)} onClick={() => onFeedback('套餐编辑入口', plan.plan_name)}>编辑</button>,
    },
  ]
}
