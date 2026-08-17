import { useEffect, useState } from "react";
import { Plus, Trash2 } from "lucide-react";
import type { AdminVideoRouteConfig, RouteModel, RouteModelCandidate } from "../../../shared/api-types";
import { adminApi } from "../../../shared/admin-api";
import { cn } from "../../../shared/classnames";
import { Badge, Field, InlineFeedback, TooltipIconButton } from "../components";
import { adminButton } from "../ui/classes";

type MappingRow = { candidateID: string; routeResolution: string; nativeResolution: string };

export function VideoRouteConfigPanel({ route, config, candidates, onSaved }: { route: RouteModel; config?: AdminVideoRouteConfig; candidates: RouteModelCandidate[]; onSaved: () => void }) {
  const [mappings, setMappings] = useState<MappingRow[]>([]);
  const [mappingDraft, setMappingDraft] = useState<MappingRow>({ candidateID: "", routeResolution: "720p", nativeResolution: "768p" });
  const [maxOutput, setMaxOutput] = useState(1);
  const [enabled, setEnabled] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [message, setMessage] = useState("");

  useEffect(() => {
    setMappings(flattenMappings(config?.candidate_parameter_mappings ?? {}));
    setMappingDraft({ candidateID: String(config?.candidate_account_model_ids?.[0] ?? ""), routeResolution: "720p", nativeResolution: "768p" });
    setMaxOutput(config?.max_output_count ?? 1);
    setEnabled(Boolean(config?.enabled));
  }, [config, route.id]);

  async function save() {
    setBusy(true); setError(""); setMessage("");
    try {
      const visibleOptions = { ...(config?.visible_options ?? {}) };
      delete visibleOptions.combinations;
      await adminApi.saveRouteVideoConfig(route.id, {
        expected_version: config?.config_version ?? "", config_version: nextVersion(config?.config_version),
        candidate_parameter_mappings: buildMappings(mappings), minimum_task_points: config?.minimum_task_points ?? "0.00000",
        rounding_step_points: config?.rounding_step_points ?? 1,
        task_types: config?.task_types ?? [], visible_options: visibleOptions, defaults: config?.defaults ?? {},
        max_output_count: maxOutput, enabled,
      });
      setMessage("视频路由参数已保存"); onSaved();
    } catch (caught) { setError(caught instanceof Error ? caught.message : "保存失败"); }
    finally { setBusy(false); }
  }

  async function remove() {
    if (!config || !window.confirm("删除当前视频参数配置？路由模型和候选不会被删除。")) return;
    setBusy(true); setError(""); setMessage("");
    try {
      await adminApi.deleteRouteVideoConfig(route.id);
      onSaved();
    } catch (caught) { setError(caught instanceof Error ? caught.message : "删除失败"); }
    finally { setBusy(false); }
  }

  return <section className="grid gap-4 rounded-lg border border-[var(--border)] bg-[var(--surface-solid)] p-4" data-video-route-config-panel>
    <header className="flex flex-wrap items-start justify-between gap-3 border-b border-[var(--border)] pb-4">
      <div><div className="flex items-center gap-2"><h2 className="m-0 text-base">{route.name} · 视频参数</h2><Badge tone={enabled ? "success" : "warning"}>{enabled ? "已启用" : "未启用"}</Badge></div><p className="m-0 mt-1 text-xs text-[var(--muted)]">用户可选参数由已绑定真实模型的能力自动汇总；此处只维护跨厂商参数映射。</p></div>
      <div className="flex flex-wrap gap-2">
        {config ? <button className={cn(adminButton.base, adminButton.ghost, adminButton.small)} disabled={busy} onClick={() => void remove()}><Trash2 size={15} />删除视频配置</button> : null}
        <button className={cn(adminButton.base, adminButton.primary, adminButton.small)} disabled={busy} onClick={() => void save()}>保存视频配置</button>
      </div>
    </header>
    {error ? <InlineFeedback tone="danger" message={error} /> : null}{message ? <InlineFeedback tone="success" message={message} /> : null}
    <div className="grid gap-3 md:grid-cols-3"><Field label="平台最大输出数"><input type="number" min="1" max="4" value={maxOutput} onChange={(e) => setMaxOutput(Math.min(4, Math.max(1, Number(e.target.value))))} /></Field><Field label="状态"><select value={enabled ? "enabled" : "disabled"} onChange={(e) => setEnabled(e.target.value === "enabled")}><option value="disabled">停用</option><option value="enabled">启用</option></select></Field></div>
    <div className="grid gap-3 border-t border-[var(--border)] pt-4"><strong className="text-sm">候选分辨率映射</strong>
      {mappings.map((row, index) => <div key={`${row.candidateID}-${row.routeResolution}-${index}`} className="grid items-end gap-2 md:grid-cols-[1fr_1fr_1fr_auto]"><Field label="真实模型"><input value={candidateLabel(row.candidateID, candidates)} readOnly /></Field><Field label="路由分辨率"><input value={row.routeResolution} readOnly /></Field><Field label="厂商原生分辨率"><input value={row.nativeResolution} readOnly /></Field><TooltipIconButton label="删除映射" onClick={() => setMappings((items) => items.filter((_, i) => i !== index))}><Trash2 /></TooltipIconButton></div>)}
      <div className="grid items-end gap-2 md:grid-cols-[1fr_1fr_1fr_auto]"><Field label="真实模型"><select value={mappingDraft.candidateID} onChange={(e) => setMappingDraft({ ...mappingDraft, candidateID: e.target.value })}>{(config?.candidate_account_model_ids ?? []).map((id) => <option key={id} value={id}>{candidateLabel(String(id), candidates)}</option>)}</select></Field><Field label="路由分辨率"><input value={mappingDraft.routeResolution} onChange={(e) => setMappingDraft({ ...mappingDraft, routeResolution: e.target.value })} /></Field><Field label="厂商原生分辨率"><input value={mappingDraft.nativeResolution} onChange={(e) => setMappingDraft({ ...mappingDraft, nativeResolution: e.target.value })} /></Field><button className={cn(adminButton.base, adminButton.secondary, adminButton.small)} onClick={() => { if (mappingDraft.candidateID && mappingDraft.routeResolution && mappingDraft.nativeResolution) setMappings((items) => [...items.filter((item) => !(item.candidateID === mappingDraft.candidateID && item.routeResolution === mappingDraft.routeResolution)), mappingDraft]); }}><Plus size={15} />添加</button></div>
    </div>
  </section>;
}

function nextVersion(current?: string) { const match = current?.match(/(\d+)$/); return `video-route-v${Number(match?.[1] ?? 0) + 1}`; }
function flattenMappings(value: AdminVideoRouteConfig["candidate_parameter_mappings"]): MappingRow[] { return Object.entries(value).flatMap(([candidateID, config]) => Object.entries(config.resolutions ?? {}).map(([routeResolution, nativeResolution]) => ({ candidateID, routeResolution, nativeResolution }))); }
function buildMappings(rows: MappingRow[]): AdminVideoRouteConfig["candidate_parameter_mappings"] { const result: AdminVideoRouteConfig["candidate_parameter_mappings"] = {}; for (const row of rows) { result[row.candidateID] ??= { resolutions: {} }; result[row.candidateID].resolutions![row.routeResolution] = row.nativeResolution; } return result; }
function candidateLabel(id: string, candidates: RouteModelCandidate[]) { const candidate = candidates.find((item) => String(item.account_model_id) === id); return candidate ? [candidate.account_name, candidate.model_code].filter(Boolean).join(" / ") || "未命名真实模型" : "已移除真实模型"; }
