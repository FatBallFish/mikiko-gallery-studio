import { FormEvent, useEffect, useRef, useState } from 'react'
import type { AdminUser, LedgerEntry, RedeemCode } from '../../../shared/api-types'
import { cn } from '../../../shared/classnames'
import { adminApi } from '../../../shared/admin-api'
import { ActionMenu, Badge, Drawer, EmptyBlock, ErrorBlock, Field, InlineFeedback, LoadingBlock, MetricStrip, Modal, PageHeader, RefreshIconButton } from '../components'
import { ColumnDef, DataTable, FilterToolbar, ListPage, Pager } from '../ui/dataTable'
import {
  redeemBatchCreatePayload,
  redeemCodeRows,
  redeemCodesCSV,
  redeemExportFilename,
  redeemRedemptionRows,
  redeemStatusLabel,
  redeemStatusOptions,
} from './redeemRows'
import { createLatestListRequestGuard } from './listRefresh'
import './list-redesign.css'

type RedeemDialog =
  | { type: 'create' }
  | { type: 'batch' }
  | { type: 'redemptions'; row: RedeemCode }
  | { type: 'status'; row: RedeemCode; status: string }

const redeemPrimaryActionLabel = '查看核销'

export function RedeemPage({ onFeedback }: { onFeedback: (title: string, detail?: string) => void }) {
  const [rows, setRows] = useState<RedeemCode[]>([])
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(20)
  const [codeFilter, setCodeFilter] = useState('')
  const [appliedCode, setAppliedCode] = useState('')
  const [statusFilter, setStatusFilter] = useState('')
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [code, setCode] = useState('')
  const [rewardValue, setRewardValue] = useState('20.00000')
  const [validDays, setValidDays] = useState('30')
  const [maxRedemptions, setMaxRedemptions] = useState('1')
  const [batchCount, setBatchCount] = useState('20')
  const [batchRewardValue, setBatchRewardValue] = useState('20.00000')
  const [batchValidDays, setBatchValidDays] = useState('30')
  const [batchMaxRedemptions, setBatchMaxRedemptions] = useState('1')
  const [dialog, setDialog] = useState<RedeemDialog | null>(null)
  const [saving, setSaving] = useState(false)
  const [exporting, setExporting] = useState(false)
  const [redemptions, setRedemptions] = useState<LedgerEntry[]>([])
  const [redemptionsTotal, setRedemptionsTotal] = useState(0)
  const [redemptionsPage, setRedemptionsPage] = useState(1)
  const [redemptionsLoading, setRedemptionsLoading] = useState(false)
  const [redemptionsError, setRedemptionsError] = useState<string | null>(null)
  const [redemptionUsers, setRedemptionUsers] = useState<Map<string, AdminUser>>(new Map())
  const [dialogError, setDialogError] = useState('')
  const requestGuard = useRef(createLatestListRequestGuard()).current
  const [reloadKey, setReloadKey] = useState(0)

  const load = async () => {
    const request = requestGuard.begin()
    setLoading(true)
    setError(null)
    try {
      const result = await adminApi.listRedeemCodes({ page, page_size: pageSize, status: statusFilter || undefined, code: appliedCode.trim() || undefined })
      if (!requestGuard.isCurrent(request)) return
      setRows(result.items)
      setTotal(result.total)
    } catch (caught) {
      if (!requestGuard.isCurrent(request)) return
      setError(caught instanceof Error ? caught.message : '兑换码载入失败')
    } finally {
      if (!requestGuard.isCurrent(request)) return
      setLoading(false)
    }
  }

  useEffect(() => {
    void load()
    return () => requestGuard.invalidate()
  }, [page, pageSize, statusFilter, appliedCode, reloadKey])

  const loadRedemptions = async (row: RedeemCode, nextPage = 1) => {
    setRedemptionsLoading(true)
    setRedemptionsError(null)
    try {
      const result = await adminApi.listRedeemCodeRedemptions(row.id, nextPage, 20)
      setRedemptions(result.items)
      setRedemptionsTotal(result.total)
      setRedemptionsPage(nextPage)
    } catch (caught) {
      setRedemptionsError(caught instanceof Error ? caught.message : '核销记录载入失败')
    } finally {
      setRedemptionsLoading(false)
    }
  }

  const create = async (event: FormEvent) => {
    event.preventDefault()
    setSaving(true)
    setDialogError('')
    try {
      const created = await adminApi.createRedeemCode({
        code,
        status: 'available',
        reward_type: 'points',
        reward_value: rewardValue,
        valid_until: new Date(Date.now() + Number(validDays || 30) * 86400_000).toISOString(),
        max_redemptions: Number(maxRedemptions || 1),
        batch_id: Date.now(),
      })
      onFeedback('兑换码已创建', created.code)
      setCode('')
      setDialog(null)
      setReloadKey((value) => value + 1)
    } catch (caught) {
      setDialogError(caught instanceof Error ? caught.message : '兑换码创建失败')
    } finally {
      setSaving(false)
    }
  }

  const batchCreate = async (event: FormEvent) => {
    event.preventDefault()
    setSaving(true)
    setDialogError('')
    try {
      const payload = redeemBatchCreatePayload({
        count: batchCount,
        rewardValue: batchRewardValue,
        validDays: batchValidDays,
        maxRedemptions: batchMaxRedemptions,
      })
      const result = await adminApi.batchCreateRedeemCodes(payload)
      const exported = await adminApi.exportRedeemCodes({ batch_id: result.batch_id })
      downloadCodes(exported.items, result.batch_id)
      onFeedback('兑换码已批量生成', `${result.count} 个兑换码已生成，${exported.count} 个已下载`)
      setDialog(null)
      setReloadKey((value) => value + 1)
    } catch (caught) {
      const message = caught instanceof Error ? caught.message : '请检查批量生成参数'
      setDialogError(message)
      onFeedback('批量生成失败', message)
    } finally {
      setSaving(false)
    }
  }

  const exportCodes = async () => {
    setExporting(true)
    try {
      const exported = await adminApi.exportRedeemCodes({ status: statusFilter || undefined, code: appliedCode.trim() || undefined })
      if (!exported.items.length) {
        onFeedback('没有可导出的兑换码', '调整筛选或创建兑换码后再导出。')
        return
      }
      downloadCodes(exported.items, 'all')
      onFeedback('兑换码已导出', `${exported.count} 个兑换码已下载`)
    } catch (caught) {
      onFeedback('兑换码导出失败', caught instanceof Error ? caught.message : '请稍后重试')
    } finally {
      setExporting(false)
    }
  }

  const saveStatus = async () => {
    if (!dialog || dialog.type !== 'status') return
    setSaving(true)
    setDialogError('')
    try {
      await adminApi.updateRedeemCodeStatus(dialog.row.id, dialog.status)
      onFeedback('兑换码状态已更新', `${dialog.row.code} · ${redeemStatusLabel(dialog.status)}`)
      setDialog(null)
      setReloadKey((value) => value + 1)
    } catch (caught) {
      setDialogError(caught instanceof Error ? caught.message : '兑换码状态更新失败')
    } finally {
      setSaving(false)
    }
  }

  const openRedemptions = async (row: RedeemCode) => {
    setDialog({ type: 'redemptions', row })
    setRedemptions([])
    setRedemptionsTotal(0)
    setRedemptionsError(null)
    setRedemptionsPage(1)
    await loadRedemptions(row, 1)
    if (!redemptionUsers.size) {
      try {
        const page = await adminApi.listUsersPage('', 1, 200)
        setRedemptionUsers(new Map(page.items.map((user) => [String(user.id), user])))
      } catch {
        // Mapping is best-effort; records still show the raw user id.
      }
    }
  }

  const openDialog = (next: RedeemDialog) => {
    setDialogError('')
    setDialog(next)
  }

  if (loading && !rows.length) return <LoadingBlock label="载入兑换码" />
  if (error && !rows.length) return <ErrorBlock message={error} onRetry={load} />

  const available = rows.filter((row) => row.status === 'available').length
  const redeemed = rows.filter((row) => row.status === 'redeemed').length
  const disabled = rows.filter((row) => row.status === 'disabled' || row.status === 'expired').length
  const metrics = [
    { label: '兑换码总数', value: String(total), trend: `当前页 ${rows.length} 个`, tone: 'neutral' as const },
    { label: '当前页可用', value: String(available), trend: '可被用户兑换', tone: 'good' as const },
    { label: '当前页已核销', value: String(redeemed), trend: '已完成兑换', tone: 'neutral' as const },
    { label: '当前页停用 / 过期', value: String(disabled), trend: '不可继续兑换', tone: disabled ? 'warn' as const : 'neutral' as const },
  ]

  return (
    <section className="a-page-stack">
      <PageHeader
        eyebrow="Redeem Codes"
        title="兑换码"
        description="单码创建或批量生成，核销记录在抽屉中查看。状态与使用进度直接展示，停用不再需要翻二级页面。"
        primaryAction={(
          <div className="lr-row">
            <button className="a-btn a-btn--sm" type="button" onClick={() => openDialog({ type: 'batch' })}>批量生成</button>
            <button className="a-btn a-btn--sm a-btn--primary" type="button" onClick={() => openDialog({ type: 'create' })}>创建兑换码</button>
          </div>
        )}
        secondaryActions={(
          <div className="lr-row">
            <ActionMenu actions={[{ id: 'export-codes', label: exporting ? '导出中...' : '导出兑换码', run: () => { if (!exporting) void exportCodes() } }]} />
            <RefreshIconButton label="刷新兑换码列表" refreshing={loading} onClick={() => void load()} />
          </div>
        )}
      />
      {error ? <InlineFeedback tone="danger" message={`兑换码刷新失败：${error}`} /> : null}
      {loading && rows.length ? <InlineFeedback tone="neutral" message="正在刷新兑换码，当前数据会保留到新结果返回。" /> : null}
      <MetricStrip metrics={metrics} />
      <ListPage
        resultSummary={<><h2>兑换码列表</h2><p>点击「查看核销」查看领取用户与核销记录。</p></>}
        actions={<span className="a-hint">按创建时间倒序</span>}
        filters={(
          <form onSubmit={(event) => { event.preventDefault(); setPage(1); setAppliedCode(codeFilter); setReloadKey((value) => value + 1) }}>
            <FilterToolbar
              fields={[
                { key: 'code', label: '兑换码 / 批次', primary: true, control: <input className="a-input" value={codeFilter} onChange={(event) => setCodeFilter(event.target.value)} placeholder="REDEEM-XXXX / batch_..." /> },
                { key: 'status', label: '状态', primary: true, control: <select className="a-select" value={statusFilter} onChange={(event) => { setPage(1); setStatusFilter(event.target.value) }}>{[{ value: '', label: '全部状态' }, ...redeemStatusOptions].map((option) => <option key={option.value || 'all'} value={option.value}>{option.label}</option>)}</select> },
              ]}
              actions={<button type="submit" className="a-btn a-btn--sm a-btn--primary">查询</button>}
              resultSummary={`共 ${total} 个兑换码 · 当前显示 ${rows.length} 个`}
            />
          </form>
        )}
        pagination={<Pager page={page} pageSize={pageSize} total={total} onChange={setPage} onPageSizeChange={(size) => { setPageSize(size); setPage(1) }} />}
      >
        {!rows.length ? <EmptyBlock title="暂无兑换码" detail="创建一个兑换码后可在用户侧兑换。" /> : (
          <DataTable
            columns={redeemColumns(openRedemptions, (source, status) => openDialog({ type: 'status', row: source, status }))}
            rows={rows}
            rowKey={(row) => row.id}
            minWidth={1120}
          />
        )}
      </ListPage>

      {dialog?.type === 'create' ? (
        <Drawer
          title="创建兑换码"
          description="单码创建 · 奖励为积分"
          onClose={() => setDialog(null)}
          footer={(
            <>
              <button className="a-btn a-btn--sm a-btn--quiet" type="button" disabled={saving} onClick={() => setDialog(null)}>取消</button>
              <button className="a-btn a-btn--sm a-btn--primary" type="submit" form="redeem-create-form" disabled={saving}>{saving ? '保存中...' : '创建'}</button>
            </>
          )}
        >
          {dialogError ? <InlineFeedback tone="danger" message={dialogError} /> : null}
          <form id="redeem-create-form" className="a-form" onSubmit={create}>
            <section className="a-form__section">
              <div className="a-form__head"><h3>兑换码信息</h3><p>留空代码时由服务端自动生成。</p></div>
              <Field label="兑换码" hint="仅允许大写字母、数字与连字符。">
                <div className="lr-row">
                  <input className="a-input" value={code} onChange={(event) => setCode(event.target.value)} placeholder="REDEEM-XXXX-XXXX" style={{ fontFamily: 'var(--font-admin-mono)' }} />
                  <button className="a-btn a-btn--sm" type="button" onClick={() => setCode(randomRedeemCode())}>随机生成</button>
                </div>
              </Field>
              <Field label="奖励积分"><input className="a-input" value={rewardValue} onChange={(event) => setRewardValue(event.target.value)} inputMode="decimal" /></Field>
              <div className="lr-form-pair">
                <Field label="有效天数"><input className="a-input" type="number" min="1" value={validDays} onChange={(event) => setValidDays(event.target.value)} /></Field>
                <Field label="每码核销次数"><input className="a-input" type="number" min="1" value={maxRedemptions} onChange={(event) => setMaxRedemptions(event.target.value)} /></Field>
              </div>
            </section>
          </form>
        </Drawer>
      ) : null}

      {dialog?.type === 'batch' ? (
        <Drawer
          title="批量生成兑换码"
          description="生成后自动下载本批次 CSV"
          onClose={() => setDialog(null)}
          footer={(
            <>
              <button className="a-btn a-btn--sm a-btn--quiet" type="button" disabled={saving} onClick={() => setDialog(null)}>取消</button>
              <button className="a-btn a-btn--sm a-btn--primary" type="submit" form="redeem-batch-form" disabled={saving}>{saving ? '生成中...' : '生成并下载'}</button>
            </>
          )}
        >
          {dialogError ? <InlineFeedback tone="danger" message={dialogError} /> : null}
          <form id="redeem-batch-form" className="a-form" onSubmit={batchCreate}>
            <section className="a-form__section">
              <div className="a-form__head"><h3>批次参数</h3><p>单批最多 100 个兑换码。</p></div>
              <Field label="数量"><input className="a-input" type="number" min="1" max="100" value={batchCount} onChange={(event) => setBatchCount(event.target.value)} required /></Field>
              <Field label="每个奖励积分"><input className="a-input" value={batchRewardValue} onChange={(event) => setBatchRewardValue(event.target.value)} placeholder="20.00000" required /></Field>
              <div className="lr-form-pair">
                <Field label="有效天数"><input className="a-input" type="number" min="1" value={batchValidDays} onChange={(event) => setBatchValidDays(event.target.value)} required /></Field>
                <Field label="每码核销次数"><input className="a-input" type="number" min="1" value={batchMaxRedemptions} onChange={(event) => setBatchMaxRedemptions(event.target.value)} required /></Field>
              </div>
            </section>
            <div className="a-banner a-banner--info"><span>i</span><div>批量兑换码创建后立即进入「可用」状态；未使用的码可随时停用。</div></div>
          </form>
        </Drawer>
      ) : null}

      {dialog?.type === 'status' ? (
        <Modal
          title={dialog.status === 'disabled' ? '停用兑换码' : '重新启用兑换码'}
          detail={dialog.row.code}
          onClose={() => setDialog(null)}
          footer={<><button className="a-btn a-btn--sm a-btn--quiet" type="button" disabled={saving} onClick={() => setDialog(null)}>取消</button><button className={cn('a-btn a-btn--sm', dialog.status === 'disabled' ? 'a-btn--danger' : 'a-btn--primary')} type="button" disabled={saving} onClick={() => void saveStatus()}>{saving ? '保存中...' : '确认'}</button></>}
        >
          {dialogError ? <InlineFeedback tone="danger" message={dialogError} /> : null}
          <p className="a-hint">{dialog.status === 'disabled' ? '停用后该兑换码不可继续核销，已核销记录保留。' : '重新启用后兑换码可在有效期内继续核销。'}</p>
        </Modal>
      ) : null}

      {dialog?.type === 'redemptions' ? (
        <Drawer
          title="核销记录"
          description={`${dialog.row.code} · ${dialog.row.redeemed_count} / ${dialog.row.max_redemptions} · ${redeemStatusLabel(dialog.row.status)}`}
          headerAside={<Badge tone={redeemCodeRows([dialog.row])[0]?.statusTone}>{redeemCodeRows([dialog.row])[0]?.statusLabel}</Badge>}
          onClose={() => setDialog(null)}
          footer={(
            <>
              <button className="a-btn a-btn--sm" type="button" onClick={() => void exportCodes()}>导出兑换码</button>
              <div className="lr-spacer" />
              <button className="a-btn a-btn--sm a-btn--primary" type="button" onClick={() => setDialog(null)}>关闭</button>
            </>
          )}
        >
          <div className="a-metrics lr-metrics-3">
            <div className="a-metric"><div className="a-metric__label">已核销</div><div className="a-metric__value" style={{ fontSize: 22 }}>{dialog.row.redeemed_count}</div></div>
            <div className="a-metric"><div className="a-metric__label">剩余</div><div className="a-metric__value" style={{ fontSize: 22 }}>{Math.max(0, dialog.row.max_redemptions - dialog.row.redeemed_count)}</div></div>
            <div className="a-metric"><div className="a-metric__label">发放积分</div><div className="a-metric__value" style={{ fontSize: 22 }}>{formatPoints(dialog.row.redeemed_count * numeric(dialog.row.reward_value))}</div></div>
          </div>
          {redemptionsLoading ? <LoadingBlock label="载入核销记录" /> : null}
          {redemptionsError ? <ErrorBlock message={redemptionsError} onRetry={() => void loadRedemptions(dialog.row, redemptionsPage)} /> : null}
          {!redemptionsLoading && !redemptionsError && !redemptions.length ? <EmptyBlock variant="inline" title="暂无核销记录" detail="该兑换码尚未被用户兑换。" /> : null}
          {!redemptionsLoading && !redemptionsError && redemptions.length ? (
            <>
              <RedemptionTable items={redemptions} users={redemptionUsers} />
              <div className="a-pager" style={{ border: '1px solid var(--a-line)', borderRadius: 9 }}>
                <Pager page={redemptionsPage} pageSize={20} total={redemptionsTotal} onChange={(next) => void loadRedemptions(dialog.row, next)} />
              </div>
            </>
          ) : null}
          <p className="a-hint">仅展示该兑换码的核销记录，最多按页加载。</p>
        </Drawer>
      ) : null}
    </section>
  )
}

function RedemptionTable({ items, users }: { items: LedgerEntry[]; users: Map<string, AdminUser> }) {
  const rows = redeemRedemptionRows(items)
  return (
    <div className="a-table-wrap" style={{ border: '1px solid var(--a-line)', borderRadius: 9 }}>
      <table className="a-table" style={{ minWidth: 620 }}>
        <thead><tr><th>用户</th><th>类型</th><th className="is-num">积分</th><th className="is-num">余额</th><th>来源</th><th>时间</th></tr></thead>
        <tbody>
          {rows.map((row, index) => {
            const user = items[index]?.user_id ? users.get(String(items[index].user_id)) : undefined
            return (
              <tr key={String(row.id)}>
                <td>
                  {user ? (
                    <span className="a-cell-entity">
                      <span className="a-cell-entity__avatar">{avatarText(user)}</span>
                      <span className="a-cell-entity__copy">
                        <span className="a-cell-entity__name">{user.display_name || user.nickname || user.email}</span>
                        <span className="a-cell-entity__meta">{user.email}</span>
                      </span>
                    </span>
                  ) : <span className="lr-entity-fallback">{row.userLabel}</span>}
                </td>
                <td>{row.typeLabel}</td>
                <td className={cn('is-num', row.amountTone === 'debit' ? 'lr-neg' : 'lr-pos')}>{row.amount}</td>
                <td className="is-num">{row.balanceAfter}</td>
                <td className="is-muted">{row.sourceLabel}</td>
                <td className="is-muted">{row.occurredAt}</td>
              </tr>
            )
          })}
        </tbody>
      </table>
    </div>
  )
}

function redeemColumns(
  onOpenRedemptions: (row: RedeemCode) => void,
  onStatus: (source: RedeemCode, status: string) => void,
): ColumnDef<RedeemCode>[] {
  return [
    {
      key: 'code',
      title: '兑换码',
      width: 'minmax(200px,2fr)',
      render: (source) => {
        const row = redeemCodeRows([source])[0]
        return <span className="lr-cell"><strong><span className="rd-code">{row.code}</span></strong><span>批次 {row.batchLabel} · {source.max_redemptions > 1 ? `批量 ${source.max_redemptions}` : '单码'}</span></span>
      },
    },
    {
      key: 'reward',
      title: '奖励',
      width: 'minmax(110px,.9fr)',
      align: 'right',
      kind: 'number',
      render: (source) => <span className="lr-num">{formatPoints(numeric(source.reward_value))}</span>,
    },
    {
      key: 'valid',
      title: '有效期',
      width: 'minmax(150px,1.1fr)',
      render: (source) => <span className="lr-muted">{redeemCodeRows([source])[0].validUntilLabel} 到期</span>,
    },
    {
      key: 'usage',
      title: '使用情况',
      width: 'minmax(150px,1.1fr)',
      render: (source) => {
        const row = redeemCodeRows([source])[0]
        const progress = couponProgress(source)
        return (
          <div className="lr-progress">
            <div className="lr-progress__track" aria-label={`核销进度 ${Math.round(progress)}%`}><i style={{ width: `${progress}%` }} /></div>
            <span className="lr-progress__meta">{row.redeemedLabel}</span>
          </div>
        )
      },
    },
    {
      key: 'status',
      title: '状态',
      width: 'minmax(90px,.8fr)',
      render: (source) => {
        const row = redeemCodeRows([source])[0]
        return <Badge tone={row.statusTone}>{row.statusLabel}</Badge>
      },
    },
    {
      key: 'created',
      title: '创建时间',
      width: 'minmax(140px,1fr)',
      render: (source) => <span className="lr-muted">{formatDateTime(source.created_at)}</span>,
    },
    {
      key: 'actions',
      title: '操作',
      width: 'minmax(190px,1.2fr)',
      align: 'right',
      render: (source) => {
        const row = redeemCodeRows([source])[0]
        const action = row.statusAction
        return (
          <div className="a-row-actions">
            <button className="a-row-btn a-row-btn--primary" type="button" onClick={() => void onOpenRedemptions(source)}>{redeemPrimaryActionLabel}</button>
            {action ? <ActionMenu actions={[{ id: 'change-status', label: action.label, run: () => onStatus(source, action.status) }]} /> : null}
          </div>
        )
      },
    },
  ]
}

function couponProgress(row: RedeemCode) {
  if (!row.max_redemptions || row.max_redemptions <= 0) return 100
  return Math.max(0, Math.min(100, (row.redeemed_count / row.max_redemptions) * 100))
}

function numeric(value?: string | null) {
  const parsed = Number(value)
  return Number.isFinite(parsed) ? parsed : 0
}

function formatPoints(value: number) {
  return value.toLocaleString('zh-CN', { maximumFractionDigits: 5 })
}

function formatDateTime(value?: string | null) {
  const input = value ?? ''
  const match = input.match(/^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2})/)
  if (!match) return input || '-'
  return `${match[1]}/${match[2]}/${match[3]} ${match[4]}:${match[5]}`
}

function avatarText(user: AdminUser) {
  return (user.display_name || user.nickname || user.email || '?').trim().slice(0, 1).toUpperCase()
}

function randomRedeemCode() {
  const alphabet = 'ABCDEFGHJKLMNPQRSTUVWXYZ23456789'
  const block = (length: number) => Array.from({ length }, () => alphabet[Math.floor(Math.random() * alphabet.length)]).join('')
  return `REDEEM-${block(4)}-${block(4)}`
}

function downloadCodes(codes: RedeemCode[], batchID: number | string) {
  const blob = new Blob([`\uFEFF${redeemCodesCSV(codes)}`], { type: 'text/csv;charset=utf-8' })
  const url = URL.createObjectURL(blob)
  const anchor = document.createElement('a')
  anchor.href = url
  anchor.download = redeemExportFilename(batchID)
  anchor.click()
  URL.revokeObjectURL(url)
}