import { useEffect, useMemo, useState } from "react";
import { Pencil, Plus, Trash2 } from "lucide-react";
import type {
  AdminVideoConfiguration,
  ModelAccount,
  ModelAccountModel,
} from "../../../shared/api-types";
import { adminApi } from "../../../shared/admin-api";
import { cn } from "../../../shared/classnames";
import {
  Badge,
  EmptyBlock,
  Field,
  InlineFeedback,
  LoadingBlock,
  Modal,
  PageHeader,
  RefreshIconButton,
  TooltipIconButton,
} from "../components";
import { adminButton, adminPage } from "../ui/classes";
import { modelAccountMediaType } from "./adminModelMedia";

const videoTasks = [
  { value: "text_to_video", label: "文生视频" },
  { value: "image_to_video", label: "图生视频" },
  { value: "first_last_frame_to_video", label: "首尾帧生视频" },
] as const;

type AccountDraft = {
  row?: ModelAccount;
  name: string;
  adapter: "seedance" | "minimax";
  baseURL: string;
  apiKey: string;
  status: string;
  priority: number;
  weight: number;
  concurrency: number;
  timeout: number;
};
type ModelDraft = {
  account: ModelAccount;
  row?: ModelAccountModel;
  modelCode: string;
  displayName: string;
  taskTypes: string[];
  durations: string;
  resolutions: string;
  ratios: string;
  audioModes: string[];
  providerMaxN: number;
  promptMaxRunes: number;
  inputFormats: string;
  inputMaxMB: number;
  rateRows: Record<string, { primary: string; secondary: string }>;
  freeImageCount: number;
  extraImageCNY: string;
  validationStatus: "untested" | "verified";
  enabled: boolean;
};

export function VideoProviderAccountsPanel() {
  const [accounts, setAccounts] = useState<ModelAccount[]>([]);
  const [models, setModels] = useState<Record<string, ModelAccountModel[]>>({});
  const [snapshot, setSnapshot] = useState<AdminVideoConfiguration | null>(
    null,
  );
  const [selectedID, setSelectedID] = useState("");
  const [accountDraft, setAccountDraft] = useState<AccountDraft | null>(null);
  const [modelDraft, setModelDraft] = useState<ModelDraft | null>(null);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  async function load(preferredID?: string) {
    setLoading(true);
    setError("");
    try {
      const [allAccounts, nextSnapshot] = await Promise.all([
        adminApi.listModelAccounts({ page_size: 100 }),
        adminApi.getVideoConfiguration(),
      ]);
      const nextAccounts = allAccounts.filter(
        (account) => modelAccountMediaType(account) === "video",
      );
      const entries = await Promise.all(
        nextAccounts.map(
          async (account) =>
            [
              String(account.id),
              await adminApi.listModelAccountModels(account.id),
            ] as const,
        ),
      );
      setAccounts(nextAccounts);
      setModels(Object.fromEntries(entries));
      setSnapshot(nextSnapshot);
      setSelectedID((current) => {
        const wanted = preferredID || current;
        return nextAccounts.some((item) => String(item.id) === wanted)
          ? wanted
          : String(nextAccounts[0]?.id ?? "");
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
  const selected = useMemo(
    () => accounts.find((item) => String(item.id) === selectedID),
    [accounts, selectedID],
  );
  const selectedModels = selected ? (models[String(selected.id)] ?? []) : [];

  async function saveAccount() {
    if (!accountDraft) return;
    setBusy(true);
    setError("");
    try {
      const input = {
        name: accountDraft.name,
        adapter_type: accountDraft.adapter,
        auth_type: "api_key",
        base_url: accountDraft.baseURL,
        credentials: accountDraft.apiKey
          ? { api_key: accountDraft.apiKey }
          : undefined,
        status: accountDraft.status,
        priority: accountDraft.priority,
        weight: accountDraft.weight,
        concurrency_limit: accountDraft.concurrency,
        timeout_ms: accountDraft.timeout,
        extra: { media_type: "video" },
      };
      const saved = accountDraft.row
        ? await adminApi.updateModelAccount(accountDraft.row.id, input)
        : await adminApi.createModelAccount(input);
      setAccountDraft(null);
      await load(String(saved.id));
    } catch (caught) {
      setError(errorText(caught));
    } finally {
      setBusy(false);
    }
  }

  async function saveModel() {
    if (!modelDraft) return;
    setBusy(true);
    setError("");
    try {
      const baseInput = {
        model_code: modelDraft.modelCode,
        display_name: modelDraft.displayName,
        task_types: modelDraft.taskTypes,
        base_resolution: [],
        quality: [],
        max_reference_image_count: modelDraft.taskTypes.includes(
          "first_last_frame_to_video",
        )
          ? 2
          : modelDraft.taskTypes.includes("image_to_video")
            ? 1
            : 0,
        max_image_count: modelDraft.providerMaxN,
        size_modes: [],
        supported_ratios: [],
        supported_pixel_sizes: [],
        supports_custom_ratio: false,
        supported_backgrounds: [],
        min_width: 0,
        max_width: 0,
        min_height: 0,
        max_height: 0,
        output_format: ["mp4"],
        supports_output_compression: false,
        supports_custom_size: false,
        moderation: [],
        cost_per_image: "0.00000",
        currency: "CNY",
        enabled: modelDraft.enabled,
        extra: { media_type: "video" },
      };
      const saved = modelDraft.row
        ? await adminApi.updateModelAccountModel(
            modelDraft.account.id,
            modelDraft.row.id,
            baseInput,
          )
        : await adminApi.createModelAccountModel(
            modelDraft.account.id,
            baseInput,
          );
      const taskTypes = Object.fromEntries(
        modelDraft.taskTypes.map((task) => [
          task,
          {
            durations: { values: numberList(modelDraft.durations) },
            resolutions: stringList(modelDraft.resolutions),
            aspect_ratios: stringList(modelDraft.ratios),
            audio_modes: modelDraft.audioModes,
            inputs: videoTaskInputs(task, modelDraft),
          },
        ]),
      );
      const existingCapability = snapshot?.capabilities.find(
        (item) => String(item.account_model_id) === String(saved.id),
      );
      await adminApi.saveVideoCapability(saved.id, {
        expected_version: existingCapability?.capability_version ?? "",
        capability_version: nextVersion(
          "video-cap",
          existingCapability?.capability_version,
        ),
        capability: {
          schema_version: 1,
          provider_native_max_n: modelDraft.providerMaxN,
          prompt_max_runes: modelDraft.promptMaxRunes,
          task_types: taskTypes,
        },
        validation_status: modelDraft.validationStatus,
        enabled: modelDraft.enabled,
      });
      const existingRate = snapshot?.rate_cards
        .filter((item) => String(item.account_model_id) === String(saved.id))
        .sort((a, b) => b.rate_version - a.rate_version)[0];
      const resolutions = stringList(modelDraft.resolutions);
      if (modelDraft.account.adapter_type === "minimax") {
        await adminApi.saveVideoRateCard(saved.id, {
          pricing_schema: "minimax_h3_second_v1",
          expected_rate_version: existingRate?.rate_version ?? 0, enabled: modelDraft.enabled,
          rate_config: {
            resolutions: Object.fromEntries(resolutions.map((resolution) => [resolution, {
              output_second_cny: modelDraft.rateRows[resolution]?.primary || "0",
              input_video_second_cny: modelDraft.rateRows[resolution]?.secondary || modelDraft.rateRows[resolution]?.primary || "0",
            }])),
            free_image_count: modelDraft.freeImageCount, extra_image_cny: modelDraft.extraImageCNY, input_audio_free: true,
          },
        });
      } else {
        await adminApi.saveVideoRateCard(saved.id, {
          pricing_schema: "seedance_token_v1",
          expected_rate_version: existingRate?.rate_version ?? 0, enabled: modelDraft.enabled,
          rate_config: { resolutions: Object.fromEntries(resolutions.map((resolution) => [resolution, {
            without_input_video_million_tokens_cny: modelDraft.rateRows[resolution]?.primary || "0",
            with_input_video_million_tokens_cny: modelDraft.rateRows[resolution]?.secondary || undefined,
          }])) },
        });
      }
      setModelDraft(null);
      await load(String(modelDraft.account.id));
    } catch (caught) {
      setError(errorText(caught));
    } finally {
      setBusy(false);
    }
  }

  async function removeAccount(account: ModelAccount) {
    if (!window.confirm(`删除视频账号“${account.name}”？`)) return;
    setBusy(true);
    try {
      await adminApi.deleteModelAccount(account.id);
      await load();
    } catch (caught) {
      setError(errorText(caught));
    } finally {
      setBusy(false);
    }
  }
  async function removeModel(account: ModelAccount, model: ModelAccountModel) {
    if (
      !window.confirm(
        `删除真实模型“${model.display_name || model.model_code}”？`,
      )
    )
      return;
    setBusy(true);
    try {
      await adminApi.deleteModelAccountModel(account.id, model.id);
      await load(String(account.id));
    } catch (caught) {
      setError(errorText(caught));
    } finally {
      setBusy(false);
    }
  }

  if (loading && !accounts.length)
    return <LoadingBlock label="载入视频接入账号" />;
  return (
    <section className={adminPage.stack} data-video-provider-accounts>
      <PageHeader
        title="视频接入账号"
        description="一个账号可配置多个真实视频模型；能力与厂商原生销售费率按真实模型独立维护。"
        primaryAction={
          <button
            type="button"
            className={cn(adminButton.base, adminButton.primary)}
            onClick={() => setAccountDraft(blankAccount())}
          >
            <Plus size={16} />
            新增账号
          </button>
        }
        secondaryActions={
          <RefreshIconButton
            label="刷新视频接入账号"
            refreshing={loading}
            onClick={() => void load()}
          />
        }
      />
      {error ? <InlineFeedback tone="danger" message={error} /> : null}
      {!accounts.length ? (
        <EmptyBlock
          title="暂无视频账号"
          detail="新增 Seedance 或 MiniMax 账号后，再添加真实模型。"
        />
      ) : (
        <div className="grid min-w-0 overflow-hidden rounded-lg border border-[var(--border)] bg-[var(--surface-solid)] lg:grid-cols-[280px_minmax(0,1fr)]">
          <aside className="border-b border-[var(--border)] p-2 lg:border-b-0 lg:border-r">
            {accounts.map((account) => (
              <button
                key={String(account.id)}
                type="button"
                className={cn(
                  "mb-1 flex min-h-14 w-full items-center justify-between rounded-md px-3 text-left",
                  selectedID === String(account.id)
                    ? "bg-[var(--elevated)]"
                    : "hover:bg-[var(--surface)]",
                )}
                onClick={() => setSelectedID(String(account.id))}
              >
                <span className="min-w-0">
                  <strong className="block truncate text-sm">
                    {account.name}
                  </strong>
                  <small className="text-[var(--muted)]">
                    {account.adapter_type === "seedance"
                      ? "Seedance"
                      : "MiniMax"}{" "}
                    · {(models[String(account.id)] ?? []).length} 个模型
                  </small>
                </span>
                <Badge
                  tone={account.status === "enabled" ? "success" : "warning"}
                >
                  {account.status === "enabled" ? "启用" : "停用"}
                </Badge>
              </button>
            ))}
          </aside>
          <section className="min-w-0 p-4">
            {selected ? (
              <>
                <header className="mb-4 flex flex-wrap items-start justify-between gap-3 border-b border-[var(--border)] pb-4">
                  <div>
                    <h2 className="m-0 text-base font-semibold">
                      {selected.name}
                    </h2>
                    <p className="m-0 mt-1 text-xs text-[var(--muted)]">
                      {selected.base_url}
                    </p>
                  </div>
                  <div className="flex gap-2">
                    <TooltipIconButton
                      label="编辑视频账号"
                      onClick={() => setAccountDraft(editAccount(selected))}
                    >
                      <Pencil />
                    </TooltipIconButton>
                    <TooltipIconButton
                      label="删除视频账号"
                      disabled={busy || selectedModels.length > 0}
                      disabledReason="请先删除账号下的真实模型"
                      onClick={() => void removeAccount(selected)}
                    >
                      <Trash2 />
                    </TooltipIconButton>
                    <button
                      type="button"
                      className={cn(
                        adminButton.base,
                        adminButton.primary,
                        adminButton.small,
                      )}
                      onClick={() => setModelDraft(blankModel(selected))}
                    >
                      <Plus size={15} />
                      新增真实模型
                    </button>
                  </div>
                </header>
                {!selectedModels.length ? (
                  <EmptyBlock
                    title="暂无真实视频模型"
                    detail="为当前账号添加 Seedance 2.5/2.0 或 MiniMax H3 等真实模型。"
                  />
                ) : (
                  <div className="overflow-x-auto">
                    <table className="admin-table min-w-[760px]">
                      <thead>
                        <tr>
                          <th>真实模型</th>
                          <th>任务类型</th>
                          <th>能力版本</th>
                          <th>销售费率版本</th>
                          <th>状态</th>
                          <th>操作</th>
                        </tr>
                      </thead>
                      <tbody>
                        {selectedModels.map((model) => {
                          const capability = snapshot?.capabilities.find(
                            (item) =>
                              String(item.account_model_id) ===
                              String(model.id),
                          );
                          const rate = snapshot?.rate_cards
                            .filter(
                              (item) =>
                                String(item.account_model_id) ===
                                String(model.id),
                            )
                            .sort((a, b) => b.rate_version - a.rate_version)[0];
                          return (
                            <tr key={String(model.id)}>
                              <td>
                                <strong>
                                  {model.display_name || model.model_code}
                                </strong>
                                <code className="mt-1 block text-xs text-[var(--muted)]">
                                  {model.model_code}
                                </code>
                              </td>
                              <td>
                                {model.task_types
                                  .map(videoTaskLabel)
                                  .join(" / ")}
                              </td>
                              <td>
                                {capability?.capability_version || "未配置"}
                              </td>
                              <td>
                                {rate
                                  ? `v${rate.rate_version} · ${rate.currency}`
                                  : "未配置"}
                              </td>
                              <td>
                                <Badge
                                  tone={
                                    model.enabled &&
                                    capability?.enabled &&
                                    rate?.enabled
                                      ? "success"
                                      : "warning"
                                  }
                                >
                                  {model.enabled &&
                                  capability?.enabled &&
                                  rate?.enabled
                                    ? "可用"
                                    : "待完善"}
                                </Badge>
                              </td>
                              <td>
                                <div className="flex justify-end gap-1">
                                  <TooltipIconButton
                                    label="编辑真实视频模型"
                                    onClick={() =>
                                      setModelDraft(
                                        editModel(selected, model, snapshot),
                                      )
                                    }
                                  >
                                    <Pencil />
                                  </TooltipIconButton>
                                  <TooltipIconButton
                                    label="删除真实视频模型"
                                    onClick={() =>
                                      void removeModel(selected, model)
                                    }
                                  >
                                    <Trash2 />
                                  </TooltipIconButton>
                                </div>
                              </td>
                            </tr>
                          );
                        })}
                      </tbody>
                    </table>
                  </div>
                )}
              </>
            ) : null}
          </section>
        </div>
      )}
      {accountDraft ? (
        <Modal
          title={accountDraft.row ? "编辑视频账号" : "新增视频账号"}
          detail="账号凭据只用于服务端调用，不会返回前端。"
          onClose={() => setAccountDraft(null)}
          footer={
            <>
              <button
                className={cn(adminButton.base, adminButton.ghost)}
                onClick={() => setAccountDraft(null)}
              >
                取消
              </button>
              <button
                className={cn(adminButton.base, adminButton.primary)}
                disabled={busy || !accountDraft.name || !accountDraft.baseURL}
                onClick={() => void saveAccount()}
              >
                保存账号
              </button>
            </>
          }
        >
          <div className={adminPage.formGrid}>
            <Field label="账号名称">
              <input
                value={accountDraft.name}
                onChange={(e) =>
                  setAccountDraft({ ...accountDraft, name: e.target.value })
                }
              />
            </Field>
            <Field label="厂商">
              <select
                value={accountDraft.adapter}
                onChange={(e) =>
                  setAccountDraft({
                    ...accountDraft,
                    adapter: e.target.value as AccountDraft["adapter"],
                  })
                }
              >
                <option value="seedance">Seedance</option>
                <option value="minimax">MiniMax</option>
              </select>
            </Field>
            <Field label="Base URL">
              <input
                value={accountDraft.baseURL}
                onChange={(e) =>
                  setAccountDraft({ ...accountDraft, baseURL: e.target.value })
                }
              />
            </Field>
            <Field label="API Key">
              <input
                type="password"
                value={accountDraft.apiKey}
                placeholder={accountDraft.row ? "留空保持原密钥" : ""}
                onChange={(e) =>
                  setAccountDraft({ ...accountDraft, apiKey: e.target.value })
                }
              />
            </Field>
            <Field label="优先级">
              <input
                type="number"
                min="1"
                value={accountDraft.priority}
                onChange={(e) =>
                  setAccountDraft({
                    ...accountDraft,
                    priority: Number(e.target.value),
                  })
                }
              />
            </Field>
            <Field label="权重">
              <input
                type="number"
                min="0"
                value={accountDraft.weight}
                onChange={(e) =>
                  setAccountDraft({
                    ...accountDraft,
                    weight: Number(e.target.value),
                  })
                }
              />
            </Field>
            <Field label="并发限制">
              <input
                type="number"
                min="1"
                value={accountDraft.concurrency}
                onChange={(e) =>
                  setAccountDraft({
                    ...accountDraft,
                    concurrency: Number(e.target.value),
                  })
                }
              />
            </Field>
            <Field label="超时毫秒">
              <input
                type="number"
                min="1000"
                value={accountDraft.timeout}
                onChange={(e) =>
                  setAccountDraft({
                    ...accountDraft,
                    timeout: Number(e.target.value),
                  })
                }
              />
            </Field>
            <Field label="状态">
              <select
                value={accountDraft.status}
                onChange={(e) =>
                  setAccountDraft({ ...accountDraft, status: e.target.value })
                }
              >
                <option value="enabled">启用</option>
                <option value="disabled">停用</option>
              </select>
            </Field>
          </div>
        </Modal>
      ) : null}
      {modelDraft ? (
        <Modal
          title={modelDraft.row ? "编辑真实视频模型" : "新增真实视频模型"}
          detail={`${modelDraft.account.name} · 能力和销售费率将同步保存`}
          onClose={() => setModelDraft(null)}
          footer={
            <>
              <button
                className={cn(adminButton.base, adminButton.ghost)}
                onClick={() => setModelDraft(null)}
              >
                取消
              </button>
              <button
                className={cn(adminButton.base, adminButton.primary)}
                disabled={
                  busy ||
                  !modelDraft.modelCode ||
                  !modelDraft.taskTypes.length ||
                  (modelDraft.enabled && modelDraft.validationStatus !== "verified")
                }
                onClick={() => void saveModel()}
              >
                保存模型、能力和费率
              </button>
            </>
          }
        >
          <div className={adminPage.formGrid}>
            <Field label="模型代码">
              <input
                value={modelDraft.modelCode}
                onChange={(e) =>
                  setModelDraft({ ...modelDraft, modelCode: e.target.value })
                }
              />
            </Field>
            <Field label="展示名称">
              <input
                value={modelDraft.displayName}
                onChange={(e) =>
                  setModelDraft({ ...modelDraft, displayName: e.target.value })
                }
              />
            </Field>
            <Field label="任务类型">
              <div className="grid gap-2">
                {videoTasks.map((option) => (
                  <label key={option.value} className="flex items-center gap-2">
                    <input
                      type="checkbox"
                      checked={modelDraft.taskTypes.includes(option.value)}
                      onChange={(e) =>
                        setModelDraft({
                          ...modelDraft,
                          taskTypes: e.target.checked
                            ? [...modelDraft.taskTypes, option.value]
                            : modelDraft.taskTypes.filter(
                                (item) => item !== option.value,
                              ),
                        })
                      }
                    />
                    {option.label}
                  </label>
                ))}
              </div>
            </Field>
            <Field label="Provider 原生最大 n">
              <input
                type="number"
                min="1"
                max="10"
                value={modelDraft.providerMaxN}
                onChange={(e) =>
                  setModelDraft({
                    ...modelDraft,
                    providerMaxN: Math.min(
                      10,
                      Math.max(1, Number(e.target.value)),
                    ),
                  })
                }
              />
            </Field>
            <Field label="提示词最大字符数">
              <input
                type="number"
                min="0"
                value={modelDraft.promptMaxRunes}
                onChange={(e) =>
                  setModelDraft({
                    ...modelDraft,
                    promptMaxRunes: Math.max(0, Number(e.target.value)),
                  })
                }
              />
            </Field>
            <Field label="支持时长（秒）">
              <input
                value={modelDraft.durations}
                placeholder="5,10"
                onChange={(e) =>
                  setModelDraft({ ...modelDraft, durations: e.target.value })
                }
              />
            </Field>
            <Field label="支持分辨率">
              <input
                value={modelDraft.resolutions}
                placeholder="720p,1080p"
                onChange={(e) =>
                  setModelDraft({ ...modelDraft, resolutions: e.target.value })
                }
              />
            </Field>
            <Field label="支持比例">
              <input
                value={modelDraft.ratios}
                placeholder="16:9,9:16"
                onChange={(e) =>
                  setModelDraft({ ...modelDraft, ratios: e.target.value })
                }
              />
            </Field>
            <Field label="音频模式">
              <div className="flex gap-4">
                {["silent", "generated"].map((value) => (
                  <label key={value} className="flex items-center gap-2">
                    <input
                      type="checkbox"
                      checked={modelDraft.audioModes.includes(value)}
                      onChange={(e) =>
                        setModelDraft({
                          ...modelDraft,
                          audioModes: e.target.checked
                            ? [...modelDraft.audioModes, value]
                            : modelDraft.audioModes.filter(
                                (item) => item !== value,
                              ),
                        })
                      }
                    />
                    {value === "silent" ? "静音" : "生成音频"}
                  </label>
                ))}
              </div>
            </Field>
            <Field label="输入图片格式">
              <input
                value={modelDraft.inputFormats}
                placeholder="jpg,jpeg,png,webp"
                onChange={(e) =>
                  setModelDraft({ ...modelDraft, inputFormats: e.target.value })
                }
              />
            </Field>
            <Field label="单张输入上限（MB）">
              <input
                type="number"
                min="1"
                value={modelDraft.inputMaxMB}
                onChange={(e) =>
                  setModelDraft({
                    ...modelDraft,
                    inputMaxMB: Math.max(1, Number(e.target.value)),
                  })
                }
              />
            </Field>
            <div className="grid gap-3 lg:col-span-2">
              <strong className="text-sm">厂商原生销售费率</strong>
              {stringList(modelDraft.resolutions).map((resolution) => (
                <div key={resolution} className="grid gap-3 rounded-md border border-[var(--border)] p-3 md:grid-cols-[100px_1fr_1fr]">
                  <span className="self-center text-sm font-medium">{resolution}</span>
                  <Field label={modelDraft.account.adapter_type === "minimax" ? "输出视频 CNY/秒" : "不含输入视频 CNY/百万 Token"}>
                    <input inputMode="decimal" value={modelDraft.rateRows[resolution]?.primary ?? ""} onChange={(e) => setModelDraft({ ...modelDraft, rateRows: { ...modelDraft.rateRows, [resolution]: { primary: e.target.value, secondary: modelDraft.rateRows[resolution]?.secondary ?? "" } } })} />
                  </Field>
                  {modelDraft.account.adapter_type === "minimax" ? (
                    <Field label="输入视频 CNY/秒">
                      <input inputMode="decimal" value={modelDraft.rateRows[resolution]?.secondary ?? ""} onChange={(e) => setModelDraft({ ...modelDraft, rateRows: { ...modelDraft.rateRows, [resolution]: { primary: modelDraft.rateRows[resolution]?.primary ?? "", secondary: e.target.value } } })} />
                    </Field>
                  ) : (
                    <Field label="包含输入视频 CNY/百万 Token">
                      <input inputMode="decimal" value={modelDraft.rateRows[resolution]?.secondary ?? ""} placeholder="未接入输入视频时可留空" onChange={(e) => setModelDraft({ ...modelDraft, rateRows: { ...modelDraft.rateRows, [resolution]: { primary: modelDraft.rateRows[resolution]?.primary ?? "", secondary: e.target.value } } })} />
                    </Field>
                  )}
                </div>
              ))}
              {modelDraft.account.adapter_type === "minimax" ? (
                <div className="grid gap-3 md:grid-cols-2">
                  <Field label="免费输入图片数"><input type="number" min="0" value={modelDraft.freeImageCount} onChange={(e) => setModelDraft({ ...modelDraft, freeImageCount: Math.max(0, Number(e.target.value)) })} /></Field>
                  <Field label="超额图片 CNY/张"><input inputMode="decimal" value={modelDraft.extraImageCNY} onChange={(e) => setModelDraft({ ...modelDraft, extraImageCNY: e.target.value })} /></Field>
                </div>
              ) : null}
            </div>
            <Field label="能力验证状态">
              <select
                value={modelDraft.validationStatus}
                onChange={(e) =>
                  setModelDraft({
                    ...modelDraft,
                    validationStatus: e.target.value as ModelDraft["validationStatus"],
                    enabled:
                      e.target.value === "verified" ? modelDraft.enabled : false,
                  })
                }
              >
                <option value="untested">待验证</option>
                <option value="verified">已验证</option>
              </select>
            </Field>
            <Field label="状态">
              <select
                value={modelDraft.enabled ? "enabled" : "disabled"}
                disabled={modelDraft.validationStatus !== "verified"}
                onChange={(e) =>
                  setModelDraft({
                    ...modelDraft,
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
    </section>
  );
}

function blankAccount(): AccountDraft {
  return {
    name: "",
    adapter: "seedance",
    baseURL: "",
    apiKey: "",
    status: "enabled",
    priority: 1,
    weight: 100,
    concurrency: 5,
    timeout: 300000,
  };
}
function editAccount(row: ModelAccount): AccountDraft {
  return {
    row,
    name: row.name,
    adapter: row.adapter_type === "minimax" ? "minimax" : "seedance",
    baseURL: row.base_url,
    apiKey: "",
    status: row.status,
    priority: row.priority,
    weight: row.weight,
    concurrency: row.concurrency_limit,
    timeout: row.timeout_ms,
  };
}
function blankModel(account: ModelAccount): ModelDraft {
	const minimax = account.adapter_type === "minimax";
	return {
    account,
    modelCode: "",
    displayName: "",
    taskTypes: ["text_to_video"],
    durations: "5,10",
    resolutions: minimax ? "768p,2k" : "720p",
    ratios: "16:9,9:16,1:1",
    audioModes: ["silent"],
    providerMaxN: 1,
    promptMaxRunes: 2000,
    inputFormats:
      account.adapter_type === "minimax"
        ? "jpg,jpeg,png,webp,heic,heif"
        : "jpg,jpeg,png,webp,bmp,tiff,gif",
    inputMaxMB: 30,
    rateRows: minimax
      ? { "768p": { primary: "0.80000", secondary: "0.80000" }, "2k": { primary: "1.20000", secondary: "1.20000" } }
      : { "720p": { primary: "46.00000", secondary: "" } },
    freeImageCount: 5,
    extraImageCNY: "0.10000",
    validationStatus: "untested",
    enabled: false,
  };
}
function editModel(
  account: ModelAccount,
  row: ModelAccountModel,
  snapshot: AdminVideoConfiguration | null,
): ModelDraft {
  const capabilitySummary = snapshot?.capabilities.find(
    (item) => String(item.account_model_id) === String(row.id),
  );
  const capability = capabilitySummary?.capability as
    | Record<string, any>
    | undefined;
  const task = Object.values(capability?.task_types ?? {})[0] as
    Record<string, any> | undefined;
  const input = Object.values(task?.inputs ?? {})[0] as
    Record<string, any> | undefined;
  const latestRate = snapshot?.rate_cards
    .filter((item) => String(item.account_model_id) === String(row.id))
    .sort((a, b) => b.rate_version - a.rate_version)[0];
  const rateConfig = latestRate?.rate_config;
  const rateRows = Object.fromEntries(Object.entries(rateConfig?.resolutions ?? {}).map(([resolution, value]) => [resolution, {
    primary: "output_second_cny" in value ? value.output_second_cny : value.without_input_video_million_tokens_cny,
    secondary: "input_video_second_cny" in value ? value.input_video_second_cny : value.with_input_video_million_tokens_cny ?? "",
  }]));
  return {
    account,
    row,
    modelCode: row.model_code,
    displayName: row.display_name,
    taskTypes: row.task_types,
    durations: (task?.durations?.values ?? [5]).join(","),
    resolutions: (task?.resolutions ?? ["720p"]).join(","),
    ratios: (task?.aspect_ratios ?? ["16:9"]).join(","),
    audioModes: task?.audio_modes ?? ["silent"],
    providerMaxN: Number(
      capability?.provider_native_max_n ?? row.max_image_count ?? 1,
    ),
    promptMaxRunes: Number(capability?.prompt_max_runes ?? 2000),
    inputFormats: (input?.formats ?? ["jpg", "jpeg", "png", "webp"]).join(","),
    inputMaxMB: Math.max(
      1,
      Math.round(
        Number(input?.max_bytes ?? 30 * 1024 * 1024) / (1024 * 1024),
      ),
    ),
    rateRows,
    freeImageCount: latestRate?.pricing_schema === "minimax_h3_second_v1" ? latestRate.rate_config.free_image_count : 5,
    extraImageCNY: latestRate?.pricing_schema === "minimax_h3_second_v1" ? latestRate.rate_config.extra_image_cny : "0.10000",
    validationStatus:
      capabilitySummary?.validation_status === "verified"
        ? "verified"
        : "untested",
    enabled: row.enabled,
  };
}
function videoTaskInputs(task: string, draft: ModelDraft) {
  const input = {
    required: true,
    max_count: 1,
    max_bytes: Math.round(draft.inputMaxMB * 1024 * 1024),
    media_types: ["image"],
    formats: stringList(draft.inputFormats),
  };
  if (task === "image_to_video") return { first_frame: input };
  if (task === "first_last_frame_to_video") {
    return { first_frame: input, last_frame: { ...input } };
  }
  return {};
}
function stringList(value: string) {
  return value
    .split(",")
    .map((item) => item.trim())
    .filter(Boolean);
}
function numberList(value: string) {
  return stringList(value)
    .map(Number)
    .filter((item) => Number.isInteger(item) && item > 0);
}
function videoTaskLabel(value: string) {
  return videoTasks.find((item) => item.value === value)?.label ?? value;
}
function nextVersion(prefix: string, current?: string) {
  const match = current?.match(/(\d+)$/);
  return `${prefix}-v${Number(match?.[1] ?? 0) + 1}`;
}
function errorText(value: unknown) {
  return value instanceof Error ? value.message : "视频模型配置操作失败";
}
