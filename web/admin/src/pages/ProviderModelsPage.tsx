import { useEffect, useState } from 'react'
import type { ModelProvider, ProviderModel } from '../../../shared/api-types'
import { adminApi } from '../../../shared/admin-api'
import { Badge, EmptyBlock, ErrorBlock, LoadingBlock, PageHeader } from '../components'

export function ProviderModelsPage() {
  const [providers, setProviders] = useState<ModelProvider[]>([])
  const [models, setModels] = useState<ProviderModel[]>([])
  const [page, setPage] = useState(1)
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const load = async () => {
    setLoading(true)
    setError(null)
    try {
      const [providerPage, modelPage] = await Promise.all([adminApi.listModelProviders({ page_size: 100 }), adminApi.listProviderModels({ page, page_size: 20 })])
      setProviders(providerPage.items)
      setModels(modelPage.items)
      setTotal(modelPage.total)
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : '模型接入载入失败')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => { void load() }, [page])

  if (loading) return <LoadingBlock label="载入模型接入" />
  if (error) return <ErrorBlock message={error} onRetry={load} />

  return (
    <section className="page-stack">
      <PageHeader eyebrow="Provider Models" title="模型接入" detail="Provider 与具体模型能力、成本、健康状态均来自真实后台接口。" actions={<button className="btn" type="button" onClick={() => void load()}>刷新</button>} />
      <section className="ops-status-strip compact-strip">
        <div className="status-cell"><label>Providers</label><strong>{providers.length}</strong></div>
        <div className="status-cell"><label>Models</label><strong>{models.length}</strong></div>
        <div className="status-cell"><label>Enabled</label><strong>{models.filter((item) => item.enabled).length}</strong></div>
        <div className="status-cell"><label>Healthy</label><strong>{models.filter((item) => item.health_status === 'healthy').length}</strong></div>
      </section>
      <section className="pg-admin-card ops-surface full-main">
        <section className="main-lane table-lane no-divider">
          <div className="card-header lane-head compact"><span>第 {page} 页 / 共 {total} 条</span><div className="row-actions buttons"><button className="ghost small" type="button" disabled={page <= 1} onClick={() => setPage((value) => value - 1)}>上一页</button><button className="ghost small" type="button" disabled={page * 20 >= total} onClick={() => setPage((value) => value + 1)}>下一页</button></div></div>
          {!models.length ? <EmptyBlock title="暂无 Provider Model" detail="先通过接口或种子数据创建模型接入。" /> : (
            <>
              <div className="table-head route-grid"><span>模型</span><span>Provider</span><span>能力</span><span>成本</span><span>健康</span><span>状态</span></div>
              {models.map((row) => (
                <div key={row.id} className="table-row route-grid">
                  <div><strong>{row.model_code}</strong><p>{row.compat_mode}</p></div>
                  <span>{row.provider_code}</span>
                  <span>{row.supported_qualities?.join('/')} · max {row.max_image_count}</span>
                  <span>{row.input_cost}/{row.output_cost} {row.currency}</span>
                  <Badge tone={row.health_status === 'healthy' ? 'success' : 'warning'}>{row.health_status}</Badge>
                  <Badge tone={row.enabled ? 'success' : 'warning'}>{row.enabled ? '启用' : '停用'}</Badge>
                </div>
              ))}
            </>
          )}
        </section>
      </section>
    </section>
  )
}
