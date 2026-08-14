import { useEffect, useMemo, useState } from "react";
import { Plus, Trash2 } from "lucide-react";
import type {
  AdminVideoConfiguration,
  AdminVideoPriceRuleWrite,
  AdminVideoPricingStrategy,
  AdminVideoRouteConfig,
  AdminVideoSimulationResult,
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
  Modal,
  PageHeader,
  RefreshIconButton,
  TooltipIconButton,
} from "../components";
import { adminButton, adminPage } from "../ui/classes";

type StrategyDraft = {
  row?: AdminVideoPricingStrategy;
  code: string;
  name: string;
  grossPointValue: string;
  minimumIncome: string;
  maxBonusRatio: string;
  paymentFee: string;
  targetMargin: string;
  costBuffer: string;
  fixedCost: string;
  outputSecondCost: string;
  referenceCost: string;
  audioFixedCost: string;
  audioSecondCost: string;
  exactReserveMarkup: string;
  meteredReserveMarkup: string;
  enabled: boolean;
};
type RuleDraft = {
  row?: AdminVideoConfiguration["price_rules"][number];
  routeID: string;
  strategyID: string;
  taskType: string;
  resolution: string;
  audioMode: string;
  duration: number;
  pricingMode: "exact" | "metered";
  fixedPoints: string;
  outputSecondPoints: string;
  referencePoints: string;
  inputVideoSecondPoints: string;
  referenceAudioSecondPoints: string;
  generatedAudioFixedPoints: string;
  generatedAudioSecondPoints: string;
  minimumBillableSeconds: number;
  minimumPoints: string;
  reserveMarkup: string;
  effectiveAt?: string;
  expiresAt?: string;
  enabled: boolean;
};
type PricingBinding = {
  task_type: string;
  resolution: string;
  aspect_ratio?: string;
  audio_mode: string;
  duration_seconds?: number;
  pricing_strategy_id: number;
};
type BindingDraft = {
  routeID: string;
  taskType: string;
  resolution: string;
  aspectRatio: string;
  audioMode: string;
  duration: number;
  strategyID: string;
};

export function VideoPricingPanel() {
  const [snapshot, setSnapshot] = useState<AdminVideoConfiguration | null>(
    null,
  );
  const [routes, setRoutes] = useState<RouteModel[]>([]);
  const [selectedStrategyID, setSelectedStrategyID] = useState("");
  const [strategyDraft, setStrategyDraft] = useState<StrategyDraft | null>(
    null,
  );
  const [ruleDraft, setRuleDraft] = useState<RuleDraft | null>(null);
  const [bindingDraft, setBindingDraft] = useState<BindingDraft | null>(null);
  const [simulation, setSimulation] =
    useState<AdminVideoSimulationResult | null>(null);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [message, setMessage] = useState("");

  async function load(preferredStrategyID?: string) {
    setLoading(true);
    setError("");
    try {
      const [next, allRoutes] = await Promise.all([
        adminApi.getVideoConfiguration(),
        adminApi.listRouteModels({ page_size: 100 }),
      ]);
      setSnapshot(next);
      setRoutes(allRoutes.filter((route) => route.media_type === "video"));
      setSelectedStrategyID((current) => {
        const wanted = preferredStrategyID || current;
        return next.pricing_strategies.some(
          (item) => String(item.id) === wanted,
        )
          ? wanted
          : String(next.pricing_strategies[0]?.id ?? "");
      });
    } catch (caught) {
      setError(errorText(caught));
    } finally {
      setLoading(false);
    }
  }
  useEffect(() => {
    void load();
  }, []);
  const selectedStrategy = useMemo(
    () =>
      snapshot?.pricing_strategies.find(
        (item) => String(item.id) === selectedStrategyID,
      ),
    [snapshot, selectedStrategyID],
  );
  const selectedRules = useMemo(
    () =>
      snapshot?.price_rules.filter(
        (item) => String(item.pricing_strategy_id) === selectedStrategyID,
      ) ?? [],
    [snapshot, selectedStrategyID],
  );

  async function saveStrategy() {
    if (!strategyDraft) return;
    setBusy(true);
    setError("");
    setMessage("");
    const input = {
      expected_version: strategyDraft.row?.strategy_version ?? 0,
      code: strategyDraft.code,
      name: strategyDraft.name,
      gross_point_value_cny: strategyDraft.grossPointValue,
      minimum_net_point_income_cny: strategyDraft.minimumIncome,
      max_bonus_ratio: strategyDraft.maxBonusRatio,
      payment_fee_rate: strategyDraft.paymentFee,
      target_margin_rate: strategyDraft.targetMargin,
      provider_cost_buffer_rate: strategyDraft.costBuffer,
      platform_fixed_cost_cny: strategyDraft.fixedCost,
      platform_output_second_cost_cny: strategyDraft.outputSecondCost,
      platform_reference_cost_cny: strategyDraft.referenceCost,
      platform_audio_fixed_cost_cny: strategyDraft.audioFixedCost,
      platform_audio_second_cost_cny: strategyDraft.audioSecondCost,
      exact_reserve_markup: strategyDraft.exactReserveMarkup,
      metered_reserve_markup: strategyDraft.meteredReserveMarkup,
      enabled: strategyDraft.enabled,
    };
    try {
      const saved = strategyDraft.row
        ? await adminApi.updateVideoPricingStrategy(strategyDraft.row.id, input)
        : await adminApi.createVideoPricingStrategy(input);
      setStrategyDraft(null);
      setMessage("视频价格策略已保存");
      await load(String(saved.id));
    } catch (caught) {
      setError(errorText(caught));
    } finally {
      setBusy(false);
    }
  }

  async function saveRule() {
    if (!ruleDraft) return;
    setBusy(true);
    setError("");
    setMessage("");
    const input: AdminVideoPriceRuleWrite = {
      id: ruleDraft.row?.id,
      route_model_id: Number(ruleDraft.routeID),
      pricing_strategy_id: Number(ruleDraft.strategyID),
      expected_version: ruleDraft.row?.rule_version ?? 0,
      task_type: ruleDraft.taskType,
      resolution: ruleDraft.resolution,
      audio_mode: ruleDraft.audioMode,
      duration_seconds: ruleDraft.duration,
      effective_at: ruleDraft.effectiveAt || new Date().toISOString(),
      expires_at: ruleDraft.expiresAt || undefined,
      pricing_mode: ruleDraft.pricingMode,
      fixed_task_points: ruleDraft.fixedPoints,
      output_second_points: ruleDraft.outputSecondPoints,
      reference_image_points: ruleDraft.referencePoints,
      input_video_second_points: ruleDraft.inputVideoSecondPoints,
      reference_audio_second_points: ruleDraft.referenceAudioSecondPoints,
      generated_audio_fixed_points: ruleDraft.generatedAudioFixedPoints,
      generated_audio_second_points: ruleDraft.generatedAudioSecondPoints,
      minimum_billable_seconds: ruleDraft.minimumBillableSeconds,
      minimum_task_points: ruleDraft.minimumPoints,
      reserve_markup: ruleDraft.reserveMarkup,
      enabled: ruleDraft.enabled,
    };
    try {
      if (ruleDraft.row)
        await adminApi.updateVideoPriceRule(ruleDraft.row.id, input);
      else await adminApi.createVideoPriceRule(input);
      setRuleDraft(null);
      setSimulation(null);
      setMessage("视频参数价格已保存");
      await load(ruleDraft.strategyID);
    } catch (caught) {
      setError(errorText(caught));
    } finally {
      setBusy(false);
    }
  }

  async function simulateRule(draft: RuleDraft) {
    setBusy(true);
    setError("");
    try {
      setSimulation(
        await adminApi.simulateVideoPricing(draft.strategyID, {
          route_model_id: Number(draft.routeID),
          task_type: draft.taskType,
          resolution: draft.resolution,
          audio_mode: draft.audioMode,
          duration_seconds: draft.duration,
          reference_image_count: draft.taskType === "text_to_video" ? 0 : 1,
        }),
      );
    } catch (caught) {
      setError(errorText(caught));
    } finally {
      setBusy(false);
    }
  }

  async function saveBinding() {
    if (!bindingDraft || !snapshot) return;
    const route = snapshot.routes.find(
      (item) => String(item.route_model_id) === bindingDraft.routeID,
    );
    if (!route) return setError("请先在视频路由页保存路由参数配置");
    setBusy(true);
    setError("");
    setMessage("");
    try {
      const bindings = routeBindings(route).filter(
        (item) =>
          bindingKey(item) !==
          bindingKey({
            task_type: bindingDraft.taskType,
            resolution: bindingDraft.resolution,
            aspect_ratio: bindingDraft.aspectRatio,
            audio_mode: bindingDraft.audioMode,
            duration_seconds: bindingDraft.duration,
            pricing_strategy_id: 0,
          }),
      );
      bindings.push({
        task_type: bindingDraft.taskType,
        resolution: bindingDraft.resolution,
        aspect_ratio: bindingDraft.aspectRatio,
        audio_mode: bindingDraft.audioMode,
        duration_seconds: bindingDraft.duration,
        pricing_strategy_id: Number(bindingDraft.strategyID),
      });
      await saveRouteWithBindings(route, bindings);
      setBindingDraft(null);
      setMessage("参数价格策略绑定已保存");
      await load(bindingDraft.strategyID);
    } catch (caught) {
      setError(errorText(caught));
    } finally {
      setBusy(false);
    }
  }

  async function deleteBinding(
    route: AdminVideoRouteConfig,
    binding: PricingBinding,
  ) {
    setBusy(true);
    setError("");
    try {
      await saveRouteWithBindings(
        route,
        routeBindings(route).filter(
          (item) => bindingKey(item) !== bindingKey(binding),
        ),
      );
      await load();
    } catch (caught) {
      setError(errorText(caught));
    } finally {
      setBusy(false);
    }
  }

  async function deleteStrategy(row: AdminVideoPricingStrategy) {
    if (!window.confirm(`删除策略“${row.name}”？`)) return;
    setBusy(true);
    try {
      await adminApi.deleteVideoPricingStrategy(row.id, row.strategy_version);
      await load();
    } catch (caught) {
      setError(errorText(caught));
    } finally {
      setBusy(false);
    }
  }
  async function deleteRule(
    row: AdminVideoConfiguration["price_rules"][number],
  ) {
    if (!window.confirm("删除该视频参数价格？")) return;
    setBusy(true);
    try {
      await adminApi.deleteVideoPriceRule(row.id, row.rule_version);
      await load(selectedStrategyID);
    } catch (caught) {
      setError(errorText(caught));
    } finally {
      setBusy(false);
    }
  }

  if (loading && !snapshot) return <div role="status">载入视频价格策略...</div>;
  const allBindings = (snapshot?.routes ?? []).flatMap((route) =>
    routeBindings(route).map((binding) => ({ route, binding })),
  );
  return (
    <section className={adminPage.stack} data-video-pricing-panel>
      <PageHeader
        title="视频价格策略"
        description="销售积分由画质、时长、音频和参考素材规则组成；服务端保存并校验成本安全线。"
        primaryAction={
          <button
            type="button"
            className={cn(adminButton.base, adminButton.primary)}
            onClick={() => setStrategyDraft(blankStrategy())}
          >
            <Plus size={16} />
            新增策略
          </button>
        }
        secondaryActions={
          <RefreshIconButton
            label="刷新视频价格策略"
            refreshing={loading}
            onClick={() => void load()}
          />
        }
      />
      {error ? <InlineFeedback tone="danger" message={error} /> : null}
      {message ? <InlineFeedback tone="success" message={message} /> : null}
      {!snapshot?.pricing_strategies.length ? (
        <EmptyBlock
          title="暂无视频价格策略"
          detail="先创建可证明不会亏损的销售策略，再配置参数价格。"
        />
      ) : (
        <div className="grid min-w-0 overflow-hidden rounded-lg border border-[var(--border)] bg-[var(--surface-solid)] lg:grid-cols-[280px_minmax(0,1fr)]">
          <aside className="border-b border-[var(--border)] p-2 lg:border-b-0 lg:border-r">
            {snapshot.pricing_strategies.map((strategy) => (
              <button
                key={strategy.id}
                type="button"
                className={cn(
                  "mb-1 flex min-h-14 w-full items-center justify-between rounded-md px-3 text-left",
                  selectedStrategyID === String(strategy.id)
                    ? "bg-[var(--elevated)]"
                    : "hover:bg-[var(--surface)]",
                )}
                onClick={() => setSelectedStrategyID(String(strategy.id))}
              >
                <span className="min-w-0">
                  <strong className="block truncate text-sm">
                    {strategy.name}
                  </strong>
                  <small className="font-mono text-[var(--muted)]">
                    {strategy.code} · v{strategy.strategy_version}
                  </small>
                </span>
                <Badge tone={strategy.enabled ? "success" : "warning"}>
                  {strategy.enabled ? "启用" : "停用"}
                </Badge>
              </button>
            ))}
          </aside>
          <section className="min-w-0 p-4">
            {selectedStrategy ? (
              <>
                <header className="mb-4 flex flex-wrap justify-between gap-3 border-b border-[var(--border)] pb-4">
                  <div>
                    <h2 className="m-0 text-base">{selectedStrategy.name}</h2>
                    <p className="m-0 mt-1 text-xs text-[var(--muted)]">
                      目标毛利 {percent(selectedStrategy.target_margin_rate)} ·
                      成本缓冲{" "}
                      {percent(selectedStrategy.provider_cost_buffer_rate)}
                    </p>
                  </div>
                  <div className="flex gap-2">
                    <button
                      className={cn(
                        adminButton.base,
                        adminButton.ghost,
                        adminButton.small,
                      )}
                      onClick={() =>
                        setStrategyDraft(editStrategy(selectedStrategy))
                      }
                    >
                      编辑策略
                    </button>
                    <TooltipIconButton
                      label="删除视频价格策略"
                      onClick={() => void deleteStrategy(selectedStrategy)}
                    >
                      <Trash2 />
                    </TooltipIconButton>
                    <button
                      className={cn(
                        adminButton.base,
                        adminButton.primary,
                        adminButton.small,
                      )}
                      disabled={!routes.length}
                      onClick={() =>
                        setRuleDraft(blankRule(routes, selectedStrategy))
                      }
                    >
                      <Plus size={15} />
                      新增参数价格
                    </button>
                  </div>
                </header>
                {!selectedRules.length ? (
                  <EmptyBlock
                    title="暂无参数价格"
                    detail="为该策略新增任务类型、分辨率、音频模式对应的积分规则。"
                  />
                ) : (
                  <div className="overflow-x-auto">
                    <table className="admin-table min-w-[780px]">
                      <thead>
                        <tr>
                          <th>参数组合</th>
                          <th>计费模式</th>
                          <th>销售积分</th>
                          <th>成本上限</th>
                          <th>状态</th>
                          <th>操作</th>
                        </tr>
                      </thead>
                      <tbody>
                        {selectedRules.map((rule) => (
                          <tr key={rule.id}>
                            <td>
                              <strong>{videoTaskLabel(rule.task_type)}</strong>
                              <small className="mt-1 block">
                                {rule.resolution} ·{" "}
                                {rule.audio_mode === "generated"
                                  ? "生成音频"
                                  : "静音"}
                              </small>
                            </td>
                            <td>
                              {rule.pricing_mode === "metered"
                                ? "按实际时长"
                                : "按请求精确结算"}
                            </td>
                            <td>◈{rule.sales_points}</td>
                            <td>
                              ¥{rule.candidate_cost_upper_cny || "未试算"}
                            </td>
                            <td>
                              <Badge
                                tone={rule.enabled ? "success" : "warning"}
                              >
                                {rule.enabled ? "启用" : "停用"}
                              </Badge>
                            </td>
                            <td>
                              <div className="flex justify-end gap-1">
                                <button
                                  className={cn(
                                    adminButton.base,
                                    adminButton.ghost,
                                    adminButton.small,
                                  )}
                                  onClick={() =>
                                    setRuleDraft(editRule(rule, routes))
                                  }
                                >
                                  编辑
                                </button>
                                <TooltipIconButton
                                  label="删除参数价格"
                                  onClick={() => void deleteRule(rule)}
                                >
                                  <Trash2 />
                                </TooltipIconButton>
                              </div>
                            </td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </div>
                )}
              </>
            ) : null}
          </section>
        </div>
      )}
      <section className="grid gap-3 border-t border-[var(--border)] pt-5">
        <header className="flex flex-wrap items-center justify-between gap-3">
          <div>
            <h2 className="m-0 text-base">路由参数策略绑定</h2>
            <p className="m-0 mt-1 text-xs text-[var(--muted)]">
              命中参数绑定时使用指定策略，未命中时回退路由默认策略。
            </p>
          </div>
          <button
            className={cn(adminButton.base, adminButton.secondary)}
            disabled={
              !snapshot?.routes.length || !snapshot?.pricing_strategies.length
            }
            onClick={() => setBindingDraft(blankBinding(snapshot!))}
          >
            <Plus size={15} />
            新增绑定
          </button>
        </header>
        {!allBindings.length ? (
          <EmptyBlock
            title="暂无参数策略绑定"
            detail="当前所有视频路由使用各自默认策略。"
          />
        ) : (
          <div className="overflow-x-auto">
            <table className="admin-table min-w-[760px]">
              <thead>
                <tr>
                  <th>视频路由</th>
                  <th>参数组合</th>
                  <th>价格策略</th>
                  <th>操作</th>
                </tr>
              </thead>
              <tbody>
                {allBindings.map(({ route, binding }) => (
                  <tr key={`${route.route_model_id}:${bindingKey(binding)}`}>
                    <td>{route.route_name || route.route_code}</td>
                    <td>
                      {videoTaskLabel(binding.task_type)} · {binding.resolution}{" "}
                      · {binding.aspect_ratio || "任意比例"} · {binding.audio_mode} ·{" "}
                      {binding.duration_seconds || "任意"}秒
                    </td>
                    <td>
                      {snapshot?.pricing_strategies.find(
                        (item) => item.id === binding.pricing_strategy_id,
                      )?.name ?? `策略 #${binding.pricing_strategy_id}`}
                    </td>
                    <td>
                      <TooltipIconButton
                        label="删除参数策略绑定"
                        onClick={() => void deleteBinding(route, binding)}
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
      {strategyDraft ? (
        <Modal
          title={strategyDraft.row ? "编辑视频价格策略" : "新增视频价格策略"}
          detail="策略参数用于服务端计算成本安全线和目标毛利。"
          onClose={() => setStrategyDraft(null)}
          footer={
            <>
              <button
                className={cn(adminButton.base, adminButton.ghost)}
                onClick={() => setStrategyDraft(null)}
              >
                取消
              </button>
              <button
                className={cn(adminButton.base, adminButton.primary)}
                disabled={busy || !strategyDraft.code || !strategyDraft.name}
                onClick={() => void saveStrategy()}
              >
                保存策略
              </button>
            </>
          }
        >
          <div className={adminPage.formGrid}>
            <Field label="策略代码">
              <input
                value={strategyDraft.code}
                onChange={(e) =>
                  setStrategyDraft({ ...strategyDraft, code: e.target.value })
                }
              />
            </Field>
            <Field label="策略名称">
              <input
                value={strategyDraft.name}
                onChange={(e) =>
                  setStrategyDraft({ ...strategyDraft, name: e.target.value })
                }
              />
            </Field>
            <Field label="每积分最低净收入（CNY）">
              <input
                value={strategyDraft.minimumIncome}
                onChange={(e) =>
                  setStrategyDraft({
                    ...strategyDraft,
                    minimumIncome: e.target.value,
                  })
                }
              />
            </Field>
            <Field label="每积分标称价值（CNY）">
              <input
                value={strategyDraft.grossPointValue}
                onChange={(e) =>
                  setStrategyDraft({
                    ...strategyDraft,
                    grossPointValue: e.target.value,
                  })
                }
              />
            </Field>
            <Field label="赠送积分最大占比">
              <input
                value={strategyDraft.maxBonusRatio}
                onChange={(e) =>
                  setStrategyDraft({
                    ...strategyDraft,
                    maxBonusRatio: e.target.value,
                  })
                }
              />
            </Field>
            <Field label="支付费率">
              <input
                value={strategyDraft.paymentFee}
                onChange={(e) =>
                  setStrategyDraft({
                    ...strategyDraft,
                    paymentFee: e.target.value,
                  })
                }
              />
            </Field>
            <Field label="目标毛利率">
              <input
                value={strategyDraft.targetMargin}
                onChange={(e) =>
                  setStrategyDraft({
                    ...strategyDraft,
                    targetMargin: e.target.value,
                  })
                }
              />
            </Field>
            <Field label="Provider 成本缓冲">
              <input
                value={strategyDraft.costBuffer}
                onChange={(e) =>
                  setStrategyDraft({
                    ...strategyDraft,
                    costBuffer: e.target.value,
                  })
                }
              />
            </Field>
            <Field label="平台固定成本">
              <input
                value={strategyDraft.fixedCost}
                onChange={(e) =>
                  setStrategyDraft({
                    ...strategyDraft,
                    fixedCost: e.target.value,
                  })
                }
              />
            </Field>
            <Field label="每秒平台成本">
              <input
                value={strategyDraft.outputSecondCost}
                onChange={(e) =>
                  setStrategyDraft({
                    ...strategyDraft,
                    outputSecondCost: e.target.value,
                  })
                }
              />
            </Field>
            <Field label="每张参考素材成本">
              <input
                value={strategyDraft.referenceCost}
                onChange={(e) =>
                  setStrategyDraft({
                    ...strategyDraft,
                    referenceCost: e.target.value,
                  })
                }
              />
            </Field>
            <Field label="音频固定附加成本">
              <input
                value={strategyDraft.audioFixedCost}
                onChange={(e) => setStrategyDraft({ ...strategyDraft, audioFixedCost: e.target.value })}
              />
            </Field>
            <Field label="音频每秒附加成本">
              <input
                value={strategyDraft.audioSecondCost}
                onChange={(e) => setStrategyDraft({ ...strategyDraft, audioSecondCost: e.target.value })}
              />
            </Field>
            <Field label="精确结算预留倍率">
              <input
                value={strategyDraft.exactReserveMarkup}
                onChange={(e) => setStrategyDraft({ ...strategyDraft, exactReserveMarkup: e.target.value })}
              />
            </Field>
            <Field label="按量结算预留倍率">
              <input
                value={strategyDraft.meteredReserveMarkup}
                onChange={(e) => setStrategyDraft({ ...strategyDraft, meteredReserveMarkup: e.target.value })}
              />
            </Field>
            <Field label="状态">
              <select
                value={strategyDraft.enabled ? "enabled" : "disabled"}
                onChange={(e) =>
                  setStrategyDraft({
                    ...strategyDraft,
                    enabled: e.target.value === "enabled",
                  })
                }
              >
                <option value="enabled">启用</option>
                <option value="disabled">停用</option>
              </select>
            </Field>
          </div>
        </Modal>
      ) : null}
      {ruleDraft ? (
        <Modal
          title={ruleDraft.row ? "编辑视频参数价格" : "新增视频参数价格"}
          detail="启用前必须完成成本试算且不低于服务端安全线。"
          onClose={() => {
            setRuleDraft(null);
            setSimulation(null);
          }}
          footer={
            <>
              <button
                className={cn(adminButton.base, adminButton.ghost)}
                onClick={() => void simulateRule(ruleDraft)}
              >
                试算安全线
              </button>
              <button
                className={cn(adminButton.base, adminButton.primary)}
                disabled={busy || !ruleDraft.routeID || !ruleDraft.strategyID}
                onClick={() => void saveRule()}
              >
                保存参数价格
              </button>
            </>
          }
        >
          <div className={adminPage.formGrid}>
            <Field label="视频路由">
              <select
                value={ruleDraft.routeID}
                onChange={(e) =>
                  setRuleDraft({ ...ruleDraft, routeID: e.target.value })
                }
              >
                {routes.map((route) => (
                  <option key={String(route.id)} value={String(route.id)}>
                    {route.name}
                  </option>
                ))}
              </select>
            </Field>
            <Field label="价格策略">
              <select
                value={ruleDraft.strategyID}
                onChange={(e) =>
                  setRuleDraft({ ...ruleDraft, strategyID: e.target.value })
                }
              >
                {snapshot?.pricing_strategies.map((item) => (
                  <option key={item.id} value={item.id}>
                    {item.name}
                  </option>
                ))}
              </select>
            </Field>
            <Field label="任务类型">
              <select
                value={ruleDraft.taskType}
                onChange={(e) =>
                  setRuleDraft({ ...ruleDraft, taskType: e.target.value })
                }
              >
                {taskOptions()}
              </select>
            </Field>
            <Field label="分辨率">
              <input
                value={ruleDraft.resolution}
                onChange={(e) =>
                  setRuleDraft({ ...ruleDraft, resolution: e.target.value })
                }
              />
            </Field>
            <Field label="音频模式">
              <select
                value={ruleDraft.audioMode}
                onChange={(e) =>
                  setRuleDraft({ ...ruleDraft, audioMode: e.target.value })
                }
              >
                <option value="silent">静音</option>
                <option value="generated">生成音频</option>
              </select>
            </Field>
            <Field label="试算时长（秒）">
              <input
                type="number"
                min="1"
                value={ruleDraft.duration}
                onChange={(e) =>
                  setRuleDraft({
                    ...ruleDraft,
                    duration: Number(e.target.value),
                  })
                }
              />
            </Field>
            <Field label="计费模式">
              <select
                value={ruleDraft.pricingMode}
                onChange={(e) =>
                  setRuleDraft({
                    ...ruleDraft,
                    pricingMode: e.target.value as RuleDraft["pricingMode"],
                  })
                }
              >
                <option value="exact">按请求精确结算</option>
                <option value="metered">按实际输出时长结算</option>
              </select>
            </Field>
            <Field label="固定任务积分">
              <input
                value={ruleDraft.fixedPoints}
                onChange={(e) =>
                  setRuleDraft({ ...ruleDraft, fixedPoints: e.target.value })
                }
              />
            </Field>
            <Field label="每输出秒积分">
              <input
                value={ruleDraft.outputSecondPoints}
                onChange={(e) =>
                  setRuleDraft({
                    ...ruleDraft,
                    outputSecondPoints: e.target.value,
                  })
                }
              />
            </Field>
            <Field label="每张参考图积分">
              <input
                value={ruleDraft.referencePoints}
                onChange={(e) =>
                  setRuleDraft({
                    ...ruleDraft,
                    referencePoints: e.target.value,
                  })
                }
              />
            </Field>
            <Field label="输入视频每秒积分">
              <input
                value={ruleDraft.inputVideoSecondPoints}
                onChange={(e) =>
                  setRuleDraft({
                    ...ruleDraft,
                    inputVideoSecondPoints: e.target.value,
                  })
                }
              />
            </Field>
            <Field label="参考音频每秒积分">
              <input
                value={ruleDraft.referenceAudioSecondPoints}
                onChange={(e) =>
                  setRuleDraft({
                    ...ruleDraft,
                    referenceAudioSecondPoints: e.target.value,
                  })
                }
              />
            </Field>
            <Field label="生成音频固定积分">
              <input
                value={ruleDraft.generatedAudioFixedPoints}
                onChange={(e) =>
                  setRuleDraft({
                    ...ruleDraft,
                    generatedAudioFixedPoints: e.target.value,
                  })
                }
              />
            </Field>
            <Field label="生成音频每秒积分">
              <input
                value={ruleDraft.generatedAudioSecondPoints}
                onChange={(e) =>
                  setRuleDraft({
                    ...ruleDraft,
                    generatedAudioSecondPoints: e.target.value,
                  })
                }
              />
            </Field>
            <Field label="最低任务积分">
              <input
                value={ruleDraft.minimumPoints}
                onChange={(e) =>
                  setRuleDraft({ ...ruleDraft, minimumPoints: e.target.value })
                }
              />
            </Field>
            <Field label="最低计费时长（秒）">
              <input
                type="number"
                min="1"
                value={ruleDraft.minimumBillableSeconds}
                onChange={(e) =>
                  setRuleDraft({
                    ...ruleDraft,
                    minimumBillableSeconds: Math.max(1, Number(e.target.value)),
                  })
                }
              />
            </Field>
            <Field label="预留倍率">
              <input
                value={ruleDraft.reserveMarkup}
                onChange={(e) =>
                  setRuleDraft({ ...ruleDraft, reserveMarkup: e.target.value })
                }
              />
            </Field>
            <Field label="状态">
              <select
                value={ruleDraft.enabled ? "enabled" : "disabled"}
                onChange={(e) =>
                  setRuleDraft({
                    ...ruleDraft,
                    enabled: e.target.value === "enabled",
                  })
                }
              >
                <option value="enabled">启用</option>
                <option value="disabled">停用</option>
              </select>
            </Field>
            {simulation ? (
              <InlineFeedback
                tone="warning"
                message={`Provider 最坏成本 ¥${simulation.worst_candidate_cost_cny}，服务端安全线 ◈${simulation.safety_points}`}
              />
            ) : null}
          </div>
        </Modal>
      ) : null}
      {bindingDraft ? (
        <Modal
          title="新增路由参数策略绑定"
          detail="完整参数命中后覆盖路由默认价格策略。"
          onClose={() => setBindingDraft(null)}
          footer={
            <>
              <button
                className={cn(adminButton.base, adminButton.ghost)}
                onClick={() => setBindingDraft(null)}
              >
                取消
              </button>
              <button
                className={cn(adminButton.base, adminButton.primary)}
                disabled={
                  busy || !bindingDraft.routeID || !bindingDraft.strategyID
                }
                onClick={() => void saveBinding()}
              >
                保存绑定
              </button>
            </>
          }
        >
          <div className={adminPage.formGrid}>
            <Field label="视频路由">
              <select
                value={bindingDraft.routeID}
                onChange={(e) =>
                  setBindingDraft({ ...bindingDraft, routeID: e.target.value })
                }
              >
                {snapshot?.routes.map((route) => (
                  <option
                    key={route.route_model_id}
                    value={route.route_model_id}
                  >
                    {route.route_name || route.route_code}
                  </option>
                ))}
              </select>
            </Field>
            <Field label="任务类型">
              <select
                value={bindingDraft.taskType}
                onChange={(e) =>
                  setBindingDraft({ ...bindingDraft, taskType: e.target.value })
                }
              >
                {taskOptions()}
              </select>
            </Field>
            <Field label="分辨率">
              <input
                value={bindingDraft.resolution}
                onChange={(e) =>
                  setBindingDraft({
                    ...bindingDraft,
                    resolution: e.target.value,
                  })
                }
              />
            </Field>
            <Field label="比例">
              <input
                value={bindingDraft.aspectRatio}
                onChange={(e) =>
                  setBindingDraft({
                    ...bindingDraft,
                    aspectRatio: e.target.value,
                  })
                }
              />
            </Field>
            <Field label="音频模式">
              <select
                value={bindingDraft.audioMode}
                onChange={(e) =>
                  setBindingDraft({
                    ...bindingDraft,
                    audioMode: e.target.value,
                  })
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
                value={bindingDraft.duration}
                onChange={(e) =>
                  setBindingDraft({
                    ...bindingDraft,
                    duration: Number(e.target.value),
                  })
                }
              />
            </Field>
            <Field label="使用策略">
              <select
                value={bindingDraft.strategyID}
                onChange={(e) =>
                  setBindingDraft({
                    ...bindingDraft,
                    strategyID: e.target.value,
                  })
                }
              >
                {snapshot?.pricing_strategies.map((strategy) => (
                  <option key={strategy.id} value={strategy.id}>
                    {strategy.name}
                  </option>
                ))}
              </select>
            </Field>
          </div>
        </Modal>
      ) : null}
    </section>
  );

  async function saveRouteWithBindings(
    route: AdminVideoRouteConfig,
    bindings: PricingBinding[],
  ) {
    const combinations: AdminVideoVisibleCombination[] = Array.isArray(
      route.visible_options?.combinations,
    )
      ? (route.visible_options
          .combinations as AdminVideoVisibleCombination[])
      : [];
    return adminApi.saveRouteVideoConfig(route.route_model_id, {
      expected_version: route.config_version,
      config_version: nextVersion(route.config_version),
      pricing_strategy_id: route.pricing_strategy_id,
      task_types: route.task_types,
      visible_options: { ...route.visible_options, pricing_bindings: bindings },
      defaults: route.defaults,
      visible_combinations: combinations,
      max_output_count: route.max_output_count,
      enabled: route.enabled,
    });
  }
}

function blankStrategy(): StrategyDraft {
  return {
    code: "",
    name: "",
    grossPointValue: "0.25260",
    minimumIncome: "0.25260",
    maxBonusRatio: "0.50000",
    paymentFee: "0.03000",
    targetMargin: "0.25000",
    costBuffer: "0.10000",
    fixedCost: "0",
    outputSecondCost: "0",
    referenceCost: "0",
    audioFixedCost: "0",
    audioSecondCost: "0",
    exactReserveMarkup: "1",
    meteredReserveMarkup: "1.1",
    enabled: false,
  };
}
function editStrategy(row: AdminVideoPricingStrategy): StrategyDraft {
  return {
    row,
    code: row.code,
    name: row.name,
    grossPointValue:
      row.gross_point_value_cny || row.minimum_net_point_income_cny,
    minimumIncome: row.minimum_net_point_income_cny,
    maxBonusRatio: row.max_bonus_ratio || "0.50000",
    paymentFee: row.payment_fee_rate,
    targetMargin: row.target_margin_rate,
    costBuffer: row.provider_cost_buffer_rate,
    fixedCost: row.platform_fixed_cost_cny || "0",
    outputSecondCost: row.platform_output_second_cost_cny || "0",
    referenceCost: row.platform_reference_cost_cny || "0",
    audioFixedCost: row.platform_audio_fixed_cost_cny || "0",
    audioSecondCost: row.platform_audio_second_cost_cny || "0",
    exactReserveMarkup: row.exact_reserve_markup || "1",
    meteredReserveMarkup: row.metered_reserve_markup || "1.1",
    enabled: row.enabled,
  };
}
function blankRule(
  routes: RouteModel[],
  strategy: AdminVideoPricingStrategy,
): RuleDraft {
  return {
    routeID: String(routes[0]?.id ?? ""),
    strategyID: String(strategy.id),
    taskType: "text_to_video",
    resolution: "720p",
    audioMode: "silent",
    duration: 5,
    pricingMode: "exact",
    fixedPoints: "0",
    outputSecondPoints: "0",
    referencePoints: "0",
    inputVideoSecondPoints: "0",
    referenceAudioSecondPoints: "0",
    generatedAudioFixedPoints: "0",
    generatedAudioSecondPoints: "0",
    minimumBillableSeconds: 1,
    minimumPoints: "10.00000",
    reserveMarkup: "1",
    enabled: false,
  };
}
function editRule(
  row: AdminVideoConfiguration["price_rules"][number],
  routes: RouteModel[],
): RuleDraft {
  return {
    ...blankRule(routes, {
      id: row.pricing_strategy_id,
    } as AdminVideoPricingStrategy),
    row,
    strategyID: String(row.pricing_strategy_id),
    taskType: row.task_type,
    resolution: row.resolution,
    audioMode: row.audio_mode,
    pricingMode: row.pricing_mode,
    fixedPoints: row.fixed_task_points,
    outputSecondPoints: row.output_second_points,
    referencePoints: row.reference_image_points,
    inputVideoSecondPoints: row.input_video_second_points,
    referenceAudioSecondPoints: row.reference_audio_second_points,
    generatedAudioFixedPoints: row.generated_audio_fixed_points,
    generatedAudioSecondPoints: row.generated_audio_second_points,
    minimumBillableSeconds: row.minimum_billable_seconds,
    minimumPoints: row.minimum_task_points,
    reserveMarkup: row.reserve_markup,
    effectiveAt: row.effective_at,
    expiresAt: row.expires_at,
    enabled: row.enabled,
  };
}
function blankBinding(snapshot: AdminVideoConfiguration): BindingDraft {
  return {
    routeID: String(snapshot.routes[0]?.route_model_id ?? ""),
    taskType: "text_to_video",
    resolution: "720p",
    aspectRatio: "16:9",
    audioMode: "silent",
    duration: 5,
    strategyID: String(snapshot.pricing_strategies[0]?.id ?? ""),
  };
}
function routeBindings(route: AdminVideoRouteConfig): PricingBinding[] {
  return Array.isArray(route.visible_options?.pricing_bindings)
    ? (route.visible_options.pricing_bindings as PricingBinding[])
    : [];
}
function bindingKey(binding: PricingBinding) {
  return `${binding.task_type}/${binding.resolution}/${binding.aspect_ratio || ""}/${binding.audio_mode}/${binding.duration_seconds || 0}`;
}
function nextVersion(value: string) {
  const match = value.match(/(\d+)$/);
  return `${value.replace(/\d+$/, "") || "video-route-v"}${Number(match?.[1] ?? 0) + 1}`;
}
function taskOptions() {
  return (
    <>
      <option value="text_to_video">文生视频</option>
      <option value="image_to_video">图生视频</option>
      <option value="first_last_frame_to_video">首尾帧生视频</option>
    </>
  );
}
function videoTaskLabel(value: string) {
  return value === "text_to_video"
    ? "文生视频"
    : value === "image_to_video"
      ? "图生视频"
      : value === "first_last_frame_to_video"
        ? "首尾帧生视频"
        : value;
}
function percent(value: string) {
  const parsed = Number(value);
  return Number.isFinite(parsed) ? `${(parsed * 100).toFixed(0)}%` : value;
}
function errorText(value: unknown) {
  return value instanceof Error ? value.message : "视频价格策略操作失败";
}
