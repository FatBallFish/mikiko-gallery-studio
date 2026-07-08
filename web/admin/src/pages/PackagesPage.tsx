import { useEffect, useState } from 'react'
import type { CashierPlan } from '../../../shared/api-types'
import { adminApi } from '../../../shared/admin-api'
import { cn } from '../../../shared/classnames'
import { Badge, EmptyBlock, ErrorBlock, LoadingBlock, PageHeader } from '../components'
import { adminButton, adminPage, adminSurface } from '../ui/classes'
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
      {!rows.length ? <EmptyBlock title={cashierPlanEmptyState.title} detail={cashierPlanEmptyState.detail} /> : (
        <section className="grid grid-cols-[repeat(auto-fill,minmax(260px,1fr))] gap-3">
          {rows.map((plan) => {
            const status = cashierPlanStatusBadge(plan.status)
            const purchase = cashierPlanPurchaseBadge(plan)
            return (
              <article key={plan.id} className={cn(adminSurface.card, 'grid gap-4 p-4')}>
                <header className="grid gap-1">
                  <div className="flex items-start justify-between gap-3">
                    <strong className="text-base">{plan.plan_name}</strong>
                    <Badge tone={status.tone}>{status.label}</Badge>
                  </div>
                  <p className="text-xs">{plan.plan_code} · {cashierPlanTypeLabel(plan.plan_type)}</p>
                </header>
                <div className="grid grid-cols-2 gap-3">
                  <PlanMetric label="价格" value={`¥ ${plan.price_cny}`} />
                  <PlanMetric label="积分" value={plan.points} />
                  <PlanMetric label="赠送" value={plan.bonus_points || '0'} />
                  <PlanMetric label="排序" value={String(plan.sort_order ?? 0)} />
                </div>
                <footer className="flex flex-wrap items-center justify-between gap-2 border-t border-[var(--border)] pt-3">
                  <Badge tone={purchase.tone}>{purchase.label}</Badge>
                  <button type="button" className={cn(adminButton.base, adminButton.ghost, adminButton.small)} onClick={() => onFeedback('套餐编辑入口', plan.plan_name)}>编辑</button>
                </footer>
              </article>
            )
          })}
        </section>
      )}
    </section>
  )
}

function PlanMetric({ label, value }: { label: string; value: string }) {
  return (
    <div className="grid gap-1">
      <span className="text-[10px] font-bold text-[var(--muted)]">{label}</span>
      <strong className="font-mono text-sm">{value}</strong>
    </div>
  )
}
