import { useEffect, useMemo, useState } from "react";
import type { AdminVideoConfiguration, AdminVideoQuoteCandidate, AdminVideoQuoteSimulationResult, AdminVideoVisibleCombination, RouteModel } from "../../../shared/api-types";
import { adminApi } from "../../../shared/admin-api";
import { cn } from "../../../shared/classnames";
import { ApiError } from "../../../shared/http-client";
import { Badge, EmptyBlock, Field, InlineFeedback, LoadingBlock, PageHeader, RefreshIconButton } from "../components";
import { adminButton, adminPage } from "../ui/classes";

type SimulationDraft = { taskType: string; resolution: string; ratio: string; audioMode: string; duration: number; outputCount: number; imageCount: number; inputVideoSeconds: string; hasInputAudio: boolean };

export function VideoPricingPanel() {
  const [snapshot, setSnapshot] = useState<AdminVideoConfiguration | null>(null);
  const [routes, setRoutes] = useState<RouteModel[]>([]);
  const [selectedID, setSelectedID] = useState("");
  const [minimumPoints, setMinimumPoints] = useState("0.00000");
  const [roundingStep, setRoundingStep] = useState(1);
  const [draft, setDraft] = useState<SimulationDraft>({ taskType: "text_to_video", resolution: "720p", ratio: "16:9", audioMode: "silent", duration: 5, outputCount: 1, imageCount: 0, inputVideoSeconds: "", hasInputAudio: false });
  const [result, setResult] = useState<AdminVideoQuoteSimulationResult | null>(null);
  const [failedCandidates, setFailedCandidates] = useState<AdminVideoQuoteCandidate[]>([]);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [message, setMessage] = useState("");

  async function load(preferred?: string) {
    setLoading(true); setError("");
    try {
      const [nextSnapshot, allRoutes] = await Promise.all([adminApi.getVideoConfiguration(), adminApi.listRouteModels({ page_size: 100 })]);
      const nextRoutes = allRoutes.filter((route) => route.media_type === "video");
      setSnapshot(nextSnapshot); setRoutes(nextRoutes);
      setSelectedID((current) => preferred || current || String(nextRoutes[0]?.id ?? ""));
    } catch (caught) { setError(errorText(caught)); }
    finally { setLoading(false); }
  }
  useEffect(() => { void load(); }, []);
  const selectedRoute = useMemo(() => routes.find((route) => String(route.id) === selectedID), [routes, selectedID]);
  const config = useMemo(() => snapshot?.routes.find((item) => String(item.route_model_id) === selectedID), [snapshot, selectedID]);
  useEffect(() => { setMinimumPoints(config?.minimum_task_points ?? "0.00000"); setRoundingStep(config?.rounding_step_points ?? 1); setResult(null); setFailedCandidates([]); }, [config]);
  const rateReady = config?.candidate_account_model_ids?.filter((id) => snapshot?.rate_cards.some((card) => String(card.account_model_id) === String(id) && card.enabled)).length ?? 0;

  async function saveSettings() {
    if (!selectedRoute || !config) return;
    setBusy(true); setError(""); setMessage("");
    try {
      const combinations = Array.isArray(config.visible_options?.combinations) ? config.visible_options.combinations as AdminVideoVisibleCombination[] : [];
      await adminApi.saveRouteVideoConfig(selectedRoute.id, {
        expected_version: config.config_version, config_version: nextVersion(config.config_version),
        candidate_parameter_mappings: config.candidate_parameter_mappings ?? {}, minimum_task_points: minimumPoints, rounding_step_points: roundingStep,
        task_types: config.task_types, visible_options: config.visible_options, defaults: config.defaults,
        visible_combinations: combinations, max_output_count: config.max_output_count, enabled: config.enabled,
      });
      setMessage("报价设置已保存"); await load(selectedID);
    } catch (caught) { setError(errorText(caught)); }
    finally { setBusy(false); }
  }

  async function simulate() {
    if (!selectedRoute) return;
    setBusy(true); setError(""); setMessage(""); setResult(null); setFailedCandidates([]);
    try {
      setResult(await adminApi.simulateVideoRouteQuote(selectedRoute.id, {
        task_type: draft.taskType, resolution: draft.resolution, aspect_ratio: draft.ratio, audio_mode: draft.audioMode,
        duration_seconds: draft.duration, output_count: draft.outputCount, reference_image_count: draft.imageCount,
        input_video_seconds: draft.inputVideoSeconds, has_input_audio: draft.hasInputAudio,
      }));
    } catch (caught) {
      if (caught instanceof ApiError) {
        const candidates = caught.details?.candidates;
        if (Array.isArray(candidates)) setFailedCandidates(candidates as AdminVideoQuoteCandidate[]);
      }
      setError(errorText(caught));
    }
    finally { setBusy(false); }
  }

  if (loading && !snapshot) return <LoadingBlock label="载入视频报价配置" />;
  const resultCandidates = result ? result.candidates : null;
  const displayedCandidates = resultCandidates ?? failedCandidates;
  return <section className={adminPage.stack} data-video-pricing-overview>
    <PageHeader title="视频报价总览" description="视频销售价由每个真实模型的厂商原生费率计算；混合路由按可用候选最高价锁定积分。" secondaryActions={<RefreshIconButton label="刷新视频报价" refreshing={loading} onClick={() => void load(selectedID)} />} />
    {error ? <InlineFeedback tone="danger" message={error} /> : null}{message ? <InlineFeedback tone="success" message={message} /> : null}
    {!routes.length ? <EmptyBlock title="暂无视频路由" detail="先在路由模型中新增视频路由并绑定真实模型。" /> : <>
      <div className="grid gap-3 rounded-lg border border-[var(--border)] bg-[var(--surface-solid)] p-4 md:grid-cols-4">
        <Field label="视频路由"><select value={selectedID} onChange={(e) => setSelectedID(e.target.value)}>{routes.map((route) => <option key={String(route.id)} value={String(route.id)}>{route.name}</option>)}</select></Field>
        <Field label="费率完整性"><div className="flex min-h-10 items-center"><Badge tone={rateReady === (config?.candidate_count ?? 0) && rateReady > 0 ? "success" : "warning"}>{rateReady}/{config?.candidate_count ?? 0} 个候选</Badge></div></Field>
        <Field label="最低任务积分"><input inputMode="decimal" value={minimumPoints} onChange={(e) => setMinimumPoints(e.target.value)} /></Field>
        <Field label="积分取整步长"><select value={roundingStep} onChange={(e) => setRoundingStep(Number(e.target.value))}><option value={1}>1</option><option value={5}>5</option><option value={10}>10</option></select></Field>
        <div className="md:col-span-4 flex justify-end"><button className={cn(adminButton.base, adminButton.secondary)} disabled={busy || !config} onClick={() => void saveSettings()}>保存报价设置</button></div>
      </div>
      <div className="grid gap-4 rounded-lg border border-[var(--border)] bg-[var(--surface-solid)] p-4">
        <div><h2 className="m-0 text-base">报价试算</h2><p className="m-0 mt-1 text-xs text-[var(--muted)]">完整模拟一次用户请求，不创建任务、不扣积分。</p></div>
        <div className="grid gap-3 md:grid-cols-4 lg:grid-cols-5">
          <Field label="任务类型"><select value={draft.taskType} onChange={(e) => setDraft({ ...draft, taskType: e.target.value })}><option value="text_to_video">文生视频</option><option value="image_to_video">图生视频</option><option value="first_last_frame_to_video">首尾帧</option></select></Field>
          <Field label="路由分辨率"><input value={draft.resolution} onChange={(e) => setDraft({ ...draft, resolution: e.target.value })} /></Field><Field label="比例"><input value={draft.ratio} onChange={(e) => setDraft({ ...draft, ratio: e.target.value })} /></Field>
          <Field label="输出时长（秒）"><input type="number" min="1" value={draft.duration} onChange={(e) => setDraft({ ...draft, duration: Number(e.target.value) })} /></Field><Field label="输出数量"><input type="number" min="1" max="4" value={draft.outputCount} onChange={(e) => setDraft({ ...draft, outputCount: Number(e.target.value) })} /></Field>
          <Field label="参考图片数"><input type="number" min="0" value={draft.imageCount} onChange={(e) => setDraft({ ...draft, imageCount: Number(e.target.value) })} /></Field><Field label="输入视频秒数"><input inputMode="decimal" value={draft.inputVideoSeconds} onChange={(e) => setDraft({ ...draft, inputVideoSeconds: e.target.value })} /></Field>
          <Field label="输出音频"><select value={draft.audioMode} onChange={(e) => setDraft({ ...draft, audioMode: e.target.value })}><option value="silent">静音</option><option value="generated">生成音频</option></select></Field><Field label="包含输入音频"><select value={draft.hasInputAudio ? "yes" : "no"} onChange={(e) => setDraft({ ...draft, hasInputAudio: e.target.value === "yes" })}><option value="no">否</option><option value="yes">是</option></select></Field>
          <div className="flex items-end"><button className={cn(adminButton.base, adminButton.primary)} disabled={busy || !config} onClick={() => void simulate()}>运行试算</button></div>
        </div>
        {result ? <div className="grid gap-3 border-y border-[var(--border)] py-3 sm:grid-cols-4"><Summary label="最高销售价" value={`${result.highest_cny} CNY`} /><Summary label="全局汇率" value={`${result.cny_per_point} CNY/积分`} /><Summary label="单结果积分" value={result.unit_points} /><Summary label="总积分" value={result.total_points} /></div> : null}
        {displayedCandidates.length ? <div className="overflow-x-auto"><table className="admin-table min-w-[760px]"><thead><tr><th>真实模型</th><th>厂商</th><th>映射分辨率</th><th>状态</th><th>CNY 报价</th><th>说明</th></tr></thead><tbody>{displayedCandidates.map((candidate) => <tr key={candidate.route_candidate_id}><td>{candidate.model_code}<small className="block text-[var(--muted)]">#{candidate.account_model_id}</small></td><td>{candidate.provider_code}</td><td>{candidate.mapped_resolution}</td><td><Badge tone={candidate.eligible ? "success" : "warning"}>{candidate.eligible ? "可报价" : "已排除"}</Badge></td><td>{candidate.estimated_cny}</td><td>{candidate.exclusion_code || (result && candidate.account_model_id === result.highest_account_model_id ? "最高价来源" : "参与最高价比较")}</td></tr>)}</tbody></table></div> : null}
      </div>
    </>}
  </section>;
}

function Summary({ label, value }: { label: string; value: string }) { return <div><small className="block text-[var(--muted)]">{label}</small><strong className="mt-1 block">{value}</strong></div>; }
function nextVersion(current?: string) { const match = current?.match(/(\d+)$/); return `video-route-v${Number(match?.[1] ?? 0) + 1}`; }
function errorText(value: unknown) { return value instanceof Error ? value.message : "视频报价操作失败"; }
