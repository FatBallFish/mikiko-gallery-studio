import { FormEvent, useEffect, useState } from 'react'
import type { RedeemCode } from '../../../shared/api-types'
import { adminApi } from '../../../shared/admin-api'
import { Badge, EmptyBlock, ErrorBlock, LoadingBlock, PageHeader } from '../components'

export function RedeemPage({ onFeedback }: { onFeedback: (title: string, detail?: string) => void }) {
  const [rows, setRows] = useState<RedeemCode[]>([])
  const [page, setPage] = useState(1)
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [code, setCode] = useState('')
  const [rewardValue, setRewardValue] = useState('20.00000')

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
    await load()
  }

  if (loading) return <LoadingBlock label="载入兑换码" />
  if (error) return <ErrorBlock message={error} onRetry={load} />

  return (
    <section className="page-stack">
      <PageHeader eyebrow="Redeem" title="兑换码管理" detail="创建、停用与核销记录全部连接真实后台接口。" actions={<form className="search-form" onSubmit={create}><input value={code} onChange={(event) => setCode(event.target.value)} placeholder="新兑换码" required /><input value={rewardValue} onChange={(event) => setRewardValue(event.target.value)} placeholder="奖励积分" /><button className="btn" type="submit">创建</button></form>} />
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
                  <button type="button" className="ghost small" onClick={async () => { await adminApi.updateRedeemCodeStatus(row.id, row.status === 'available' ? 'disabled' : 'available'); await load() }}>{row.status === 'available' ? '停用' : '启用'}</button>
                </div>
              ))}
            </>
          )}
        </section>
      </section>
    </section>
  )
}
