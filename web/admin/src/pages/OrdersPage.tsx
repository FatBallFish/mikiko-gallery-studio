import { FormEvent, useEffect, useMemo, useRef, useState } from 'react'
import type { PaymentOrder } from '../../../shared/api-types'
import { adminApi } from '../../../shared/admin-api'
import { Badge, Drawer, EmptyBlock, ErrorBlock, Field, InlineFeedback, LoadingBlock, MetricStrip, Modal, PageHeader, RefreshIconButton } from '../components'
import { ColumnDef, DataTable, FilterToolbar, ListPage, Pager } from '../ui/dataTable'
import { cashierAdminDateTime, cashierOrderPaymentLabel, cashierOrderPurchaseTypeLabel } from './cashierPaymentDisplay'
import { cashierOrderStatusBadge } from './cashierStatusRows'
import { createLatestListRequestGuard } from './listRefresh'
import './list-redesign.css'

const quickFilters = [
  { label: '全部', value: '' },
  { label: '待支付', value: 'pending' },
  { label: '支付失败', value: 'failed' },
  { label: '支付超时', value: 'expired' },
  { label: '人工取消', value: 'canceled' },
  { label: '待回调', value: 'paid' },
  { label: '退款中', value: 'partially_refunded' },
  { label: '同步失败', value: 'sync_failed' },
] as const

const refundableStatuses = ['paid', 'completed', 'partially_refunded']

type RefundDraft = { refund_trade_no: string; refund_amount_cny: string; reason: string }

export function OrdersPage({ onFeedback }: { onFeedback: (title: string, detail?: string) => void }) {
  const [rows, setRows] = useState<PaymentOrder[]>([])
  const [query, setQuery] = useState('')
  const [status, setStatus] = useState('')
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(20)
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [selected, setSelected] = useState<PaymentOrder | null>(null)
  const [detailLoading, setDetailLoading] = useState(false)
  const [detailError, setDetailError] = useState<string | null>(null)
  const [detailNotice, setDetailNotice] = useState('')
  const [busy, setBusy] = useState(false)
  const [refundDraft, setRefundDraft] = useState<RefundDraft | null>(null)
  const [refundError, setRefundError] = useState('')
  const [peekOrder, setPeekOrder] = useState<PaymentOrder | null>(null)
  const requestGuard = useRef(createLatestListRequestGuard()).current

  const load = async (nextPage = page, nextPageize = pageSize) => {
    const request = requestGuard.begin()
    setLoading(true)
    setError(null)
    try {
      const result = await adminApi.listPaymentOrders({
        page: nextPage,
        page_size: nextPageize,
        query: query.trim(),
        status: status || undefined,
      })
      if (!requestGuard.isCurrent(request)) return
      setRows(result.items)
      setTotal(result.total)
      setPage(nextPage)
    } catch (caught) {
      if (!requestGuard.isCurrent(request)) return
      setError(caught instanceof Error ? caught.message : '订单载入失败')
    } finally {
      if (!requestGuard.isCurrent(request)) return
      setLoading(false)
    }
  }

  useEffect(() => {
    void load(1)
    return () => requestGuard.invalidate()
  }, [status])

  const openDetail = async (order: PaymentOrder) => {
    setPeekOrder(null)
    setSelected(order)
    setDetailError(null)
    setDetailNotice('')
    setRefundDraft(null)
    setDetailLoading(true)
    try {
      setSelected(await adminApi.getPaymentOrder(order.id))
    } catch (caught) {
      setDetailError(caught instanceof Error ? caught.message : '订单详情载入失败')
    } finally {
      setDetailLoading(false)
    }
  }

  const replaceOrder = (next: PaymentOrder) => {
    setSelected(next)
    setRows((current) => current.map((item) => item.id === next.id ? next : item))
  }

  const syncOrder = async () => {
    if (!selected) return
    setBusy(true)
    setDetailNotice('')
    try {
      const result = await adminApi.syncPaymentOrder(selected.id)
      replaceOrder(result.order)
      const message = result.sync.message || `渠道状态：${result.sync.query_status}`
      setDetailNotice(`查单完成 · ${message}`)
      onFeedback('订单查单完成', `${selected.order_no} · ${message}`)
      await load(page, pageSize)
    } catch (caught) {
      setDetailNotice(`查单失败：${caught instanceof Error ? caught.message : '请稍后重试'}`)
    } finally {
      setBusy(false)
    }
  }

  const copyOrder = async (orderNo: string) => {
    try {
      await navigator.clipboard.writeText(orderNo)
      onFeedback('订单号已复制', orderNo)
    } catch {
      onFeedback('复制失败', '请打开订单详情后手动选择订单号。')
    }
  }

  const openRefund = () => {
    if (!selected) return
    setRefundError('')
    setRefundDraft({ refund_trade_no: selected.refund_trade_no || '', refund_amount_cny: selected.refunded_amount_cny || '', reason: '' })
  }

  const submitRefund = async (event: FormEvent) => {
    event.preventDefault()
    if (!selected || !refundDraft) return
    setBusy(true)
    setRefundError('')
    try {
      const updated = await adminApi.refundPaymentOrder(selected.id, {
        refund_trade_no: refundDraft.refund_trade_no.trim(),
        refund_amount_cny: refundDraft.refund_amount_cny.trim() || undefined,
        reason: refundDraft.reason.trim() || undefined,
      })
      replaceOrder(updated)
      setRefundDraft(null)
      setDetailNotice('退款已提交，订单与积分流水已刷新。')
      onFeedback('订单退款已提交', `${selected.order_no} · ${cashierOrderStatusBadge(updated.status).label}`)
      await load(page, pageSize)
    } catch (caught) {
      setRefundError(caught instanceof Error ? caught.message : '退款提交失败')
    } finally {
      setBusy(false)
    }
  }

  if (loading && !rows.length) return <LoadingBlock label="载入订单列表" />
  if (error && !rows.length) return <ErrorBlock message={error} onRetry={() => void load(page)} />

  const columns: ColumnDef<PaymentOrder>[] = [
    {
      key: 'order_no',
      title: '订单',
      width: 'minmax(220px,2fr)',
      render: (order) => <span className="lr-cell"><strong style={{ fontFamily: 'var(--font-admin-mono)', fontSize: 12 }}>{order.order_no}</strong><span>{order.plan_name || order.plan_code || '自定义金额'}</span></span>,
    },
    {
      key: 'user',
      title: '用户',
      width: 'minmax(210px,1.5fr)',
      render: (order) => (
        <button className="a-cell-entity" type="button" onClick={() => setPeekOrder(order)}>
          <span className="a-cell-entity__avatar">{avatarText(order)}</span>
          <span className="a-cell-entity__copy">
            <span className="a-cell-entity__name">{order.user_nickname || order.user_email || `用户 #${order.user_id ?? '-'}`}</span>
            <span className="a-cell-entity__meta">{order.user_email || `ID ${order.user_id ?? '-'}`}</span>
          </span>
        </button>
      ),
    },
    {
      key: 'status',
      title: '状态',
      width: 'minmax(96px,.8fr)',
      render: (order) => {
        const badge = cashierOrderStatusBadge(order.status)
        return <Badge tone={badge.tone}>{badge.label}</Badge>
      },
    },
    {
      key: 'type',
      title: '类型',
      width: 'minmax(120px,1fr)',
      render: (order) => cashierOrderPurchaseTypeLabel(order),
    },
    {
      key: 'amount',
      title: '金额',
      width: 'minmax(110px,.9fr)',
      align: 'right',
      kind: 'number',
      render: (order) => <span className="lr-num">¥ {order.amount_cny}</span>,
    },
    {
      key: 'points',
      title: '积分明细',
      width: 'minmax(150px,1.2fr)',
      align: 'right',
      render: (order) => (
        <span className="lr-cell" style={{ justifyItems: 'end' }}>
          <strong className="lr-num">{order.points} 积分</strong>
          <span>基础 {order.points} + 赠送 {order.bonus_points}</span>
          <span>合计 {order.total_points ?? order.points}</span>
        </span>
      ),
    },
    {
      key: 'payment',
      title: '支付方式',
      width: 'minmax(130px,1fr)',
      render: (order) => cashierOrderPaymentLabel(order),
    },
    {
      key: 'created_at',
      title: '创建时间',
      width: 'minmax(140px,1fr)',
      render: (order) => <span className="lr-muted">{cashierAdminDateTime(order.created_at)}</span>,
    },
    {
      key: 'actions',
      title: '操作',
      width: 'minmax(130px,1fr)',
      align: 'right',
      render: (order) => (
        <div className="a-row-actions">
          <button className="a-row-btn a-row-btn--primary" type="button" onClick={() => void openDetail(order)}>详情</button>
          <button className="a-icon-btn" type="button" style={{ width: 32, height: 32 }} aria-label="复制订单号" title="复制订单号" onClick={() => void copyOrder(order.order_no)}>⧉</button>
        </div>
      ),
    },
  ]

  const pagePaid = rows.filter((order) => order.status === 'paid' || order.status === 'completed').length
  const pagePending = rows.filter((order) => order.status === 'pending').length
  const pageRefund = rows.filter((order) => order.status === 'partially_refunded' || order.status === 'refunded').length
  const pageAmount = rows.reduce((sum, order) => sum + numeric(order.amount_cny), 0)
  const metrics = [
    { label: '筛选结果', value: String(total), trend: `当前页 ${rows.length} 笔`, tone: 'neutral' as const },
    { label: '当前页已支付', value: String(pagePaid), trend: `金额 ¥ ${pageAmount.toFixed(2)}`, tone: 'good' as const },
    { label: '当前页待支付', value: String(pagePending), trend: '等待用户完成支付', tone: pagePending ? 'warn' as const : 'neutral' as const },
    { label: '当前页退款', value: String(pageRefund), trend: '部分退款与已退款', tone: pageRefund ? 'warn' as const : 'neutral' as const },
  ]

  return (
    <section className="a-page-stack">
      <PageHeader
        eyebrow="Orders"
        title="订单管理"
        description="订单、支付状态与积分明细。用户列已映射为昵称与邮箱，支付异常可直接打开订单抽屉处理。"
        secondaryActions={<RefreshIconButton label="刷新订单列表" refreshing={loading} onClick={() => void load(page)} />}
      />
      {error ? <InlineFeedback tone="danger" message={`订单列表刷新失败：${error}`} /> : null}
      {loading && rows.length ? <InlineFeedback tone="neutral" message="正在刷新订单，当前数据会保留到新结果返回。" /> : null}
      <MetricStrip metrics={metrics} />
      <ListPage
        resultSummary={<><h2>订单列表</h2><p>点击任意行查看支付时间线、积分明细与操作。</p></>}
        actions={<span className="a-hint">按创建时间倒序</span>}
        filters={(
          <form onSubmit={(event: FormEvent) => { event.preventDefault(); void load(1) }}>
            <FilterToolbar
              fields={[
                { key: 'query', label: '关键词', primary: true, control: <input className="a-input" value={query} onChange={(event) => setQuery(event.target.value)} placeholder="订单号 / 用户 / 渠道流水号" /> },
                { key: 'status', label: '订单状态', primary: true, control: <select className="a-select" value={status} onChange={(event) => setStatus(event.target.value)}>{quickFilters.map((filter) => <option key={filter.value || 'all'} value={filter.value}>{filter.label}</option>)}</select> },
              ]}
              actions={<button type="submit" className="a-btn a-btn--sm a-btn--primary">查询</button>}
              resultSummary={`共 ${total} 条订单 · 当前显示 ${rows.length} 条`}
            />
          </form>
        )}
        pagination={<Pager page={page} pageSize={pageSize} total={total} onChange={(next) => void load(next)} onPageSizeChange={(size) => { setPageSize(size); void load(1, size) }} />}
      >
        {!rows.length ? (
          <EmptyBlock title="暂无订单" detail="订单产生后会优先以表格形式展示，异常状态可通过上方筛选快速定位。" />
        ) : (
          <DataTable columns={columns} rows={rows} rowKey={(order) => order.id} minWidth={1220} />
        )}
      </ListPage>

      {peekOrder ? (
        <Modal
          title="用户摘要"
          detail={`订单 ${peekOrder.order_no}`}
          onClose={() => setPeekOrder(null)}
          footer={<><button className="a-btn a-btn--sm a-btn--quiet" type="button" onClick={() => setPeekOrder(null)}>关闭</button><button className="a-btn a-btn--sm a-btn--primary" type="button" onClick={() => { window.location.hash = '/users' }}>打开用户管理</button></>}
        >
          <dl className="a-kv">
            <div><dt>用户</dt><dd>{peekOrder.user_nickname || peekOrder.user_email || `用户 #${peekOrder.user_id ?? '-'}`}</dd></div>
            <div><dt>邮箱</dt><dd>{peekOrder.user_email || '—'}</dd></div>
            <div><dt>用户 ID</dt><dd><span className="a-mono">#{peekOrder.user_id ?? '-'}</span></dd></div>
            <div><dt>累计订单</dt><dd>本笔 ¥ {peekOrder.amount_cny} · {peekOrder.points} + {peekOrder.bonus_points} 积分</dd></div>
          </dl>
        </Modal>
      ) : null}

      {selected ? (
        <Drawer
          title="订单详情"
          description={`${selected.order_no} · ${selected.user_nickname || selected.user_email || `用户 #${selected.user_id ?? '-'}`} · ${cashierAdminDateTime(selected.created_at)}`}
          headerAside={<Badge tone={cashierOrderStatusBadge(selected.status).tone}>{cashierOrderStatusBadge(selected.status).label}</Badge>}
          onClose={() => { if (!refundDraft) { setSelected(null); setDetailNotice('') } }}
          footer={(
            <>
              <button className="a-btn a-btn--sm" type="button" disabled={busy} onClick={() => void syncOrder()}>{busy ? '处理中...' : '查单'}</button>
              <button className="a-btn a-btn--sm" type="button" disabled={busy || !refundableStatuses.includes(selected.status)} onClick={openRefund}>退款</button>
              <div className="lr-spacer" />
              <button className="a-btn a-btn--sm a-btn--primary" type="button" onClick={() => { setSelected(null); setRefundDraft(null); setDetailNotice('') }}>关闭</button>
            </>
          )}
        >
          {detailLoading ? <LoadingBlock label="载入订单详情" /> : null}
          {detailError ? <ErrorBlock message={detailError} onRetry={() => void openDetail(selected)} /> : null}
          {detailNotice ? <InlineFeedback tone={detailNotice.includes('失败') ? 'danger' : 'success'} message={detailNotice} /> : null}
          {!detailLoading && !detailError ? (
            <div className="lr-stack">
              <div className="a-metrics lr-metrics-3">
                <div className="a-metric"><div className="a-metric__label">订单金额</div><div className="a-metric__value" style={{ fontSize: 21 }}>¥ {selected.amount_cny}</div></div>
                <div className="a-metric"><div className="a-metric__label">到账积分</div><div className="a-metric__value" style={{ fontSize: 21 }}>{selected.total_points ?? selected.points}</div></div>
                <div className="a-metric"><div className="a-metric__label">支付方式</div><div className="a-metric__value" style={{ fontSize: 17 }}>{cashierOrderPaymentLabel(selected)}</div></div>
              </div>
              <section className="a-drawer__section">
                <h3>订单信息</h3>
                <dl className="a-kv">
                  <div><dt>订单号</dt><dd><span className="a-mono">{selected.order_no}</span> <button className="a-row-btn" style={{ minHeight: 24, padding: '0 6px', fontSize: 11 }} type="button" onClick={() => void copyOrder(selected.order_no)}>复制</button></dd></div>
                  <div><dt>用户</dt><dd>{selected.user_nickname || selected.user_email || `用户 #${selected.user_id ?? '-'}`}</dd></div>
                  <div><dt>购买类型</dt><dd>{cashierOrderPurchaseTypeLabel(selected)} · {selected.plan_name || selected.plan_code || '自定义金额'}</dd></div>
                  <div><dt>积分明细</dt><dd>基础 {selected.points} + 赠送 {selected.bonus_points} = {selected.total_points ?? selected.points}</dd></div>
                  <div><dt>有效期</dt><dd>{selected.credit_expiry_enabled ? `${selected.credit_valid_days ?? '—'} 天` : '长期有效'}{selected.credit_expires_at ? ` · ${cashierAdminDateTime(selected.credit_expires_at)} 到期` : ''}</dd></div>
                  <div><dt>渠道流水号</dt><dd><span className="a-mono">{selected.trade_no || '—'}</span></dd></div>
                  <div><dt>渠道</dt><dd>{selected.provider_type || selected.provider || '—'}</dd></div>
                  <div><dt>积分流水</dt><dd>{selected.ledger_id ? <span className="a-mono">#{selected.ledger_id}</span> : '—'}</dd></div>
                  {selected.failure_reason ? <div><dt>失败原因</dt><dd className="lr-neg">{selected.failure_reason}</dd></div> : null}
                </dl>
              </section>
              <section className="a-drawer__section">
                <h3>支付时间线</h3>
                <PaymentTimeline order={selected} />
              </section>
              <div className="a-banner a-banner--info"><span>i</span><div>订单与积分流水一一对应。退款会生成反向流水并恢复对应余额桶。</div></div>
            </div>
          ) : null}
        </Drawer>
      ) : null}

      {refundDraft && selected ? (
        <Drawer
          stacked
          title="提交退款"
          description={`${selected.order_no} · ¥ ${selected.amount_cny}`}
          onClose={() => { setRefundDraft(null); setRefundError('') }}
          footer={(
            <>
              <button className="a-btn a-btn--sm a-btn--quiet" type="button" disabled={busy} onClick={() => { setRefundDraft(null); setRefundError('') }}>取消</button>
              <button className="a-btn a-btn--sm a-btn--danger" type="submit" form="order-refund-form" disabled={busy || !refundDraft.refund_trade_no.trim()}>{busy ? '提交中...' : '确认退款'}</button>
            </>
          )}
        >
          {refundError ? <InlineFeedback tone="danger" message={refundError} /> : null}
          <div className="a-banner a-banner--warn" style={{ marginBottom: 14 }}><span>!</span><div><strong style={{ fontSize: 13 }}>退款会生成反向积分流水</strong><div className="a-hint">请确认渠道退款流水号与金额；提交后以支付渠道结果为准。</div></div></div>
          <form id="order-refund-form" className="a-form" onSubmit={submitRefund}>
            <section className="a-form__section">
              <div className="a-form__head"><h3>退款信息</h3><p>渠道退款流水号由支付渠道返回，必填。</p></div>
              <Field label="渠道退款流水号"><input className="a-input" value={refundDraft.refund_trade_no} onChange={(event) => setRefundDraft({ ...refundDraft, refund_trade_no: event.target.value })} style={{ fontFamily: 'var(--font-admin-mono)' }} required /></Field>
              <Field label="退款金额（CNY）" hint="留空表示按订单金额全额退款。"><input className="a-input" value={refundDraft.refund_amount_cny} onChange={(event) => setRefundDraft({ ...refundDraft, refund_amount_cny: event.target.value })} inputMode="decimal" placeholder={selected.amount_cny} /></Field>
              <Field label="退款原因"><textarea className="a-textarea" value={refundDraft.reason} onChange={(event) => setRefundDraft({ ...refundDraft, reason: event.target.value })} placeholder="写明退款原因，便于审计追踪" /></Field>
            </section>
          </form>
        </Drawer>
      ) : null}
    </section>
  )
}

function PaymentTimeline({ order }: { order: PaymentOrder }) {
  const events: Array<{ time: string; title: string; detail: string }> = []
  events.push({ time: order.created_at, title: '创建订单', detail: `用户发起购买 · 金额 ¥${order.amount_cny}` })
  if (order.paid_at) events.push({ time: order.paid_at, title: '支付已确认', detail: `${cashierOrderPaymentLabel(order)} · 流水号 ${order.trade_no || '—'}` })
  if (order.credited_at) events.push({ time: order.credited_at, title: '积分到账', detail: `${order.points} + 赠送 ${order.bonus_points} 积分 · 流水 ${order.ledger_id ? `#${order.ledger_id}` : '—'}` })
  if (order.completed_at) events.push({ time: order.completed_at, title: '订单完成', detail: '支付与到账流程已完成' })
  if (order.refunded_at) events.push({ time: order.refunded_at, title: '退款完成', detail: `退款金额 ¥${order.refunded_amount_cny || order.amount_cny} · 流水 ${order.refund_trade_no || '—'}` })
  if (order.closed_at) events.push({ time: order.closed_at, title: '订单关闭', detail: order.failure_reason || '订单已关闭' })
  if (!order.paid_at && order.status === 'pending') events.push({ time: order.expires_at, title: '支付截止', detail: '超过该时间将自动关闭' })
  return (
    <div className="or-timeline">
      {events.map((event, index) => (
        <div className="or-event" key={`${event.title}-${index}`}>
          <time>{cashierAdminDateTime(event.time)}</time>
          <div><strong>{event.title}</strong><span>{event.detail}</span></div>
        </div>
      ))}
    </div>
  )
}

function numeric(value?: string | null) {
  const parsed = Number(value)
  return Number.isFinite(parsed) ? parsed : 0
}

function avatarText(order: PaymentOrder) {
  return (order.user_nickname || order.user_email || `#${order.user_id ?? '?'}`).trim().slice(0, 1).toUpperCase()
}