import { useEffect, useMemo, useRef, useState } from 'react'
import type { PriceRow } from '../../../shared/api-types'
import { mockApi } from '../../../shared/mock-api'
import { Badge, EmptyBlock, ErrorBlock, InlineFeedback, LoadingBlock, PageHeader } from '../components'

type PriceDrafts = Record<string, Pick<PriceRow, 'q1k' | 'q2k' | 'q4k' | 'reference_multiplier'>>

const toDrafts = (rows: PriceRow[]): PriceDrafts => Object.fromEntries(rows.map((row) => [row.id, { q1k: row.q1k, q2k: row.q2k, q4k: row.q4k, reference_multiplier: row.reference_multiplier }]))

function validatePriceDraft(draft: Pick<PriceRow, 'q1k' | 'q2k' | 'q4k' | 'reference_multiplier'>) {
  const values = [draft.q1k, draft.q2k, draft.q4k, draft.reference_multiplier]
  if (values.some((value) => Number.isNaN(Number(value)) || Number(value) <= 0)) return '价格与倍率必须是大于 0 的数字。'
  if (Number(draft.q4k) < Number(draft.q2k) || Number(draft.q2k) < Number(draft.q1k)) return '分辨率价格应保持 1K <= 2K <= 4K。'
  return null
}

export function PricingPage({ onFeedback }: { onFeedback: (title: string, detail?: string) => void }) {
  const [rows, setRows] = useState<PriceRow[]>([])
  const [drafts, setDrafts] = useState<PriceDrafts>({})
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [savingId, setSavingId] = useState<string | null>(null)
  const [notice, setNotice] = useState('价格修改先进入草稿，发布后对估价接口生效。')
  const baselineRef = useRef<PriceDrafts>({})

  const load = async () => {
    setLoading(true)
    setError(null)
    try {
      const nextRows = await mockApi.listPrices()
      setRows(nextRows)
      const nextDrafts = toDrafts(nextRows)
      setDrafts(nextDrafts)
      baselineRef.current = nextDrafts
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : '价格载入失败')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    void load()
  }, [])

  const dirtyRows = useMemo(() => rows.filter((row) => {
    const draft = drafts[row.id]
    return row.state === 'draft' || Boolean(draft && JSON.stringify(draft) !== JSON.stringify({ q1k: row.q1k, q2k: row.q2k, q4k: row.q4k, reference_multiplier: row.reference_multiplier }))
  }), [drafts, rows])
  const conflicts = rows.flatMap((row) => {
    const draft = drafts[row.id]
    const conflict = draft ? validatePriceDraft(draft) : null
    return conflict ? [{ id: row.id, group: row.group, conflict }] : []
  })

  const patchDraft = (rowId: string, key: keyof PriceDrafts[string], value: string) => {
    setDrafts((current) => ({ ...current, [rowId]: { ...current[rowId], [key]: value } }))
  }

  const saveRow = async (row: PriceRow) => {
    const draft = drafts[row.id]
    if (!draft) return
    const conflict = validatePriceDraft(draft)
    if (conflict) {
      setNotice(`${row.group}: ${conflict}`)
      return
    }
    setSavingId(row.id)
    try {
      const updated = await mockApi.updatePrice(row.id, draft)
      setRows((current) => current.map((item) => item.id === row.id ? updated : item))
      setNotice(`${row.group} 价格已保存为草稿。`)
      onFeedback('价格草稿已保存', row.group)
    } finally {
      setSavingId(null)
    }
  }

  const publish = async () => {
    if (conflicts.length) {
      setNotice(`仍有 ${conflicts.length} 项价格冲突，发布已阻止。`)
      return
    }
    setSavingId('publish')
    try {
      for (const row of dirtyRows) {
        const draft = drafts[row.id]
        if (draft) await mockApi.updatePrice(row.id, draft)
      }
      const nextRows = await mockApi.publishPrices()
      setRows(nextRows)
      const nextDrafts = toDrafts(nextRows)
      setDrafts(nextDrafts)
      baselineRef.current = nextDrafts
      setNotice('价格矩阵已发布；估价、扣费与文档示例将读取新版本。')
      onFeedback('价格发布成功', `发布 ${dirtyRows.length} 个模型分组`)
    } finally {
      setSavingId(null)
    }
  }

  const revert = async () => {
    setSavingId('revert')
    try {
      for (const row of dirtyRows) {
        const baseline = baselineRef.current[row.id]
        if (baseline) await mockApi.updatePrice(row.id, baseline)
      }
      const nextRows = await mockApi.publishPrices()
      setRows(nextRows)
      const nextDrafts = toDrafts(nextRows)
      setDrafts(nextDrafts)
      baselineRef.current = nextDrafts
      setNotice('价格草稿已按最近发布版本回滚。')
      onFeedback('价格已回滚', '已写入价格审计轨迹')
    } finally {
      setSavingId(null)
    }
  }

  if (loading) return <LoadingBlock label="载入价格策略" />
  if (error) return <ErrorBlock message={error} onRetry={load} />
  if (!rows.length) return <EmptyBlock title="暂无价格" detail="价格策略尚未配置。" />

  return (
    <section className="page-stack">
      <PageHeader
        eyebrow="Pricing"
        title="价格策略"
        detail="价格矩阵、参考图倍率与发布摘要保持在一页内闭环。"
        actions={
          <>
            <button className="ghost" type="button" onClick={revert} disabled={!dirtyRows.length || Boolean(savingId)}>回滚草稿</button>
            <button className="btn primary" type="button" onClick={publish} disabled={!dirtyRows.length || Boolean(savingId) || Boolean(conflicts.length)}>{savingId === 'publish' ? '发布中...' : `发布 ${dirtyRows.length}`}</button>
          </>
        }
      />
      <section className="pg-admin-card overview-surface">
        <section className="main-lane pricing-lane">
          <InlineFeedback tone={conflicts.length ? 'warning' : 'neutral'} message={notice} />
          <div className="table-head price-grid"><span>模型分组</span><span>1K</span><span>2K</span><span>4K</span><span>参考倍率</span><span>状态</span><span>操作</span></div>
          {rows.map((row) => {
            const draft = drafts[row.id]
            const conflict = draft ? validatePriceDraft(draft) : null
            const dirty = dirtyRows.some((item) => item.id === row.id)
            return (
              <div key={row.id} className={`table-row price-grid editable-row ${conflict ? 'has-conflict' : ''}`}>
                <div><strong>{row.group}</strong><p>v{row.version}</p></div>
                <input value={draft?.q1k ?? row.q1k} onChange={(event) => patchDraft(row.id, 'q1k', event.target.value)} />
                <input value={draft?.q2k ?? row.q2k} onChange={(event) => patchDraft(row.id, 'q2k', event.target.value)} />
                <input value={draft?.q4k ?? row.q4k} onChange={(event) => patchDraft(row.id, 'q4k', event.target.value)} />
                <input value={draft?.reference_multiplier ?? row.reference_multiplier} onChange={(event) => patchDraft(row.id, 'reference_multiplier', event.target.value)} />
                <Badge tone={conflict ? 'warning' : dirty ? 'warning' : 'success'}>{conflict ? '冲突' : dirty ? '草稿' : '已生效'}</Badge>
                <button type="button" className="btn small" onClick={() => void saveRow(row)} disabled={savingId === row.id || Boolean(conflict) || !dirty}>{savingId === row.id ? '保存中' : '保存'}</button>
                {conflict ? <InlineFeedback tone="warning" message={conflict} /> : null}
              </div>
            )
          })}
        </section>

        <aside className="signal-rail">
          <section className="signal-section"><strong>发布摘要</strong><p>{dirtyRows.length ? `${dirtyRows.length} 个分组待发布，影响估价与扣费。` : '当前价格矩阵已全量生效。'}</p></section>
          <section className="signal-section"><strong>计费公式</strong><p>总积分 = 基础单价 x 输出张数 x 参考图系数 x 用户组倍率。</p></section>
          <section className="signal-section"><strong>倍率规则</strong><div className="compact-item"><span>默认用户组</span><p>1.00000</p></div><div className="compact-item"><span>渠道合作组</span><p>0.85000</p></div></section>
        </aside>
      </section>
    </section>
  )
}
