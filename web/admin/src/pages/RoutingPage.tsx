import { useEffect, useState } from 'react'
import type { ModelRoute } from '../../../shared/api-types'
import { adminApi } from '../../../shared/admin-api'
import { Badge, EmptyBlock, ErrorBlock, Field, InlineFeedback, LoadingBlock, Modal, PageHeader } from '../components'

type RouteDialog = { route: ModelRoute; provider: string; priority: string; weightPercent: string; fallbackOrder: string; enabled: boolean }

export function RoutingPage({ onFeedback }: { onFeedback: (title: string, detail?: string) => void }) {
  const [rows, setRows] = useState<ModelRoute[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [savingId, setSavingId] = useState<string | null>(null)
  const [notice, setNotice] = useState('路由策略修改会立即写入审计日志。')
  const [dialog, setDialog] = useState<RouteDialog | null>(null)

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

  const updateRoute = async () => {
    if (!dialog) return
    setSavingId(dialog.route.id)
    try {
      const updated = await adminApi.updateRoute(dialog.route.id, {
        group_code: dialog.route.group_code,
        task_type: dialog.route.task_type,
        provider_model_id: dialog.route.provider_model_id,
        provider_code: dialog.provider,
        priority: Number(dialog.priority),
        weight_percent: Number(dialog.weightPercent),
        fallback_order: Number(dialog.fallbackOrder),
        enabled: dialog.enabled,
      })
      setRows((current) => current.map((item) => item.id === dialog.route.id ? updated : item))
      setDialog(null)
      setNotice(`${updated.scene} 已更新`)
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
            <div key={row.id} className="table-row route-grid">
              <div><strong>{row.scene}</strong><p>{row.note}</p></div>
              <span>{row.provider}</span>
              <span>{row.policy}</span>
              <code>{row.priority}</code>
              <Badge tone={row.enabled ? 'success' : 'warning'}>{row.enabled ? '启用' : '停用'}</Badge>
              <div className="row-actions buttons">
                <button type="button" className="ghost small" onClick={() => setDialog({ route: row, provider: row.provider_code ?? row.provider, priority: String(row.priority), weightPercent: String(row.weight_percent ?? 100), fallbackOrder: String(row.fallback_order ?? 0), enabled: row.enabled })} disabled={savingId === row.id}>编辑</button>
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
      {dialog ? (
        <Modal title="编辑路由策略" detail={dialog.route.scene} onClose={() => setDialog(null)} footer={<><button className="ghost" type="button" disabled={Boolean(savingId)} onClick={() => setDialog(null)}>取消</button><button className="btn primary" type="button" disabled={Boolean(savingId)} onClick={() => void updateRoute()}>{savingId ? '保存中...' : '保存'}</button></>}>
          <div className="form-grid">
            <Field label="Provider"><input value={dialog.provider} onChange={(event) => setDialog({ ...dialog, provider: event.target.value })} /></Field>
            <Field label="优先级"><input type="number" min="0" value={dialog.priority} onChange={(event) => setDialog({ ...dialog, priority: event.target.value })} /></Field>
            <Field label="权重"><input type="number" min="0" max="100" value={dialog.weightPercent} onChange={(event) => setDialog({ ...dialog, weightPercent: event.target.value })} /></Field>
            <Field label="Fallback 顺序"><input type="number" min="0" value={dialog.fallbackOrder} onChange={(event) => setDialog({ ...dialog, fallbackOrder: event.target.value })} /></Field>
            <Field label="状态"><select value={dialog.enabled ? 'enabled' : 'disabled'} onChange={(event) => setDialog({ ...dialog, enabled: event.target.value === 'enabled' })}><option value="enabled">启用</option><option value="disabled">停用</option></select></Field>
          </div>
        </Modal>
      ) : null}
    </section>
  )
}
