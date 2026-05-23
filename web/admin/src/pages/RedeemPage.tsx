import { FormEvent, useEffect, useState } from 'react'
import type { RedeemCode } from '../../../shared/api-types'
import { adminApi } from '../../../shared/admin-api'
import { Badge, EmptyBlock, ErrorBlock, Field, LoadingBlock, Modal, PageHeader } from '../components'

type RedeemDialog =
  | { type: 'create' }
  | { type: 'status'; row: RedeemCode; status: string }

export function RedeemPage({ onFeedback }: { onFeedback: (title: string, detail?: string) => void }) {
  const [rows, setRows] = useState<RedeemCode[]>([])
  const [page, setPage] = useState(1)
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [code, setCode] = useState('')
  const [rewardValue, setRewardValue] = useState('20.00000')
  const [dialog, setDialog] = useState<RedeemDialog | null>(null)
  const [saving, setSaving] = useState(false)

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

  const saveStatus = async () => {
    if (!dialog || dialog.type !== 'status') return
    setSaving(true)
    try {
      await adminApi.updateRedeemCodeStatus(dialog.row.id, dialog.status)
      onFeedback('兑换码状态已更新', `${dialog.row.code} · ${dialog.status}`)
      setDialog(null)
      await load()
    } finally {
      setSaving(false)
    }
  }

  if (loading) return <LoadingBlock label="载入兑换码" />
  if (error) return <ErrorBlock message={error} onRetry={load} />

  return (
    <section className="page-stack">
      <PageHeader eyebrow="Redeem" title="兑换码管理" detail="创建、停用与核销记录全部连接真实后台接口。" actions={<button className="btn primary" type="button" onClick={() => setDialog({ type: 'create' })}>创建兑换码</button>} />
      <section className="pg-admin-card ops-surface full-main">
        <section className="main-lane table-lane no-divider">
          <div className="card-header lane-head compact"><span>第 {page} 页 / 共 {total} 条</span><div className="row-actions buttons"><button className="ghost small" type="button" disabled={page <= 1} onClick={() => setPage((value) => value - 1)}>上一页</button><button className="ghost small" type="button" disabled={page * 20 >= total} onClick={() => setPage((value) => value + 1)}>下一页</button></div></div>
          {!rows.length ? <EmptyBlock title="暂无兑换码" detail="创建一个兑换码后可在用户侧兑换。" /> : (
            <>
              <div className="table-head users-grid"><span>兑换码</span><span>状态</span><span>奖励</span><span>批次</span><span>有效期</span><span>操作</span></div>
              {rows.map((row) => (
                <div key={row.id} className="table-row users-grid">
                  <strong>{row.code}</strong>
                  <Badge tone={row.status === 'available' ? 'success' : 'warning'}>{row.status}</Badge>
                  <span>{row.reward_type} / {row.reward_value}</span>
                  <span>{row.batch_id}</span>
                  <span>{row.valid_until}</span>
                  <button type="button" className="ghost small" onClick={() => setDialog({ type: 'status', row, status: row.status === 'available' ? 'disabled' : 'available' })}>变更状态</button>
                </div>
              ))}
            </>
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
      {dialog?.type === 'status' ? (
        <Modal title="变更兑换码状态" detail={dialog.row.code} onClose={() => setDialog(null)} footer={<><button className="ghost" type="button" disabled={saving} onClick={() => setDialog(null)}>取消</button><button className="btn primary" type="button" disabled={saving} onClick={() => void saveStatus()}>{saving ? '保存中...' : '保存'}</button></>}>
          <Field label="新状态"><select value={dialog.status} onChange={(event) => setDialog({ ...dialog, status: event.target.value })}><option value="available">available</option><option value="disabled">disabled</option></select></Field>
        </Modal>
      ) : null}
    </section>
  )
}
