import { FormEvent, useEffect, useState } from 'react'
import type { PaymentOrder } from '../../../shared/api-types'
import { adminApi } from '../../../shared/admin-api'
import { cn } from '../../../shared/classnames'
import { Badge, EmptyBlock, ErrorBlock, LoadingBlock, PageHeader } from '../components'
import { adminButton, adminPage } from '../ui/classes'
import { ColumnDef, DataTable, FilterToolbar, ListPage, Pager } from '../ui/dataTable'
import { FilterIcon } from '../ui/listIcons'
import { cashierAdminDateTime, cashierOrderPaymentLabel, cashierOrderPurchaseTypeLabel } from './cashierPaymentDisplay'
import { cashierOrderStatusBadge } from './cashierStatusRows'

const quickFilters = [
  { label: '全部', value: '' },
  { label: '待支付', value: 'pending' },
  { label: '支付失败', value: 'failed' },
  { label: '待回调', value: 'paid' },
  { label: '退款中', value: 'partially_refunded' },
  { label: '同步失败', value: 'sync_failed' },
] as const

export function OrdersPage({ onFeedback }: { onFeedback: (title: string, detail?: string) => void }) {
  const [rows, setRows] = useState<PaymentOrder[]>([])
  const [query, setQuery] = useState('')
  const [status, setStatus] = useState('')
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(20)
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const load = async (nextPage = page, nextPageize = pageSize) => {
    setLoading(true)
    setError(null)
    try {
      const result = await adminApi.listPaymentOrders({
        page: nextPage,
        page_size: nextPageize,
        query: query.trim() || undefined,
        status: status || undefined,
      })
      setRows(result.items)
      setTotal(result.total)
      setPage(nextPage)
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : '订单载入失败')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    void load(1)
  }, [status])

  if (loading) return <LoadingBlock label="载入订单列表" />
  if (error) return <ErrorBlock message={error} onRetry={() => void load(page)} />

  const columns: ColumnDef<PaymentOrder>[] = [
    {
      key: 'order_no',
      title: '订单',
      width: 'minmax(220px,2fr)',
      render: (order) => (
        <div className="flex min-w-0 flex-col gap-1">
          <strong className="text-[var(--text)]">{order.order_no}</strong>
          <span className="text-xs text-[var(--soft)]">{order.plan_name || order.plan_code}</span>
        </div>
      ),
    },
    {
      key: 'status',
      title: '状态',
      width: 'minmax(90px,0.8fr)',
      render: (order) => {
        const badge = cashierOrderStatusBadge(order.status)
        return <Badge tone={badge.tone}>{badge.label}</Badge>
      },
    },
    {
      key: 'type',
      title: '类型',
      width: 'minmax(110px,1fr)',
      render: (order) => cashierOrderPurchaseTypeLabel(order),
    },
    {
      key: 'amount',
      title: '金额',
      width: 'minmax(100px,0.8fr)',
      render: (order) => <code>¥ {order.amount_cny}</code>,
    },
    {
      key: 'points',
      title: '积分',
      width: 'minmax(100px,0.8fr)',
      render: (order) => <code>{order.points}</code>,
    },
    {
      key: 'payment',
      title: '支付方式',
      width: 'minmax(120px,1fr)',
      render: (order) => cashierOrderPaymentLabel(order),
    },
    {
      key: 'created_at',
      title: '创建时间',
      width: 'minmax(160px,1.2fr)',
      render: (order) => cashierAdminDateTime(order.created_at),
    },
  ]

  return (
    <section className={adminPage.stack}>
      <PageHeader
        title="订单管理"
        description="按订单号、用户、渠道流水号定位订单，并优先处理异常状态。"
        secondaryActions={<button type="button" className={cn(adminButton.base, adminButton.ghost)} onClick={() => void load(page)}>刷新</button>}
      />
      <ListPage
        filters={(
          <form onSubmit={(event: FormEvent) => { event.preventDefault(); void load(1) }}>
            <FilterToolbar
              fields={[
                { key: 'query', label: '关键词', primary: true, control: <input value={query} onChange={(event) => setQuery(event.target.value)} placeholder="订单号 / 用户 / 渠道流水号" /> },
                { key: 'status', label: '状态', primary: true, control: <select value={status} onChange={(event) => setStatus(event.target.value)}>{quickFilters.map((filter) => <option key={filter.value || 'all'} value={filter.value}>{filter.label}</option>)}</select> },
              ]}
              actions={<button type="submit" className={cn(adminButton.base, adminButton.primary, adminButton.small, 'gap-1.5')} aria-label="查询" title="查询"><FilterIcon className="size-4" /><span>查询</span></button>}
              resultSummary={`共 ${total} 条订单 · 当前显示 ${rows.length} 条`}
            />
          </form>
        )}
        pagination={<Pager page={page} pageSize={pageSize} total={total} onChange={(next) => void load(next)} onPageSizeChange={(size) => { setPageSize(size); void load(1, size) }} />}
      >
        {!rows.length ? (
          <EmptyBlock title="暂无订单" detail="订单产生后会优先以表格形式展示，异常状态可通过上方筛选快速定位。" />
        ) : (
          <DataTable
            columns={columns}
            rows={rows}
            rowKey={(order) => order.id}
          />
        )}
      </ListPage>
    </section>
  )
}
