import { useEffect, useMemo, useState } from 'react'
import type { ImageTaskType, RouteModel, RouteModelPrice } from '../../../shared/api-types'
import { adminApi } from '../../../shared/admin-api'
import { cn } from '../../../shared/classnames'
import { ActionMenu, Badge, EmptyBlock, ErrorBlock, Field, InlineFeedback, LoadingBlock, MetricStrip, Modal, PageHeader } from '../components'
import { adminButton, adminPage, adminType } from '../ui/classes'
import { adminDataGrid } from '../ui/dataGrid'
import type { ColumnDef } from '../ui/dataTable'
import { DataTable, FilterToolbar, ListPage } from '../ui/dataTable'
import { ChevronDownIcon, InfoIcon } from '../ui/icons'
import { FilterIcon, XIcon } from '../ui/listIcons'
import { adminTaskTypeLabel, adminTaskTypeOptions } from './adminTaskTypes'
import { loadAllRouteModelPrices } from './loadAllRouteModelPrices'
import {
  pricingEnabledBadge,
  pricingFieldHints,
  pricingBaseResolutionLabel,
  pricingBaseResolutionOptions,
  pricingRouteLabel,
  pricingRouteSecondaryLabel,
  pricingStatusOptions,
  pricingSummary,
} from './pricingRows'

type PricingDialog = { row?: RouteModelPrice; routeModelId: string; taskType: ImageTaskType; baseResolution: string; basePoints: string; referenceMultiplier: string; enabled: boolean }
type PriceGroup = { key: string; route: RouteModel | undefined; routeID: string | number; routeLabel: string; routeSecondary: string; taskType: ImageTaskType; rows: RouteModelPrice[] }
type PricingFilters = { routeID: string; taskType: string; status: string }

const initialFilters: PricingFilters = { routeID: '', taskType: '', status: '' }

const pricingClasses = {
  help: 'flex items-start gap-3 border-y border-[var(--border)] py-3 text-sm text-[var(--soft)]',
  helpIcon: 'mt-0.5 grid size-8 shrink-0 place-items-center rounded-lg bg-[var(--accent)]/10 text-[var(--accent)]',
  riskList: 'grid gap-2 border-b border-[var(--border)] pb-4',
  riskRow: 'flex flex-wrap items-center justify-between gap-3 border-l-2 border-[var(--amber)] py-2 pl-3',
  routeName: 'font-semibold text-[var(--fg)]',
  routeMeta: 'mt-1 font-[family-name:var(--admin-font-mono)] text-xs text-[var(--soft)]',
  actionRow: 'flex flex-wrap items-center justify-end gap-2',
  expandedSection: 'grid min-w-0 gap-3 border-b border-[var(--border)] bg-[color-mix(in_oklch,var(--surface-solid)_96%,var(--accent)_4%)] px-4 py-4',
  expandedHeader: 'flex flex-wrap items-center justify-between gap-3',
}

export function PricingPage({ onFeedback }: { onFeedback: (title: string, detail?: string) => void }) {
  const [routes, setRoutes] = useState<RouteModel[]>([])
  const [prices, setPrices] = useState<RouteModelPrice[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [dialog, setDialog] = useState<PricingDialog | null>(null)
  const [saving, setSaving] = useState(false)
  const [mutationError, setMutationError] = useState<string | null>(null)
  const [filters, setFilters] = useState<PricingFilters>(initialFilters)
  const [expandedGroups, setExpandedGroups] = useState<Record<string, boolean>>({})

  const load = async () => {
    setLoading(true)
    setError(null)
    try {
      const [nextRoutes, nextPrices] = await Promise.all([
        adminApi.listRouteModels({ page_size: 100 }),
        loadAllRouteModelPrices((priceQuery) => adminApi.listRouteModelPrices(priceQuery)),
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
  const missingEnabledRoutes = useMemo(
    () => routes.filter((route) => route.enabled && route.visibility !== 'hidden' && (route.visibility !== 'groups' || Boolean(route.group_ids?.length)) && !prices.some((price) => String(price.route_model_id) === String(route.id) && price.enabled)),
    [prices, routes],
  )
  const priceGroups = useMemo(() => groupPrices(routes, prices), [routes, prices])
  const visibleGroups = useMemo(() => priceGroups.filter((group) => {
    if (filters.routeID && String(group.routeID) !== filters.routeID) return false
    if (filters.taskType && group.taskType !== filters.taskType) return false
    if (filters.status === 'enabled' && !group.rows.some((row) => row.enabled)) return false
    if (filters.status === 'disabled' && !group.rows.some((row) => !row.enabled)) return false
    return true
  }), [filters, priceGroups])
  const hasActiveFilters = Boolean(filters.routeID || filters.taskType || filters.status)

  const metrics = [
    { label: '启用路由', value: String(stats.enabledRoutes), trend: `共 ${stats.totalRoutes} 个路由`, tone: 'neutral' as const },
    { label: '有效价格项', value: String(stats.enabledPrices), trend: `共 ${stats.totalPrices} 个配置`, tone: 'good' as const },
    { label: '任务价格组', value: String(priceGroups.length), trend: '按路由与任务类型聚合', tone: 'neutral' as const },
    { label: '缺价风险', value: String(missingEnabledRoutes.length), trend: missingEnabledRoutes.length ? '启用路由当前不可计费' : '所有启用路由均有价格', tone: missingEnabledRoutes.length ? 'bad' as const : 'good' as const },
  ]

  function openDialog(nextDialog: PricingDialog) {
    setMutationError(null)
    setDialog(nextDialog)
  }

  async function savePricing() {
    if (!dialog) return
    setSaving(true)
    setMutationError(null)
    try {
      const payload = {
        route_model_id: Number(dialog.routeModelId),
        task_type: dialog.taskType,
        base_resolution: dialog.baseResolution,
        base_points: dialog.basePoints,
        reference_multiplier: dialog.referenceMultiplier,
        enabled: dialog.enabled,
      }
      const saved = dialog.row ? await adminApi.updateRouteModelPrice(dialog.row.id, payload) : await adminApi.createRouteModelPrice(payload)
      setDialog(null)
      onFeedback('价格配置已更新', `${adminTaskTypeLabel(saved.task_type)} · ${pricingBaseResolutionLabel(saved.base_resolution)}`)
      await load()
    } catch (caught) {
      setMutationError(caught instanceof Error ? caught.message : '价格配置保存失败')
    } finally {
      setSaving(false)
    }
  }

  if (loading) return <LoadingBlock label="载入价格策略" />
  if (error) return <ErrorBlock message={error} onRetry={load} />

  return (
    <section className={adminPage.stack}>
      <PageHeader
        title="价格策略"
        description="按路由模型、任务类型和基础分辨率维护用户积分价格，并直接处理启用路由的缺价风险。"
        primaryAction={<button className={cn(adminButton.base, adminButton.primary)} type="button" disabled={!routes.length} onClick={() => openDialog(newPriceDialog(routes))}>新增配置</button>}
      />
      <MetricStrip metrics={metrics} />

      <section data-admin-pricing-risk className={pricingClasses.riskList} aria-label="缺价风险">
        {missingEnabledRoutes.length ? (
          <>
            <InlineFeedback tone="warning" message={`发现 ${missingEnabledRoutes.length} 个启用路由缺少有效价格，相关生成请求会被阻断。`} />
            {missingEnabledRoutes.map((route) => (
              <div key={String(route.id)} className={pricingClasses.riskRow}>
                <div className="min-w-0">
                  <strong className={pricingClasses.routeName}>{route.name}</strong>
                  <div className={pricingClasses.routeMeta}>{route.code}</div>
                </div>
                <button className={cn(adminButton.base, adminButton.secondary, adminButton.small)} type="button" onClick={() => openDialog(newPriceDialogForRoute(route))}>补齐价格</button>
              </div>
            ))}
          </>
        ) : (
          <InlineFeedback tone="success" message="所有启用路由均已配置有效价格。" />
        )}
      </section>

      <div className={pricingClasses.help}>
        <span className={pricingClasses.helpIcon}><InfoIcon /></span>
        <p className="m-0 min-w-0 leading-5">最终积分按基础消耗计算；包含参考图时再应用参考图倍率。后端保留 5 位小数，这里维护的是用户积分，不是 Provider 成本。</p>
      </div>

      {!routes.length ? (
        <EmptyBlock
          title="请先创建路由模型"
          detail="价格项必须绑定路由模型。创建并启用路由后再配置任务价格。"
          action={<a className={cn(adminButton.base, adminButton.primary)} href="#/routing">配置路由模型</a>}
        />
      ) : (
        <ListPage
          filters={(
            <FilterToolbar
              fields={[
                { key: 'route', label: '路由模型', primary: true, control: <select aria-label="路由模型筛选" value={filters.routeID} onChange={(event) => setFilters({ ...filters, routeID: event.target.value })}><option value="">全部路由</option>{routes.map((route) => <option key={String(route.id)} value={String(route.id)}>{route.name} ({route.code})</option>)}</select> },
                { key: 'taskType', label: '任务类型', primary: true, control: <select aria-label="任务类型筛选" value={filters.taskType} onChange={(event) => setFilters({ ...filters, taskType: event.target.value })}><option value="">全部任务</option>{adminTaskTypeOptions.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}</select> },
                { key: 'status', label: '价格状态', control: <select aria-label="价格状态筛选" value={filters.status} onChange={(event) => setFilters({ ...filters, status: event.target.value })}><option value="">全部状态</option>{pricingStatusOptions.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}</select> },
              ]}
              actions={(
                <>
                  <span className="inline-flex items-center gap-1.5 text-xs font-semibold text-[var(--soft)]"><FilterIcon className="size-4" />{hasActiveFilters ? '已应用筛选' : '全部价格组'}</span>
                  <button className={cn(adminButton.base, adminButton.ghost, adminButton.small)} type="button" onClick={() => setFilters(initialFilters)}><XIcon className="size-4" /><span>清空</span></button>
                </>
              )}
              resultSummary={`共 ${visibleGroups.length} 个价格组 · ${prices.length} 个基础分辨率配置`}
            />
          )}
        >
          <DataTable
            columns={priceGroupColumns(expandedGroups, (key) => setExpandedGroups((current) => ({ ...current, [key]: !current[key] })), openDialog)}
            rows={visibleGroups}
            rowKey={(group) => group.key}
            renderAfterRow={(group) => expandedGroups[group.key] ? pricingExpandedGroup(group, openDialog) : null}
            empty={<EmptyBlock title="没有匹配的价格组" detail="清空筛选或为当前路由新增价格配置。" action={<button className={cn(adminButton.base, adminButton.primary)} type="button" onClick={() => openDialog(newPriceDialog(routes))}>新增配置</button>} />}
          />
        </ListPage>
      )}

      {dialog ? (
        <Modal title={dialog.row ? '调整价格配置' : '新增价格配置'} detail={pricingFieldHints.dialogDetail} onClose={() => setDialog(null)} footer={<><button className={cn(adminButton.base, adminButton.ghost)} type="button" disabled={saving} onClick={() => setDialog(null)}>取消</button><button className={cn(adminButton.base, adminButton.primary)} type="button" disabled={saving || !dialog.routeModelId || !dialog.basePoints} onClick={() => void savePricing()}>{saving ? '保存中...' : '保存'}</button></>}>
          {mutationError ? <InlineFeedback tone="danger" message={mutationError} /> : null}
          <div className={adminPage.formGrid}>
            <Field label="路由模型"><select value={dialog.routeModelId} onChange={(event) => setDialog({ ...dialog, routeModelId: event.target.value })}>{routes.map((route) => <option key={String(route.id)} value={String(route.id)}>{route.name} ({route.code})</option>)}</select></Field>
            <Field label="任务类型"><select value={dialog.taskType} onChange={(event) => setDialog({ ...dialog, taskType: event.target.value as ImageTaskType })}>{adminTaskTypeOptions.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}</select></Field>
            <Field label="基础分辨率" hint="auto 不可直接配置价格；后端会按尺寸动态映射到 1K、2K 或 4K 档位。"><select value={dialog.baseResolution} onChange={(event) => setDialog({ ...dialog, baseResolution: event.target.value })}>{pricingBaseResolutionOptions.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}</select></Field>
            <Field label="基础积分" hint={pricingFieldHints.basePoints}><input value={dialog.basePoints} onChange={(event) => setDialog({ ...dialog, basePoints: event.target.value })} placeholder="8.00000" /></Field>
            <Field label="参考图倍率" hint={pricingFieldHints.referenceMultiplier}><input value={dialog.referenceMultiplier} onChange={(event) => setDialog({ ...dialog, referenceMultiplier: event.target.value })} placeholder="1.25000" /></Field>
            <Field label="状态"><select value={dialog.enabled ? 'enabled' : 'disabled'} onChange={(event) => setDialog({ ...dialog, enabled: event.target.value === 'enabled' })}>{pricingStatusOptions.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}</select></Field>
          </div>
        </Modal>
      ) : null}
    </section>
  )
}

function pricingExpandedGroup(group: PriceGroup, onOpenDialog: (dialog: PricingDialog) => void) {
  return (
    <section className={pricingClasses.expandedSection} aria-label={`${group.routeLabel} 价格明细`}>
      <header className={pricingClasses.expandedHeader}>
        <div>
          <h2 className={cn('m-0', adminType.sectionTitle)}>{group.routeLabel}</h2>
          <p className={cn('mt-1', adminType.support)}>{adminTaskTypeLabel(group.taskType)} · {group.rows.length} 个基础分辨率配置</p>
        </div>
        <button className={cn(adminButton.base, adminButton.secondary, adminButton.small)} type="button" onClick={() => onOpenDialog(newPriceDialogForGroup(group))}>新增分辨率</button>
      </header>
      <DataTable columns={priceDetailColumns(onOpenDialog)} rows={group.rows} rowKey={(row) => row.id} />
    </section>
  )
}

function priceGroupColumns(expandedGroups: Record<string, boolean>, onToggle: (key: string) => void, onOpenDialog: (dialog: PricingDialog) => void): ColumnDef<PriceGroup>[] {
  return [
    {
      key: 'route',
      title: '路由模型',
      width: 'minmax(220px,2fr)',
      render: (group) => <div className="min-w-0"><strong className={pricingClasses.routeName}>{group.routeLabel}</strong><div className={pricingClasses.routeMeta}>{group.routeSecondary}</div></div>,
    },
    {
      key: 'taskType',
      title: '任务类型',
      width: 'minmax(140px,1.2fr)',
      render: (group) => <Badge tone="neutral">{adminTaskTypeLabel(group.taskType)}</Badge>,
    },
    {
      key: 'resolutions',
      title: '基础分辨率',
      width: 'minmax(130px,1fr)',
      align: 'right',
      kind: 'number',
      render: (group) => `${group.rows.length} 项`,
    },
    {
      key: 'status',
      title: '有效配置',
      width: 'minmax(120px,1fr)',
      align: 'right',
      kind: 'number',
      render: (group) => `${group.rows.filter((row) => row.enabled).length} / ${group.rows.length}`,
    },
    {
      key: 'actions',
      title: '操作',
      width: 'minmax(180px,1.3fr)',
      align: 'right',
      render: (group) => (
        <div className={pricingClasses.actionRow}>
          <button className={cn(adminButton.base, adminButton.secondary, adminButton.small)} type="button" aria-expanded={Boolean(expandedGroups[group.key])} onClick={() => onToggle(group.key)}>
            <ChevronDownIcon className={cn('size-4 transition-transform', expandedGroups[group.key] && 'rotate-180')} />
            <span>{expandedGroups[group.key] ? '收起' : '展开'}</span>
          </button>
          <ActionMenu actions={[{ id: `add-${group.key}`, label: '新增分辨率价格', run: () => onOpenDialog(newPriceDialogForGroup(group)) }]} />
        </div>
      ),
    },
  ]
}

function priceDetailColumns(onOpenDialog: (dialog: PricingDialog) => void): ColumnDef<RouteModelPrice>[] {
  return [
    { key: 'resolution', title: '基础分辨率', width: 'minmax(140px,1.2fr)', render: (row) => <strong className="text-[var(--fg)]">{pricingBaseResolutionLabel(row.base_resolution)}</strong> },
    { key: 'points', title: '基础消耗', width: 'minmax(130px,1fr)', align: 'right', kind: 'number', render: (row) => <code className={adminDataGrid.code}>{row.base_points} ◈</code> },
    { key: 'multiplier', title: '参考图倍率', width: 'minmax(130px,1fr)', align: 'right', kind: 'number', render: (row) => <code className={adminDataGrid.code}>x {row.reference_multiplier}</code> },
    { key: 'status', title: '状态', width: 'minmax(100px,.8fr)', render: (row) => <PricingBadge enabled={row.enabled} /> },
    { key: 'actions', title: '操作', width: 'minmax(100px,.8fr)', align: 'right', render: (row) => <button className={cn(adminButton.base, adminButton.ghost, adminButton.small)} type="button" onClick={() => onOpenDialog(editPriceDialog(row))}>调整</button> },
  ]
}

function newPriceDialog(routes: RouteModel[]): PricingDialog {
  return { routeModelId: String(routes[0]?.id ?? ''), taskType: 'text_to_image', baseResolution: '1K', basePoints: '8.00000', referenceMultiplier: '1.00000', enabled: true }
}

function newPriceDialogForRoute(route: RouteModel): PricingDialog {
  return { routeModelId: String(route.id), taskType: 'text_to_image', baseResolution: '1K', basePoints: '8.00000', referenceMultiplier: '1.00000', enabled: true }
}

function newPriceDialogForGroup(group: PriceGroup): PricingDialog {
  return { routeModelId: String(group.routeID), taskType: group.taskType, baseResolution: '1K', basePoints: '8.00000', referenceMultiplier: '1.00000', enabled: true }
}

function editPriceDialog(row: RouteModelPrice): PricingDialog {
  return { row, routeModelId: String(row.route_model_id), taskType: row.task_type, baseResolution: row.base_resolution, basePoints: row.base_points, referenceMultiplier: row.reference_multiplier, enabled: row.enabled }
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
    rows: group.rows.slice().sort((left, right) => pricingBaseResolutionLabel(left.base_resolution).localeCompare(pricingBaseResolutionLabel(right.base_resolution))),
  }))
}
