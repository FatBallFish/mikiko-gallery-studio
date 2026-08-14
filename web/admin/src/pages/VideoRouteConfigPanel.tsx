import { useEffect, useState } from "react";
import { Plus, Trash2 } from "lucide-react";
import type {
  AdminVideoPricingStrategy,
  AdminVideoRouteConfig,
  AdminVideoVisibleCombination,
  RouteModel,
} from "../../../shared/api-types";
import { adminApi } from "../../../shared/admin-api";
import { cn } from "../../../shared/classnames";
import {
  Badge,
  EmptyBlock,
  Field,
  InlineFeedback,
  TooltipIconButton,
} from "../components";
import { adminButton } from "../ui/classes";

const tasks = [
  { value: "text_to_video", label: "文生视频" },
  { value: "image_to_video", label: "图生视频" },
  { value: "first_last_frame_to_video", label: "首尾帧生视频" },
];

export function VideoRouteConfigPanel({
  route,
  config,
  strategies,
  onSaved,
}: {
  route: RouteModel;
  config?: AdminVideoRouteConfig;
  strategies: AdminVideoPricingStrategy[];
  onSaved: () => void;
}) {
  const [strategyID, setStrategyID] = useState("");
  const [maxOutput, setMaxOutput] = useState(1);
  const [enabled, setEnabled] = useState(false);
  const [combinations, setCombinations] = useState<
    AdminVideoVisibleCombination[]
  >([]);
  const [draft, setDraft] =
    useState<AdminVideoVisibleCombination>(blankCombination());
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [message, setMessage] = useState("");

  useEffect(() => {
    setStrategyID(
      String(config?.pricing_strategy_id ?? strategies[0]?.id ?? ""),
    );
    setMaxOutput(config?.max_output_count ?? 1);
    setEnabled(Boolean(config?.enabled));
    setCombinations(
      Array.isArray(config?.visible_options?.combinations)
        ? (config!.visible_options
            .combinations as AdminVideoVisibleCombination[])
        : [],
    );
    setDraft(blankCombination());
  }, [config, route.id, strategies]);

  function addCombination() {
    if (
      !draft.task_type ||
      !draft.resolution ||
      !draft.aspect_ratio ||
      !draft.audio_mode ||
      draft.duration_seconds < 1
    )
      return;
    const key = combinationKey(draft);
    setCombinations((items) =>
      items.some((item) => combinationKey(item) === key)
        ? items
        : [...items, draft],
    );
    setDraft(blankCombination());
  }
  async function save() {
    setBusy(true);
    setError("");
    setMessage("");
    try {
      const nextOptions = { ...(config?.visible_options ?? {}), combinations };
      const defaults = combinations[0]
        ? { ...combinations[0] }
        : (config?.defaults ?? {});
      await adminApi.saveRouteVideoConfig(route.id, {
        expected_version: config?.config_version ?? "",
        config_version: nextVersion(config?.config_version),
        pricing_strategy_id: Number(strategyID),
        task_types: Array.from(
          new Set(combinations.map((item) => item.task_type)),
        ),
        visible_options: nextOptions,
        defaults,
        visible_combinations: combinations,
        max_output_count: maxOutput,
        enabled,
      });
      setMessage("视频路由参数配置已保存");
      onSaved();
    } catch (caught) {
      setError(
        caught instanceof Error ? caught.message : "视频路由配置保存失败",
      );
    } finally {
      setBusy(false);
    }
  }
  async function remove() {
    if (
      !config ||
      !window.confirm("删除当前视频路由参数配置？路由模型和候选不会被删除。")
    )
      return;
    setBusy(true);
    try {
      await adminApi.deleteRouteVideoConfig(route.id);
      onSaved();
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : "删除失败");
    } finally {
      setBusy(false);
    }
  }

  return (
    <section
      className="grid gap-4 rounded-lg border border-[var(--border)] bg-[var(--surface-solid)] p-4"
      data-video-route-config-panel
    >
      <header className="flex flex-wrap items-start justify-between gap-3 border-b border-[var(--border)] pb-4">
        <div>
          <div className="flex items-center gap-2">
            <h2 className="m-0 text-base">{route.name} · 视频参数</h2>
            <Badge tone={config?.enabled ? "success" : "warning"}>
              {config?.enabled ? "已启用" : "未启用"}
            </Badge>
          </div>
          <p className="m-0 mt-1 text-xs text-[var(--muted)]">
            配置用户可选择的完整组合、默认价格策略和平台最大输出数。
          </p>
        </div>
        <div className="flex gap-2">
          {config ? (
            <button
              className={cn(
                adminButton.base,
                adminButton.ghost,
                adminButton.small,
              )}
              disabled={busy}
              onClick={() => void remove()}
            >
              删除视频配置
            </button>
          ) : null}
          <button
            className={cn(adminButton.base, adminButton.primary)}
            disabled={busy || !strategyID || (enabled && !combinations.length)}
            onClick={() => void save()}
          >
            保存视频路由
          </button>
        </div>
      </header>
      {error ? <InlineFeedback tone="danger" message={error} /> : null}
      {message ? <InlineFeedback tone="success" message={message} /> : null}
      <div className="grid gap-3 md:grid-cols-3">
        <Field label="默认价格策略">
          <select
            value={strategyID}
            onChange={(event) => setStrategyID(event.target.value)}
          >
            <option value="">请选择</option>
            {strategies.map((item) => (
              <option key={item.id} value={item.id}>
                {item.name}
                {item.enabled ? "" : "（停用）"}
              </option>
            ))}
          </select>
        </Field>
        <Field label="平台最大输出数">
          <input
            type="number"
            min="1"
            max="4"
            value={maxOutput}
            onChange={(event) =>
              setMaxOutput(Math.min(4, Math.max(1, Number(event.target.value))))
            }
          />
        </Field>
        <Field label="状态">
          <select
            value={enabled ? "enabled" : "disabled"}
            onChange={(event) => setEnabled(event.target.value === "enabled")}
          >
            <option value="disabled">停用</option>
            <option value="enabled">启用</option>
          </select>
        </Field>
      </div>
      <section className="grid gap-3 border-t border-[var(--border)] pt-4">
        <h3 className="m-0 text-sm">用户可见参数组合</h3>
        <div className="grid gap-2 md:grid-cols-6">
          <Field label="任务类型">
            <select
              value={draft.task_type}
              onChange={(event) =>
                setDraft({ ...draft, task_type: event.target.value })
              }
            >
              {tasks.map((item) => (
                <option key={item.value} value={item.value}>
                  {item.label}
                </option>
              ))}
            </select>
          </Field>
          <Field label="分辨率">
            <input
              value={draft.resolution}
              onChange={(event) =>
                setDraft({ ...draft, resolution: event.target.value })
              }
            />
          </Field>
          <Field label="比例">
            <input
              value={draft.aspect_ratio}
              onChange={(event) =>
                setDraft({ ...draft, aspect_ratio: event.target.value })
              }
            />
          </Field>
          <Field label="音频">
            <select
              value={draft.audio_mode}
              onChange={(event) =>
                setDraft({ ...draft, audio_mode: event.target.value })
              }
            >
              <option value="silent">静音</option>
              <option value="generated">生成音频</option>
            </select>
          </Field>
          <Field label="时长（秒）">
            <input
              type="number"
              min="1"
              value={draft.duration_seconds}
              onChange={(event) =>
                setDraft({
                  ...draft,
                  duration_seconds: Number(event.target.value),
                })
              }
            />
          </Field>
          <div className="flex items-end">
            <button
              type="button"
              className={cn(adminButton.base, adminButton.secondary, "w-full")}
              onClick={addCombination}
            >
              <Plus size={15} />
              添加组合
            </button>
          </div>
        </div>
        {!combinations.length ? (
          <EmptyBlock
            title="暂无可见组合"
            detail="停用状态可保存空配置；启用前至少添加一个完整参数组合。"
          />
        ) : (
          <div className="overflow-x-auto">
            <table className="admin-table min-w-[680px]">
              <thead>
                <tr>
                  <th>任务</th>
                  <th>分辨率</th>
                  <th>比例</th>
                  <th>音频</th>
                  <th>时长</th>
                  <th>操作</th>
                </tr>
              </thead>
              <tbody>
                {combinations.map((item) => (
                  <tr key={combinationKey(item)}>
                    <td>
                      {tasks.find((task) => task.value === item.task_type)
                        ?.label ?? item.task_type}
                    </td>
                    <td>{item.resolution}</td>
                    <td>{item.aspect_ratio}</td>
                    <td>
                      {item.audio_mode === "generated" ? "生成音频" : "静音"}
                    </td>
                    <td>{item.duration_seconds} 秒</td>
                    <td>
                      <TooltipIconButton
                        label="删除可见组合"
                        onClick={() =>
                          setCombinations((items) =>
                            items.filter(
                              (current) =>
                                combinationKey(current) !==
                                combinationKey(item),
                            ),
                          )
                        }
                      >
                        <Trash2 />
                      </TooltipIconButton>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </section>
    </section>
  );
}

function blankCombination(): AdminVideoVisibleCombination {
  return {
    task_type: "text_to_video",
    resolution: "720p",
    aspect_ratio: "16:9",
    audio_mode: "silent",
    duration_seconds: 5,
  };
}
function combinationKey(item: AdminVideoVisibleCombination) {
  return `${item.task_type}/${item.resolution}/${item.aspect_ratio}/${item.audio_mode}/${item.duration_seconds}`;
}
function nextVersion(current?: string) {
  const match = current?.match(/(\d+)$/);
  return `video-route-v${Number(match?.[1] ?? 0) + 1}`;
}
