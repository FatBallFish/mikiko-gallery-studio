import { FormEvent, useEffect, useState } from 'react'
import type { LedgerEntry, RedeemCode } from '../../../shared/api-types'
import { adminApi } from '../../../shared/admin-api'
import { Badge, EmptyBlock, ErrorBlock, Field, LoadingBlock, Modal, PageHeader } from '../components'
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

export function RedeemPage({ onFeedback }: { onFeedback: (title: string, detail?: string) => void }) {
  const [rows, setRows] = useState<RedeemCode[]>([])
  const [page, setPage] = useState(1)
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

  const load = async () => {
    setLoading(true)
    setError(null)
    try {
      const result = await adminApi.listRedeemCodes({ page, page_size: 20 })
      setRows(result.items)
      setTotal(result.total)
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : '兑换码载入失败')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => { void load() }, [page])

  const create = async (event: FormEvent) => {
    event.preventDefault()
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
  }

  const batchCreate = async (event: FormEvent) => {
    event.preventDefault()
    setSaving(true)
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
      onFeedback('批量生成失败', caught instanceof Error ? caught.message : '请检查批量生成参数')
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
    try {
      await adminApi.updateRedeemCodeStatus(dialog.row.id, dialog.status)
      onFeedback('兑换码状态已更新', `${dialog.row.code} · ${redeemStatusLabel(dialog.status)}`)
      setDialog(null)
      await load()
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

  if (loading) return <LoadingBlock label="载入兑换码" />
  if (error) return <ErrorBlock message={error} onRetry={load} />

  return (
    <section className="page-stack">
      <PageHeader
        eyebrow="Redeem"
        title="兑换码管理"
        detail="创建、停用、批量生成与核销记录全部连接真实后台接口。"
        actions={(
          <div className="row-actions buttons">
            <button className="ghost" type="button" disabled={exporting} onClick={() => void exportCodes()}>{exporting ? '导出中...' : '导出兑换码'}</button>
            <button className="ghost" type="button" disabled={exporting} onClick={() => setDialog({ type: 'batch' })}>批量生成</button>
            <button className="btn primary" type="button" onClick={() => setDialog({ type: 'create' })}>创建兑换码</button>
          </div>
        )}
      />
      <section className="pg-admin-card ops-surface full-main">
        <section className="main-lane table-lane no-divider">
          <div className="card-header lane-head compact"><span>第 {page} 页 / 共 {total} 条</span><div className="row-actions buttons"><button className="ghost small" type="button" disabled={page <= 1} onClick={() => setPage((value) => value - 1)}>上一页</button><button className="ghost small" type="button" disabled={page * 20 >= total} onClick={() => setPage((value) => value + 1)}>下一页</button></div></div>
          {!rows.length ? <EmptyBlock title="暂无兑换码" detail="创建一个兑换码后可在用户侧兑换。" /> : (
            <div className="admin-data-grid redeem-grid">
              <div className="table-head"><span>兑换码</span><span>状态</span><span>奖励</span><span>批次</span><span>有效期</span><span>核销</span><span>操作</span></div>
              {redeemCodeRows(rows).map((row) => {
                const action = row.statusAction
                return (
                  <div key={row.id} className="table-row">
                    <strong>{row.code}</strong>
                    <Badge tone={row.statusTone}>{row.statusLabel}</Badge>
                    <span>{row.rewardLabel}</span>
                    <span>{row.batchLabel}</span>
                    <span>{row.validUntilLabel}</span>
                    <span>{row.redeemedLabel}</span>
                    <div className="row-actions buttons">
                      <button type="button" className="ghost small" onClick={() => {
                        const source = rows.find((item) => item.id === row.id)
                        if (source) void openRedemptions(source)
                      }}>核销记录</button>
                      {action ? (
                        <button type="button" className="ghost small" onClick={() => {
                          const source = rows.find((item) => item.id === row.id)
                          if (source) setDialog({ type: 'status', row: source, status: action.status })
                        }}>{action.label}</button>
                      ) : <span>无需操作</span>}
                    </div>
                  </div>
                )
              })}
            </div>
          )}
        </section>
      </section>
      {dialog?.type === 'create' ? (
        <Modal title="创建兑换码" detail="创建后可在用户工作台兑换积分。" onClose={() => setDialog(null)} footer={<><button className="ghost" type="button" onClick={() => setDialog(null)}>取消</button><button className="btn primary" type="submit" form="redeem-create-form">保存</button></>}>
          <form id="redeem-create-form" className="form-grid" onSubmit={create}>
            <Field label="兑换码"><input value={code} onChange={(event) => setCode(event.target.value)} placeholder="新兑换码" required /></Field>
            <Field label="奖励积分"><input value={rewardValue} onChange={(event) => setRewardValue(event.target.value)} placeholder="奖励积分" /></Field>
          </form>
        </Modal>
      ) : null}
      {dialog?.type === 'batch' ? (
        <Modal title="批量生成兑换码" detail="生成后会自动下载本批次 CSV，便于投放和留档。" onClose={() => setDialog(null)} footer={<><button className="ghost" type="button" disabled={saving} onClick={() => setDialog(null)}>取消</button><button className="btn primary" type="submit" form="redeem-batch-form" disabled={saving}>{saving ? '生成中...' : '生成并下载'}</button></>}>
          <form id="redeem-batch-form" className="form-grid" onSubmit={batchCreate}>
            <Field label="生成数量"><input type="number" min="1" max="100" value={batchCount} onChange={(event) => setBatchCount(event.target.value)} required /></Field>
            <Field label="奖励积分"><input value={batchRewardValue} onChange={(event) => setBatchRewardValue(event.target.value)} placeholder="20.00000" required /></Field>
            <Field label="有效天数"><input type="number" min="1" value={batchValidDays} onChange={(event) => setBatchValidDays(event.target.value)} required /></Field>
            <Field label="每码可核销次数"><input type="number" min="1" value={batchMaxRedemptions} onChange={(event) => setBatchMaxRedemptions(event.target.value)} required /></Field>
          </form>
        </Modal>
      ) : null}
      {dialog?.type === 'status' ? (
        <Modal title="变更兑换码状态" detail={dialog.row.code} onClose={() => setDialog(null)} footer={<><button className="ghost" type="button" disabled={saving} onClick={() => setDialog(null)}>取消</button><button className="btn primary" type="button" disabled={saving} onClick={() => void saveStatus()}>{saving ? '保存中...' : '保存'}</button></>}>
          <Field label="新状态"><select value={dialog.status} onChange={(event) => setDialog({ ...dialog, status: event.target.value })}>{redeemStatusOptions.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}</select></Field>
        </Modal>
      ) : null}
      {dialog?.type === 'redemptions' ? (
        <Modal title="核销记录" detail={`${dialog.row.code} · 共 ${redemptionsTotal} 条`} onClose={() => setDialog(null)} footer={<button className="btn primary" type="button" onClick={() => setDialog(null)}>关闭</button>}>
          {redemptionsLoading ? <LoadingBlock label="载入核销记录" /> : null}
          {redemptionsError ? <ErrorBlock message={redemptionsError} onRetry={() => void openRedemptions(dialog.row)} /> : null}
          {!redemptionsLoading && !redemptionsError && !redemptions.length ? <EmptyBlock title="暂无核销记录" detail="该兑换码尚未被用户兑换。" /> : null}
          {!redemptionsLoading && !redemptionsError && redemptions.length ? (
            <div className="admin-data-grid redeem-redemption-grid">
              <div className="table-head"><span>用户</span><span>类型</span><span>积分</span><span>余额</span><span>来源</span><span>时间</span></div>
              {redeemRedemptionRows(redemptions).map((row) => (
                <div key={row.id} className="table-row">
                  <strong>{row.userLabel}</strong>
                  <span>{row.typeLabel}</span>
                  <span className={row.amountTone === 'debit' ? 'danger-text' : 'positive-code'}>{row.amount}</span>
                  <span>{row.balanceAfter}</span>
                  <span>{row.sourceLabel}</span>
                  <span>{row.occurredAt}</span>
                </div>
              ))}
            </div>
          ) : null}
        </Modal>
      ) : null}
    </section>
  )
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
