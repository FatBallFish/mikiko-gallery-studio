import { useEffect, useState } from 'react'
import type { ModelRoute } from '../../../shared/api-types'
import { adminApi } from '../../../shared/admin-api'
import { Badge, EmptyBlock, ErrorBlock, InlineFeedback, LoadingBlock, PageHeader } from '../components'

export function RoutingPage({ onFeedback }: { onFeedback: (title: string, detail?: string) => void }) {
  const [rows, setRows] = useState<ModelRoute[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [savingId, setSavingId] = useState<string | null>(null)
  const [notice, setNotice] = useState('路由策略修改会立即写入审计日志。')

  const load = async () => {
    setLoading(true)
    setError(null)
    try {
      setRows(await adminApi.listRoutes())
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : '路由载入失败')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    void load()
  }, [])

  const updateRoute = async (route: ModelRoute, patch: Partial<ModelRoute>) => {
    setSavingId(route.id)
    try {
      const updated = await adminApi.updateRoute(route.id, {
        group_code: route.group_code,
        task_type: route.task_type,
        provider_model_id: route.provider_model_id,
        provider_code: patch.provider ?? route.provider_code ?? route.provider,
        priority: patch.priority ?? route.priority,
        weight_percent: route.weight_percent,
        fallback_order: route.fallback_order,
        enabled: patch.enabled ?? route.enabled,
      })
      setRows((current) => current.map((item) => item.id === route.id ? updated : item))
      setNotice(`${updated.scene} 已更新：${Object.keys(patch).join(', ')}`)
      onFeedback('路由策略已更新', updated.scene)
    } catch (caught) {
      setNotice(caught instanceof Error ? caught.message : '路由更新失败')
    } finally {
      setSavingId(null)
    }
  }

  if (loading) return <LoadingBlock label="载入模型路由" />
  if (error) return <ErrorBlock message={error} onRetry={load} />
  if (!rows.length) return <EmptyBlock title="暂无路由" detail="未找到 Provider 路由策略。" />

  return (
    <section className="page-stack">
      <PageHeader eyebrow="Routing" title="模型路由" detail="Provider 开关、优先级和错误策略在一个连续工作区里更新。" />
      <section className="pg-admin-card ops-surface full-main">
        <section className="main-lane table-lane no-divider">
          <InlineFeedback tone="neutral" message={notice} />
          <div className="table-head route-grid"><span>场景</span><span>Provider</span><span>策略</span><span>优先级</span><span>状态</span><span>操作</span></div>
          {rows.map((row) => (
            <div key={row.id} className="table-row route-grid editable-row">
              <div><strong>{row.scene}</strong><p>{row.note}</p></div>
              <select value={row.provider} onChange={(event) => void updateRoute(row, { provider: event.target.value })} disabled={savingId === row.id}>
                <option>OpenAI</option><option>OpenRouter</option><option>Internal</option>
              </select>
              <input value={row.policy} onChange={(event) => setRows((current) => current.map((item) => item.id === row.id ? { ...item, policy: event.target.value } : item))} />
              <input type="number" min="1" max="9" value={row.priority} onChange={(event) => setRows((current) => current.map((item) => item.id === row.id ? { ...item, priority: Number(event.target.value) } : item))} />
              <Badge tone={row.enabled ? 'success' : 'warning'}>{row.enabled ? '启用' : '停用'}</Badge>
              <div className="row-actions buttons">
                <button type="button" className="ghost small" onClick={() => void updateRoute(row, { enabled: !row.enabled })} disabled={savingId === row.id}>{row.enabled ? '停用' : '启用'}</button>
                <button type="button" className="btn small" onClick={() => void updateRoute(row, { policy: row.policy, priority: row.priority })} disabled={savingId === row.id}>{savingId === row.id ? '保存中' : '保存'}</button>
              </div>
            </div>
          ))}

          <div className="lane-divider" />
          <div className="policy-stack">
            <div className="policy-line"><strong>429 / rate_limit</strong><span>指数退避后优先原 provider 重试，再评估切换。</span><Badge tone="warning">retry</Badge></div>
            <div className="policy-line"><strong>5xx / upstream_unavailable</strong><span>能力矩阵允许时切到备用 provider。</span><Badge tone="primary">fallback</Badge></div>
            <div className="policy-line"><strong>400 / invalid_image</strong><span>包装成平台可读错误，避免暴露上游原始字段。</span><Badge>wrap</Badge></div>
          </div>
        </section>
      </section>
    </section>
  )
}
