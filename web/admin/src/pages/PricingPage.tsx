import { useEffect, useMemo, useState } from 'react'
import type { ConfigItem, ProviderModel } from '../../../shared/api-types'
import { adminApi } from '../../../shared/admin-api'
import { Badge, EmptyBlock, ErrorBlock, Field, InlineFeedback, LoadingBlock, Modal, PageHeader } from '../components'

type PricingDialog = { row: ProviderModel; inputCost: string; outputCost: string; currency: string; enabled: boolean; healthStatus: string }

export function PricingPage({ onFeedback }: { onFeedback: (title: string, detail?: string) => void }) {
  const [models, setModels] = useState<ProviderModel[]>([])
  const [configs, setConfigs] = useState<ConfigItem[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [notice, setNotice] = useState('价格策略来自 provider model 成本字段与配置中心计费项。')
  const [dialog, setDialog] = useState<PricingDialog | null>(null)
  const [saving, setSaving] = useState(false)

  const load = async () => {
    setLoading(true)
    setError(null)
    try {
      const [providerModels, configRows] = await Promise.all([adminApi.listProviderModels({ page_size: 100 }), adminApi.listConfig()])
      setModels(providerModels.items)
      setConfigs(configRows.filter((row) => `${row.tab} ${row.key}`.toLowerCase().includes('pricing') || `${row.tab} ${row.key}`.includes('计费')))
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : '价格策略载入失败')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    void load()
  }, [])

  const enabledModels = useMemo(() => models.filter((item) => item.enabled).length, [models])

  const savePricing = async () => {
    if (!dialog) return
    setSaving(true)
    try {
      await adminApi.updateProviderModel(dialog.row.id, {
        ...dialog.row,
        input_cost: dialog.inputCost,
        output_cost: dialog.outputCost,
        currency: dialog.currency,
        enabled: dialog.enabled,
        health_status: dialog.healthStatus,
      })
      setDialog(null)
      setNotice(`${dialog.row.model_code} 价格已更新。`)
      onFeedback('价格配置已更新', dialog.row.model_code)
      await load()
    } finally {
      setSaving(false)
    }
  }

  if (loading) return <LoadingBlock label="载入价格策略" />
  if (error) return <ErrorBlock message={error} onRetry={load} />

  return (
    <section className="page-stack">
      <PageHeader
        eyebrow="Pricing"
        title="价格策略"
        detail="展示真实模型成本、计费配置与发布摘要；具体配置修改在配置中心按 tab 发布。"
        actions={<button className="btn primary" type="button" onClick={() => { setNotice('已刷新模型成本与计费配置。'); onFeedback('价格策略已刷新', `${models.length} 个 Provider Model`) ; void load() }}>刷新</button>}
      />
      <section className="ops-status-strip compact-strip">
        <div className="status-cell"><label>Provider Models</label><strong>{models.length}</strong></div>
        <div className="status-cell"><label>启用模型</label><strong>{enabledModels}</strong></div>
        <div className="status-cell"><label>计费配置</label><strong>{configs.length}</strong></div>
        <div className="status-cell"><label>生效来源</label><strong>Real API</strong></div>
      </section>
      <section className="pg-admin-card overview-surface">
        <section className="main-lane pricing-lane">
          <InlineFeedback tone="neutral" message={notice} />
          {!models.length ? <EmptyBlock title="暂无模型成本" detail="Provider Model 尚未配置成本字段。" /> : null}
          <div className="table-head price-grid"><span>模型</span><span>输入成本</span><span>输出成本</span><span>币种</span><span>能力</span><span>状态</span><span>健康</span><span>操作</span></div>
          {models.map((row) => (
            <div key={row.id} className="table-row price-grid">
              <div><strong>{row.model_code}</strong><p>{row.provider_code} · {row.compat_mode}</p></div>
              <code>{row.input_cost}</code>
              <code>{row.output_cost}</code>
              <span>{row.currency}</span>
              <span>{row.supported_qualities?.join('/') || '-'} · max {row.max_image_count}</span>
              <Badge tone={row.enabled ? 'success' : 'warning'}>{row.enabled ? '启用' : '停用'}</Badge>
              <Badge tone={row.health_status === 'healthy' ? 'success' : 'warning'}>{row.health_status}</Badge>
              <button className="ghost small" type="button" onClick={() => setDialog({ row, inputCost: row.input_cost, outputCost: row.output_cost, currency: row.currency, enabled: row.enabled, healthStatus: row.health_status })}>调整</button>
            </div>
          ))}
        </section>

        <aside className="signal-rail">
          <section className="signal-section"><strong>计费配置项</strong>{configs.length ? configs.slice(0, 6).map((item) => <p key={item.key}>{item.tab} / {item.key}: {item.value}</p>) : <p>未发现计费配置项。</p>}</section>
          <section className="signal-section"><strong>计费公式</strong><p>总积分 = 基础单价 x 输出张数 x 参考图系数 x 用户组倍率。</p></section>
          <section className="signal-section"><strong>修改入口</strong><p>价格与倍率的写操作统一走配置中心，确保版本发布和审计一致。</p></section>
        </aside>
      </section>
      {dialog ? (
        <Modal title="调整价格配置" detail={`${dialog.row.provider_code} / ${dialog.row.model_code}`} onClose={() => setDialog(null)} footer={<><button className="ghost" type="button" disabled={saving} onClick={() => setDialog(null)}>取消</button><button className="btn primary" type="button" disabled={saving} onClick={() => void savePricing()}>{saving ? '保存中...' : '保存'}</button></>}>
          <div className="form-grid">
            <Field label="输入成本"><input value={dialog.inputCost} onChange={(event) => setDialog({ ...dialog, inputCost: event.target.value })} /></Field>
            <Field label="输出成本"><input value={dialog.outputCost} onChange={(event) => setDialog({ ...dialog, outputCost: event.target.value })} /></Field>
            <Field label="币种"><input value={dialog.currency} onChange={(event) => setDialog({ ...dialog, currency: event.target.value })} /></Field>
            <Field label="启用状态"><select value={dialog.enabled ? 'enabled' : 'disabled'} onChange={(event) => setDialog({ ...dialog, enabled: event.target.value === 'enabled' })}><option value="enabled">启用</option><option value="disabled">停用</option></select></Field>
            <Field label="健康状态"><select value={dialog.healthStatus} onChange={(event) => setDialog({ ...dialog, healthStatus: event.target.value })}><option value="healthy">healthy</option><option value="degraded">degraded</option><option value="down">down</option><option value="unknown">unknown</option></select></Field>
          </div>
        </Modal>
      ) : null}
    </section>
  )
}
