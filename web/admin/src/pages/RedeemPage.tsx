import { FormEvent, useEffect, useState } from 'react'
import type { LedgerEntry, RedeemCode } from '../../../shared/api-types'
import { cn } from '../../../shared/classnames'
import { adminApi } from '../../../shared/admin-api'
import { ActionMenu, Badge, EmptyBlock, ErrorBlock, Field, InlineFeedback, LoadingBlock, Modal, PageHeader } from '../components'
import { adminButton, adminPage } from '../ui/classes'
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

type RedeemDialog =
  | { type: 'create' }
  | { type: 'batch' }
  | { type: 'redemptions'; row: RedeemCode }
  | { type: 'status'; row: RedeemCode; status: string }

const redeemPrimaryActionLabel = '查看核销'

const redeemClasses = {
  actionRow: 'flex flex-wrap items-center gap-2',
  codeCell: 'min-w-0 overflow-hidden text-ellipsis whitespace-nowrap font-mono text-sm font-extrabold text-[var(--text)]',
  textCell: 'min-w-0 overflow-hidden text-ellipsis whitespace-nowrap text-[var(--soft)]',
  amountCredit: 'min-w-0 overflow-hidden text-ellipsis whitespace-nowrap text-[var(--accent)]',
  rewardValue: 'font-mono text-sm font-semibold text-[var(--text)]',
  rewardUnit: 'text-xs font-medium text-[var(--muted-strong)]',
  progressTrack: 'h-1.5 w-28 overflow-hidden rounded-full bg-[var(--canvas)]',
  progressFill: 'h-full rounded-full bg-[var(--accent)]',
  progressMeta: 'mb-1.5 block text-xs font-bold text-[var(--muted)]',
}

export function RedeemPage({ onFeedback }: { onFeedback: (title: string, detail?: string) => void }) {
  const [rows, setRows] = useState<RedeemCode[]>([])
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(20)
  const [statusFilter, setStatusFilter] = useState('')
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [code, setCode] = useState('')
  const [rewardValue, setRewardValue] = useState('20.00000')
  const [batchCount, setBatchCount] = useState('20')
  const [batchRewardValue, setBatchRewardValue] = useState('20.00000')
  const [batchValidDays, setBatchValidDays] = useState('30')
  const [batchMaxRedemptions, setBatchMaxRedemptions] = useState('1')
  const [dialog, setDialog] = useState<RedeemDialog | null>(null)
  const [saving, setSaving] = useState(false)
  const [exporting, setExporting] = useState(false)
  const [redemptions, setRedemptions] = useState<LedgerEntry[]>([])
  const [redemptionsTotal, setRedemptionsTotal] = useState(0)
  const [redemptionsLoading, setRedemptionsLoading] = useState(false)
  const [redemptionsError, setRedemptionsError] = useState<string | null>(null)
  const [dialogError, setDialogError] = useState('')

  const load = async () => {
    setLoading(true)
    setError(null)
    try {
      const result = await adminApi.listRedeemCodes({ page, page_size: pageSize, status: statusFilter || undefined })
      setRows(result.items)
      setTotal(result.total)
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : '兑换码载入失败')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => { void load() }, [page, pageSize, statusFilter])

  const create = async (event: FormEvent) => {
    event.preventDefault()
    setSaving(true)
    setDialogError('')
    try {
      await adminApi.createRedeemCode({
        code,
        status: 'available',
        reward_type: 'points',
        reward_value: rewardValue,
        valid_until: new Date(Date.now() + 30 * 86400_000).toISOString(),
        max_redemptions: 1,
        batch_id: Date.now(),
      })
      onFeedback('兑换码已创建', code)
      setCode('')
      setDialog(null)
      await load()
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
      await load()
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
      const exported = await adminApi.exportRedeemCodes({})
      if (!exported.items.length) {
        onFeedback('没有可导出的兑换码', '创建兑换码后再导出。')
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
      await load()
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
    setRedemptionsLoading(true)
    try {
      const result = await adminApi.listRedeemCodeRedemptions(row.id, 1, 20)
      setRedemptions(result.items)
      setRedemptionsTotal(result.total)
    } catch (caught) {
      setRedemptionsError(caught instanceof Error ? caught.message : '核销记录载入失败')
    } finally {
      setRedemptionsLoading(false)
    }
  }

  const openDialog = (next: RedeemDialog) => {
    setDialogError('')
    setDialog(next)
  }

  if (loading) return <LoadingBlock label="载入兑换码" />
  if (error) return <ErrorBlock message={error} onRetry={load} />

  return (
    <section className={adminPage.stack}>
      <PageHeader
        title="兑换码"
        description="创建、停用、批量生成与核销记录全部连接真实后台接口。"
        actions={(
          <div className={redeemClasses.actionRow}>
            <button className={cn(adminButton.base, adminButton.primary)} type="button" onClick={() => openDialog({ type: 'create' })}>创建兑换码</button>
            <ActionMenu actions={[
              { id: 'batch-create', label: '批量生成', run: () => openDialog({ type: 'batch' }) },
              { id: 'export-codes', label: exporting ? '导出中...' : '导出兑换码', run: () => { if (!exporting) void exportCodes() } },
            ]} />
          </div>
        )}
      />
      <ListPage
        filters={(
          <FilterToolbar
            fields={[
              { key: 'status', label: '状态', primary: true, control: <select value={statusFilter} onChange={(event) => { setPage(1); setStatusFilter(event.target.value) }}>{[{ value: '', label: '全部' }, ...redeemStatusOptions].map((option) => <option key={option.value || 'all'} value={option.value}>{option.label}</option>)}</select> },
            ]}
            resultSummary={`共 ${total} 个兑换码 · 当前显示 ${rows.length} 个`}
          />
        )}
        pagination={<Pager page={page} pageSize={pageSize} total={total} onChange={setPage} onPageSizeChange={(size) => { setPageSize(size); setPage(1) }} />}
      >
          {!rows.length ? <EmptyBlock title="暂无兑换码" detail="创建一个兑换码后可在用户侧兑换。" /> : (
            <DataTable
              columns={redeemColumns(openRedemptions, (source, status) => openDialog({ type: 'status', row: source, status }))}
              rows={rows}
              rowKey={(row) => row.id}
            />
          )}
      </ListPage>
      {dialog?.type === 'create' ? (
        <Modal title="创建兑换码" detail="创建后可在用户工作台兑换积分。" onClose={() => setDialog(null)} footer={<><button className={cn(adminButton.base, adminButton.ghost)} type="button" disabled={saving} onClick={() => setDialog(null)}>取消</button><button className={cn(adminButton.base, adminButton.primary)} type="submit" form="redeem-create-form" disabled={saving}>{saving ? '保存中...' : '保存'}</button></>}>
          {dialogError ? <InlineFeedback tone="danger" message={dialogError} /> : null}
          <form id="redeem-create-form" className={adminPage.formGrid} onSubmit={create}>
            <Field label="兑换码"><input value={code} onChange={(event) => setCode(event.target.value)} placeholder="新兑换码" required /></Field>
            <Field label="奖励积分"><input value={rewardValue} onChange={(event) => setRewardValue(event.target.value)} placeholder="奖励积分" /></Field>
          </form>
        </Modal>
      ) : null}
      {dialog?.type === 'batch' ? (
        <Modal title="批量生成兑换码" detail="生成后会自动下载本批次 CSV，便于投放和留档。" onClose={() => setDialog(null)} footer={<><button className={cn(adminButton.base, adminButton.ghost)} type="button" disabled={saving} onClick={() => setDialog(null)}>取消</button><button className={cn(adminButton.base, adminButton.primary)} type="submit" form="redeem-batch-form" disabled={saving}>{saving ? '生成中...' : '生成并下载'}</button></>}>
          {dialogError ? <InlineFeedback tone="danger" message={dialogError} /> : null}
          <form id="redeem-batch-form" className={adminPage.formGrid} onSubmit={batchCreate}>
            <Field label="生成数量"><input type="number" min="1" max="100" value={batchCount} onChange={(event) => setBatchCount(event.target.value)} required /></Field>
            <Field label="奖励积分"><input value={batchRewardValue} onChange={(event) => setBatchRewardValue(event.target.value)} placeholder="20.00000" required /></Field>
            <Field label="有效天数"><input type="number" min="1" value={batchValidDays} onChange={(event) => setBatchValidDays(event.target.value)} required /></Field>
            <Field label="每码可核销次数"><input type="number" min="1" value={batchMaxRedemptions} onChange={(event) => setBatchMaxRedemptions(event.target.value)} required /></Field>
          </form>
        </Modal>
      ) : null}
      {dialog?.type === 'status' ? (
        <Modal title="变更兑换码状态" detail={dialog.row.code} onClose={() => setDialog(null)} footer={<><button className={cn(adminButton.base, adminButton.ghost)} type="button" disabled={saving} onClick={() => setDialog(null)}>取消</button><button className={cn(adminButton.base, adminButton.primary)} type="button" disabled={saving} onClick={() => void saveStatus()}>{saving ? '保存中...' : '保存'}</button></>}>
          {dialogError ? <InlineFeedback tone="danger" message={dialogError} /> : null}
          <Field label="新状态"><select value={dialog.status} onChange={(event) => setDialog({ ...dialog, status: event.target.value })}>{redeemStatusOptions.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}</select></Field>
        </Modal>
      ) : null}
      {dialog?.type === 'redemptions' ? (
        <Modal title="核销记录" detail={`${dialog.row.code} · 共 ${redemptionsTotal} 条`} onClose={() => setDialog(null)} footer={<button className={cn(adminButton.base, adminButton.primary)} type="button" onClick={() => setDialog(null)}>关闭</button>}>
          {redemptionsLoading ? <LoadingBlock label="载入核销记录" /> : null}
          {redemptionsError ? <ErrorBlock message={redemptionsError} onRetry={() => void openRedemptions(dialog.row)} /> : null}
          {!redemptionsLoading && !redemptionsError && !redemptions.length ? <EmptyBlock title="暂无核销记录" detail="该兑换码尚未被用户兑换。" /> : null}
          {!redemptionsLoading && !redemptionsError && redemptions.length ? (
            <DataTable columns={redemptionColumns()} rows={redeemRedemptionRows(redemptions)} rowKey={(row) => row.id} />
          ) : null}
        </Modal>
      ) : null}
    </section>
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
      width: 'minmax(180px,2fr)',
      render: (source) => {
        const row = redeemCodeRows([source])[0]
        return (
          <div className="flex min-w-0 flex-col gap-1">
            <span className={redeemClasses.codeCell}>{row.code}</span>
            <span className="text-xs font-medium text-[var(--muted-strong)]">批次 {row.batchLabel}</span>
          </div>
        )
      },
    },
    {
      key: 'reward',
      title: '奖励值',
      width: 'minmax(80px,0.8fr)',
      render: (source) => (
        <div className="flex items-baseline gap-1.5">
          <span className={redeemClasses.rewardValue}>{source.reward_value}</span>
          <span className={redeemClasses.rewardUnit}>POINTS</span>
        </div>
      ),
    },
    {
      key: 'valid',
      title: '有效期',
      width: 'minmax(120px,1.2fr)',
      render: (source) => <span className={redeemClasses.textCell}>{redeemCodeRows([source])[0].validUntilLabel}</span>,
    },
    {
      key: 'usage',
      title: '使用情况',
      width: 'minmax(120px,1fr)',
      render: (source) => {
        const row = redeemCodeRows([source])[0]
        const progress = couponProgress(source)
        return (
          <div className="flex flex-col gap-1">
            <span className={redeemClasses.progressMeta}>{row.redeemedLabel}</span>
            <div className={redeemClasses.progressTrack} aria-label={`核销进度 ${Math.round(progress)}%`}>
              <div className={redeemClasses.progressFill} style={{ width: `${progress}%` }} />
            </div>
          </div>
        )
      },
    },
    {
      key: 'status',
      title: '状态',
      width: 'minmax(70px,0.7fr)',
      render: (source) => {
        const row = redeemCodeRows([source])[0]
        return <Badge tone={row.statusTone}>{row.statusLabel}</Badge>
      },
    },
    {
      key: 'actions',
      title: '操作',
      width: 'minmax(170px,1.2fr)',
      align: 'right',
      render: (source) => {
        const row = redeemCodeRows([source])[0]
        const action = row.statusAction
        return (
          <div className={redeemClasses.actionRow}>
            <button type="button" className={cn(adminButton.base, adminButton.primary, adminButton.small)} onClick={() => void onOpenRedemptions(source)}>{redeemPrimaryActionLabel}</button>
            {action ? (
              <ActionMenu actions={[{ id: 'change-status', label: action.label, run: () => onStatus(source, action.status) }]} />
            ) : null}
          </div>
        )
      },
    },
  ]
}

function redemptionColumns(): ColumnDef<ReturnType<typeof redeemRedemptionRows>[number]>[] {
  return [
    { key: 'user', title: '用户', width: 'minmax(110px,1fr)', render: (row) => <strong className={redeemClasses.codeCell}>{row.userLabel}</strong> },
    { key: 'type', title: '类型', width: 'minmax(120px,1fr)', render: (row) => <span className={redeemClasses.textCell}>{row.typeLabel}</span> },
    { key: 'amount', title: '积分', width: 'minmax(90px,.8fr)', kind: 'number', align: 'right', render: (row) => <span className={row.amountTone === 'debit' ? 'text-[var(--red)]' : redeemClasses.amountCredit}>{row.amount}</span> },
    { key: 'balance', title: '余额', width: 'minmax(90px,.8fr)', kind: 'number', align: 'right', render: (row) => <span className={redeemClasses.textCell}>{row.balanceAfter}</span> },
    { key: 'source', title: '来源', width: 'minmax(110px,1fr)', render: (row) => <span className={redeemClasses.textCell}>{row.sourceLabel}</span> },
    { key: 'time', title: '时间', width: 'minmax(140px,1.2fr)', render: (row) => <span className={redeemClasses.textCell}>{row.occurredAt}</span> },
  ]
}

function couponProgress(row: RedeemCode) {
  if (!row.max_redemptions || row.max_redemptions <= 0) return 100
  return Math.max(0, Math.min(100, (row.redeemed_count / row.max_redemptions) * 100))
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
