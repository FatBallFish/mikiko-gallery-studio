import { useEffect, useMemo, useState } from "react";
import { Trash2 } from "lucide-react";
import type {
  AdminVideoConfiguration,
  ModelAccount,
  ModelAccountModel,
} from "../../../shared/api-types";
import { adminApi } from "../../../shared/admin-api";
import { cn } from "../../../shared/classnames";
import {
  Badge,
  Button,
  Drawer,
  EmptyBlock,
  InlineFeedback,
  LoadingBlock,
  Modal,
  PageHeader,
  RefreshIconButton,
} from "../components";
import { DataTable, FilterToolbar, type ColumnDef } from "../ui/dataTable";
import {
  AdminMediaTabs,
  ChecklistField,
  FormSection,
  LedgerField,
  MultiSelectField,
  SwitchField,
  type AdminMediaAccountCounts,
} from "./AdminMediaTabs";
import { modelAccountMediaType, modelMediaType } from "./adminModelMedia";

const videoTasks = [
  { value: "text_to_video", label: "文生视频" },
  { value: "image_to_video", label: "图生视频" },
  { value: "first_last_frame_to_video", label: "首尾帧生视频" },
  { value: "reference_to_video", label: "参考生视频" },
  { value: "video_edit", label: "视频编辑" },
  { value: "video_extend", label: "视频延长" },
] as const;

const durationOptions = ["4", "5", "8", "10", "12", "15"];
const resolutionOptions = ["480p", "720p", "768p", "1080p", "2k"];
const ratioOptions = ["16:9", "9:16", "1:1", "4:3", "3:4", "21:9", "adaptive"];
const audioOptions = [
  { value: "silent", label: "静音" },
  { value: "generated", label: "生成音频" },
];
const inputFormatOptions = ["jpg", "jpeg", "png", "webp", "bmp", "tiff", "gif", "heic", "heif"];

type AccountDraft = {
  row?: ModelAccount;
  name: string;
  adapter: "seedance" | "minimax" | "gasic" | "bailian";
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
  durations: string[];
  resolutions: string[];
  ratios: string[];
  audioModes: string[];
  providerMaxN: number;
  promptMaxRunes: number;
  inputFormats: string[];
  inputMaxMB: number;
  rateRows: Record<string, { primary: string; secondary: string }>;
  freeImageCount: number;
  extraImageCNY: string;
  validationStatus: "untested" | "verified";
  enabled: boolean;
  baseline?: string;
};
type DeleteTarget = { kind: "account"; account: ModelAccount } | { kind: "model"; account: ModelAccount; model: ModelAccountModel };

const adapterBadgeLabels: Record<string, string> = {
  seedance: "Seedance",
  minimax: "MiniMax",
  gasic: "GASIC 中转",
  bailian: "阿里百炼",
};

function adapterBadgeLabel(adapterType: string) {
  return adapterBadgeLabels[adapterType] ?? adapterType;
}

export function VideoProviderAccountsPanel({ onFeedback = () => undefined, counts, onAccountsChanged }: { onFeedback?: (title: string, detail?: string) => void; counts?: AdminMediaAccountCounts; onAccountsChanged?: () => void }) {
  const [accounts, setAccounts] = useState<ModelAccount[]>([]);
  const [models, setModels] = useState<Record<string, ModelAccountModel[]>>({});
  const [snapshot, setSnapshot] = useState<AdminVideoConfiguration | null>(null);
  const [query, setQuery] = useState("");
  const [modelAccountID, setModelAccountID] = useState("");
  const [accountDraft, setAccountDraft] = useState<AccountDraft | null>(null);
  const [modelDraft, setModelDraft] = useState<ModelDraft | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<DeleteTarget | null>(null);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [error, setError] = useState("");
  const [mutationError, setMutationError] = useState("");

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
        nextAccounts.map(async (account) => {
          const accountModels = await adminApi.listModelAccountModels(account.id);
          return [
            String(account.id),
            accountModels.filter((model) => modelMediaType(model, account) === "video"),
          ] as const;
        }),
      );
      setAccounts(nextAccounts);
      setModels(Object.fromEntries(entries));
      setSnapshot(nextSnapshot);
      setModelAccountID((current) => {
        const wanted = preferredID || current;
        return nextAccounts.some((item) => String(item.id) === wanted)
          ? wanted
          : "";
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

  const filteredAccounts = useMemo(() => {
    const keyword = query.trim().toLowerCase();
    if (!keyword) return accounts;
    return accounts.filter((account) => [
      account.name,
      account.adapter_type,
      account.base_url,
      account.status,
    ].some((value) => String(value ?? "").toLowerCase().includes(keyword)));
  }, [accounts, query]);

  const modelAccount = accounts.find((item) => String(item.id) === modelAccountID);
  const selectedModels = modelAccount ? (models[String(modelAccount.id)] ?? []) : [];
  const allModels = Object.values(models).flat();
  const verifiedModels = allModels.filter((model) => capabilityFor(snapshot, model)?.validation_status === "verified");
  const pendingModels = allModels.filter((model) => !modelReady(snapshot, model));
  const enabledAccounts = accounts.filter((account) => account.status === "enabled").length;
  const accountValidation = accountDraft ? validateAccountDraft(accountDraft) : {};
  const accountDirty = accountDraft ? accountDraftDirtyCount(accountDraft, accountDraft.row) : 0;
  const modelValidation = modelDraft ? validateModelDraft(modelDraft) : {};
  const modelDirty = modelDraft ? modelDraftDirtyCount(modelDraft) : 0;

  async function saveAccount() {
    if (!accountDraft) return;
    setBusy(true);
    setMutationError("");
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
      onAccountsChanged?.();
      onFeedback(accountDraft.row ? "视频账号已保存" : "视频账号已创建", saved.name);
    } catch (caught) {
      setMutationError(errorText(caught));
    } finally {
      setBusy(false);
    }
  }

  async function saveModel() {
    if (!modelDraft) return;
    setBusy(true);
    setMutationError("");
    try {
      const shouldEnable = modelDraft.enabled;
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
        enabled: false,
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
            resolutions: [...modelDraft.resolutions],
            aspect_ratios: [...modelDraft.ratios],
            audio_modes: [...modelDraft.audioModes],
            ...videoTaskInputs(task, modelDraft),
          },
        ]),
      );
      const existingCapability = snapshot?.capabilities.find(
        (item) => String(item.account_model_id) === String(saved.id),
      );
      const capabilityInput = {
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
        enabled: false,
      };
      const savedCapability = await adminApi.saveVideoCapability(
        saved.id,
        capabilityInput,
      );
      const existingRate = snapshot?.rate_cards
        .filter((item) => String(item.account_model_id) === String(saved.id))
        .sort((a, b) => b.rate_version - a.rate_version)[0];
      const resolutions = [...modelDraft.resolutions];
      if (modelDraft.account.adapter_type === "minimax") {
        await adminApi.saveVideoRateCard(saved.id, {
          pricing_schema: "minimax_h3_second_v1",
          expected_rate_version: existingRate?.rate_version ?? 0, enabled: shouldEnable,
          rate_config: {
            resolutions: Object.fromEntries(resolutions.map((resolution) => [resolution, {
              output_second_cny: modelDraft.rateRows[resolution]?.primary || "0",
              input_video_second_cny: modelDraft.rateRows[resolution]?.secondary || modelDraft.rateRows[resolution]?.primary || "0",
            }])),
            free_image_count: modelDraft.freeImageCount, extra_image_cny: modelDraft.extraImageCNY, input_audio_free: true,
          },
        });
      } else if (modelDraft.account.adapter_type === "bailian") {
        await adminApi.saveVideoRateCard(saved.id, {
          pricing_schema: "bailian_per_second_v1",
          expected_rate_version: existingRate?.rate_version ?? 0, enabled: shouldEnable,
          rate_config: { resolutions: Object.fromEntries(resolutions.map((resolution) => [resolution, {
            output_second_cny: modelDraft.rateRows[resolution]?.primary || "0",
            input_video_second_cny: modelDraft.rateRows[resolution]?.secondary || modelDraft.rateRows[resolution]?.primary || "0",
          }])) },
        });
      } else if (modelDraft.account.adapter_type === "gasic") {
        await adminApi.saveVideoRateCard(saved.id, {
          pricing_schema: "gasic_per_task_v1",
          expected_rate_version: existingRate?.rate_version ?? 0, enabled: shouldEnable,
          rate_config: {
            per_task_cny: modelDraft.rateRows["per_task"]?.primary || "2.00000",
          },
        });
      } else {
        await adminApi.saveVideoRateCard(saved.id, {
          pricing_schema: "seedance_token_v1",
          expected_rate_version: existingRate?.rate_version ?? 0, enabled: shouldEnable,
          rate_config: { resolutions: Object.fromEntries(resolutions.map((resolution) => [resolution, {
            without_input_video_million_tokens_cny: modelDraft.rateRows[resolution]?.primary || "0",
            with_input_video_million_tokens_cny: modelDraft.rateRows[resolution]?.secondary || undefined,
          }])) },
        });
      }
      if (shouldEnable) {
        await adminApi.saveVideoCapability(saved.id, {
          ...capabilityInput,
          expected_version: savedCapability.capability_version,
          capability_version: nextVersion(
            "video-cap",
            savedCapability.capability_version,
          ),
          enabled: true,
        });
        await adminApi.updateModelAccountModel(
          modelDraft.account.id,
          saved.id,
          { ...baseInput, enabled: true },
        );
      }
      setModelDraft(null);
      await load(String(modelDraft.account.id));
      onFeedback("视频真实模型已保存", `${modelDraft.account.name} · ${modelDraft.displayName || modelDraft.modelCode}`);
    } catch (caught) {
      setMutationError(errorText(caught));
    } finally {
      setBusy(false);
    }
  }

  async function confirmDelete() {
    if (!deleteTarget) return;
    setDeleting(true);
    setMutationError("");
    try {
      if (deleteTarget.kind === "account") {
        await adminApi.deleteModelAccount(deleteTarget.account.id);
      } else {
        await adminApi.deleteModelAccountModel(deleteTarget.account.id, deleteTarget.model.id);
      }
      const label = deleteTarget.kind === "account" ? deleteTarget.account.name : deleteTarget.model.model_code;
      const deletingAccount = deleteTarget.kind === "account";
      setDeleteTarget(null);
      await load();
      if (deletingAccount) onAccountsChanged?.();
      onFeedback("视频配置已删除", label);
    } catch (caught) {
      setMutationError(errorText(caught));
    } finally {
      setDeleting(false);
    }
  }

  if (loading && !accounts.length)
    return <LoadingBlock label="载入视频接入账号" />;
  if (error && !accounts.length)
    return <EmptyBlock title="视频接入账号载入失败" detail={error} action={<Button variant="primary" size="sm" onClick={() => void load()}>重试</Button>} />;

  return (
    <section className="a-page-stack" data-video-provider-accounts>
      <PageHeader
        eyebrow="Provider Accounts"
        title="接入账号"
        description="图片与视频账号分开维护。视频账号按适配器接入；任务类型、时长、分辨率、音频与厂商原生费率在真实模型配置中维护。"
        primaryAction={<Button variant="primary" size="sm" onClick={() => setAccountDraft(blankAccount())}>添加视频账号</Button>}
        secondaryActions={<RefreshIconButton label="刷新视频接入账号" refreshing={loading} onClick={() => void load()} />}
      />
      <AdminMediaTabs route="access-accounts" value="video" counts={counts} />
      {error ? <InlineFeedback tone="danger" message={error} /> : null}

      <section className="a-metrics" aria-label="视频接入概况">
        <div className="a-metric"><div className="a-metric__label">视频账号</div><div className="a-metric__value">{accounts.length}</div><div className="a-metric__foot">{enabledAccounts} 启用 · {accounts.length - enabledAccounts} 未启用</div></div>
        <div className="a-metric"><div className="a-metric__label">视频真实模型</div><div className="a-metric__value">{allModels.length}</div><div className="a-metric__foot">全部为视频能力模型</div></div>
        <div className="a-metric"><div className="a-metric__label">已验证</div><div className="a-metric__value">{verifiedModels.length}</div><div className="a-metric__foot">能力验证通过</div></div>
        <div className="a-metric"><div className="a-metric__label">待完善</div><div className="a-metric__value" style={{ color: pendingModels.length ? "var(--a-danger)" : undefined }}>{pendingModels.length}</div><div className="a-metric__foot">缺费率 / 能力 / 启用状态</div></div>
      </section>

      {accounts.length ? (
        <FilterToolbar
          fields={[{ key: "query", label: "搜索账号", primary: true, minWidth: "220px", maxWidth: "420px", control: <input value={query} onChange={(event) => setQuery(event.target.value)} className="a-input" placeholder="账号名称 / 适配器 / Base URL" aria-label="搜索视频接入账号" /> }]}
          resultSummary={`共 ${accounts.length} 个视频账号 · 当前显示 ${filteredAccounts.length} 个`}
        />
      ) : null}

      <section className="a-panel a-panel--flush">
        <div className="a-panel__head">
          <div>
            <h2>视频接入账号</h2>
            <p>视频账号按适配器区分；任务类型与时长/分辨率/音频能力在真实模型中配置。</p>
          </div>
        </div>
        {!accounts.length ? (
          <div className="a-panel__body"><EmptyBlock title="暂无视频账号" detail="新增 Seedance、MiniMax、Gasic 或阿里百炼账号后，再添加真实模型。" action={<Button variant="primary" size="sm" onClick={() => setAccountDraft(blankAccount())}>添加视频账号</Button>} /></div>
        ) : (
          <DataTable
            className="cfg-account-table"
            minWidth={1180}
            columns={videoAccountColumns({
              models,
              onEdit: (account) => { setMutationError(""); setAccountDraft(editAccount(account)); },
              onConfigureModels: (account) => setModelAccountID(String(account.id)),
              onDelete: (account) => { setMutationError(""); setDeleteTarget({ kind: "account", account }); },
            })}
            rows={filteredAccounts}
            rowKey={(account) => account.id}
            empty={<EmptyBlock title="未找到视频账号" detail="换一个账号名称、适配器或 Base URL 关键词再试。" />}
          />
        )}
      </section>

      {accountDraft ? (
        <Drawer
          wide
          title={accountDraft.row ? "编辑视频接入账号" : "新增视频接入账号"}
          description={`${accountDraft.name || "新账号"} · ${adapterBadgeLabel(accountDraft.adapter)}`}
          headerAside={<span className={cn("a-badge", accountDraft.status === "enabled" ? "a-badge--ok" : "a-badge--warn")}><i />{accountDraft.status === "enabled" ? "启用" : "停用"}</span>}
          onClose={() => setAccountDraft(null)}
          footer={<>
            <span className={cn("a-badge", Object.keys(accountValidation).length ? "a-badge--bad" : accountDirty ? "a-badge--warn" : "a-badge--ok")}><i />{Object.keys(accountValidation).length ? `${Object.keys(accountValidation).length} 项校验未通过` : accountDirty ? `${accountDirty} 项未保存` : "未修改"}</span>
            <div style={{ flex: 1 }} />
            {accountDraft.row ? <Button variant="danger" size="sm" disabled={busy} onClick={() => { setMutationError(""); setDeleteTarget({ kind: "account", account: accountDraft.row! }) }}>删除账号</Button> : null}
            <Button variant="ghost" size="sm" disabled={busy} onClick={() => setAccountDraft(null)}>取消</Button>
            <Button variant="primary" size="sm" disabled={busy || Object.keys(accountValidation).length > 0} onClick={() => void saveAccount()}>{busy ? "保存中..." : "保存账号"}</Button>
          </>}
        >
          {mutationError ? <InlineFeedback tone="danger" message={mutationError} /> : null}
          <form className="a-form" onSubmit={(event) => event.preventDefault()}>
            <FormSection index={1} title="连接" description="视频账号按适配器接入；适配器决定厂商原生费率结构。">
              <LedgerField label="账号名称" required error={accountValidation.name}><input className="a-input" value={accountDraft.name} onChange={(event) => setAccountDraft({ ...accountDraft, name: event.target.value })} /></LedgerField>
              <LedgerField label="适配器" hint="Seedance 按 Token、MiniMax/百炼按秒、Gasic 按次计费。"><select className="a-select" value={accountDraft.adapter} onChange={(event) => setAccountDraft({ ...accountDraft, adapter: event.target.value as AccountDraft["adapter"] })}><option value="seedance">Seedance</option><option value="minimax">MiniMax</option><option value="gasic">Gasic</option><option value="bailian">阿里百炼</option></select></LedgerField>
              <LedgerField label="Base URL" required error={accountValidation.baseURL}><input className="a-input" value={accountDraft.baseURL} onChange={(event) => setAccountDraft({ ...accountDraft, baseURL: event.target.value })} placeholder="https://ark.cn-beijing.volces.com/api/v3" style={{ fontFamily: "var(--font-admin-mono)", fontSize: 12.5 }} /></LedgerField>
              <LedgerField label="账号状态"><select className="a-select" value={accountDraft.status} onChange={(event) => setAccountDraft({ ...accountDraft, status: event.target.value })}><option value="enabled">启用</option><option value="disabled">停用</option><option value="error">异常</option></select></LedgerField>
            </FormSection>
            <FormSection index={2} title="鉴权" description="密钥只写不读，保存后显示掩码。">
              <LedgerField label="API Key" hint={accountDraft.row ? "留空表示保持现有密钥。" : "首次创建账号时必须填写 API Key。"}><input className="a-input" type="password" value={accountDraft.apiKey} onChange={(event) => setAccountDraft({ ...accountDraft, apiKey: event.target.value })} placeholder={accountDraft.row ? "已配置 · 留空表示保持不变" : "sk-..."} /></LedgerField>
            </FormSection>
            <FormSection index={3} title="调度">
              <div className="a-form__pair">
                <LedgerField label="优先级" error={accountValidation.priority}><input className="a-input" type="number" min="1" value={accountDraft.priority} onChange={(event) => setAccountDraft({ ...accountDraft, priority: Number(event.target.value) })} /></LedgerField>
                <LedgerField label="权重" error={accountValidation.weight}><input className="a-input" type="number" min="0" value={accountDraft.weight} onChange={(event) => setAccountDraft({ ...accountDraft, weight: Number(event.target.value) })} /></LedgerField>
              </div>
              <div className="a-form__pair">
                <LedgerField label="并发上限" error={accountValidation.concurrency}><input className="a-input" type="number" min="1" value={accountDraft.concurrency} onChange={(event) => setAccountDraft({ ...accountDraft, concurrency: Number(event.target.value) })} /></LedgerField>
                <LedgerField label="超时（毫秒）" error={accountValidation.timeout}><input className="a-input" type="number" min="1000" value={accountDraft.timeout} onChange={(event) => setAccountDraft({ ...accountDraft, timeout: Number(event.target.value) })} /></LedgerField>
              </div>
              <SwitchField label="启用该账号" checked={accountDraft.status === "enabled"} onChange={(checked) => setAccountDraft({ ...accountDraft, status: checked ? "enabled" : "disabled" })} />
            </FormSection>
          </form>
        </Drawer>
      ) : null}

      {modelAccount ? (
        <Drawer
          wide
          title="视频模型配置"
          description={`${modelAccount.name} · ${selectedModels.length} 个视频真实模型 · ${adapterBadgeLabel(modelAccount.adapter_type)}`}
          headerAside={<span className={cn("a-badge", modelAccount.status === "enabled" ? "a-badge--ok" : "a-badge--warn")}><i />{modelAccount.status === "enabled" ? "账号启用中" : "账号未启用"}</span>}
          onClose={() => { if (modelDraft) return; setModelAccountID(""); }}
          footer={<><Button variant="ghost" size="sm" onClick={() => { setModelAccountID(""); setModelDraft(null); }}>关闭</Button><div style={{ flex: 1 }} /><Button variant="primary" size="sm" onClick={() => { setMutationError(""); setModelDraft(blankModel(modelAccount)); }}>添加视频模型</Button></>}
        >
          <div className="aa-summary">
            <div><span>视频模型</span><strong>{selectedModels.length} 个</strong></div>
            <div><span>任务类型</span><strong>6 种</strong></div>
            <div><span>已验证</span><strong>{selectedModels.filter((model) => capabilityFor(snapshot, model)?.validation_status === "verified").length} 个</strong></div>
            <div><span>待完善</span><strong>{selectedModels.filter((model) => !modelReady(snapshot, model)).length} 个</strong></div>
          </div>
          <div className="a-section-title" style={{ marginBottom: 0 }}>
            <div><h3 style={{ fontSize: 15 }}>视频真实模型</h3><p>任务类型、时长、分辨率、比例、音频、输入格式与厂商原生费率在第二层抽屉维护。</p></div>
          </div>
          {!selectedModels.length ? (
            <div className="aa-empty"><strong>暂无视频真实模型</strong><p>为当前账号添加视频模型，并配置厂商原生费率与能力验证。</p><Button variant="primary" size="sm" onClick={() => { setMutationError(""); setModelDraft(blankModel(modelAccount)); }}>添加视频模型</Button></div>
          ) : (
            <div className="aa-model-grid">
              {selectedModels.map((model) => {
                const rate = latestRate(snapshot, model);
                const capability = capabilityFor(snapshot, model);
                return (
                  <article key={String(model.id)} className="aa-model">
                    <div className="aa-model__copy">
                      <strong>{model.display_name || model.model_code} <span className={cn("a-badge", modelReady(snapshot, model) ? "a-badge--ok" : "a-badge--warn")} style={{ marginLeft: 6 }}><i />{modelReady(snapshot, model) ? "可用" : "待完善"}</span></strong>
                      <span>{model.model_code} · {model.task_types.map(videoTaskLabel).join(" / ") || "未配置任务类型"}</span>
                      <div className="aa-model__tags">
                        {capability?.validation_status === "verified" ? <span className="a-badge a-badge--ok"><i />已验证</span> : <span className="a-badge a-badge--warn"><i />待验证</span>}
                        {rate ? <span className="a-badge">费率 v{rate.rate_version} · {rate.currency}</span> : <span className="a-badge a-badge--warn">缺原生费率</span>}
                        {model.enabled ? <span className="a-badge">启用</span> : <span className="a-badge">停用</span>}
                      </div>
                    </div>
                    <div className="aa-model__actions">
                      <button className="a-row-btn a-row-btn--primary" type="button" onClick={() => { setMutationError(""); setModelDraft(editModel(modelAccount, model, snapshot)); }}>编辑</button>
                      <button className="a-row-btn a-row-btn--danger" type="button" onClick={() => { setMutationError(""); setDeleteTarget({ kind: "model", account: modelAccount, model }); }}>删除</button>
                    </div>
                  </article>
                );
              })}
            </div>
          )}
          <div className="a-banner a-banner--info">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.6" aria-hidden="true"><circle cx="12" cy="12" r="9" /><path d="M12 11v5M12 7.5h.01" /></svg>
            <div><strong style={{ fontSize: 13 }}>视频模型不配置图片侧能力</strong><div className="a-hint">质量、尺寸模式、支持背景、输出压缩、输出格式与审核等级只属于图片模型；视频模型使用任务类型、时长、分辨率、比例、音频和输入格式。</div></div>
          </div>
        </Drawer>
      ) : null}

      {modelDraft ? (
        <Drawer
          stacked={Boolean(modelAccount)}
          wide
          title="视频真实模型配置"
          description={`${modelDraft.account.name} · ${modelDraft.displayName || "新增视频模型"}${modelDraft.modelCode ? ` · ${modelDraft.modelCode}` : ""}`}
          headerAside={<span className={cn("a-badge", modelDraft.validationStatus === "verified" ? "a-badge--ok" : "a-badge--warn")}><i />{modelDraft.validationStatus === "verified" ? "已验证" : "待验证"}</span>}
          onClose={() => setModelDraft(null)}
          footer={<>
            <span className={cn("a-badge", Object.keys(modelValidation).length ? "a-badge--bad" : modelDirty ? "a-badge--warn" : "a-badge--ok")}><i />{Object.keys(modelValidation).length ? `${Object.keys(modelValidation).length} 项校验未通过` : modelDirty ? `${modelDirty} 项未保存` : "未修改"}</span>
            <div style={{ flex: 1 }} />
            <Button variant="ghost" size="sm" disabled={busy} onClick={() => setModelDraft(null)}>取消</Button>
            <Button variant="primary" size="sm" disabled={busy || Object.keys(modelValidation).length > 0} onClick={() => void saveModel()}>{busy ? "保存中..." : "保存模型、能力和费率"}</Button>
          </>}
        >
          {mutationError ? <InlineFeedback tone="danger" message={mutationError} /> : null}
          <form className="a-form" onSubmit={(event) => event.preventDefault()}>
            <FormSection index={1} title="模型标识" description="视频模型的任务类型是 6 种视频能力，不包含图片任务。">
              <LedgerField label="模型代码" required error={modelValidation.modelCode}><input className="a-input" value={modelDraft.modelCode} onChange={(event) => setModelDraft({ ...modelDraft, modelCode: event.target.value })} style={{ fontFamily: "var(--font-admin-mono)" }} /></LedgerField>
              <LedgerField label="展示名称"><input className="a-input" value={modelDraft.displayName} onChange={(event) => setModelDraft({ ...modelDraft, displayName: event.target.value })} /></LedgerField>
              <ChecklistField label="任务类型" values={modelDraft.taskTypes} options={videoTasks.map((task) => ({ value: task.value, label: task.label }))} onChange={(taskTypes) => setModelDraft({ ...modelDraft, taskTypes })} error={modelValidation.taskTypes} />
              <div className="a-form__pair">
                <LedgerField label="能力验证状态"><select className="a-select" value={modelDraft.validationStatus} onChange={(event) => setModelDraft({ ...modelDraft, validationStatus: event.target.value as ModelDraft["validationStatus"], enabled: event.target.value === "verified" ? modelDraft.enabled : false })}><option value="untested">待验证</option><option value="verified">已验证</option></select></LedgerField>
                <LedgerField label="状态" error={modelValidation.enabled}><select className="a-select" value={modelDraft.enabled ? "enabled" : "disabled"} disabled={modelDraft.validationStatus !== "verified"} onChange={(event) => setModelDraft({ ...modelDraft, enabled: event.target.value === "enabled" })}><option value="enabled">启用</option><option value="disabled">停用</option></select></LedgerField>
              </div>
            </FormSection>

            <FormSection index={2} title="视频能力" description="时长、分辨率、比例、音频与输入格式共同构成可用参数组合。">
              <div className="a-form__pair">
                <LedgerField label="Provider 原生最大 n" error={modelValidation.providerMaxN}><input className="a-input" type="number" min="1" max="10" value={modelDraft.providerMaxN} onChange={(event) => setModelDraft({ ...modelDraft, providerMaxN: Math.min(10, Math.max(1, Number(event.target.value) || 1)) })} /></LedgerField>
                <LedgerField label="提示词最大字符数" error={modelValidation.promptMaxRunes}><input className="a-input" type="number" min="0" value={modelDraft.promptMaxRunes} onChange={(event) => setModelDraft({ ...modelDraft, promptMaxRunes: Math.max(0, Number(event.target.value) || 0) })} /></LedgerField>
              </div>
              <MultiSelectField label="支持时长（秒）" values={modelDraft.durations} options={durationOptions} onChange={(durations) => setModelDraft({ ...modelDraft, durations })} customPlaceholder="自定义时长" error={modelValidation.durations} />
              <MultiSelectField label="支持分辨率" values={modelDraft.resolutions} options={resolutionOptions} onChange={(resolutions) => setModelDraft({ ...modelDraft, resolutions, rateRows: ensureRateRows(modelDraft.rateRows, resolutions) })} customPlaceholder="自定义分辨率" error={modelValidation.resolutions} />
              <MultiSelectField label="支持比例" values={modelDraft.ratios} options={ratioOptions} onChange={(ratios) => setModelDraft({ ...modelDraft, ratios })} customPlaceholder="自定义比例" normalize={normalizeRatio} error={modelValidation.ratios} />
              <MultiSelectField label="音频模式" values={modelDraft.audioModes} options={audioOptions} onChange={(audioModes) => setModelDraft({ ...modelDraft, audioModes })} customPlaceholder="自定义音频模式" error={modelValidation.audioModes} allowCustom={false} />
              <MultiSelectField label="输入图片格式" values={modelDraft.inputFormats} options={inputFormatOptions} onChange={(inputFormats) => setModelDraft({ ...modelDraft, inputFormats: inputFormats.map((item) => item.toLowerCase()) })} customPlaceholder="自定义输入格式" normalize={(value) => value.trim().toLowerCase()} error={modelValidation.inputFormats} />
              <LedgerField label="单张输入上限（MB）" hint="参考图、首尾帧等输入素材统一使用该上限。" error={modelValidation.inputMaxMB}><input className="a-input" type="number" min="1" value={modelDraft.inputMaxMB} onChange={(event) => setModelDraft({ ...modelDraft, inputMaxMB: Math.max(1, Number(event.target.value) || 1) })} /></LedgerField>
            </FormSection>

            <FormSection index={3} title="厂商原生费率" description={`费率结构由账号适配器决定：${adapterRateHint(modelDraft.account.adapter_type)}。`}>
              {modelValidation.rates ? <InlineFeedback tone="danger" message={modelValidation.rates} /> : null}
              <div className="cfg-scroll-x">
                <table className="a-table" style={{ minWidth: 620 }}>
                  <thead>
                    <tr>
                      <th>分辨率</th>
                      <th className="is-num">{primaryRateLabel(modelDraft.account.adapter_type)}</th>
                      {modelDraft.account.adapter_type !== "gasic" ? <th className="is-num">{secondaryRateLabel(modelDraft.account.adapter_type)}</th> : null}
                    </tr>
                  </thead>
                  <tbody>
                    {modelDraft.account.adapter_type === "gasic" ? (
                      <tr>
                        <td>按次计费</td>
                        <td className="is-num"><input className="a-input cfg-table-input" inputMode="decimal" value={modelDraft.rateRows["per_task"]?.primary ?? ""} onChange={(event) => setModelDraft({ ...modelDraft, rateRows: { ...modelDraft.rateRows, per_task: { primary: event.target.value, secondary: "" } } })} /></td>
                      </tr>
                    ) : modelDraft.resolutions.length ? modelDraft.resolutions.map((resolution) => (
                      <tr key={resolution}>
                        <td className="a-mono">{resolution}</td>
                        <td className="is-num"><input className="a-input cfg-table-input" inputMode="decimal" value={modelDraft.rateRows[resolution]?.primary ?? ""} onChange={(event) => setModelDraft({ ...modelDraft, rateRows: { ...modelDraft.rateRows, [resolution]: { primary: event.target.value, secondary: modelDraft.rateRows[resolution]?.secondary ?? "" } } })} /></td>
                        <td className="is-num"><input className="a-input cfg-table-input" inputMode="decimal" value={modelDraft.rateRows[resolution]?.secondary ?? ""} onChange={(event) => setModelDraft({ ...modelDraft, rateRows: { ...modelDraft.rateRows, [resolution]: { primary: modelDraft.rateRows[resolution]?.primary ?? "", secondary: event.target.value } } })} /></td>
                      </tr>
                    )) : <tr><td colSpan={3}><span className="a-hint">请先选择支持分辨率，再填写原生费率。</span></td></tr>}
                  </tbody>
                </table>
              </div>
              {modelDraft.account.adapter_type === "minimax" ? (
                <div className="a-form__pair">
                  <LedgerField label="免费输入图片数（MiniMax）"><input className="a-input" type="number" min="0" value={modelDraft.freeImageCount} onChange={(event) => setModelDraft({ ...modelDraft, freeImageCount: Math.max(0, Number(event.target.value) || 0) })} /></LedgerField>
                  <LedgerField label="超额图片 CNY/张（MiniMax）"><input className="a-input" inputMode="decimal" value={modelDraft.extraImageCNY} onChange={(event) => setModelDraft({ ...modelDraft, extraImageCNY: event.target.value })} /></LedgerField>
                </div>
              ) : null}
              <div className="a-banner a-banner--warn">
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.6" aria-hidden="true"><path d="M12 3 2.5 20h19L12 3Z" /><path d="M12 9v5M12 17h.01" /></svg>
                <div><strong style={{ fontSize: 13 }}>MiniMax / 百炼使用按秒费率，Gasic 使用按次费率</strong><div className="a-hint">切换账号适配器后，费率表字段会变为「输出视频 CNY/秒 / 输入视频 CNY/秒」或「每条视频 CNY」。</div></div>
              </div>
            </FormSection>
          </form>
        </Drawer>
      ) : null}

      {deleteTarget ? (
        <Modal
          title={deleteTarget.kind === "account" ? "删除视频账号" : "删除视频真实模型"}
          detail={deleteTarget.kind === "account" ? deleteTarget.account.name : `${deleteTarget.account.name} / ${deleteTarget.model.model_code}`}
          onClose={() => { if (!deleting) setDeleteTarget(null); }}
          footer={<><Button variant="ghost" disabled={deleting} onClick={() => setDeleteTarget(null)}>取消</Button><Button variant="danger" disabled={deleting} onClick={() => void confirmDelete()}>{deleting ? "删除中..." : "确认删除"}</Button></>}
        >
          {mutationError ? <InlineFeedback tone="danger" message={mutationError} /> : null}
          <p className="m-0 text-sm leading-6 text-[var(--a-ink-2)]">删除后该配置不再参与新请求；历史任务仍按已保存的能力与费率快照展示。</p>
        </Modal>
      ) : null}
    </section>
  );
}

function videoAccountColumns({ models, onEdit, onConfigureModels, onDelete }: {
  models: Record<string, ModelAccountModel[]>;
  onEdit: (account: ModelAccount) => void;
  onConfigureModels: (account: ModelAccount) => void;
  onDelete: (account: ModelAccount) => void;
}): ColumnDef<ModelAccount>[] {
  return [
    {
      key: "account",
      title: "账号",
      width: "minmax(220px,2fr)",
      render: (account) => (
        <span className="cfg-account-cell">
          <span className="cfg-account-cell__logo">{account.name.slice(0, 1).toUpperCase()}</span>
          <span className="a-cell-stack"><strong>{account.name}</strong><span>{adapterBadgeLabel(account.adapter_type)} · 视频账号</span></span>
        </span>
      ),
    },
    { key: "adapter", title: "适配器", width: "minmax(130px,1fr)", render: (account) => <Badge tone="primary">{adapterBadgeLabel(account.adapter_type)}</Badge> },
    { key: "baseUrl", title: "Base URL", width: "minmax(220px,2fr)", kind: "code", render: (account) => <code className="a-mono text-[11.5px]" title={account.base_url}>{account.base_url}</code> },
    { key: "credentials", title: "密钥", width: "minmax(100px,.8fr)", render: (account) => <Badge tone={account.credentials_status?.has_api_key ? "success" : "danger"}>{account.credentials_status?.has_api_key ? "已配置" : "未配置"}</Badge> },
    { key: "models", title: "真实模型", width: "minmax(90px,.7fr)", align: "right", kind: "number", render: (account) => <span className="a-num">{models[String(account.id)]?.length ?? 0}</span> },
    { key: "concurrency", title: "并发 / 超时", width: "minmax(110px,.8fr)", align: "right", kind: "number", render: (account) => <span className="a-num">{account.concurrency_limit} / {Math.round(account.timeout_ms / 1000)}s</span> },
    { key: "status", title: "状态", width: "minmax(90px,.7fr)", render: (account) => <Badge tone={account.status === "enabled" ? "success" : account.status === "error" ? "danger" : "warning"}>{account.status === "enabled" ? "启用" : account.status === "error" ? "异常" : "停用"}</Badge> },
    {
      key: "actions",
      title: "操作",
      width: "minmax(220px,1.4fr)",
      align: "right",
      render: (account) => (
        <span className="a-row-actions">
          <button className="a-row-btn" type="button" onClick={() => onEdit(account)}>编辑</button>
          <button className="a-row-btn a-row-btn--primary" type="button" onClick={() => onConfigureModels(account)}>模型配置</button>
          <button className="a-icon-btn" style={{ width: 32, height: 32 }} type="button" aria-label={`删除账号 ${account.name}`} title="删除账号" onClick={() => onDelete(account)}><Trash2 className="size-4" /></button>
        </span>
      ),
    },
  ];
}

function blankAccount(): AccountDraft {
  return { name: "", adapter: "seedance", baseURL: "", apiKey: "", status: "enabled", priority: 1, weight: 100, concurrency: 5, timeout: 300000 };
}

function editAccount(row: ModelAccount): AccountDraft {
  return {
    row,
    name: row.name,
    adapter: row.adapter_type === "minimax" ? "minimax" : row.adapter_type === "gasic" ? "gasic" : row.adapter_type === "bailian" ? "bailian" : "seedance",
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
  const gasic = account.adapter_type === "gasic";
  const bailian = account.adapter_type === "bailian";
  const resolutions = minimax ? ["768p", "2k"] : bailian || gasic ? ["480p", "720p", "1080p"] : ["720p"];
  return {
    account,
    modelCode: "",
    displayName: "",
    taskTypes: ["text_to_video"],
    durations: ["5", "10"],
    resolutions,
    ratios: bailian ? ["21:9", "16:9", "4:3", "1:1", "3:4", "9:16"] : ["16:9", "9:16", "1:1"],
    audioModes: minimax ? ["generated"] : ["silent", "generated"],
    providerMaxN: 1,
    promptMaxRunes: minimax ? 7000 : 2000,
    inputFormats: account.adapter_type === "minimax" ? ["jpg", "jpeg", "png", "webp", "heic", "heif"] : ["jpg", "jpeg", "png", "webp", "bmp", "tiff", "gif"],
    inputMaxMB: 30,
    rateRows: minimax
      ? { "768p": { primary: "0.50000", secondary: "0.50000" }, "2k": { primary: "0.80000", secondary: "0.80000" } }
      : bailian
        ? { "480p": { primary: "0.30000", secondary: "0.30000" }, "720p": { primary: "0.60000", secondary: "0.60000" }, "1080p": { primary: "1.20000", secondary: "1.20000" } }
        : gasic
          ? { per_task: { primary: "2.00000", secondary: "" } }
          : { "720p": { primary: "46.00000", secondary: "" } },
    freeImageCount: 5,
    extraImageCNY: "0.10000",
    validationStatus: "untested",
    enabled: false,
  };
}

function editModel(account: ModelAccount, row: ModelAccountModel, snapshot: AdminVideoConfiguration | null): ModelDraft {
  const capabilitySummary = snapshot?.capabilities.find((item) => String(item.account_model_id) === String(row.id));
  const capability = capabilitySummary?.capability as Record<string, any> | undefined;
  const task = Object.values(capability?.task_types ?? {})[0] as Record<string, any> | undefined;
  const input = Object.values(task?.inputs ?? {})[0] as Record<string, any> | undefined;
  const latestRate = snapshot?.rate_cards
    .filter((item) => String(item.account_model_id) === String(row.id))
    .sort((a, b) => b.rate_version - a.rate_version)[0];
  const rateConfig = latestRate?.rate_config;
  const rateRows = rateConfig && "per_task_cny" in rateConfig
    ? { per_task: { primary: String(rateConfig.per_task_cny ?? ""), secondary: "" } }
    : Object.fromEntries(
        (Object.entries((rateConfig as Record<string, any> | undefined)?.resolutions ?? {}) as [string, Record<string, any>][]).map(([resolution, value]) => [resolution, {
          primary: "output_second_cny" in value ? value.output_second_cny : value.without_input_video_million_tokens_cny,
          secondary: "input_video_second_cny" in value ? value.input_video_second_cny : value.with_input_video_million_tokens_cny ?? "",
        }]),
      );
  const draft: ModelDraft = {
    account,
    row,
    modelCode: row.model_code,
    displayName: row.display_name,
    taskTypes: row.task_types,
    durations: (task?.durations?.values ?? [5]).map(String),
    resolutions: (task?.resolutions ?? ["720p"]).map(String),
    ratios: (task?.aspect_ratios ?? ["16:9"]).map(String),
    audioModes: task?.audio_modes ?? ["silent"],
    providerMaxN: Number(capability?.provider_native_max_n ?? row.max_image_count ?? 1),
    promptMaxRunes: Number(capability?.prompt_max_runes ?? 2000),
    inputFormats: (input?.formats ?? ["jpg", "jpeg", "png", "webp"]).map(String),
    inputMaxMB: Math.max(1, Math.round(Number(input?.max_bytes ?? 30 * 1024 * 1024) / (1024 * 1024))),
    rateRows,
    freeImageCount: latestRate?.pricing_schema === "minimax_h3_second_v1" ? latestRate.rate_config.free_image_count : 5,
    extraImageCNY: latestRate?.pricing_schema === "minimax_h3_second_v1" ? latestRate.rate_config.extra_image_cny : "0.10000",
    validationStatus: capabilitySummary?.validation_status === "verified" ? "verified" : "untested",
    enabled: row.enabled,
  };
  draft.baseline = JSON.stringify(modelSignature(draft));
  return draft;
}

function modelSignature(draft: ModelDraft) {
  return {
    modelCode: draft.modelCode,
    displayName: draft.displayName,
    taskTypes: draft.taskTypes,
    durations: draft.durations,
    resolutions: draft.resolutions,
    ratios: draft.ratios,
    audioModes: draft.audioModes,
    providerMaxN: draft.providerMaxN,
    promptMaxRunes: draft.promptMaxRunes,
    inputFormats: draft.inputFormats,
    inputMaxMB: draft.inputMaxMB,
    rateRows: draft.rateRows,
    freeImageCount: draft.freeImageCount,
    extraImageCNY: draft.extraImageCNY,
    validationStatus: draft.validationStatus,
    enabled: draft.enabled,
  };
}

function videoTaskInputs(task: string, draft: ModelDraft) {
  const input = {
    required: true,
    max_count: 1,
    max_bytes: Math.round(draft.inputMaxMB * 1024 * 1024),
    media_types: ["image"],
    formats: [...draft.inputFormats],
  };
  const optional = { ...input, required: false };
  const videoInput = { ...input, media_types: ["video"], formats: ["video/mp4", "video/quicktime"] };
  if (task === "image_to_video") return { inputs: { first_frame: input } };
  if (task === "first_last_frame_to_video") return { inputs: { first_frame: input, last_frame: { ...input } } };
  if (task === "reference_to_video") {
    return {
      inputs: {
        reference_image: { ...optional, max_count: 10 },
        reference_video: { ...optional, media_types: ["video"], max_count: 5, max_bytes: 100 * 1024 * 1024, formats: ["video/mp4", "video/quicktime"] },
        reference_audio: { ...optional, media_types: ["audio"], max_count: 3, max_bytes: 15 * 1024 * 1024, formats: ["audio/wav", "audio/x-wav", "audio/mpeg"] },
      },
      min_inputs: 1,
    };
  }
  if (task === "video_edit" || task === "video_extend") return { inputs: { reference_video: { ...videoInput, max_bytes: 100 * 1024 * 1024 } } };
  return { inputs: {} };
}

function validateAccountDraft(draft: AccountDraft): Record<string, string> {
  const errors: Record<string, string> = {};
  if (!draft.name.trim()) errors.name = "请填写账号名称。";
  if (!draft.baseURL.trim()) errors.baseURL = "请填写 Base URL。";
  else if (!/^https?:\/\//i.test(draft.baseURL.trim())) errors.baseURL = "Base URL 必须以 http:// 或 https:// 开头。";
  if (!Number.isFinite(draft.priority) || draft.priority < 0) errors.priority = "优先级必须是非负整数。";
  if (!Number.isFinite(draft.weight) || draft.weight < 0) errors.weight = "权重必须是非负整数。";
  if (!Number.isInteger(draft.concurrency) || draft.concurrency < 1) errors.concurrency = "并发上限必须大于 0。";
  if (!Number.isInteger(draft.timeout) || draft.timeout < 1000) errors.timeout = "超时至少为 1000 毫秒。";
  return errors;
}

function accountDraftDirtyCount(draft: AccountDraft, row?: ModelAccount): number {
  const baseline = row ? editAccount(row) : blankAccount();
  const keys: Array<keyof AccountDraft> = ["name", "adapter", "baseURL", "status", "priority", "weight", "concurrency", "timeout"];
  let count = keys.filter((key) => String(draft[key]) !== String(baseline[key])).length;
  if (draft.apiKey) count += 1;
  return count;
}

function validateModelDraft(draft: ModelDraft): Record<string, string> {
  const errors: Record<string, string> = {};
  if (!draft.modelCode.trim()) errors.modelCode = "请填写模型代码。";
  if (!draft.taskTypes.length) errors.taskTypes = "至少选择一种视频任务类型。";
  if (draft.providerMaxN < 1) errors.providerMaxN = "Provider 原生最大 n 至少为 1。";
  if (draft.promptMaxRunes <= 0) errors.promptMaxRunes = "提示词最大字符数必须大于 0。";
  if (!draft.durations.length) errors.durations = "至少选择一个支持时长。";
  if (!draft.resolutions.length) errors.resolutions = "至少选择一个支持分辨率。";
  if (!draft.ratios.length) errors.ratios = "至少选择一个支持比例。";
  if (!draft.audioModes.length) errors.audioModes = "至少选择一个音频模式。";
  if (!draft.inputFormats.length) errors.inputFormats = "至少选择一种输入图片格式。";
  if (draft.inputMaxMB < 1) errors.inputMaxMB = "单张输入上限至少为 1MB。";
  if (draft.enabled && draft.validationStatus !== "verified") errors.enabled = "只有已验证的能力才能启用模型。";
  if (draft.account.adapter_type === "gasic") {
    if (!(Number(draft.rateRows["per_task"]?.primary) > 0)) errors.rates = "请填写每条视频的 CNY 费率。";
  } else {
    const missing = draft.resolutions.filter((resolution) => !(Number(draft.rateRows[resolution]?.primary) > 0));
    if (missing.length) errors.rates = `以下分辨率缺少原生费率：${missing.join("、")}。`;
  }
  return errors;
}

function modelDraftDirtyCount(draft: ModelDraft): number {
  if (!draft.row) return draft.modelCode.trim() ? 1 : 0;
  if (!draft.baseline) return 1;
  return JSON.stringify(modelSignature(draft)) === draft.baseline ? 0 : 1;
}

function ensureRateRows(rows: ModelDraft["rateRows"], resolutions: string[]) {
  const next = { ...rows };
  for (const resolution of resolutions) next[resolution] ??= { primary: "", secondary: "" };
  return next;
}

function latestRate(snapshot: AdminVideoConfiguration | null, model: ModelAccountModel) {
  return snapshot?.rate_cards
    .filter((item) => String(item.account_model_id) === String(model.id))
    .sort((a, b) => b.rate_version - a.rate_version)[0];
}

function capabilityFor(snapshot: AdminVideoConfiguration | null, model: ModelAccountModel) {
  return snapshot?.capabilities.find((item) => String(item.account_model_id) === String(model.id));
}

function modelReady(snapshot: AdminVideoConfiguration | null, model: ModelAccountModel) {
  const capability = capabilityFor(snapshot, model);
  const rate = latestRate(snapshot, model);
  return Boolean(model.enabled && capability?.enabled && capability.validation_status === "verified" && rate?.enabled);
}

function primaryRateLabel(adapterType: string) {
  if (adapterType === "minimax" || adapterType === "bailian") return "输出视频 CNY/秒";
  if (adapterType === "gasic") return "每条视频 CNY";
  return "不含输入视频 CNY/百万 Token";
}

function secondaryRateLabel(adapterType: string) {
  if (adapterType === "minimax" || adapterType === "bailian") return "输入视频 CNY/秒";
  return "包含输入视频 CNY/百万 Token";
}

function adapterRateHint(adapterType: string) {
  if (adapterType === "minimax") return "MiniMax 按秒计费";
  if (adapterType === "bailian") return "阿里百炼按秒计费";
  if (adapterType === "gasic") return "Gasic 按次计费";
  return "Seedance Token 计费";
}

function stringList(values: string[]) {
  return values.map((item) => item.trim()).filter(Boolean);
}

function numberList(values: string[]) {
  return values.map(Number).filter((item) => Number.isInteger(item) && item > 0);
}

function videoTaskLabel(value: string) {
  return videoTasks.find((item) => item.value === value)?.label ?? value;
}

function normalizeRatio(value: string) {
  const parts = value.trim().split(":").map((item) => Number(item));
  if (parts.length !== 2 || parts.some((item) => !Number.isFinite(item) || item <= 0)) return "";
  const gcd = (a: number, b: number): number => (b ? gcd(b, a % b) : a);
  const divisor = gcd(Math.round(parts[0]), Math.round(parts[1])) || 1;
  return `${parts[0] / divisor}:${parts[1] / divisor}`;
}

function nextVersion(prefix: string, current?: string) {
  const match = current?.match(/(\d+)$/);
  return `${prefix}-v${Number(match?.[1] ?? 0) + 1}`;
}

function errorText(value: unknown) {
  return value instanceof Error ? value.message : "视频模型配置操作失败";
}