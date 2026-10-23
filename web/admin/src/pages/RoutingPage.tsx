import { useEffect, useMemo, useRef, useState } from 'react'
import type {
  AdminMetric,
  AdminVideoConfiguration,
  AdminVideoQuoteCandidate,
  AdminVideoQuoteSimulationResult,
  AdminVideoRateCard,
  AdminVideoRouteConfig,
  ImageTaskType,
  ModelAccount,
  ModelAccountModel,
  RouteModel,
  RouteModelCandidate,
  RouteModelMediaType,
  RouteModelPrice,
  RouteModelVisibility,
  UserGroup,
} from '../../../shared/api-types'
import { cn } from '../../../shared/classnames'
import { ApiError } from '../../../shared/http-client'
import { adminApi } from '../../../shared/admin-api'
import { Trash2 } from 'lucide-react'
import {
  AdminTabs,
  Badge,
  Button,
  Drawer,
  EmptyBlock,
  ErrorBlock,
  GroupOptionGrid,
  InlineFeedback,
  LoadingBlock,
  MetricStrip,
  Modal,
  PageHeader,
  RefreshIconButton,
  TooltipIconButton,
} from '../components'
import { FilterToolbar } from '../ui/dataTable'
import { ModelIconPicker, RouteModelGroupsPanel } from './RouteModelGroupsPanel'
import { loadAllRouteModelPrices } from './loadAllRouteModelPrices'
import { modelLifecycleErrorMessage } from './adminModelLifecycle'
import { createLatestListRequestGuard } from './listRefresh'
import { VideoConfigurationImpact } from './VideoConfigurationImpact'
import { AdminMediaTabs, FormSection, LedgerField, MultiSelectField, SwitchField, useAdminModelMediaTab } from './AdminMediaTabs'
import { modelMediaType } from './adminModelMedia'
import './routing-redesign.css'
import {
  routeCandidateLabel,
  routeCandidateSummary,
  routeCodeError,
  routeEnabledOptions,
  routingFieldHints,
  routingFieldLabels,
  routeGroupNames,
  routeReadinessBadge,
  routeVisibilityBadge,
  routeVisibilityOptions,
} from './routingRows'

type RouteEditorTab = 'base' | 'pricing' | 'params' | 'quote'
type PriceDraftRow = { key: string; id?: string | number; taskType: ImageTaskType; baseResolution: string; basePoints: string; referenceMultiplier: string; enabled: boolean }
type MappingDraftRow = { key: string; candidateID: string; routeResolution: string; nativeResolution: string }
type VideoDefaultsDraft = { taskType: string; durationSeconds: string; resolution: string; aspectRatio: string; audioMode: string }
type RouteEditorDraft = {
  row?: RouteModel
  tab: RouteEditorTab
  code: string
  name: string
  description: string
  visibility: RouteModelVisibility
  enabled: boolean
  sortOrder: string
  groupIds: string[]
  iconKey: string
  iconSvg: string
  priceRows: PriceDraftRow[]
  mappings: MappingDraftRow[]
  maxOutputCount: string
  videoEnabled: boolean
  minimumTaskPoints: string
  roundingStepPoints: string
  defaults: VideoDefaultsDraft
  quote?: { result?: AdminVideoQuoteSimulationResult; failedCandidates?: AdminVideoQuoteCandidate[]; error?: string; busy?: boolean }
  baseline?: string
}
type CandidateDialog = { route: RouteModel; row?: RouteModelCandidate; accountModelId: string; priority: string; weight: string; fallbackOrder: string; enabled: boolean }
type DeleteTarget = { kind: 'route'; route: RouteModel } | { kind: 'candidate'; route: RouteModel; candidate: RouteModelCandidate }

const imageTaskOptions: Array<{ value: ImageTaskType; label: string }> = [
  { value: 'text_to_image', label: '文生图' },
  { value: 'image_edit', label: '图片编辑' },
]
const imageTaskLabel = (task: string) => imageTaskOptions.find((option) => option.value === task)?.label ?? task
const baseResolutionOrder = ['1K', '2K', '4K']
const roundingOptions = ['1', '5', '10']
const videoResolutionOptions = ['480p', '720p', '768p', '1080p', '2k']
const videoRatioOptions = ['16:9', '9:16', '1:1', '4:3', '3:4', '21:9', 'adaptive']
const videoDurationOptions = ['4', '5', '8', '10', '12', '15']

const routingClasses = {
  actionRow: 'flex flex-wrap items-center gap-2',
  paragraph: 'm-0 text-xs leading-5 text-[var(--a-ink-3)] [overflow-wrap:anywhere]',
  routeTitle: 'text-sm font-semibold text-[var(--a-ink)]',
  routeCode: 'font-[family-name:var(--font-admin-mono)] text-xs text-[var(--a-ink-3)]',
  detailHead: 'flex flex-wrap items-start justify-between gap-4 border-b border-[var(--a-line)] pb-4',
}

export function RoutingPage({ onFeedback }: { onFeedback: (title: string, detail?: string) => void }) {
  const media = useAdminModelMediaTab()
  if (media === 'image' || media === 'video') {
    return <MediaRoutingPanel media={media} onFeedback={onFeedback} />
  }
  return (
    <section className="a-page-stack">
      <PageHeader
        eyebrow="Model Routing"
        title="路由模型"
        description="图片与视频路由分开维护。图片路由按路由 + 任务类型 + 基础分辨率定价；视频路由使用候选厂商原生费率、分辨率映射与最低任务积分。"
      />
      <AdminMediaTabs route="routing" value={media} />
      {media === 'audio' ? (
        <EmptyBlock title="音频路由暂未开放" detail="音频模型运行时接入后，可在此维护多候选路由、参数映射与原生费率。" />
      ) : (
        <EmptyBlock title="文本暂使用默认模型" detail="提示词优化当前通过默认文本模型工作，不提供尚未实现的多候选路由。" action={<a className="a-btn a-btn--primary" href="#/access-accounts?media=text">管理文本模型</a>} />
      )}
    </section>
  )
}

function MediaRoutingPanel({ media, onFeedback }: { media: RouteModelMediaType; onFeedback: (title: string, detail?: string) => void }) {
  const [routes, setRoutes] = useState<RouteModel[]>([])
  const [groups, setGroups] = useState<UserGroup[]>([])
  const [modelAccounts, setModelAccounts] = useState<ModelAccount[]>([])
  const [accountModels, setAccountModels] = useState<ModelAccountModel[]>([])
  const [videoSnapshot, setVideoSnapshot] = useState<AdminVideoConfiguration | null>(null)
  const [candidates, setCandidates] = useState<Record<string, RouteModelCandidate[]>>({})
  const [prices, setPrices] = useState<RouteModelPrice[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [notice, setNotice] = useState('路由模型可设置全员可见、按分组可见或隐藏，并绑定多个真实模型候选。')
  const [routeEditor, setRouteEditor] = useState<RouteEditorDraft | null>(null)
  const [candidateDialog, setCandidateDialog] = useState<CandidateDialog | null>(null)
  const [selectedRouteId, setSelectedRouteId] = useState('')
  const [query, setQuery] = useState('')
  const [saving, setSaving] = useState(false)
  const [mutationError, setMutationError] = useState<string | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<DeleteTarget | null>(null)
  const [deleting, setDeleting] = useState(false)
  const requestGuard = useRef(createLatestListRequestGuard()).current

  const load = async (preferredRouteId?: string) => {
    const request = requestGuard.begin()
    setLoading(true)
    setError(null)
    try {
      const [nextRoutes, nextGroups, nextAccounts, nextPrices, nextVideoSnapshot] = await Promise.all([
        adminApi.listRouteModels({ page_size: 100 }),
        adminApi.listUserGroups(),
        adminApi.listModelAccounts({ page_size: 100 }),
        loadAllRouteModelPrices((priceQuery) => adminApi.listRouteModelPrices(priceQuery)),
        media === 'video' ? adminApi.getVideoConfiguration() : Promise.resolve(null),
      ])
      const mediaRoutes = nextRoutes.filter((route) => route.media_type === media)
      const modelLists = await Promise.all(nextAccounts.map((account) => adminApi.listModelAccountModels(account.id)))
      const candidatePairs = await Promise.all(mediaRoutes.map(async (route) => [String(route.id), await adminApi.listRouteModelCandidates(route.id)] as const))
      if (!requestGuard.isCurrent(request)) return
      const mediaRouteIDs = new Set(mediaRoutes.map((route) => String(route.id)))
      setRoutes(mediaRoutes)
      setGroups(nextGroups)
      setModelAccounts(nextAccounts)
      setAccountModels(modelLists.flat())
      setCandidates(Object.fromEntries(candidatePairs))
      setPrices(nextPrices.filter((price) => mediaRouteIDs.has(String(price.route_model_id))))
      setVideoSnapshot(nextVideoSnapshot)
      setSelectedRouteId((current) => {
        const preferred = preferredRouteId || current
        return mediaRoutes.some((route) => String(route.id) === preferred) ? preferred : String(mediaRoutes[0]?.id ?? '')
      })
    } catch (caught) {
      if (!requestGuard.isCurrent(request)) return
      setError(caught instanceof Error ? caught.message : '路由载入失败')
    } finally {
      if (!requestGuard.isCurrent(request)) return
      setLoading(false)
    }
  }

  useEffect(() => {
    void load()
    return () => requestGuard.invalidate()
  }, [media])

  const totals = useMemo(() => ({
    enabled: routes.filter((row) => row.enabled).length,
    groupVisible: routes.filter((row) => row.visibility === 'groups').length,
    candidateCount: Object.values(candidates).flat().length,
    blocked: routes.filter((route) => routeReadinessForMedia({
      route,
      candidates: effectiveRouteCandidates(candidates[String(route.id)] ?? route.candidates ?? [], accountModels, modelAccounts),
      prices: prices.filter((price) => String(price.route_model_id) === String(route.id)),
      videoConfig: videoSnapshot?.routes.find((item) => String(item.route_model_id) === String(route.id)),
      videoImpacts: videoSnapshot?.impacts ?? [],
    }).state !== 'ready').length,
  }), [routes, candidates, prices, accountModels, modelAccounts, videoSnapshot])
  const summaryMetrics = useMemo<AdminMetric[]>(() => [
    { label: media === 'video' ? '视频路由' : '图片路由', value: String(routes.length), trend: `${totals.enabled} 个启用`, tone: 'neutral' },
    { label: '候选模型', value: String(totals.candidateCount), trend: '已绑定真实模型', tone: 'neutral' },
    { label: media === 'video' ? '原生费率路由' : '分组可见', value: media === 'video' ? String(videoSnapshot?.routes.length ?? 0) : String(totals.groupVisible), trend: media === 'video' ? '已配置视频参数' : '受权益分组约束', tone: 'neutral' },
    { label: '阻断项', value: String(totals.blocked), trend: totals.blocked ? '需要完成配置' : '全部可用', tone: totals.blocked ? 'warn' : 'good' },
  ], [media, routes.length, totals, videoSnapshot?.routes.length])
  const visibleRoutes = useMemo(() => {
    const keyword = query.trim().toLowerCase()
    if (!keyword) return routes
    return routes.filter((row) => [
      row.name,
      row.code,
      row.description,
      routeGroupNames(row.group_ids, groups),
    ].filter(Boolean).some((value) => String(value).toLowerCase().includes(keyword)))
  }, [groups, query, routes])
  useEffect(() => {
    if (visibleRoutes.length && !visibleRoutes.some((route) => String(route.id) === selectedRouteId)) {
      setSelectedRouteId(String(visibleRoutes[0].id))
    }
  }, [selectedRouteId, visibleRoutes])
  const selectedRoute = useMemo(() => visibleRoutes.find((route) => String(route.id) === selectedRouteId), [visibleRoutes, selectedRouteId])
  const eligibleAccountModels = useMemo(() => {
    const mediaModels = accountModels.filter((model) => {
      const account = modelAccounts.find((item) => String(item.id) === String(model.account_id))
      return account ? modelMediaType(model, account) === media : false
    })
    if (media !== 'video') return mediaModels
    const verified = new Set((videoSnapshot?.capabilities ?? []).filter((item) => item.enabled && item.validation_status === 'verified').map((item) => String(item.account_model_id)))
    const enabledAccounts = new Set(modelAccounts.filter((account) => account.status === 'enabled').map((account) => String(account.id)))
    return mediaModels.filter((model) => model.enabled && enabledAccounts.has(String(model.account_id)) && verified.has(String(model.id)))
  }, [accountModels, media, modelAccounts, videoSnapshot])

  const routeCandidates = selectedRoute ? candidates[String(selectedRoute.id)] ?? selectedRoute.candidates ?? [] : []
  const routePrices = selectedRoute ? prices.filter((price) => String(price.route_model_id) === String(selectedRoute.id)) : []
  const routeVideoConfig = media === 'video' ? videoSnapshot?.routes.find((item) => String(item.route_model_id) === String(selectedRoute?.id)) : undefined
  const editorErrors = routeEditor
    ? routeEditorErrors(routeEditor, media, accountModels, routeEditor.row ? (candidates[String(routeEditor.row.id)] ?? []) : [])
    : {}

  useEffect(() => {
    if (!routeEditor || media !== 'video' || routeEditor.tab !== 'quote' || !routeEditor.row || !routeCandidates.length) return
    if (routeEditor.quote?.busy || routeEditor.quote?.result || routeEditor.quote?.error) return
    void simulateQuote(routeEditor, setRouteEditor)
  }, [media, routeCandidates.length, routeEditor?.row?.id, routeEditor?.tab, routeEditor?.quote?.busy, routeEditor?.quote?.error])

  function openNewRoute() {
    setMutationError(null)
    setRouteEditor(buildNewRouteEditor(media, groups))
  }

  function openEditRoute(route: RouteModel, tab: RouteEditorTab = 'base') {
    setMutationError(null)
    setRouteEditor(buildRouteEditor({
      route,
      media,
      tab,
      prices: prices.filter((price) => String(price.route_model_id) === String(route.id)),
      candidates: candidates[String(route.id)] ?? route.candidates ?? [],
      accountModels,
      videoConfig: videoSnapshot?.routes.find((item) => String(item.route_model_id) === String(route.id)),
      rateCards: videoSnapshot?.rate_cards ?? [],
    }))
  }

  async function deletePriceRow(row: PriceDraftRow) {
    if (!routeEditor) return
    setSaving(true)
    setMutationError(null)
    try {
      if (row.id) await adminApi.deleteRouteModelPrice(row.id)
      setRouteEditor({ ...routeEditor, priceRows: routeEditor.priceRows.filter((item) => item.key !== row.key) })
      onFeedback('价格配置已删除', `${imageTaskLabel(row.taskType)} · ${row.baseResolution}`)
      await load(String(routeEditor.row?.id ?? ''))
    } catch (caught) {
      setMutationError(caught instanceof Error ? caught.message : '删除价格配置失败')
    } finally {
      setSaving(false)
    }
  }

  async function deleteVideoConfig() {
    if (!routeEditor?.row) return
    setSaving(true)
    setMutationError(null)
    try {
      await adminApi.deleteRouteVideoConfig(routeEditor.row.id)
      setRouteEditor({ ...routeEditor, mappings: [], videoEnabled: false, quote: undefined })
      onFeedback('视频配置已删除', `${routeEditor.row.name} · 未删除路由与候选`)
      await load(String(routeEditor.row.id))
    } catch (caught) {
      setMutationError(caught instanceof Error ? caught.message : '删除视频配置失败')
    } finally {
      setSaving(false)
    }
  }

  async function saveRoute() {
    if (!routeEditor) return
    setSaving(true)
    setMutationError(null)
    const editorErrors = routeEditorErrors(routeEditor, media, accountModels, routeEditor.row ? (candidates[String(routeEditor.row.id)] ?? []) : [])
    if (Object.keys(editorErrors).length) {
      setMutationError(Object.values(editorErrors)[0])
      setSaving(false)
      return
    }
    try {
      const groupIds = routeEditor.visibility === 'groups' ? routeEditor.groupIds.map(Number).filter((id) => Number.isFinite(id) && id > 0) : []
      const payload = {
        code: routeEditor.code.trim().toLowerCase(),
        name: routeEditor.name,
        description: routeEditor.description,
        visibility: routeEditor.visibility,
        media_type: media,
        enabled: routeEditor.enabled,
        sort_order: Number(routeEditor.sortOrder),
        group_ids: groupIds,
        icon_key: routeEditor.iconKey,
        icon_svg: routeEditor.iconSvg,
      }
      const saved = routeEditor.row ? await adminApi.updateRouteModel(routeEditor.row.id, payload) : await adminApi.createRouteModel(payload)
      if (media === 'image') {
        await Promise.all(routeEditor.priceRows.map(async (row) => {
          if (!row.basePoints.trim() && !row.id) return
          const pricePayload = {
            route_model_id: Number(saved.id),
            task_type: row.taskType,
            base_resolution: row.baseResolution,
            base_points: row.basePoints,
            reference_multiplier: row.referenceMultiplier || '1.00000',
            enabled: row.enabled,
          }
          if (row.id) await adminApi.updateRouteModelPrice(row.id, pricePayload)
          else await adminApi.createRouteModelPrice(pricePayload)
        }))
      } else {
        const currentCandidates = routeEditor.row ? candidates[String(routeEditor.row.id)] ?? [] : []
        const shouldSaveVideoConfig = Boolean(routeEditor.row) && (currentCandidates.length > 0 || Boolean(routeVideoConfig))
        if (shouldSaveVideoConfig) {
          const existingConfig = routeVideoConfig
          const visibleOptions = { ...(existingConfig?.visible_options ?? {}) }
          delete visibleOptions.combinations
          await adminApi.saveRouteVideoConfig(saved.id, {
            expected_version: existingConfig?.config_version ?? '',
            config_version: nextVersion('video-route', existingConfig?.config_version),
            candidate_parameter_mappings: buildMappings(routeEditor.mappings),
            minimum_task_points: routeEditor.minimumTaskPoints || '0.00000',
            rounding_step_points: Number(routeEditor.roundingStepPoints) || 1,
            task_types: Array.from(new Set(currentCandidates.map((candidate) => {
              const model = accountModels.find((item) => String(item.id) === String(candidate.account_model_id))
              return model?.task_types ?? []
            }).flat())),
            visible_options: visibleOptions,
            defaults: {
              task_type: routeEditor.defaults.taskType,
              duration_seconds: Number(routeEditor.defaults.durationSeconds) || 5,
              resolution: routeEditor.defaults.resolution,
              aspect_ratio: routeEditor.defaults.aspectRatio,
              audio_mode: routeEditor.defaults.audioMode,
              generate_audio: routeEditor.defaults.audioMode === 'generated',
            },
            max_output_count: Number(routeEditor.maxOutputCount) || 1,
            enabled: routeEditor.videoEnabled,
          })
        }
      }
      const created = !routeEditor.row
      setRouteEditor(null)
      setMutationError(null)
      setNotice(`${saved.name} 路由模型已保存。`)
      onFeedback('路由模型已保存', saved.code)
      setSelectedRouteId(String(saved.id))
      if (created) setQuery('')
      await load(String(saved.id))
    } catch (caught) {
      setMutationError(caught instanceof Error ? caught.message : '路由模型保存失败')
    } finally {
      setSaving(false)
    }
  }

  async function saveCandidate() {
    if (!candidateDialog) return
    setSaving(true)
    setMutationError(null)
    try {
      const payload = {
        account_model_id: Number(candidateDialog.accountModelId),
        priority: Number(candidateDialog.priority),
        weight: Number(candidateDialog.weight),
        fallback_order: Number(candidateDialog.fallbackOrder),
        enabled: candidateDialog.enabled,
      }
      if (candidateDialog.row) await adminApi.updateRouteModelCandidate(candidateDialog.route.id, candidateDialog.row.id, payload)
      else await adminApi.createRouteModelCandidate(candidateDialog.route.id, payload)
      setCandidateDialog(null)
      setMutationError(null)
      setNotice(`${candidateDialog.route.code} 候选已保存。`)
      await load(String(candidateDialog.route.id))
    } catch (caught) {
      setMutationError(caught instanceof Error ? caught.message : '候选模型保存失败')
    } finally {
      setSaving(false)
    }
  }

  async function confirmDelete() {
    if (!deleteTarget) return
    setDeleting(true)
    setMutationError(null)
    try {
      if (deleteTarget.kind === 'route') await adminApi.deleteRouteModel(deleteTarget.route.id)
      else await adminApi.deleteRouteModelCandidate(deleteTarget.route.id, deleteTarget.candidate.id)
      const deletedLabel = deleteTarget.kind === 'route' ? deleteTarget.route.code : deleteTarget.candidate.model_code || String(deleteTarget.candidate.id)
      setDeleteTarget(null)
      setNotice(`${deletedLabel} 已删除。`)
      await load()
    } catch (caught) {
      setMutationError(modelLifecycleErrorMessage(caught, '删除路由配置失败'))
    } finally {
      setDeleting(false)
    }
  }

  if (loading && !routes.length) return <LoadingBlock label="载入模型路由" />
  if (error && !routes.length) return <ErrorBlock message={error} onRetry={load} />

  return (
    <section className="a-page-stack">
      <PageHeader
        eyebrow="Model Routing"
        title="路由模型"
        description={media === 'video'
          ? '视频路由使用候选厂商原生费率、分辨率映射与最低任务积分；销售价由系统推导，不允许手工填写每秒售价。'
          : '图片路由按路由 + 任务类型 + 基础分辨率定价；缺价组合会在提交前阻断。'}
        primaryAction={<Button variant="primary" size="sm" onClick={openNewRoute}>{media === 'video' ? '新增视频路由' : '新增图片路由'}</Button>}
        secondaryActions={<RefreshIconButton label="刷新路由模型" refreshing={loading} onClick={() => void load()} />}
      />
      <AdminMediaTabs route="routing" value={media} />
      {media === 'video' ? <VideoConfigurationImpact context="routing" /> : null}
      <RouteModelGroupsPanel media={media} routes={routes} onFeedback={onFeedback} onChanged={() => void load()} />
      {error ? <InlineFeedback tone="danger" message={`路由模型刷新失败：${error}`} /> : null}
      <MetricStrip metrics={summaryMetrics} />
      <FilterToolbar
        fields={[{
          key: 'query',
          label: '搜索路由',
          primary: true,
          minWidth: '240px',
          maxWidth: '420px',
          control: <input value={query} onChange={(event) => setQuery(event.target.value)} className="a-input" placeholder="名称、代码或分组" aria-label="搜索路由名称或代码" />,
        }]}
        resultSummary={`共 ${routes.length} 个路由 · 当前显示 ${visibleRoutes.length} 个`}
      />
      <InlineFeedback tone={totals.blocked ? 'warning' : 'success'} message={notice} />
      {!routes.length ? <EmptyBlock title="暂无路由模型" detail="创建路由模型后继续配置候选真实模型、可见范围和价格。" action={<Button variant="primary" size="sm" onClick={openNewRoute}>{media === 'video' ? '新增视频路由' : '新增图片路由'}</Button>} /> : null}
      {routes.length && !visibleRoutes.length ? <EmptyBlock title="未找到路由模型" detail="换一个名称、代码或分组关键词再试。" /> : null}
      {visibleRoutes.length ? (
        <div className="rt-layout" data-admin-routing-workspace>
          <section className="a-panel a-panel--flush">
            <div className="a-panel__head" style={{ padding: '13px 15px' }}>
              <div><h2 style={{ fontSize: 13 }}>{media === 'video' ? '视频路由列表' : '图片路由列表'}</h2></div>
              <span className="a-hint">{visibleRoutes.length}</span>
            </div>
            <div style={{ padding: '11px 13px', borderBottom: '1px solid var(--a-line)' }}>
              <div className="a-search">
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.6" aria-hidden="true"><circle cx="11" cy="11" r="7" /><path d="m20 20-3.5-3.5" /></svg>
                <input className="a-input a-input--sm" value={query} onChange={(event) => setQuery(event.target.value)} placeholder="搜索名称 / 代码 / 分组" />
              </div>
            </div>
            <div className="rt-list">
              {visibleRoutes.map((route) => {
                const routeCandidateList = candidates[String(route.id)] ?? route.candidates ?? []
                const readiness = routeReadinessForMedia({
                  route,
                  candidates: effectiveRouteCandidates(routeCandidateList, accountModels, modelAccounts),
                  prices: prices.filter((price) => String(price.route_model_id) === String(route.id)),
                  videoConfig: videoSnapshot?.routes.find((item) => String(item.route_model_id) === String(route.id)),
                  videoImpacts: videoSnapshot?.impacts ?? [],
                })
                const active = String(route.id) === selectedRouteId
                return (
                  <button key={String(route.id)} className={cn('rt-item', active && 'is-active')} type="button" aria-pressed={active} onClick={() => setSelectedRouteId(String(route.id))}>
                    <span className="rt-item__head"><strong>{route.name}</strong><RouteBadge badge={readiness} /></span>
                    <span>{route.code} · {routeCandidateList.length} 个候选</span>
                  </button>
                )
              })}
            </div>
            <div className="a-pager" style={{ padding: '10px 13px' }}>
              <span className="a-pager__info">共 {visibleRoutes.length} 条路由</span>
              <span className="a-pager__btn is-active">1</span>
            </div>
          </section>
          {selectedRoute ? (
            <RouteDetailWorkspace
              route={selectedRoute}
              groups={groups}
              candidates={routeCandidates}
              prices={routePrices}
              accountModels={eligibleAccountModels}
              modelAccounts={modelAccounts}
              videoConfig={routeVideoConfig}
              videoImpacts={media === 'video' ? videoSnapshot?.impacts ?? [] : []}
              rateCards={videoSnapshot?.rate_cards ?? []}
              onEditRoute={(tab) => openEditRoute(selectedRoute, tab)}
              onAddCandidate={() => { setMutationError(null); setCandidateDialog(newCandidateDialog(selectedRoute, eligibleAccountModels)) }}
              onEditCandidate={(candidate) => { setMutationError(null); setCandidateDialog(editCandidateDialog(selectedRoute, candidate)) }}
              onDeleteRoute={() => { setMutationError(null); setDeleteTarget({ kind: 'route', route: selectedRoute }) }}
              onDeleteCandidate={(candidate) => { setMutationError(null); setDeleteTarget({ kind: 'candidate', route: selectedRoute, candidate }) }}
            />
          ) : <EmptyBlock title="选择路由模型" detail="从左侧选择一个路由，查看候选、能力与价格状态。" />}
        </div>
      ) : null}

      {routeEditor ? (
        <Drawer
          wide={routeEditor.tab !== 'base'}
          title={`${routeEditor.row ? '编辑' : '新增'}${media === 'video' ? '视频' : '图片'}路由模型`}
          description={`${routeEditor.row?.name || routeEditor.name || '新路由'}${routeEditor.row ? ` · ${routeEditor.code} · ${media === 'video' ? '视频' : '图片'}` : ''}`}
          headerAside={<span className={cn('a-badge', routeEditor.enabled ? 'a-badge--ok' : 'a-badge--warn')}><i />{routeEditor.enabled ? '可被用户使用' : '已停用'}</span>}
          onClose={() => { setRouteEditor(null); setMutationError(null) }}
          footer={<>
            <span className={cn('a-badge', Object.keys(editorErrors).length ? 'a-badge--bad' : routeEditorDirtyCount(routeEditor) ? 'a-badge--warn' : 'a-badge--ok')}><i />{Object.keys(editorErrors).length ? `${Object.keys(editorErrors).length} 项校验未通过` : routeEditorDirtyCount(routeEditor) ? `${routeEditorDirtyCount(routeEditor)} 项未保存` : '未修改'}</span>
            <div style={{ flex: 1 }} />
            {routeEditor.row ? <Button variant="danger" size="sm" disabled={saving} onClick={() => { setMutationError(null); setDeleteTarget({ kind: 'route', route: routeEditor.row! }) }}>删除路由</Button> : null}
            <Button variant="ghost" size="sm" disabled={saving} onClick={() => { setRouteEditor(null); setMutationError(null) }}>取消</Button>
            <Button variant="primary" size="sm" disabled={saving || Object.keys(editorErrors).length > 0} onClick={() => void saveRoute()}>{saving ? '保存中...' : '保存路由'}</Button>
          </>}
        >
          {mutationError ? <InlineFeedback tone="danger" message={mutationError} /> : null}
          <AdminTabs<RouteEditorTab>
            ariaLabel="路由编辑分区"
            value={routeEditor.tab}
            onChange={(tab) => setRouteEditor({ ...routeEditor, tab })}
            items={[
              { id: 'base', label: '基础信息' },
              ...(media === 'image' ? [{ id: 'pricing' as const, label: '图片价格' }] : [{ id: 'params' as const, label: '视频参数' }, { id: 'quote' as const, label: '视频报价' }]),
            ]}
          />
          {routeEditor.tab === 'base' ? (
            <form className="a-form" onSubmit={(event) => event.preventDefault()}>
              <FormSection index={1} title="标识">
                <LedgerField label="路由代码" required error={editorErrors.code} hint={routeEditor.row ? '用户端与 API 使用的抽象模型代码；保存后新请求立即使用新代码（统一转为小写），历史记录保留原代码快照。' : '用户端与 API 使用的抽象模型代码，支持字母、数字、点、下划线与短横线；保存时统一转为小写。'}>
                  <input className="a-input" value={routeEditor.code} aria-invalid={Boolean(editorErrors.code)} autoCapitalize="none" autoCorrect="off" spellCheck={false} placeholder="例如：plus-image" onChange={(event) => setRouteEditor({ ...routeEditor, code: event.target.value })} style={{ fontFamily: 'var(--font-admin-mono)' }} />
                </LedgerField>
                <LedgerField label="路由名称" required error={editorErrors.name}><input className="a-input" value={routeEditor.name} onChange={(event) => setRouteEditor({ ...routeEditor, name: event.target.value })} /></LedgerField>
                <LedgerField label="路由图标" hint="显示在用户端模型选择与后台路由列表；缺省使用站点图标。">
                  <ModelIconPicker iconKey={routeEditor.iconKey} iconSvg={routeEditor.iconSvg} onChange={(next) => setRouteEditor({ ...routeEditor, ...next })} />
                </LedgerField>
                <LedgerField label="描述"><textarea className="a-textarea" value={routeEditor.description} onChange={(event) => setRouteEditor({ ...routeEditor, description: event.target.value })} /></LedgerField>
              </FormSection>
              <FormSection index={2} title="可见性">
                <LedgerField label="可见范围"><select className="a-select" value={routeEditor.visibility} onChange={(event) => setRouteEditor({ ...routeEditor, visibility: event.target.value, groupIds: event.target.value === 'groups' ? routeEditor.groupIds : [] })}>{routeVisibilityOptions.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}</select></LedgerField>
                {routeEditor.visibility === 'groups' ? <LedgerField label="可见分组" error={editorErrors.groups}><GroupOptionGrid selected={routeEditor.groupIds} groups={groups} onChange={(groupIds) => setRouteEditor({ ...routeEditor, groupIds })} /></LedgerField> : null}
                <div className="a-form__pair">
                  <LedgerField label="排序值" error={editorErrors.sortOrder} hint="数值越小展示越靠前。"><input className="a-input" type="number" value={routeEditor.sortOrder} onChange={(event) => setRouteEditor({ ...routeEditor, sortOrder: event.target.value })} /></LedgerField>
                  <LedgerField label="启用状态"><select className="a-select" value={routeEditor.enabled ? 'enabled' : 'disabled'} onChange={(event) => setRouteEditor({ ...routeEditor, enabled: event.target.value === 'enabled' })}>{routeEnabledOptions.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}</select></LedgerField>
                </div>
              </FormSection>
              <FormSection index={3} title="能力检查">
                <div className="a-banner a-banner--info">
                  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.6" aria-hidden="true"><circle cx="12" cy="12" r="9" /><path d="M12 11v5M12 7.5h.01" /></svg>
                  <div><strong style={{ fontSize: 13 }}>当前可用</strong><div className="a-hint">{routeEditor.row ? `${routeCandidates.filter((candidate) => candidate.enabled).length} 个候选启用，${routePrices.filter((price) => price.enabled).length} 个价格启用，${routeEditor.groupIds.length} 个分组可见。` : '保存后继续配置候选真实模型、可见范围与价格。'}</div></div>
                </div>
                <div className="a-row-actions">
                  {media === 'image' ? <Button variant="secondary" size="sm" onClick={() => setRouteEditor({ ...routeEditor, tab: 'pricing' })}>配置价格</Button> : <><Button variant="secondary" size="sm" onClick={() => setRouteEditor({ ...routeEditor, tab: 'params' })}>配置视频参数</Button><Button variant="secondary" size="sm" onClick={() => setRouteEditor({ ...routeEditor, tab: 'quote' })}>查看视频报价</Button></>}
                  {routeEditor.row ? <Button variant="secondary" size="sm" onClick={() => { setRouteEditor(null); setCandidateDialog(newCandidateDialog(routeEditor.row!, eligibleAccountModels)) }}>配置候选</Button> : null}
                </div>
              </FormSection>
            </form>
          ) : null}

          {routeEditor.tab === 'pricing' ? (
            <form className="a-form" onSubmit={(event) => event.preventDefault()}>
              <FormSection title="图片价格" description="图片价格按路由 + 任务类型 + 基础分辨率维护；参考图倍率只影响含参考图的图片编辑任务。">
                <div className="a-banner a-banner--info">
                  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.6" aria-hidden="true"><circle cx="12" cy="12" r="9" /><path d="M12 11v5M12 7.5h.01" /></svg>
                  <div><strong style={{ fontSize: 13 }}>缺价组合会在提交前阻断</strong><div className="a-hint">价格保存后立即用于用户端估价；历史流水不会回溯重算。</div></div>
                </div>
                {routeEditor.priceRows.length ? (
                  <div className="rt-price-wrap">
                    <table className="rt-price-table">
                      <thead><tr><th>任务类型</th><th>基础分辨率</th><th className="is-num">基础积分</th><th className="is-num">参考图倍率</th><th>状态</th><th className="is-actions">操作</th></tr></thead>
                      <tbody>
                        {routeEditor.priceRows.map((row, index) => {
                          const missing = !row.basePoints.trim()
                          return (
                            <tr key={row.key}>
                              <td>{imageTaskLabel(row.taskType)}</td>
                              <td className="a-mono">{row.baseResolution}</td>
                              <td className="is-num"><input className="a-input cfg-table-input" inputMode="decimal" value={row.basePoints} placeholder="8.00000" onChange={(event) => setRouteEditor({ ...routeEditor, priceRows: updatePriceRow(routeEditor.priceRows, index, { basePoints: event.target.value }) })} /></td>
                              <td className="is-num"><input className="a-input cfg-table-input" inputMode="decimal" value={row.referenceMultiplier} placeholder="1.00000" onChange={(event) => setRouteEditor({ ...routeEditor, priceRows: updatePriceRow(routeEditor.priceRows, index, { referenceMultiplier: event.target.value }) })} /></td>
                              <td>
                                <select className="a-select a-select--sm" value={row.enabled ? 'enabled' : 'disabled'} onChange={(event) => setRouteEditor({ ...routeEditor, priceRows: updatePriceRow(routeEditor.priceRows, index, { enabled: event.target.value === 'enabled' }) })}>
                                  <option value="enabled">启用</option>
                                  <option value="disabled">停用</option>
                                </select>
                                {missing ? <span className="a-badge a-badge--warn" style={{ marginLeft: 6 }}><i />缺价格</span> : null}
                              </td>
                              <td className="is-actions">
                                <TooltipIconButton label={`删除 ${imageTaskLabel(row.taskType)} ${row.baseResolution} 价格`} disabled={saving} onClick={() => void deletePriceRow(row)}><Trash2 /></TooltipIconButton>
                              </td>
                            </tr>
                          )
                        })}
                      </tbody>
                    </table>
                  </div>
                ) : (
                  <div className="cfg-empty-inline">当前路由尚未绑定图片候选。绑定候选后，系统会按候选模型的任务类型与基础分辨率生成价格组合；未绑定候选时可以先保存路由。</div>
                )}
                {routeEditorPriceError(routeEditor, media, accountModels) ? <InlineFeedback tone="warning" message={routeEditorPriceError(routeEditor, media, accountModels) ?? ''} /> : null}
              </FormSection>
            </form>
          ) : null}

          {routeEditor.tab === 'params' ? (
            <form className="a-form" onSubmit={(event) => event.preventDefault()}>
              <FormSection index={1} title="输出与状态" description="平台最大输出数受候选能力限制；最低任务积分与取整步长决定最终报价。">
                <div className="a-form__pair">
                  <LedgerField label="平台最大输出数" error={editorErrors.maxOutputCount} hint="1–4；实际可用数量受候选能力限制。"><input className="a-input" type="number" min="1" max="4" value={routeEditor.maxOutputCount} onChange={(event) => setRouteEditor({ ...routeEditor, maxOutputCount: String(Math.min(4, Math.max(1, Number(event.target.value) || 1))), quote: undefined })} /></LedgerField>
                  <LedgerField label="状态"><select className="a-select" value={routeEditor.videoEnabled ? 'enabled' : 'disabled'} onChange={(event) => setRouteEditor({ ...routeEditor, videoEnabled: event.target.value === 'enabled' })}><option value="enabled">启用</option><option value="disabled">停用</option></select></LedgerField>
                </div>
                <div className="a-form__pair">
                  <LedgerField label="最低任务积分" error={editorErrors.minimumTaskPoints}><input className="a-input" inputMode="decimal" value={routeEditor.minimumTaskPoints} onChange={(event) => setRouteEditor({ ...routeEditor, minimumTaskPoints: event.target.value, quote: undefined })} placeholder="10.00000" /></LedgerField>
                  <LedgerField label="积分取整步长" error={editorErrors.roundingStepPoints}><select className="a-select" value={routeEditor.roundingStepPoints} onChange={(event) => setRouteEditor({ ...routeEditor, roundingStepPoints: event.target.value, quote: undefined })}>{roundingOptions.map((value) => <option key={value} value={value}>{value}</option>)}</select></LedgerField>
                </div>
              </FormSection>
              <FormSection index={2} title="默认参数" description="用户可选参数由已绑定真实模型的能力自动汇总；这里只维护默认值。">
                <div className="a-form__pair">
                  <LedgerField label="默认任务类型"><select className="a-select" value={routeEditor.defaults.taskType} onChange={(event) => setRouteEditor({ ...routeEditor, defaults: { ...routeEditor.defaults, taskType: event.target.value }, quote: undefined })}>{videoTaskOptionsFromCandidates(routeCandidates, accountModels).map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}</select></LedgerField>
                  <LedgerField label="默认时长（秒）"><select className="a-select" value={routeEditor.defaults.durationSeconds} onChange={(event) => setRouteEditor({ ...routeEditor, defaults: { ...routeEditor.defaults, durationSeconds: event.target.value }, quote: undefined })}>{videoDurationOptions.map((value) => <option key={value} value={value}>{value}</option>)}</select></LedgerField>
                </div>
                <div className="a-form__pair">
                  <LedgerField label="默认分辨率" error={editorErrors.defaults}><select className="a-select" value={routeEditor.defaults.resolution} onChange={(event) => setRouteEditor({ ...routeEditor, defaults: { ...routeEditor.defaults, resolution: event.target.value }, quote: undefined })}>{videoResolutionOptions.map((value) => <option key={value} value={value}>{value}</option>)}</select></LedgerField>
                  <LedgerField label="默认比例"><select className="a-select" value={routeEditor.defaults.aspectRatio} onChange={(event) => setRouteEditor({ ...routeEditor, defaults: { ...routeEditor.defaults, aspectRatio: event.target.value }, quote: undefined })}>{videoRatioOptions.map((value) => <option key={value} value={value}>{value}</option>)}</select></LedgerField>
                </div>
                <LedgerField label="默认音频"><select className="a-select" value={routeEditor.defaults.audioMode} onChange={(event) => setRouteEditor({ ...routeEditor, defaults: { ...routeEditor.defaults, audioMode: event.target.value }, quote: undefined })}><option value="silent">静音</option><option value="generated">生成音频</option></select></LedgerField>
              </FormSection>
              <FormSection index={3} title="候选分辨率映射" description="路由分辨率到厂商原生分辨率的映射；未映射的组合不参与报价，提交前会被阻断。">
                <div className="cfg-scroll-x">
                  <table className="a-table" style={{ minWidth: 680 }}>
                    <thead><tr><th>真实模型</th><th>路由分辨率</th><th>厂商原生分辨率</th><th className="is-actions">操作</th></tr></thead>
                    <tbody>
                      {routeEditor.mappings.map((row, index) => (
                        <tr key={row.key}>
                          <td>{candidateLabel(row.candidateID, routeCandidates)}</td>
                          <td><input className="a-input cfg-table-input" value={row.routeResolution} onChange={(event) => setRouteEditor({ ...routeEditor, mappings: updateMappingRow(routeEditor.mappings, index, { routeResolution: event.target.value }), quote: undefined })} /></td>
                          <td><input className="a-input cfg-table-input" value={row.nativeResolution} onChange={(event) => setRouteEditor({ ...routeEditor, mappings: updateMappingRow(routeEditor.mappings, index, { nativeResolution: event.target.value }), quote: undefined })} /></td>
                          <td className="is-actions"><button className="a-row-btn a-row-btn--danger" type="button" onClick={() => setRouteEditor({ ...routeEditor, mappings: routeEditor.mappings.filter((_, rowIndex) => rowIndex !== index), quote: undefined })}>删除</button></td>
                        </tr>
                      ))}
                      {!routeEditor.mappings.length ? <tr><td colSpan={4}><span className="a-hint">暂无映射。绑定候选后添加路由分辨率到厂商原生分辨率的映射。</span></td></tr> : null}
                    </tbody>
                  </table>
                </div>
                <div className="a-row-actions">
                  <select className="a-select a-select--sm" value={routeEditor.defaults.resolution} onChange={(event) => setRouteEditor({ ...routeEditor, defaults: { ...routeEditor.defaults, resolution: event.target.value }, quote: undefined })}>
                    {routeCandidates.length ? <option value="">选择候选后添加映射</option> : <option value="">暂无候选</option>}
                  </select>
                  <Button variant="secondary" size="sm" disabled={!routeCandidates.length} onClick={() => setRouteEditor({ ...routeEditor, mappings: [...routeEditor.mappings, newMappingRow(routeCandidates, routeEditor.mappings)], quote: undefined })}>添加映射</Button>
                </div>
                {routeEditorMappingError(routeEditor, routeCandidates) ? <InlineFeedback tone="danger" message={routeEditorMappingError(routeEditor, routeCandidates) ?? ''} /> : null}
              </FormSection>
            </form>
          ) : null}

          {routeEditor.tab === 'quote' ? (
            <div className="cfg-tab-panel">
              <div className="a-banner a-banner--info">
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.6" aria-hidden="true"><circle cx="12" cy="12" r="9" /><path d="M12 11v5M12 7.5h.01" /></svg>
                <div><strong style={{ fontSize: 13 }}>视频销售价来自候选厂商原生费率</strong><div className="a-hint">系统按可用候选中的最高人民币成本，经全局汇率换算为积分，再应用最低任务积分与取整步长；这里不手工填写每秒售价。</div></div>
              </div>
              <div className="cfg-scroll-x">
                <table className="a-table" style={{ minWidth: 760 }}>
                  <thead><tr><th>真实模型</th><th>厂商</th><th>费率版本</th><th>原生费率摘要</th><th>状态</th></tr></thead>
                  <tbody>
                    {routeCandidates.map((candidate) => {
                      const model = accountModels.find((item) => String(item.id) === String(candidate.account_model_id))
                      const rate = latestRateCard(videoSnapshot?.rate_cards ?? [], candidate.account_model_id)
                      return (
                        <tr key={String(candidate.id)}>
                          <td><span className="a-cell-stack"><strong>{model?.display_name || model?.model_code || routeCandidateLabel(candidate)}</strong><span>{candidate.account_name || '-'} / {model?.model_code}</span></span></td>
                          <td>{adapterLabelForRate(rate)}</td>
                          <td className="a-mono">{rate ? `v${rate.rate_version}` : '未配置'}</td>
                          <td>{rate ? rateSummary(rate, routeEditor.mappings.filter((row) => row.candidateID === String(candidate.account_model_id)).map((row) => row.nativeResolution)) : '缺少原生费率'}</td>
                          <td><Badge tone={rate?.enabled ? 'success' : 'warning'}>{rate?.enabled ? '可报价' : '不可报价'}</Badge></td>
                        </tr>
                      )
                    })}
                    {!routeCandidates.length ? <tr><td colSpan={5}><span className="a-hint">绑定候选真实模型后才能报价。</span></td></tr> : null}
                  </tbody>
                </table>
              </div>
              <section className="a-grid a-grid--4">
                <div className="a-panel" style={{ boxShadow: 'none', padding: 13 }}><div className="a-hint">最高销售价</div><strong style={{ fontSize: 14 }}>{routeEditor.quote?.result ? `¥ ${routeEditor.quote.result.highest_cny}` : '运行试算后显示'}</strong></div>
                <div className="a-panel" style={{ boxShadow: 'none', padding: 13 }}><div className="a-hint">全局汇率</div><strong style={{ fontSize: 14 }}>{routeEditor.quote?.result ? `1 积分 = ¥${routeEditor.quote.result.cny_per_point}` : '运行试算后显示'}</strong></div>
                <div className="a-panel" style={{ boxShadow: 'none', padding: 13 }}><div className="a-hint">单结果积分</div><strong style={{ fontSize: 14 }}>{routeEditor.quote?.result?.unit_points ?? '—'}</strong></div>
                <div className="a-panel" style={{ boxShadow: 'none', padding: 13 }}><div className="a-hint">总积分</div><strong style={{ fontSize: 14 }}>{routeEditor.quote?.result?.total_points ?? '—'}</strong></div>
              </section>
              <div className="a-row-actions">
                <Button variant="secondary" size="sm" disabled={!routeEditor.row || !routeCandidates.length || routeEditor.quote?.busy} onClick={() => void simulateQuote(routeEditor, setRouteEditor)}>{routeEditor.quote?.busy ? '试算中...' : '运行报价试算'}</Button>
                {routeEditor.row && routeVideoConfig ? <Button variant="danger" size="sm" disabled={saving} onClick={() => void deleteVideoConfig()}>删除视频配置</Button> : null}
                <span className="a-hint">使用默认任务、时长、分辨率、比例与音频做一次完整试算，不创建任务、不扣积分。</span>
              </div>
              {routeEditor.quote?.error ? <InlineFeedback tone="danger" message={routeEditor.quote.error} /> : null}
              {routeEditor.quote?.failedCandidates?.length ? (
                <div className="cfg-scroll-x">
                  <table className="a-table" style={{ minWidth: 760 }}>
                    <thead><tr><th>真实模型</th><th>厂商</th><th>映射分辨率</th><th>状态</th><th>排除原因</th></tr></thead>
                    <tbody>
                      {routeEditor.quote.failedCandidates.map((candidate) => (
                        <tr key={candidate.route_candidate_id}>
                          <td>{candidate.model_code}<small className="block text-[var(--a-ink-3)]">#{candidate.account_model_id}</small></td>
                          <td>{candidate.provider_code}</td>
                          <td className="a-mono">{candidate.mapped_resolution}</td>
                          <td><Badge tone={candidate.eligible ? 'success' : 'warning'}>{candidate.eligible ? '可报价' : '已排除'}</Badge></td>
                          <td>{candidate.exclusion_code || '需要补齐候选映射或原生费率'}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              ) : null}
              {routeEditor.quote?.result ? (
                <div className="cfg-scroll-x">
                  <table className="a-table" style={{ minWidth: 760 }}>
                    <thead><tr><th>真实模型</th><th>厂商</th><th>映射分辨率</th><th>状态</th><th className="is-num">CNY 报价</th><th>说明</th></tr></thead>
                    <tbody>
                      {routeEditor.quote.result.candidates.map((candidate) => (
                        <tr key={candidate.route_candidate_id}>
                          <td>{candidate.model_code}<small className="block text-[var(--a-ink-3)]">#{candidate.account_model_id}</small></td>
                          <td>{candidate.provider_code}</td>
                          <td className="a-mono">{candidate.mapped_resolution}</td>
                          <td><Badge tone={candidate.eligible ? 'success' : 'warning'}>{candidate.eligible ? '可报价' : '已排除'}</Badge></td>
                          <td className="is-num a-num">{candidate.estimated_cny}</td>
                          <td>{candidate.exclusion_code || (candidate.account_model_id === routeEditor.quote?.result?.highest_account_model_id ? '最高价来源' : '参与最高价比较')}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              ) : null}
            </div>
          ) : null}
        </Drawer>
      ) : null}

      {candidateDialog ? (
        <Drawer
          title={candidateDialog.row ? '编辑候选真实模型' : '新增候选真实模型'}
          description={`${candidateDialog.route.name} · ${candidateDialog.row ? '修改调度参数' : '绑定真实模型'}`}
          onClose={() => { setCandidateDialog(null); setMutationError(null) }}
          footer={<><Button variant="ghost" size="sm" disabled={saving} onClick={() => setCandidateDialog(null)}>取消</Button><div style={{ flex: 1 }} /><Button variant="primary" size="sm" disabled={saving || !candidateDialog.accountModelId} onClick={() => void saveCandidate()}>{saving ? '保存中...' : '保存候选'}</Button></>}
        >
          {mutationError ? <InlineFeedback tone="danger" message={mutationError} /> : null}
          <form className="a-form" onSubmit={(event) => event.preventDefault()}>
            <FormSection title="候选模型" description={media === 'video' ? '只列出已验证且启用的视频真实模型。' : '只列出图片真实模型（文生图 / 图片编辑）。'}>
              <LedgerField label="真实模型" required><select className="a-select" value={candidateDialog.accountModelId} onChange={(event) => setCandidateDialog({ ...candidateDialog, accountModelId: event.target.value })}>{eligibleAccountModels.map((model) => <option key={String(model.id)} value={String(model.id)}>{model.account_name ? `${model.account_name} / ` : ''}{model.model_code}</option>)}</select></LedgerField>
              <div className="a-form__pair">
                <LedgerField label={routingFieldLabels.priority} hint={routingFieldHints.priority}><input className="a-input" type="number" min="1" value={candidateDialog.priority} onChange={(event) => setCandidateDialog({ ...candidateDialog, priority: event.target.value })} /></LedgerField>
                <LedgerField label={routingFieldLabels.weight} hint={routingFieldHints.weight}><input className="a-input" type="number" min="0" value={candidateDialog.weight} onChange={(event) => setCandidateDialog({ ...candidateDialog, weight: event.target.value })} /></LedgerField>
              </div>
              <LedgerField label={routingFieldLabels.fallbackOrder} hint={routingFieldHints.fallbackOrder}><input className="a-input" type="number" min="1" value={candidateDialog.fallbackOrder} onChange={(event) => setCandidateDialog({ ...candidateDialog, fallbackOrder: event.target.value })} /></LedgerField>
              <SwitchField label="启用该候选" checked={candidateDialog.enabled} onChange={(enabled) => setCandidateDialog({ ...candidateDialog, enabled })} />
            </FormSection>
          </form>
        </Drawer>
      ) : null}

      {deleteTarget ? (
        <Modal
          title={deleteTarget.kind === 'route' ? '删除路由模型' : '删除候选模型'}
          detail={deleteTarget.kind === 'route' ? deleteTarget.route.name : `${deleteTarget.route.name} / ${deleteTarget.candidate.model_code || deleteTarget.candidate.id}`}
          onClose={() => { if (!deleting) setDeleteTarget(null) }}
          footer={<><Button variant="ghost" disabled={deleting} onClick={() => setDeleteTarget(null)}>取消</Button><Button variant="danger" disabled={deleting} onClick={() => void confirmDelete()}>{deleting ? '删除中...' : '确认删除'}</Button></>}
        >
          {mutationError ? <InlineFeedback tone="danger" message={mutationError} /> : null}
          <p className="m-0 text-sm leading-6 text-[var(--a-ink-2)]">删除后配置不再参与新请求，历史调用继续使用任务中保存的路由与价格快照。</p>
        </Modal>
      ) : null}
    </section>
  )
}

function RouteBadge({ badge }: { badge: { label: string; tone: 'success' | 'warning' | 'danger' | 'neutral' | 'primary' } }) {
  return <Badge tone={badge.tone}>{badge.label}</Badge>
}

function RouteDetailWorkspace({
  route,
  groups,
  candidates: routeCandidates,
  prices: routePrices,
  accountModels,
  modelAccounts,
  videoConfig,
  videoImpacts,
  rateCards,
  onEditRoute,
  onAddCandidate,
  onEditCandidate,
  onDeleteRoute,
  onDeleteCandidate,
}: {
  route: RouteModel
  groups: UserGroup[]
  candidates: RouteModelCandidate[]
  prices: RouteModelPrice[]
  accountModels: ModelAccountModel[]
  modelAccounts: ModelAccount[]
  videoConfig?: AdminVideoRouteConfig
  videoImpacts: AdminVideoConfiguration['impacts']
  rateCards: AdminVideoRateCard[]
  onEditRoute: (tab?: RouteEditorTab) => void
  onAddCandidate: () => void
  onEditCandidate: (candidate: RouteModelCandidate) => void
  onDeleteRoute: () => void
  onDeleteCandidate: (candidate: RouteModelCandidate) => void
}) {
  const readiness = routeReadinessForMedia({
    route,
    candidates: effectiveRouteCandidates(routeCandidates, accountModels, modelAccounts),
    prices: routePrices,
    videoConfig,
    videoImpacts,
  })
  const enabledPrices = routePrices.filter((price) => price.enabled)
  return (
    <section style={{ display: 'grid', gap: 16 }}>
      <div className="a-panel a-panel--pad">
        <div className="a-section-title" style={{ marginBottom: 14 }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 10, flexWrap: 'wrap' }}>
            <h2 style={{ fontSize: 17 }}>{route.name}</h2>
            <span className="a-tag">{route.code}</span>
            <RouteBadge badge={readiness} />
          </div>
          <div className="a-row-actions">
            <button className="a-row-btn" type="button" onClick={() => onEditRoute('base')}>编辑{route.media_type === 'video' ? '视频' : '图片'}路由</button>
            <button className="a-btn a-btn--sm a-btn--primary" type="button" onClick={onAddCandidate}>配置候选</button>
          </div>
        </div>
        <div className="a-grid rt-metrics" style={{ gridTemplateColumns: 'repeat(4,minmax(0,1fr))', gap: 10 }}>
          <div className="a-panel" style={{ boxShadow: 'none', padding: 12 }}><div className="a-hint" style={{ fontSize: 11 }}>创作类型</div><strong style={{ fontSize: 13 }}>{route.media_type === 'video' ? '视频' : '图片'}</strong></div>
          <div className="a-panel" style={{ boxShadow: 'none', padding: 12 }}><div className="a-hint" style={{ fontSize: 11 }}>可见范围</div><strong style={{ fontSize: 13 }}>{routeVisibilityBadge(route.visibility).label} · {routeGroupNames(route.group_ids, groups)}</strong></div>
          <div className="a-panel" style={{ boxShadow: 'none', padding: 12 }}><div className="a-hint" style={{ fontSize: 11 }}>候选模型</div><strong style={{ fontSize: 13 }}>{routeCandidateSummary(routeCandidates)}</strong></div>
          <div className="a-panel" style={{ boxShadow: 'none', padding: 12 }}><div className="a-hint" style={{ fontSize: 11 }}>{route.media_type === 'video' ? '报价方式' : '价格配置'}</div><strong style={{ fontSize: 13 }}>{route.media_type === 'video' ? (videoConfig?.enabled ? `原生费率 · 最低 ${videoConfig.minimum_task_points} 积分` : '尚未启用') : `${enabledPrices.length} 个启用价格`}</strong></div>
        </div>
        {readiness.state !== 'ready' ? <div style={{ marginTop: 12 }}><InlineFeedback tone={readiness.tone === 'danger' ? 'danger' : 'warning'} message={routeReadinessMessage(route, readiness.state)} /></div> : null}
        <div className="a-row-actions" style={{ marginTop: 12 }}>
          {route.media_type === 'image' ? <button className="a-btn a-btn--sm" type="button" onClick={() => onEditRoute('pricing')}>配置图片价格</button> : <><button className="a-btn a-btn--sm" type="button" onClick={() => onEditRoute('params')}>配置视频参数</button><button className="a-btn a-btn--sm" type="button" onClick={() => onEditRoute('quote')}>查看视频报价</button></>}
          <button className="a-btn a-btn--sm" type="button" aria-label={`删除路由 ${route.code}`} onClick={onDeleteRoute}><Trash2 className="size-4" />删除路由</button>
        </div>
      </div>
      <div className="a-panel a-panel--flush">
        <div className="a-panel__head"><div><h2>候选模型</h2><p>{route.media_type === 'video' ? '候选只包含已验证且启用的视频真实模型；按优先级与权重调度。' : '候选只包含图片真实模型；按优先级与权重调度。'}</p></div><button className="a-row-btn" type="button" onClick={onAddCandidate}>添加候选</button></div>
        {routeCandidates.length ? (
          <div className="a-table-wrap">
            <table className="a-table rt-candidates-table" style={{ minWidth: 860 }}>
              <thead><tr><th>真实账号 / 模型</th><th>能力</th><th className="is-num">{routingFieldLabels.priority}</th><th className="is-num">{routingFieldLabels.weight}</th><th className="is-num">{routingFieldLabels.fallbackOrder}</th><th>状态</th><th className="is-actions rt-candidate-actions-cell">操作</th></tr></thead>
              <tbody>
                {routeCandidates.map((candidate) => {
                  const model = accountModels.find((item) => String(item.id) === String(candidate.account_model_id))
                  const account = modelAccounts.find((item) => String(item.id) === String(model?.account_id))
                  const effective = candidate.enabled && Boolean(model?.enabled) && Boolean(account && account.status === 'enabled')
                  return (
                    <tr key={String(candidate.id)}>
                      <td><span className="a-cell-stack"><strong>{candidate.account_name || '未命名账号'} / {model?.display_name || model?.model_code || candidate.model_code || '未知模型'}</strong><span>{model?.model_code || candidate.model_code || routeCandidateLabel(candidate)}</span></span></td>
                      <td>{candidateCapabilitySummary(route.media_type === 'video' ? model : model)}</td>
                      <td className="is-num a-num">{candidate.priority}</td>
                      <td className="is-num a-num">{candidate.weight}</td>
                      <td className="is-num a-num">{candidate.fallback_order}</td>
                      <td><Badge tone={effective ? 'success' : 'warning'}>{effective ? '可路由' : candidate.enabled ? '底层不可用' : '候选停用'}</Badge></td>
                      <td className="is-actions rt-candidate-actions-cell"><span className="rt-candidate-actions"><button className="a-row-btn" type="button" onClick={() => onEditCandidate(candidate)}>编辑</button><button className="rt-candidate-delete" type="button" title={`删除候选 ${candidate.model_code || candidate.id}`} aria-label={`删除候选 ${candidate.model_code || candidate.id}`} onClick={() => onDeleteCandidate(candidate)}><Trash2 aria-hidden="true" /></button></span></td>
                    </tr>
                  )
                })}
              </tbody>
            </table>
          </div>
        ) : (
          <div className="p-4"><EmptyBlock title="暂无候选真实模型" detail={route.media_type === 'video' ? '绑定已验证且启用的视频真实模型后才能参与路由与报价。' : '绑定图片真实模型后才能参与路由与估价。'} action={<Button variant="primary" size="sm" onClick={onAddCandidate}>添加候选</Button>} /></div>
        )}
        {route.media_type === 'video' ? (
          <div style={{ padding: '12px 16px', borderTop: '1px solid var(--a-line)' }}>
            <div className="a-section-title" style={{ marginBottom: 8 }}><div><strong style={{ fontSize: 13 }}>候选分辨率映射</strong></div><button className="a-row-btn" type="button" onClick={() => onEditRoute('params')}>配置映射</button></div>
            {videoConfig ? <span className="a-hint">已配置 {Object.values(videoConfig.candidate_parameter_mappings ?? {}).reduce((sum, item) => sum + Object.keys(item.resolutions ?? {}).length, 0)} 条映射 · 最大输出 {videoConfig.max_output_count} 个</span> : <span className="a-hint">尚未配置视频参数与候选分辨率映射。</span>}
          </div>
        ) : null}
      </div>
    </section>
  )
}

function buildNewRouteEditor(media: RouteModelMediaType, groups: UserGroup[]): RouteEditorDraft {
  void groups
  return {
    tab: 'base',
    code: '',
    name: '',
    description: '',
    visibility: 'public',
    enabled: true,
    sortOrder: '10',
    groupIds: [],
    iconKey: '',
    iconSvg: '',
    priceRows: [],
    mappings: [],
    maxOutputCount: '1',
    videoEnabled: false,
    minimumTaskPoints: '10.00000',
    roundingStepPoints: '1',
    defaults: { taskType: 'text_to_video', durationSeconds: '5', resolution: '720p', aspectRatio: '16:9', audioMode: 'silent' },
  }
}

function buildRouteEditor({
  route,
  media,
  tab,
  prices,
  candidates,
  accountModels,
  videoConfig,
  rateCards,
}: {
  route: RouteModel
  media: RouteModelMediaType
  tab: RouteEditorTab
  prices: RouteModelPrice[]
  candidates: RouteModelCandidate[]
  accountModels: ModelAccountModel[]
  videoConfig?: AdminVideoRouteConfig
  rateCards: AdminVideoRateCard[]
}): RouteEditorDraft {
  const priceRows = media === 'image' ? imagePriceRows(route, candidates, accountModels, prices) : []
  const mappings = media === 'video' ? videoMappingRows(candidates, accountModels, rateCards, videoConfig) : []
  const defaults = videoConfig?.defaults ?? {}
  const fallbackResolution = mappings[0]?.routeResolution ?? '720p'
  const editor: RouteEditorDraft = {
    row: route,
    tab,
    code: route.code,
    name: route.name,
    description: route.description ?? '',
    visibility: route.visibility,
    enabled: route.enabled,
    sortOrder: String(route.sort_order),
    groupIds: (route.group_ids ?? []).map(String),
    iconKey: route.icon_key ?? '',
    iconSvg: route.icon_svg ?? '',
    priceRows,
    mappings,
    maxOutputCount: String(videoConfig?.max_output_count ?? 1),
    videoEnabled: Boolean(videoConfig?.enabled),
    minimumTaskPoints: videoConfig?.minimum_task_points ?? '10.00000',
    roundingStepPoints: String(videoConfig?.rounding_step_points ?? 1),
    defaults: {
      taskType: String(defaults.task_type ?? 'text_to_video'),
      durationSeconds: String(defaults.duration_seconds ?? 5),
      resolution: String(defaults.resolution ?? fallbackResolution),
      aspectRatio: String(defaults.aspect_ratio ?? '16:9'),
      audioMode: String(defaults.audio_mode ?? (defaults.generate_audio === true ? 'generated' : 'silent')),
    },
  }
  editor.baseline = JSON.stringify(routeEditorSignature(editor))
  return editor
}

function routeEditorSignature(editor: RouteEditorDraft) {
  return {
    code: editor.code,
    name: editor.name,
    description: editor.description,
    visibility: editor.visibility,
    enabled: editor.enabled,
    sortOrder: editor.sortOrder,
    groupIds: editor.groupIds,
    iconKey: editor.iconKey,
    iconSvg: editor.iconSvg,
    priceRows: editor.priceRows.map((row) => ({ taskType: row.taskType, baseResolution: row.baseResolution, basePoints: row.basePoints, referenceMultiplier: row.referenceMultiplier, enabled: row.enabled })),
    mappings: editor.mappings.map((row) => ({ candidateID: row.candidateID, routeResolution: row.routeResolution, nativeResolution: row.nativeResolution })),
    maxOutputCount: editor.maxOutputCount,
    videoEnabled: editor.videoEnabled,
    minimumTaskPoints: editor.minimumTaskPoints,
    roundingStepPoints: editor.roundingStepPoints,
    defaults: editor.defaults,
  }
}

function imagePriceRows(route: RouteModel, candidates: RouteModelCandidate[], accountModels: ModelAccountModel[], prices: RouteModelPrice[]): PriceDraftRow[] {
  const models = candidates.map((candidate) => accountModels.find((model) => String(model.id) === String(candidate.account_model_id))).filter(Boolean) as ModelAccountModel[]
  const tasks = Array.from(new Set(models.flatMap((model) => model.task_types.filter((task): task is ImageTaskType => task === 'text_to_image' || task === 'image_edit'))))
  const resolutions = Array.from(new Set(models.flatMap((model) => normalizeResolutionList(model.base_resolution)))).sort(compareBaseResolution)
  const combos = tasks.flatMap((taskType) => resolutions.map((baseResolution) => ({ taskType, baseResolution })))
  const rows: PriceDraftRow[] = combos.map((combo) => {
    const existing = prices.find((price) => price.task_type === combo.taskType && price.base_resolution === combo.baseResolution)
    return {
      key: `${combo.taskType}:${combo.baseResolution}`,
      id: existing?.id,
      taskType: combo.taskType,
      baseResolution: combo.baseResolution,
      basePoints: existing?.base_points ?? '',
      referenceMultiplier: existing?.reference_multiplier ?? '1.00000',
      enabled: existing?.enabled ?? true,
    }
  })
  for (const price of prices) {
    if (rows.some((row) => row.key === `${price.task_type}:${price.base_resolution}`)) continue
    rows.push({ key: `${price.task_type}:${price.base_resolution}`, id: price.id, taskType: price.task_type, baseResolution: price.base_resolution, basePoints: price.base_points, referenceMultiplier: price.reference_multiplier, enabled: price.enabled })
  }
  void route
  return rows
}

function videoMappingRows(candidates: RouteModelCandidate[], accountModels: ModelAccountModel[], rateCards: AdminVideoRateCard[], config?: AdminVideoRouteConfig): MappingDraftRow[] {
  const existing = config?.candidate_parameter_mappings ?? {}
  const rows: MappingDraftRow[] = []
  for (const [candidateID, mapping] of Object.entries(existing)) {
    for (const [routeResolution, nativeResolution] of Object.entries(mapping.resolutions ?? {})) {
      rows.push({ key: `${candidateID}:${routeResolution}`, candidateID, routeResolution, nativeResolution: String(nativeResolution) })
    }
  }
  if (rows.length) return rows
  for (const candidate of candidates) {
    const candidateID = String(candidate.account_model_id)
    const rate = latestRateCard(rateCards, candidate.account_model_id)
    const nativeResolutions = rateResolutions(rate)
    const routeResolution = '720p'
    const nativeResolution = nativeResolutions.includes('720p') ? '720p' : (nativeResolutions[0] ?? '720p')
    rows.push({ key: `${candidateID}:${routeResolution}`, candidateID, routeResolution, nativeResolution })
  }
  return rows
}

function routeEditorPriceError(editor: RouteEditorDraft, media: RouteModelMediaType, accountModels: ModelAccountModel[]): string | null {
  if (media !== 'image' || !editor.row) return null
  const errors = routeEditorErrors(editor, media, accountModels, [])
  return errors.prices ?? null
}

function routeEditorMappingError(editor: RouteEditorDraft, candidates: RouteModelCandidate[]): string | null {
  if (!editor.row || !candidates.length) return null
  return routeEditorErrors(editor, 'video', [], candidates).mappings ?? routeEditorErrors(editor, 'video', [], candidates).defaults ?? null
}

function routeEditorErrors(editor: RouteEditorDraft, media: RouteModelMediaType, accountModels: ModelAccountModel[], candidates: RouteModelCandidate[]): Record<string, string> {
  const errors: Record<string, string> = {}
  const codeError = routeCodeError(editor.code)
  if (codeError) errors.code = codeError
  if (!editor.name.trim()) errors.name = '请填写路由名称。'
  if (editor.visibility === 'groups' && !editor.groupIds.length) errors.groups = '按分组可见时至少选择一个分组。'
  if (!Number.isFinite(Number(editor.sortOrder))) errors.sortOrder = '排序值必须是数字。'
  if (media === 'image') {
    const missing = editor.priceRows.filter((row) => !row.basePoints.trim())
    if (missing.length) errors.prices = `以下价格组合缺少基础积分：${missing.map((row) => `${imageTaskLabel(row.taskType)} ${row.baseResolution}`).join('、')}。`
    if (editor.row) {
      const routeCandidateModels = candidates.length
        ? candidates.map((candidate) => accountModels.find((model) => String(model.id) === String(candidate.account_model_id))).filter(Boolean) as ModelAccountModel[]
        : accountModels.filter(() => false)
      const expectedTasks = Array.from(new Set(routeCandidateModels.flatMap((model) => model.task_types.filter((task): task is ImageTaskType => task === 'text_to_image' || task === 'image_edit'))))
      const expectedResolutions = Array.from(new Set(routeCandidateModels.flatMap((model) => normalizeResolutionList(model.base_resolution))))
      for (const taskType of expectedTasks) {
        for (const baseResolution of expectedResolutions) {
          if (!editor.priceRows.some((row) => row.taskType === taskType && row.baseResolution === baseResolution && row.basePoints.trim())) {
            errors.prices = `缺少 ${imageTaskLabel(taskType)} · ${baseResolution} 的价格，提交前必须补齐。`
            return errors
          }
        }
      }
    }
  } else {
    if (editor.row && candidates.length) {
      const universe = Array.from(new Set(editor.mappings.map((row) => row.routeResolution.trim()).filter(Boolean)))
      if (!editor.mappings.length) errors.mappings = '请至少配置一条候选分辨率映射。'
      for (const candidate of candidates) {
        const candidateID = String(candidate.account_model_id)
        const candidateRows = editor.mappings.filter((row) => row.candidateID === candidateID && row.routeResolution.trim() && row.nativeResolution.trim())
        if (!candidateRows.length) {
          errors.mappings = `${candidate.account_name || candidate.model_code || candidateID} 缺少分辨率映射。`
          break
        }
        const missing = universe.filter((resolution) => !candidateRows.some((row) => row.routeResolution === resolution))
        if (missing.length) {
          errors.mappings = `${candidate.account_name || candidate.model_code || candidateID} 缺少 ${missing.join('、')} 的映射，未映射的组合不参与报价。`
          break
        }
      }
      if (!universe.includes(editor.defaults.resolution)) errors.defaults = '默认分辨率必须是已映射的路由分辨率。'
    }
    if (!(Number(editor.minimumTaskPoints) >= 0)) errors.minimumTaskPoints = '最低任务积分不能为负数。'
    if (!(Number(editor.roundingStepPoints) > 0)) errors.roundingStepPoints = '积分取整步长必须大于 0。'
    if (Number(editor.maxOutputCount) < 1 || Number(editor.maxOutputCount) > 4) errors.maxOutputCount = '平台最大输出数必须在 1–4 之间。'
  }
  return errors
}


function routeEditorDirtyCount(editor: RouteEditorDraft): number {
  if (!editor.row) return editor.code || editor.name ? 1 : 0
  if (!editor.baseline) return 1
  return JSON.stringify(routeEditorSignature(editor)) === editor.baseline ? 0 : 1
}

function updatePriceRow(rows: PriceDraftRow[], index: number, patch: Partial<PriceDraftRow>) {
  return rows.map((row, rowIndex) => rowIndex === index ? { ...row, ...patch } : row)
}

function updateMappingRow(rows: MappingDraftRow[], index: number, patch: Partial<MappingDraftRow>) {
  return rows.map((row, rowIndex) => rowIndex === index ? { ...row, ...patch } : row)
}

function newMappingRow(candidates: RouteModelCandidate[], mappings: MappingDraftRow[]): MappingDraftRow {
  const candidate = candidates[0]
  const candidateID = candidate ? String(candidate.account_model_id) : ''
  const resolution = mappings[0]?.routeResolution ?? '720p'
  return { key: `${candidateID}:${resolution}:${mappings.length}`, candidateID, routeResolution: resolution, nativeResolution: resolution }
}

function buildMappings(rows: MappingDraftRow[]) {
  const result: Record<string, { resolutions?: Record<string, string> }> = {}
  for (const row of rows) {
    if (!row.candidateID || !row.routeResolution.trim() || !row.nativeResolution.trim()) continue
    result[row.candidateID] ??= { resolutions: {} }
    result[row.candidateID].resolutions![row.routeResolution.trim()] = row.nativeResolution.trim()
  }
  return result
}

function candidateLabel(id: string, candidates: RouteModelCandidate[]) {
  const candidate = candidates.find((item) => String(item.account_model_id) === id)
  return candidate ? [candidate.account_name, candidate.model_code].filter(Boolean).join(' / ') || '未命名真实模型' : '已移除真实模型'
}

function latestRateCard(rateCards: AdminVideoRateCard[], accountModelID: string | number) {
  return rateCards
    .filter((card) => String(card.account_model_id) === String(accountModelID))
    .sort((a, b) => b.rate_version - a.rate_version)[0]
}

function rateResolutions(rate?: AdminVideoRateCard): string[] {
  if (!rate || !('resolutions' in rate.rate_config)) return []
  return Object.keys(rate.rate_config.resolutions ?? {})
}

function rateSummary(rate: AdminVideoRateCard, mappedResolutions: string[]): string {
  if (rate.pricing_schema === 'gasic_per_task_v1') return `¥${rate.rate_config.per_task_cny} / 次`
  if (rate.pricing_schema === 'seedance_token_v1') {
    const resolution = mappedResolutions[0] ?? Object.keys(rate.rate_config.resolutions)[0]
    const value = resolution ? rate.rate_config.resolutions[resolution] : undefined
    return value ? `${resolution} ¥${value.without_input_video_million_tokens_cny} / 百万 Token` : '缺少分辨率费率'
  }
  const resolution = mappedResolutions[0] ?? Object.keys(rate.rate_config.resolutions)[0]
  const value = resolution ? rate.rate_config.resolutions[resolution] : undefined
  return value ? `${resolution} ¥${value.output_second_cny} / 秒 · 输入 ¥${value.input_video_second_cny} / 秒` : '缺少分辨率费率'
}

function adapterLabelForRate(rate?: AdminVideoRateCard) {
  if (!rate) return '未配置'
  if (rate.provider_code === 'seedance') return 'Seedance'
  if (rate.provider_code === 'minimax') return 'MiniMax'
  if (rate.provider_code === 'gasic') return 'Gasic'
  if (rate.provider_code === 'bailian') return '阿里百炼'
  return '未知厂商'
}

function videoTaskOptionsFromCandidates(candidates: RouteModelCandidate[], accountModels: ModelAccountModel[]) {
  const labels: Record<string, string> = {
    text_to_video: '文生视频',
    image_to_video: '图生视频',
    first_last_frame_to_video: '首尾帧生视频',
    reference_to_video: '参考生视频',
    video_edit: '视频编辑',
    video_extend: '视频延长',
  }
  const values = new Set(candidates.flatMap((candidate) => accountModels.find((model) => String(model.id) === String(candidate.account_model_id))?.task_types ?? []))
  if (!values.size) ['text_to_video', 'image_to_video', 'first_last_frame_to_video'].forEach((value) => values.add(value))
  return Array.from(values).map((value) => ({ value, label: labels[value] ?? value }))
}

async function simulateQuote(editor: RouteEditorDraft, update: (next: RouteEditorDraft | null | ((current: RouteEditorDraft | null) => RouteEditorDraft | null)) => void) {
  if (!editor.row) return
  update((current) => current ? { ...current, quote: { ...current.quote, busy: true, error: undefined, failedCandidates: undefined, result: undefined } } : current)
  try {
    const result = await adminApi.simulateVideoRouteQuote(editor.row.id, {
      task_type: editor.defaults.taskType,
      resolution: editor.defaults.resolution,
      aspect_ratio: editor.defaults.aspectRatio,
      audio_mode: editor.defaults.audioMode,
      duration_seconds: Number(editor.defaults.durationSeconds) || 5,
      output_count: 1,
      reference_image_count: 0,
      input_video_seconds: '',
      has_input_audio: false,
    })
    update((current) => current ? { ...current, quote: { result, busy: false } } : current)
  } catch (caught) {
    const failedCandidates = caught instanceof ApiError && Array.isArray(caught.details?.candidates)
      ? (caught.details?.candidates as AdminVideoQuoteCandidate[])
      : []
    update((current) => current ? { ...current, quote: { error: caught instanceof Error ? caught.message : '报价试算失败', failedCandidates, busy: false } } : current)
  }
}

function newCandidateDialog(route: RouteModel, accountModels: ModelAccountModel[]): CandidateDialog {
  return { route, accountModelId: String(accountModels[0]?.id ?? ''), priority: '1', weight: '100', fallbackOrder: '1', enabled: true }
}

function editCandidateDialog(route: RouteModel, row: RouteModelCandidate): CandidateDialog {
  return { route, row, accountModelId: String(row.account_model_id), priority: String(row.priority), weight: String(row.weight), fallbackOrder: String(row.fallback_order), enabled: row.enabled }
}

function routeReadinessForMedia({
  route,
  candidates,
  prices,
  videoConfig,
  videoImpacts,
}: {
  route: RouteModel
  candidates: Pick<RouteModelCandidate, 'enabled'>[]
  prices: Pick<RouteModelPrice, 'enabled'>[]
  videoConfig?: AdminVideoConfiguration['routes'][number]
  videoImpacts: AdminVideoConfiguration['impacts']
}) {
  const base = routeReadinessBadge({
    enabled: route.enabled,
    visibility: route.visibility,
    groupCount: route.group_ids?.length ?? 0,
    candidates,
    prices: route.media_type === 'image' ? prices : undefined,
  })
  if (base.state !== 'ready' || route.media_type !== 'video') return base
  if (!videoConfig?.enabled) return { state: 'missing_price' as const, label: '不可用 · 未配置视频参数', tone: 'warning' as const }
  const impact = videoImpacts.find((item) => item.blocking && String(item.route_model_id ?? '') === String(route.id))
  if (!impact) return base
  if (impact.code === 'missing_candidate') return { state: 'missing_candidate' as const, label: '不可用 · 缺候选', tone: 'danger' as const }
  return { state: 'missing_price' as const, label: '不可用 · 视频配置阻断', tone: 'warning' as const }
}

function candidateCapabilitySummary(model?: ModelAccountModel) {
  if (!model) return '能力配置不可用'
  const tasks = model.task_types ?? []
  const resolutions = model.base_resolution ?? []
  const formats = model.output_format ?? []
  if (model.extra?.media_type === 'video') return `${tasks.length ? `${tasks.length} 类视频任务` : '缺任务类型'} · 分辨率来自视频能力`
  const compression = model.supports_output_compression ? '支持压缩' : '不支持压缩'
  return `${tasks.length ? `${tasks.length} 类任务` : '缺任务类型'} · ${resolutions.length ? resolutions.join('/') : '缺分辨率'} · ${formats.length ? formats.join('/') : '缺输出格式'} · ${compression}`
}

function routeReadinessMessage(route: RouteModel, state: ReturnType<typeof routeReadinessBadge>['state']) {
  if (state === 'missing_candidate') return '当前路由缺少启用候选，配置候选后才能接收用户请求。'
  if (state === 'missing_price') return route.media_type === 'video' ? '当前视频路由缺少启用的视频参数或原生费率，提交前需要在编辑抽屉中补齐映射与报价。' : '当前路由缺少启用价格，生成请求会被计费校验阻断。'
  if (route.visibility === 'hidden') return '当前路由已隐藏，不会向用户展示或接收请求。'
  if (route.visibility === 'groups' && !route.group_ids?.length) return '当前路由尚未绑定可见分组，不会对任何用户生效。'
  return '当前路由已停用，不会向用户展示或接收请求。'
}

function effectiveRouteCandidates(candidates: RouteModelCandidate[], models: ModelAccountModel[], accounts: ModelAccount[]) {
  return candidates.map((candidate) => {
    const model = models.find((item) => String(item.id) === String(candidate.account_model_id))
    const account = accounts.find((item) => String(item.id) === String(model?.account_id))
    const accountEnabled = account ? account.status === 'enabled' : false
    return { enabled: candidate.enabled && Boolean(model?.enabled) && accountEnabled }
  })
}

function normalizeResolutionList(values: string[]) {
  return values.map((value) => value.trim()).filter((value) => Boolean(value) && value.toLowerCase() !== 'auto')
}

function compareBaseResolution(left: string, right: string) {
  const leftIndex = baseResolutionOrder.indexOf(left.toUpperCase())
  const rightIndex = baseResolutionOrder.indexOf(right.toUpperCase())
  if (leftIndex === -1 && rightIndex === -1) return left.localeCompare(right)
  if (leftIndex === -1) return 1
  if (rightIndex === -1) return -1
  return leftIndex - rightIndex
}

function nextVersion(prefix: string, current?: string) {
  const match = current?.match(/(\d+)$/)
  return `${prefix}-v${Number(match?.[1] ?? 0) + 1}`
}