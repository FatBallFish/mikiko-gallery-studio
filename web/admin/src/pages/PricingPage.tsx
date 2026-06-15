import { useEffect, useMemo, useState } from 'react'
import type { ImageTaskType, RouteModel, RouteModelPrice } from '../../../shared/api-types'
import { adminApi } from '../../../shared/admin-api'
import { cn } from '../../../shared/classnames'
import { Badge, EmptyBlock, ErrorBlock, Field, LoadingBlock, Modal } from '../components'
import { adminButton, adminPage } from '../ui/classes'
import { adminDataGrid } from '../ui/dataGrid'
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
type PriceGroup = { key: string; route: RouteModel | undefined; routeID: string | number; routeLabel: string; routeSecondary: string; taskType: ImageTaskType; rows: RouteModelPrice[] }

const pricingClasses = {
  header: 'flex items-center justify-between gap-4',
  sectionTitle: 'flex items-center gap-3 text-sm font-bold uppercase tracking-[0.15em] text-[var(--muted-strong)] before:h-px before:w-6 before:bg-[var(--accent)]',
  notice: 'rounded-[2rem] border border-[var(--accent)]/10 bg-[var(--accent)]/5 p-8',
  noticeInner: 'flex items-start gap-4 max-[720px]:grid',
  noticeIcon: 'grid size-10 shrink-0 place-items-center rounded-xl bg-[var(--accent)]/10 text-[var(--accent)]',
  noticeTitle: 'mb-2 text-lg font-bold text-[var(--text)]',
  noticeGrid: 'grid grid-cols-1 gap-8 md:grid-cols-2',
  noticeText: 'm-0 text-sm leading-relaxed text-[var(--soft)]',
  formulaCode: 'rounded bg-white/5 px-1.5 py-0.5 font-mono text-[var(--accent)]',
  tableWrap: 'min-w-0 overflow-x-auto rounded-3xl border border-[var(--line)] bg-white/[0.01] shadow-[0_20px_70px_rgba(0,0,0,.18)] backdrop-blur-sm',
  table: 'w-full min-w-[860px] border-collapse text-left',
  th: 'border-b border-[var(--line)] bg-white/[0.02] px-6 py-4 text-[11px] font-extrabold uppercase tracking-wider text-[var(--muted-strong)]',
  tr: 'border-b border-[var(--line)]/60 transition-colors last:border-b-0 hover:bg-white/[0.03]',
  trActive: 'bg-white/[0.025]',
  td: 'px-6 py-4 align-middle text-sm text-[var(--muted)]',
  chevron: 'size-4 text-[var(--muted-strong)] transition-transform',
  routeName: 'font-bold text-[var(--text)]',
  routeMeta: 'm-0 mt-1 text-[10px] font-mono text-[var(--muted-strong)]',
  taskPill: 'w-fit rounded-lg border border-[var(--line)] bg-white/5 px-2 py-1 text-xs font-bold text-[var(--soft)]',
  qualityPanelCell: 'bg-black/10 p-6 pl-20 max-[720px]:pl-6',
  qualityPanel: 'overflow-hidden rounded-2xl border border-[var(--line)] bg-white/[0.02]',
  qualityGrid: 'grid gap-2',
  qualityTable: 'w-full min-w-[760px] border-collapse text-left',
  qualityTh: 'border-b border-[var(--line)] px-4 py-3 text-[10px] font-extrabold uppercase tracking-wider text-[var(--muted-strong)]',
  qualityTd: 'px-4 py-3 align-middle text-xs text-[var(--muted)]',
}

export function PricingPage({ onFeedback }: { onFeedback: (title: string, detail?: string) => void }) {
  const [routes, setRoutes] = useState<RouteModel[]>([])
  const [prices, setPrices] = useState<RouteModelPrice[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [dialog, setDialog] = useState<PricingDialog | null>(null)
  const [saving, setSaving] = useState(false)
  const [expandedGroups, setExpandedGroups] = useState<Record<string, boolean>>({})

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
  const priceGroups = useMemo(() => groupPrices(routes, prices), [routes, prices])

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
      <div className={pricingClasses.header}>
        <h3 className={pricingClasses.sectionTitle}>积分价格配置 / Price Strategy</h3>
        <button className={cn(adminButton.base, adminButton.primary)} type="button" disabled={!routes.length} onClick={() => setDialog(newPriceDialog(routes))}>新增配置</button>
      </div>
      <section className={pricingClasses.notice}>
        <div className={pricingClasses.noticeInner}>
          <div className={pricingClasses.noticeIcon}><InfoIcon /></div>
          <div className="min-w-0 flex-1">
            <h3 className={pricingClasses.noticeTitle}>计费规则说明</h3>
            <div className={pricingClasses.noticeGrid}>
              <div className="grid gap-2">
                <p className={pricingClasses.noticeText}>1. <strong>扣费公式</strong>: <code className={pricingClasses.formulaCode}>最终积分 = 基础消耗 * (参考图倍率 if 包含参考图)</code></p>
                <p className={pricingClasses.noticeText}>2. <strong>精度说明</strong>: 后端扣费保留 5 位小数，前端展示四舍五入保留 2 位。</p>
              </div>
              <div className="grid gap-2">
                <p className={pricingClasses.noticeText}>3. <strong>兜底逻辑</strong>: 若路由模型未配对应任务类型的价格，系统将返回配置错误。</p>
                <p className={pricingClasses.noticeText}>4. <strong>展示规则</strong>: 列表按照路由模型和任务类型进行聚合，点击行展开具体质量的价格配置。</p>
                {stats.missingEnabledRoutes ? <p className={cn(pricingClasses.noticeText, 'font-bold text-[var(--red)]')}>当前有 {stats.missingEnabledRoutes} 个启用路由缺少价格配置。</p> : null}
              </div>
            </div>
          </div>
        </div>
      </section>
      {!prices.length ? <EmptyBlock title="暂无价格配置" detail="为每个可用路由模型配置任务类型和质量价格。" /> : null}
      {prices.length ? (
        <div className={pricingClasses.tableWrap}>
          <table className={pricingClasses.table}>
            <thead>
              <tr>
                <th className="w-10 px-6 py-4"></th>
                <th className={pricingClasses.th}>路由模型</th>
                <th className={pricingClasses.th}>任务类型</th>
                <th className={pricingClasses.th}>已配置质量数</th>
                <th className={pricingClasses.th}>操作</th>
              </tr>
            </thead>
            <tbody>
              {priceGroups.map((group) => {
                const expanded = expandedGroups[group.key] ?? false
                return (
                  <PriceGroupRows
                    key={group.key}
                    group={group}
                    expanded={expanded}
                    onToggle={() => setExpandedGroups((current) => ({ ...current, [group.key]: !expanded }))}
                    onAdd={() => setDialog(newPriceDialogForGroup(group))}
                    onEdit={(row) => setDialog(editPriceDialog(row))}
                  />
                )
              })}
            </tbody>
          </table>
        </div>
      ) : null}
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

function PriceGroupRows({ group, expanded, onToggle, onAdd, onEdit }: { group: PriceGroup; expanded: boolean; onToggle: () => void; onAdd: () => void; onEdit: (row: RouteModelPrice) => void }) {
  return (
    <>
      <tr className={cn(pricingClasses.tr, 'group cursor-pointer', expanded && pricingClasses.trActive)} onClick={onToggle}>
        <td className="px-6 py-4">
          <ChevronIcon className={cn(pricingClasses.chevron, expanded && 'rotate-180')} />
        </td>
        <td className={pricingClasses.td}>
          <strong className={pricingClasses.routeName}>{group.routeLabel}</strong>
          <p className={pricingClasses.routeMeta}>{group.routeSecondary}</p>
        </td>
        <td className={pricingClasses.td}><span className={pricingClasses.taskPill}>{adminTaskTypeLabel(group.taskType)}</span></td>
        <td className={pricingClasses.td}><span className="text-xs font-bold text-[var(--soft)]">{group.rows.length} 个质量配置</span></td>
        <td className={cn(pricingClasses.td, 'text-right')}>
          <button className={cn(adminButton.base, adminButton.ghost, adminButton.small)} type="button" onClick={(event) => { event.stopPropagation(); onAdd() }}>快速添加质量</button>
        </td>
      </tr>
      {expanded ? (
        <tr className="border-b border-[var(--line)]/60">
          <td colSpan={5} className={pricingClasses.qualityPanelCell}>
            <div className={pricingClasses.qualityPanel}>
              <table className={pricingClasses.qualityTable}>
                <thead>
                  <tr>
                    <th className={pricingClasses.qualityTh}>生成质量 / Quality</th>
                    <th className={pricingClasses.qualityTh}>基础消耗 / Base</th>
                    <th className={pricingClasses.qualityTh}>参考图倍率 / Multiplier</th>
                    <th className={pricingClasses.qualityTh}>状态</th>
                    <th className={pricingClasses.qualityTh}>操作</th>
                  </tr>
                </thead>
                <tbody>
                  {group.rows.map((row) => (
                    <tr key={String(row.id)} className="border-b border-[var(--line)]/60 transition-colors last:border-b-0 hover:bg-white/[0.02]">
                      <td className={pricingClasses.qualityTd}><strong className="text-[var(--text)]">{pricingQualityLabel(row.quality)}</strong></td>
                      <td className={pricingClasses.qualityTd}><code className={adminDataGrid.code}>{row.base_points} ◈</code></td>
                      <td className={pricingClasses.qualityTd}><code className={adminDataGrid.code}>x {row.reference_multiplier}</code></td>
                      <td className={pricingClasses.qualityTd}><PricingBadge enabled={row.enabled} /></td>
                      <td className={pricingClasses.qualityTd}><button className={cn(adminButton.base, adminButton.ghost, adminButton.small)} type="button" onClick={() => onEdit(row)}>调整</button></td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </td>
        </tr>
      ) : null}
    </>
  )
}

function newPriceDialog(routes: RouteModel[]): PricingDialog {
  return { routeModelId: String(routes[0]?.id ?? ''), taskType: 'text_to_image', quality: '1K', basePoints: '8.00000', referenceMultiplier: '1.00000', enabled: true }
}

function newPriceDialogForGroup(group: PriceGroup): PricingDialog {
  return { routeModelId: String(group.routeID), taskType: group.taskType, quality: '1K', basePoints: '8.00000', referenceMultiplier: '1.00000', enabled: true }
}

function editPriceDialog(row: RouteModelPrice): PricingDialog {
  return { row, routeModelId: String(row.route_model_id), taskType: row.task_type, quality: row.quality, basePoints: row.base_points, referenceMultiplier: row.reference_multiplier, enabled: row.enabled }
}

function PricingBadge({ enabled }: { enabled: boolean }) {
  const badge = pricingEnabledBadge(enabled)
  return <Badge tone={badge.tone}>{badge.label}</Badge>
}

function groupPrices(routes: RouteModel[], prices: RouteModelPrice[]): PriceGroup[] {
  const groups = new Map<string, PriceGroup>()
  prices.forEach((price) => {
    const route = routes.find((item) => String(item.id) === String(price.route_model_id))
    const key = `${String(price.route_model_id)}:${price.task_type}`
    const existing = groups.get(key)
    if (existing) {
      existing.rows.push(price)
      return
    }
    groups.set(key, {
      key,
      route,
      routeID: price.route_model_id,
      routeLabel: pricingRouteLabel(price.route_model_id, routes, price),
      routeSecondary: pricingRouteSecondaryLabel(price.route_model_id, routes, price),
      taskType: price.task_type,
      rows: [price],
    })
  })
  return Array.from(groups.values()).map((group) => ({
    ...group,
    rows: group.rows.slice().sort((left, right) => pricingQualityLabel(left.quality).localeCompare(pricingQualityLabel(right.quality))),
  }))
}

const InfoIcon = () => <svg className="size-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><circle cx="12" cy="12" r="10" /><path d="M12 16v-4" /><path d="M12 8h.01" /></svg>
const ChevronIcon = ({ className }: { className?: string }) => <svg className={className} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="m6 9 6 6 6-6" /></svg>
