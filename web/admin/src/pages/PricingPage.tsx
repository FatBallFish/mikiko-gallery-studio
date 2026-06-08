import { useEffect, useMemo, useState } from 'react'
import type { ImageTaskType, RouteModel, RouteModelPrice } from '../../../shared/api-types'
import { adminApi } from '../../../shared/admin-api'
import { cn } from '../../../shared/classnames'
import { Badge, EmptyBlock, ErrorBlock, Field, InlineFeedback, LoadingBlock, Modal, PageHeader, StatusCell, StatusStrip } from '../components'
import { adminButton, adminPage } from '../ui/classes'
import { adminDataGrid, adminGridCols } from '../ui/dataGrid'
import { adminTaskTypeLabel, adminTaskTypeOptions } from './adminTaskTypes'
import {
  pricingEnabledBadge,
  pricingFieldHints,
  pricingQualityLabel,
  pricingQualityOptions,
  pricingRouteLabel,
  pricingRouteSecondaryLabel,
  pricingStatusOptions,
  pricingSummary,
} from './pricingRows'

type PricingDialog = { row?: RouteModelPrice; routeModelId: string; taskType: ImageTaskType; quality: string; basePoints: string; referenceMultiplier: string; enabled: boolean }

export function PricingPage({ onFeedback }: { onFeedback: (title: string, detail?: string) => void }) {
  const [routes, setRoutes] = useState<RouteModel[]>([])
  const [prices, setPrices] = useState<RouteModelPrice[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [notice, setNotice] = useState('价格按路由模型、任务类型、质量配置；后端扣费保留 5 位，前端只展示 2 位。')
  const [dialog, setDialog] = useState<PricingDialog | null>(null)
  const [saving, setSaving] = useState(false)

  const load = async () => {
    setLoading(true)
    setError(null)
    try {
      const [nextRoutes, nextPrices] = await Promise.all([
        adminApi.listRouteModels({ page_size: 100 }),
        adminApi.listRouteModelPrices({ page_size: 200 }),
      ])
      setRoutes(nextRoutes)
      setPrices(nextPrices)
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : '价格策略载入失败')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => { void load() }, [])

  const stats = useMemo(() => pricingSummary(routes, prices), [routes, prices])

  async function savePricing() {
    if (!dialog) return
    setSaving(true)
    try {
      const payload = {
        route_model_id: Number(dialog.routeModelId),
        task_type: dialog.taskType,
        quality: dialog.quality,
        base_points: dialog.basePoints,
        reference_multiplier: dialog.referenceMultiplier,
        enabled: dialog.enabled,
      }
      const saved = dialog.row ? await adminApi.updateRouteModelPrice(dialog.row.id, payload) : await adminApi.createRouteModelPrice(payload)
      setDialog(null)
      setNotice(`${pricingRouteLabel(saved.route_model_id, routes, saved)} / ${adminTaskTypeLabel(saved.task_type)} / ${pricingQualityLabel(saved.quality)} 已保存。`)
      onFeedback('价格配置已更新', `${adminTaskTypeLabel(saved.task_type)} · ${pricingQualityLabel(saved.quality)}`)
      await load()
    } finally {
      setSaving(false)
    }
  }

  if (loading) return <LoadingBlock label="载入价格策略" />
  if (error) return <ErrorBlock message={error} onRetry={load} />

  return (
    <section className={adminPage.stack}>
      <PageHeader
        eyebrow="Route Model Prices"
        title="价格配置"
        detail="基础积分和参考图倍率绑定到路由模型，不再依赖 Provider Model 成本字段。"
        actions={<><button className={cn(adminButton.base, adminButton.ghost)} type="button" onClick={() => void load()}>刷新</button><button className={cn(adminButton.base, adminButton.primary)} type="button" disabled={!routes.length} onClick={() => setDialog(newPriceDialog(routes))}>新增价格</button></>}
      />
      <StatusStrip columns={4}>
        <StatusCell label="路由模型" value={stats.totalRoutes} />
        <StatusCell label="启用路由" value={stats.enabledRoutes} />
        <StatusCell label="价格项" value={stats.totalPrices} />
        <StatusCell label="缺价格路由" value={stats.missingEnabledRoutes} />
      </StatusStrip>
      <section className={adminPage.splitSurface}>
        <section className={adminPage.mainLane}>
          <InlineFeedback tone={stats.missingEnabledRoutes ? 'warning' : 'neutral'} message={stats.missingEnabledRoutes ? '存在启用路由模型未配置价格，用户侧预估可能返回配置错误。' : notice} />
          {!prices.length ? <EmptyBlock title="暂无价格配置" detail="为每个可用路由模型配置任务类型和质量价格。" /> : null}
          {prices.length ? (
            <div className={cn(adminDataGrid.root, adminGridCols.routePrice)}>
              <div className={cn(adminDataGrid.head, adminGridCols.routePrice)}><span>路由模型</span><span>任务类型</span><span>质量</span><span>基础积分</span><span>参考图倍率</span><span>状态</span><span>操作</span></div>
              {prices.map((row) => (
                <div key={String(row.id)} className={cn(adminDataGrid.row, adminGridCols.routePrice)}>
                  <div className={adminDataGrid.stackCell}><strong>{pricingRouteLabel(row.route_model_id, routes, row)}</strong><p className={adminDataGrid.detail}>{pricingRouteSecondaryLabel(row.route_model_id, routes, row)}</p></div>
                  <span>{adminTaskTypeLabel(row.task_type)}</span>
                  <span>{pricingQualityLabel(row.quality)}</span>
                  <code className={adminDataGrid.code}>{row.base_points}</code>
                  <code className={adminDataGrid.code}>{row.reference_multiplier}</code>
                  <PricingBadge enabled={row.enabled} />
                  <button className={cn(adminButton.base, adminButton.ghost, adminButton.small)} type="button" onClick={() => setDialog(editPriceDialog(row))}>调整</button>
                </div>
              ))}
            </div>
          ) : null}
        </section>

        <aside className={adminPage.sideRail}>
          <section className={adminPage.signalSection}><strong>计费公式</strong><p>charged_points = base_points x effective_multiplier x task_multiplier x image_count。</p></section>
          <section className={adminPage.signalSection}><strong>展示规则</strong><p>列表和工作台只展示 display_points，余额校验由后端使用 5 位 charged_points。</p></section>
          {routes.map((route) => {
            const rows = prices.filter((price) => String(price.route_model_id) === String(route.id))
            return <section key={String(route.id)} className={adminPage.signalSection}><strong>{route.name}</strong><p>{rows.length ? rows.map((price) => `${adminTaskTypeLabel(price.task_type)}/${pricingQualityLabel(price.quality)}: ${price.base_points}`).join(' · ') : '未配置价格'}</p></section>
          })}
        </aside>
      </section>
      {dialog ? (
        <Modal title={dialog.row ? '调整价格配置' : '新增价格配置'} detail={pricingFieldHints.dialogDetail} onClose={() => setDialog(null)} footer={<><button className={cn(adminButton.base, adminButton.ghost)} type="button" disabled={saving} onClick={() => setDialog(null)}>取消</button><button className={cn(adminButton.base, adminButton.primary)} type="button" disabled={saving || !dialog.routeModelId || !dialog.basePoints} onClick={() => void savePricing()}>{saving ? '保存中...' : '保存'}</button></>}>
          <div className={adminPage.formGrid}>
            <Field label="路由模型"><select value={dialog.routeModelId} onChange={(event) => setDialog({ ...dialog, routeModelId: event.target.value })}>{routes.map((route) => <option key={String(route.id)} value={String(route.id)}>{route.name} ({route.code})</option>)}</select></Field>
            <Field label="任务类型"><select value={dialog.taskType} onChange={(event) => setDialog({ ...dialog, taskType: event.target.value as ImageTaskType })}>{adminTaskTypeOptions.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}</select></Field>
            <Field label="质量" hint="auto 不可直接配置价格；后端会按尺寸动态映射到 1K、2K 或 4K 档位。"><select value={dialog.quality} onChange={(event) => setDialog({ ...dialog, quality: event.target.value })}>{pricingQualityOptions.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}</select></Field>
            <Field label="基础积分" hint={pricingFieldHints.basePoints}><input value={dialog.basePoints} onChange={(event) => setDialog({ ...dialog, basePoints: event.target.value })} placeholder="8.00000" /></Field>
            <Field label="参考图倍率" hint={pricingFieldHints.referenceMultiplier}><input value={dialog.referenceMultiplier} onChange={(event) => setDialog({ ...dialog, referenceMultiplier: event.target.value })} placeholder="1.25000" /></Field>
            <Field label="状态"><select value={dialog.enabled ? 'enabled' : 'disabled'} onChange={(event) => setDialog({ ...dialog, enabled: event.target.value === 'enabled' })}>{pricingStatusOptions.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}</select></Field>
          </div>
        </Modal>
      ) : null}
    </section>
  )
}

function newPriceDialog(routes: RouteModel[]): PricingDialog {
  return { routeModelId: String(routes[0]?.id ?? ''), taskType: 'text_to_image', quality: '1K', basePoints: '8.00000', referenceMultiplier: '1.00000', enabled: true }
}

function editPriceDialog(row: RouteModelPrice): PricingDialog {
  return { row, routeModelId: String(row.route_model_id), taskType: row.task_type, quality: row.quality, basePoints: row.base_points, referenceMultiplier: row.reference_multiplier, enabled: row.enabled }
}

function PricingBadge({ enabled }: { enabled: boolean }) {
  const badge = pricingEnabledBadge(enabled)
  return <Badge tone={badge.tone}>{badge.label}</Badge>
}
