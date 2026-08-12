import { useEffect, useMemo, useState } from 'react'
import type { AdminVideoConfiguration, AdminVideoSimulationResult } from '../../../shared/api-types'
import { adminApi } from '../../../shared/admin-api'
import { cn } from '../../../shared/classnames'
import { Field, InlineFeedback } from '../components'
import { adminButton } from '../ui/classes'

type Context = 'models' | 'pricing' | 'routing'
type Draft = {
  modelID: string; capabilityVersion: string; providerNativeMaxN: string; capabilityJSON: string; costJSON: string
  strategyID: string; routeID: string; code: string; name: string; minimumIncome: string; paymentFee: string; targetMargin: string; costBuffer: string
  taskType: string; resolution: string; ratio: string; audioMode: string; duration: string; salesPoints: string
  routeVersion: string; maxOutput: string; combinationsJSON: string; enabled: boolean
}

const initialDraft: Draft = {
  modelID: '', capabilityVersion: 'video-cap-v1', providerNativeMaxN: '1', capabilityJSON: '{\n  "schema_version": 1,\n  "provider_native_max_n": 1,\n  "task_types": {}\n}',
  costJSON: '{\n  "combinations": []\n}', strategyID: '', routeID: '', code: 'video-default', name: '视频默认策略', minimumIncome: '0.25260', paymentFee: '0.03000', targetMargin: '0.25000', costBuffer: '0.10000',
  taskType: 'text_to_video', resolution: '720p', ratio: '16:9', audioMode: 'silent', duration: '5', salesPoints: '0', routeVersion: 'video-route-v1', maxOutput: '4',
  combinationsJSON: '[\n  {"task_type":"text_to_video","resolution":"720p","aspect_ratio":"16:9","audio_mode":"silent","duration_seconds":5}\n]', enabled: false,
}

export function VideoConfigurationWorkspace({ context }: { context: Context }) {
  const [snapshot, setSnapshot] = useState<AdminVideoConfiguration | null>(null)
  const [draft, setDraft] = useState<Draft>(initialDraft)
  const [busy, setBusy] = useState(false)
  const [message, setMessage] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [simulation, setSimulation] = useState<AdminVideoSimulationResult | null>(null)
  const blockingCopy = useMemo(() => ['缺少候选', '缺少价格', '低于安全线'].join(' / '), [])

  async function load() {
    try {
      const next = await adminApi.getVideoConfiguration()
      setSnapshot(next)
      setDraft((current) => ({
        ...current,
        modelID: current.modelID || String(next.capabilities[0]?.account_model_id ?? ''),
        strategyID: current.strategyID || String(next.pricing_strategies[0]?.id ?? ''),
        routeID: current.routeID || String(next.routes[0]?.route_model_id ?? ''),
      }))
    } catch (caught) { setError(caught instanceof Error ? caught.message : '视频配置载入失败') }
  }
  useEffect(() => { void load() }, [])
  function update<K extends keyof Draft>(key: K, value: Draft[K]) { setDraft((current) => ({ ...current, [key]: value })) }
  async function run(action: () => Promise<unknown>, success: string) {
    setBusy(true); setError(null); setMessage(null)
    try { await action(); setMessage(success); await load() } catch (caught) { setError(caught instanceof Error ? caught.message : success + '失败') } finally { setBusy(false) }
  }
  async function saveCapability() {
    await run(async () => {
      const capability = JSON.parse(draft.capabilityJSON) as Record<string, unknown>
      capability.provider_native_max_n = Number(draft.providerNativeMaxN)
      const current = snapshot?.capabilities.find((item) => String(item.account_model_id) === draft.modelID)
      await adminApi.saveVideoCapability(draft.modelID, { expected_version: current?.capability_version ?? '', capability_version: draft.capabilityVersion, capability, validation_status: 'verified', enabled: draft.enabled })
    }, '视频能力版本已保存')
  }
  async function saveCost() {
    const current = snapshot?.cost_rules.filter((item) => String(item.account_model_id) === draft.modelID).sort((left, right) => right.rule_version - left.rule_version)[0]
    await run(() => adminApi.saveVideoCostRule(draft.modelID, { id: current?.id, expected_rule_version: current?.rule_version ?? 0, billing_mode: 'combination', currency: 'CNY', rates: JSON.parse(draft.costJSON), validation_status: 'verified', effective_at: new Date().toISOString(), enabled: draft.enabled }), '成本版本已保存')
  }
  function strategyPayload(expectedVersion: number) {
    return { expected_version: expectedVersion, code: draft.code, name: draft.name, minimum_net_point_income_cny: draft.minimumIncome, payment_fee_rate: draft.paymentFee, target_margin_rate: draft.targetMargin, provider_cost_buffer_rate: draft.costBuffer, gross_point_value_cny: draft.minimumIncome, max_bonus_ratio: '0.5', platform_fixed_cost_cny: '0', platform_output_second_cost_cny: '0', platform_reference_cost_cny: '0', platform_audio_fixed_cost_cny: '0', platform_audio_second_cost_cny: '0', exact_reserve_markup: '1', metered_reserve_markup: '1.1', enabled: draft.enabled }
  }
  async function saveStrategy() {
    const current = snapshot?.pricing_strategies.find((item) => String(item.id) === draft.strategyID)
    await run(() => current ? adminApi.updateVideoPricingStrategy(current.id, strategyPayload(current.strategy_version)) : adminApi.createVideoPricingStrategy(strategyPayload(0)), '视频价格策略版本已保存')
  }
  const simulationRequest = () => ({ route_model_id: Number(draft.routeID), task_type: draft.taskType, resolution: draft.resolution, audio_mode: draft.audioMode, duration_seconds: Number(draft.duration), reference_image_count: draft.taskType === 'text_to_video' ? 0 : 1 })
  async function simulate() {
    setBusy(true); setError(null)
    try { setSimulation(await adminApi.simulateVideoPricing(draft.strategyID, simulationRequest())) } catch (caught) { setError(caught instanceof Error ? caught.message : '试算失败') } finally { setBusy(false) }
  }
  async function savePrice() {
    const current = snapshot?.price_rules.filter((item) => String(item.pricing_strategy_id) === draft.strategyID && item.task_type === draft.taskType && item.resolution === draft.resolution && item.audio_mode === draft.audioMode).sort((left, right) => right.rule_version - left.rule_version)[0]
    const input = { route_model_id: Number(draft.routeID), pricing_strategy_id: Number(draft.strategyID), expected_version: current?.rule_version ?? 0, task_type: draft.taskType, resolution: draft.resolution, audio_mode: draft.audioMode, duration_seconds: Number(draft.duration), effective_at: new Date().toISOString(), minimum_task_points: draft.salesPoints, enabled: draft.enabled }
    await run(() => current ? adminApi.updateVideoPriceRule(current.id, input) : adminApi.createVideoPriceRule(input), '销售价格版本已保存')
  }
  async function recalculate() {
    await run(() => adminApi.recalculateVideoPricing(draft.strategyID, { route_model_id: Number(draft.routeID), combinations: [simulationRequest()], effective_at: new Date().toISOString() }), '重新计算价格版本完成')
  }
  async function saveRoute() {
    const current = snapshot?.routes.find((item) => String(item.route_model_id) === draft.routeID)
    await run(() => adminApi.saveRouteVideoConfig(draft.routeID, { expected_version: current?.config_version ?? '', config_version: draft.routeVersion, pricing_strategy_id: Number(draft.strategyID), task_types: [draft.taskType], visible_options: {}, defaults: { task_type: draft.taskType, resolution: draft.resolution, audio_mode: draft.audioMode, duration_seconds: Number(draft.duration) }, visible_combinations: JSON.parse(draft.combinationsJSON), max_output_count: Number(draft.maxOutput), enabled: draft.enabled }), '视频路由配置已保存')
  }

  return <section className="grid gap-4 border-b border-[var(--border)] pb-5" data-video-configuration-workspace={context}>
    <div className="flex flex-wrap items-start justify-between gap-3"><div><strong className="text-sm text-[var(--text)]">视频配置工作台</strong><p className="m-0 mt-1 text-xs text-[var(--soft)]">启用前服务端会阻断：{blockingCopy}，并核验积分商品净收入保护与目标毛利。</p></div><label className="flex items-center gap-2 text-sm"><input type="checkbox" checked={draft.enabled} onChange={(event) => update('enabled', event.target.checked)} />启用当前版本</label></div>
    {error ? <InlineFeedback tone="danger" message={error} /> : null}{message ? <InlineFeedback tone="success" message={message} /> : null}
    {context === 'models' ? <div className="grid gap-3 lg:grid-cols-2">
      <div className="grid gap-3"><Field label="真实模型 ID"><input value={draft.modelID} onChange={(event) => update('modelID', event.target.value)} /></Field><Field label="能力版本"><input value={draft.capabilityVersion} onChange={(event) => update('capabilityVersion', event.target.value)} /></Field><Field label="Provider 原生最大 n（1-10）"><input type="number" min="1" max="10" value={draft.providerNativeMaxN} onChange={(event) => update('providerNativeMaxN', event.target.value)} /></Field><Field label="能力 JSON"><textarea rows={10} value={draft.capabilityJSON} onChange={(event) => update('capabilityJSON', event.target.value)} /></Field><div className="flex flex-wrap gap-2"><button className={cn(adminButton.base, adminButton.primary)} disabled={busy || !draft.modelID} onClick={() => void saveCapability()}>保存能力版本</button><button className={cn(adminButton.base, adminButton.ghost)} disabled={busy || !draft.modelID} onClick={() => void run(() => adminApi.deleteVideoCapability(draft.modelID), '能力已软删除')}>删除能力</button></div></div>
      <div className="grid gap-3 content-start"><Field label="成本组合 JSON"><textarea rows={16} value={draft.costJSON} onChange={(event) => update('costJSON', event.target.value)} /></Field><div className="flex flex-wrap gap-2"><button className={cn(adminButton.base, adminButton.secondary)} disabled={busy || !draft.modelID} onClick={() => void saveCost()}>保存成本版本</button>{snapshot?.cost_rules.filter((item) => String(item.account_model_id) === draft.modelID).slice(-1).map((item) => <button key={item.id} className={cn(adminButton.base, adminButton.ghost)} disabled={busy} onClick={() => void run(() => adminApi.deleteVideoCostRule(draft.modelID, item.id, item.rule_version), '成本规则已软删除')}>删除成本 v{item.rule_version}</button>)}</div></div>
    </div> : null}
    {context === 'pricing' ? <div className="grid gap-3 lg:grid-cols-3">
      <Field label="策略 ID"><input value={draft.strategyID} onChange={(event) => update('strategyID', event.target.value)} /></Field><Field label="策略编码"><input value={draft.code} onChange={(event) => update('code', event.target.value)} /></Field><Field label="策略名称"><input value={draft.name} onChange={(event) => update('name', event.target.value)} /></Field>
      <Field label="积分商品净收入保护"><input value={draft.minimumIncome} onChange={(event) => update('minimumIncome', event.target.value)} /></Field><Field label="支付费率"><input value={draft.paymentFee} onChange={(event) => update('paymentFee', event.target.value)} /></Field><Field label="目标毛利"><input value={draft.targetMargin} onChange={(event) => update('targetMargin', event.target.value)} /></Field><Field label="成本缓冲"><input value={draft.costBuffer} onChange={(event) => update('costBuffer', event.target.value)} /></Field><Field label="路由模型 ID"><input value={draft.routeID} onChange={(event) => update('routeID', event.target.value)} /></Field><Field label="销售积分"><input value={draft.salesPoints} onChange={(event) => update('salesPoints', event.target.value)} /></Field>
      <Field label="任务类型"><select value={draft.taskType} onChange={(event) => update('taskType', event.target.value)}><option value="text_to_video">文生视频</option><option value="image_to_video">图生视频</option><option value="first_last_frame_to_video">首尾帧</option></select></Field><Field label="分辨率"><input value={draft.resolution} onChange={(event) => update('resolution', event.target.value)} /></Field><Field label="时长（秒）"><input type="number" min="1" value={draft.duration} onChange={(event) => update('duration', event.target.value)} /></Field>
      <div className="flex flex-wrap gap-2 lg:col-span-3"><button className={cn(adminButton.base, adminButton.primary)} disabled={busy} onClick={() => void saveStrategy()}>保存价格策略</button><button className={cn(adminButton.base, adminButton.secondary)} disabled={busy || !draft.strategyID || !draft.routeID} onClick={() => void simulate()}>试算安全线</button><button className={cn(adminButton.base, adminButton.secondary)} disabled={busy || !draft.strategyID} onClick={() => void savePrice()}>保存销售价格</button><button className={cn(adminButton.base, adminButton.ghost)} disabled={busy || !draft.strategyID || !draft.routeID} onClick={() => void recalculate()}>重新计算价格版本</button>{snapshot?.pricing_strategies.filter((item) => String(item.id) === draft.strategyID).map((item) => <button key={item.id} className={cn(adminButton.base, adminButton.ghost)} disabled={busy} onClick={() => void run(() => adminApi.deleteVideoPricingStrategy(item.id, item.strategy_version), '价格策略已软删除')}>删除策略</button>)}{snapshot?.price_rules.filter((item) => String(item.pricing_strategy_id) === draft.strategyID).slice(-1).map((item) => <button key={item.id} className={cn(adminButton.base, adminButton.ghost)} disabled={busy} onClick={() => void run(() => adminApi.deleteVideoPriceRule(item.id, item.rule_version), '销售价格已软删除')}>删除价格</button>)}</div>
      {simulation ? <InlineFeedback tone="warning" message={`最坏候选成本 ¥${simulation.worst_candidate_cost_cny}；服务端安全线 ${simulation.safety_points} 积分；候选模型 ${simulation.candidate_account_model_id}`} /> : null}
    </div> : null}
    {context === 'routing' ? <div className="grid gap-3 lg:grid-cols-2"><Field label="路由模型 ID"><input value={draft.routeID} onChange={(event) => update('routeID', event.target.value)} /></Field><Field label="绑定价格策略 ID"><input value={draft.strategyID} onChange={(event) => update('strategyID', event.target.value)} /></Field><Field label="配置版本"><input value={draft.routeVersion} onChange={(event) => update('routeVersion', event.target.value)} /></Field><Field label="最大输出数（平台 1-4）"><input type="number" min="1" max="4" value={draft.maxOutput} onChange={(event) => update('maxOutput', event.target.value)} /></Field><div className="lg:col-span-2"><Field label="可见完整组合"><textarea rows={9} value={draft.combinationsJSON} onChange={(event) => update('combinationsJSON', event.target.value)} /></Field></div><div className="flex flex-wrap gap-2"><button className={cn(adminButton.base, adminButton.primary)} disabled={busy || !draft.routeID || !draft.strategyID} onClick={() => void saveRoute()}>保存视频路由配置</button><button className={cn(adminButton.base, adminButton.ghost)} disabled={busy || !draft.routeID} onClick={() => void run(() => adminApi.deleteRouteVideoConfig(draft.routeID), '视频路由配置已软删除')}>删除视频配置</button></div></div> : null}
  </section>
}
