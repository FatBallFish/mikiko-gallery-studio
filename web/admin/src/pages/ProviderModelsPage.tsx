import { ImageConfigurationFields } from './ImageConfigurationFields'
import { configValue, effectiveNativeProfile, nativeConfigurationErrors, patchConfig, switchNativePreset } from './nativeImageConfig'
import { NativeImagePricingFields } from './NativeImagePricingFields'
import { nativeImagePresets, nativeImageProfile, type NativeImagePricing } from '../../../shared/native-image'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import type { ModelAccount, ModelAccountModel, ModelAccountTestImageResult } from '../../../shared/api-types'
import { adminApi } from '../../../shared/admin-api'
import { cn } from '../../../shared/classnames'
import { Image as ImageIcon, Trash2 } from 'lucide-react'
import {
  Badge,
  Button,
  Drawer,
  EmptyBlock,
  ErrorBlock,
  InlineFeedback,
  LoadingBlock,
  Modal,
  PageHeader,
  RefreshIconButton,
} from '../components'
import { DataTable, FilterToolbar, type ColumnDef } from '../ui/dataTable'
import { adminTaskTypeLabel, adminTaskTypeOptions } from './adminTaskTypes'
import { modelLifecycleErrorMessage } from './adminModelLifecycle'
import {
  credentialsStatusLabel,
  modelAccountStatusLabel,
  modelAccountStatusTone,
  providerAdapterLabel,
  providerAuthLabel,
} from './providerModelRows'
import { createLatestListRequestGuard } from './listRefresh'
import { AdminMediaTabs, ChecklistField, FormSection, LedgerField, MultiSelectField, SwitchField, useAdminModelMediaTab, type AdminMediaAccountCounts } from './AdminMediaTabs'
import { modelAccountMediaType, modelMediaType } from './adminModelMedia'
import { TextModelsPage } from './TextModelsPage'
import { VideoProviderAccountsPanel } from './VideoProviderAccountsPanel'
import './config-redesign.css'

type AccountDraft = { id?: string | number; name: string; adapterType: string; authType: string; baseUrl: string; apiKey: string; priority: string; weight: string; concurrencyLimit: string; timeoutMS: string; status: string; sourceMode: string; extra?: Record<string, unknown> }
type ModelDraft = { account: ModelAccount; row?: ModelAccountModel; modelCode: string; displayName: string; taskTypes: string[]; base_resolution: string[]; quality: string[]; maxReferenceImageCount: string; maxImageCount: string; sizeModes: string[]; supportedRatios: string[]; supportsCustomRatio: boolean; supportedPixelSizes: string[]; supportsCustomSize: boolean; minWidth: string; maxWidth: string; minHeight: string; maxHeight: string; supportedBackgrounds: string[]; outputFormat: string[]; supportsOutputCompression: boolean; moderation: string[]; costPerImage: string; currency: string; enabled: boolean; extra?: Record<string, unknown>; extraError?: string }
type TestImageDialog = { account: ModelAccount; modelId: string; prompt: string; sourceMode: string; sizeMode: string; requestedSize: string; baseResolution: string; quality: string; outputFormat: string; background: string; outputCompression: string; moderation: string; aspectRatio: string; result?: ModelAccountTestImageResult; error?: string }
type DeleteTarget = { kind: 'account'; account: ModelAccount } | { kind: 'model'; account: ModelAccount; model: ModelAccountModel }

const baseResolutionOptions = ['1K', '2K', '4K']
const qualityOptions = ['auto', 'low', 'medium', 'high']
const outputFormatOptions = ['png', 'jpeg', 'webp']
const backgroundOptions = ['auto', 'opaque', 'transparent']
const moderationOptions = ['auto', 'low']
const sizeModeOptions = [
  { value: 'auto', label: '自动尺寸' },
  { value: 'ratio', label: '按比例' },
  { value: 'pixel', label: '按像素' },
]
const defaultRatios = ['1:1', '16:9', '9:16', '4:3', '3:4']
const defaultPixelSizes = ['1024x1024', '1536x1024', '1024x1536', '1280x720', '720x1280', '1024x768', '768x1024']
const blankAccount: AccountDraft = { name: '', adapterType: 'openai_compatible', authType: 'api_key', baseUrl: '', apiKey: '', priority: '1', weight: '100', concurrencyLimit: '5', timeoutMS: '120000', status: 'enabled', sourceMode: 'images' }
const defaultTestPrompt = 'A small product photo of a ceramic coffee cup on a clean desk'
const accountPrimaryActionLabel = '模型配置'

export function ProviderModelsPage({ accessToken, onFeedback = () => undefined }: { accessToken?: string; onFeedback?: (title: string, detail?: string) => void }) {
  const media = useAdminModelMediaTab()
  const [mediaCounts, setMediaCounts] = useState<AdminMediaAccountCounts | undefined>(undefined)
  const refreshMediaCounts = useCallback(async () => {
    const [accountResult, textResult] = await Promise.allSettled([
      adminApi.listModelAccounts({ page_size: 100 }),
      adminApi.listTextModelAccounts(),
    ])
    if (accountResult.status !== 'fulfilled') return
    const counts: AdminMediaAccountCounts = { image: 0, video: 0, audio: 0, text: 0 }
    for (const account of accountResult.value) {
      const mediaType = modelAccountMediaType(account)
      counts[mediaType] = (counts[mediaType] ?? 0) + 1
    }
    if (textResult.status === 'fulfilled') counts.text = (counts.text ?? 0) + textResult.value.length
    setMediaCounts(counts)
  }, [])
  useEffect(() => { void refreshMediaCounts() }, [media, refreshMediaCounts])
  if (media === 'image') return <ImageProviderModelsPanel accessToken={accessToken} onFeedback={onFeedback} counts={mediaCounts} onAccountsChanged={refreshMediaCounts} />
  if (media === 'video') return <VideoProviderAccountsPanel onFeedback={onFeedback} counts={mediaCounts} onAccountsChanged={refreshMediaCounts} />
  return (
    <section className="a-page-stack">
      <PageHeader
        eyebrow="Provider Accounts"
        title="接入账号"
        description="图片与视频账号分开维护。账号只负责连接、调度与密钥；真实模型的任务类型和能力参数在模型配置抽屉中维护。"
      />
      <AdminMediaTabs route="access-accounts" value={media} counts={mediaCounts} />
      {media === 'audio' ? (
        <EmptyBlock title="音频接入暂未开放" detail="音频模型运行时接入后，可在此维护账号、真实模型与厂商原生费率。" />
      ) : (
        <TextModelsPage onFeedback={onFeedback} onAccountsChanged={refreshMediaCounts} />
      )}
    </section>
  )
}

function ImageProviderModelsPanel({ accessToken, onFeedback, counts, onAccountsChanged }: { accessToken?: string; onFeedback: (title: string, detail?: string) => void; counts?: AdminMediaAccountCounts; onAccountsChanged?: () => void }) {
  const [accounts, setAccounts] = useState<ModelAccount[]>([])
  const [modelsByAccount, setModelsByAccount] = useState<Record<string, ModelAccountModel[]>>({})
  const [query, setQuery] = useState('')
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [accountDialog, setAccountDialog] = useState<AccountDraft | null>(null)
  const [modelAccountId, setModelAccountId] = useState('')
  const [modelDialog, setModelDialog] = useState<ModelDraft | null>(null)
  const [testDialog, setTestDialog] = useState<TestImageDialog | null>(null)
  const [saving, setSaving] = useState(false)
  const [testing, setTesting] = useState(false)
  const [mutationError, setMutationError] = useState<string | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<DeleteTarget | null>(null)
  const [deleting, setDeleting] = useState(false)
  const requestGuard = useRef(createLatestListRequestGuard()).current

  const load = async (preferredAccountId?: string) => {
    const request = requestGuard.begin()
    setLoading(true)
    setError(null)
    try {
      const nextAccounts = await adminApi.listModelAccounts({ page_size: 100 })
      const imageAccounts = nextAccounts.filter((account) => modelAccountMediaType(account) === 'image')
      const modelPairs = await Promise.all(imageAccounts.map(async (account) => {
        const accountModels = await adminApi.listModelAccountModels(account.id)
        return [String(account.id), accountModels.filter((model) => modelMediaType(model, account) === 'image')] as const
      }))
      if (!requestGuard.isCurrent(request)) return
      const nextModels = Object.fromEntries(modelPairs)
      setAccounts(imageAccounts)
      setModelsByAccount(nextModels)
      setModelAccountId((current) => {
        const preferred = preferredAccountId || current
        return imageAccounts.some((account) => String(account.id) === preferred) ? preferred : ''
      })
    } catch (caught) {
      if (!requestGuard.isCurrent(request)) return
      setError(caught instanceof Error ? caught.message : '模型接入载入失败')
    } finally {
      if (!requestGuard.isCurrent(request)) return
      setLoading(false)
    }
  }

  useEffect(() => {
    void load()
    return () => requestGuard.invalidate()
  }, [])

  const filteredAccounts = useMemo(() => {
    const keyword = query.trim().toLowerCase()
    if (!keyword) return accounts
    return accounts.filter((account) => [
      account.name,
      account.adapter_type,
      account.auth_type,
      account.base_url,
      account.status,
    ].some((value) => String(value ?? '').toLowerCase().includes(keyword)))
  }, [accounts, query])

  const modelAccount = accounts.find((account) => String(account.id) === modelAccountId)
  const modelAccountModels = modelAccount ? modelsByAccount[String(modelAccount.id)] ?? [] : []
  const allModels = Object.values(modelsByAccount).flat()
  const pendingModels = allModels.filter((model) => modelNeedsAttention(model))
  const enabledAccounts = accounts.filter((account) => account.status === 'enabled').length
  const errorAccounts = accounts.filter((account) => account.status === 'error').length
  const adapterCount = new Set(accounts.map((account) => account.adapter_type)).size
  const selectedTestModel = testDialog
    ? (modelsByAccount[String(testDialog.account.id)] ?? []).find((model) => String(model.id) === testDialog.modelId)
    : undefined
  const selectedTestSizeModes = testModelSizeModes(selectedTestModel)
  const selectedTestQualities = testModelOptions(selectedTestModel?.quality, ['auto'])
  const selectedTestFormats = testModelOptions(selectedTestModel?.output_format, ['png'])
    .filter((format) => testDialog?.background !== 'transparent' || format === 'png' || format === 'webp')
  const selectedTestBackgrounds = testModelOptions(selectedTestModel?.supported_backgrounds, [])
  const selectedTestModeration = testModelOptions(selectedTestModel?.moderation, ['auto'])
  const testActualParams = testDialog?.result ? Object.entries(testDialog.result.actual_params ?? {}) : []
  const accountValidation = accountDialog ? validateAccountDraft(accountDialog) : {}
  const existingAccount = accountDialog?.id ? accounts.find((account) => String(account.id) === String(accountDialog.id)) : undefined
  const accountDirty = accountDialog ? accountDraftDirtyCount(accountDialog, existingAccount) : 0
  const modelValidation = modelDialog ? validateModelDraft(modelDialog) : {}
  const modelDirty = modelDialog ? modelDraftDirtyCount(modelDialog) : 0

  async function saveAccount() {
    if (!accountDialog) return
    setSaving(true)
    setMutationError(null)
    try {
      const payload = {
        name: accountDialog.name,
        adapter_type: accountDialog.adapterType,
        auth_type: accountDialog.authType,
        base_url: accountDialog.baseUrl,
        credentials: accountDialog.apiKey ? { api_key: accountDialog.apiKey } : undefined,
        priority: Number(accountDialog.priority),
        weight: Number(accountDialog.weight),
        concurrency_limit: Number(accountDialog.concurrencyLimit),
        timeout_ms: Number(accountDialog.timeoutMS),
        status: accountDialog.status,
        extra: { ...accountDialog.extra, media_type: 'image' },
      }
      const creating = !accountDialog.id
      const saved = accountDialog.id ? await adminApi.updateModelAccount(accountDialog.id, payload) : await adminApi.createModelAccount(payload)
      setAccountDialog(null)
      await load(String(saved.id))
      if (creating) setModelAccountId(String(saved.id))
      onAccountsChanged?.()
      onFeedback(creating ? '图片账号已创建' : '图片账号已保存', saved.name)
    } catch (caught) {
      setMutationError(caught instanceof Error ? caught.message : '模型账号保存失败')
    } finally {
      setSaving(false)
    }
  }

  async function saveModel() {
    if (!modelDialog || modelDialog.extraError) return
    setSaving(true)
    setMutationError(null)
    try {
      const payload = modelPayload(modelDialog)
      if (modelDialog.row) await adminApi.updateModelAccountModel(modelDialog.account.id, modelDialog.row.id, payload)
      else await adminApi.createModelAccountModel(modelDialog.account.id, payload)
      setModelDialog(null)
      await load(String(modelDialog.account.id))
      onFeedback('图片真实模型已保存', `${modelDialog.account.name} · ${modelDialog.displayName || modelDialog.modelCode}`)
    } catch (caught) {
      setMutationError(caught instanceof Error ? caught.message : '真实模型保存失败')
    } finally {
      setSaving(false)
    }
  }

  async function confirmDelete() {
    if (!deleteTarget) return
    setDeleting(true)
    setMutationError(null)
    try {
      if (deleteTarget.kind === 'account') await adminApi.deleteModelAccount(deleteTarget.account.id)
      else await adminApi.deleteModelAccountModel(deleteTarget.account.id, deleteTarget.model.id)
      const label = deleteTarget.kind === 'account' ? deleteTarget.account.name : deleteTarget.model.model_code
      if (deleteTarget.kind === 'model' && String(deleteTarget.account.id) === modelAccountId && modelAccountModels.length <= 1) setModelAccountId('')
      setDeleteTarget(null)
      await load()
      if (deleteTarget.kind === 'account') onAccountsChanged?.()
      onFeedback('模型配置已删除', label)
    } catch (caught) {
      setMutationError(modelLifecycleErrorMessage(caught, '删除模型配置失败'))
    } finally {
      setDeleting(false)
    }
  }

  const openAccountDialog = (draft: AccountDraft) => {
    setMutationError(null)
    setAccountDialog(draft)
  }

  const openModelDialog = (draft: ModelDraft) => {
    setMutationError(null)
    setModelDialog(draft)
  }

  const closeAccountDialog = () => {
    setMutationError(null)
    setAccountDialog(null)
  }

  const closeModelDialog = () => {
    setMutationError(null)
    setModelDialog(null)
  }

  async function runTestImage() {
    if (!testDialog) return
    setTesting(true)
    setTestDialog({ ...testDialog, error: undefined, result: undefined })
    try {
      const result = await adminApi.testModelAccountImage(testDialog.account.id, {
        model_id: Number(testDialog.modelId),
        prompt: testDialog.prompt,
        source_mode: testDialog.sourceMode,
        size_mode: testDialog.sizeMode,
        ...(testDialog.sizeMode === 'pixel' ? { requested_size: testDialog.requestedSize } : {}),
        ...(testDialog.sizeMode === 'ratio' ? { base_resolution: testDialog.baseResolution, aspect_ratio: testDialog.aspectRatio } : {}),
        quality: testDialog.quality,
        output_format: testDialog.outputFormat,
        background: testDialog.background || undefined,
        output_compression: Number(testDialog.outputCompression),
        moderation: testDialog.moderation,
      })
      setTestDialog((current) => current ? { ...current, result } : current)
    } catch (caught) {
      setTestDialog((current) => current ? { ...current, error: caught instanceof Error ? caught.message : '测试出图失败' } : current)
    } finally {
      setTesting(false)
    }
  }

  if (loading && !accounts.length) return <LoadingBlock label="载入模型接入" />
  if (error && !accounts.length) return <ErrorBlock message={error} onRetry={load} />

  return (
    <section className="a-page-stack">
      <PageHeader
        eyebrow="Provider Accounts"
        title="接入账号"
        description="图片与视频账号分开维护。账号只负责连接、调度与密钥；真实模型的任务类型和能力参数在模型配置抽屉中维护。"
        primaryAction={<Button variant="primary" size="sm" onClick={() => openAccountDialog({ ...blankAccount })}>添加图片账号</Button>}
        secondaryActions={<RefreshIconButton label="刷新接入账号" refreshing={loading} onClick={() => void load()} />}
      />
      <AdminMediaTabs route="access-accounts" value="image" counts={counts} />
      {error ? <InlineFeedback tone="danger" message={`接入账号刷新失败：${error}`} /> : null}

      <section className="a-metrics" aria-label="图片接入概况">
        <div className="a-metric"><div className="a-metric__label">图片账号</div><div className="a-metric__value">{accounts.length}</div><div className="a-metric__foot">{enabledAccounts} 启用 · {errorAccounts} 异常</div></div>
        <div className="a-metric"><div className="a-metric__label">图片真实模型</div><div className="a-metric__value">{allModels.length}</div><div className="a-metric__foot">覆盖 {adapterCount} 种接入方式</div></div>
        <div className="a-metric"><div className="a-metric__label">已启用模型</div><div className="a-metric__value">{allModels.filter((model) => model.enabled).length}</div><div className="a-metric__foot">只统计图片真实模型</div></div>
        <div className="a-metric"><div className="a-metric__label">待完善</div><div className="a-metric__value" style={{ color: pendingModels.length ? 'var(--a-danger)' : undefined }}>{pendingModels.length}</div><div className="a-metric__foot">缺基础分辨率 / 尺寸能力</div></div>
      </section>

      {accounts.length ? (
        <FilterToolbar
          fields={[{
            key: 'query',
            label: '搜索账号',
            primary: true,
            minWidth: '220px',
            maxWidth: '420px',
            control: <input value={query} onChange={(event) => setQuery(event.target.value)} placeholder="账号名称 / 适配器 / Base URL" className="a-input" aria-label="搜索图片接入账号" />,
          }]}
          resultSummary={`共 ${accounts.length} 个图片账号 · 当前显示 ${filteredAccounts.length} 个`}
        />
      ) : null}

      <section className="a-panel a-panel--flush">
        <div className="a-panel__head">
          <div>
            <h2>图片接入账号</h2>
            <p>图片模型只使用文生图与图片编辑任务类型。</p>
          </div>
        </div>
        {!accounts.length ? (
          <div className="a-panel__body">
            <EmptyBlock title="暂无模型接入账号" detail="创建账号后再添加真实上游模型。" action={<Button variant="primary" size="sm" onClick={() => openAccountDialog({ ...blankAccount })}>添加图片账号</Button>} />
          </div>
        ) : (
          <DataTable
            className="cfg-account-table"
            minWidth={1180}
            columns={accountColumns({
              modelsByAccount,
              onEdit: (account) => openAccountDialog(editAccountDraft(account)),
              onConfigureModels: (account) => setModelAccountId(String(account.id)),
              onTest: (account) => setTestDialog(newTestImageDialog(account, modelsByAccount[String(account.id)] ?? [])),
              onDelete: (account) => { setMutationError(null); setDeleteTarget({ kind: 'account', account }) },
            })}
            rows={filteredAccounts}
            rowKey={(account) => account.id}
            empty={<EmptyBlock title="未找到接入账号" detail="换一个账号名称、适配器或 Base URL 关键词再试。" />}
          />
        )}
      </section>

      {accountDialog ? (
        <Drawer
          wide
          title={accountDialog.id ? '编辑图片接入账号' : '新增图片接入账号'}
          description={`${accountDialog.name || '新账号'} · ${accountDialog.id ? `${modelsByAccount[String(accountDialog.id)]?.length ?? 0} 个图片真实模型` : '填写连接与调度参数'}`}
          headerAside={<span className={cn('a-badge', accountStatusTone(accountDialog.status))}><i />{modelAccountStatusLabel(accountDialog.status)}</span>}
          onClose={closeAccountDialog}
          footer={<>
            <span className={cn('a-badge', Object.keys(accountValidation).length ? 'a-badge--bad' : accountDirty ? 'a-badge--warn' : 'a-badge--ok')}><i />{Object.keys(accountValidation).length ? `${Object.keys(accountValidation).length} 项校验未通过` : accountDirty ? `${accountDirty} 项未保存` : '未修改'}</span>
            <div style={{ flex: 1 }} />
            {existingAccount ? <Button variant="danger" size="sm" disabled={saving} onClick={() => { setMutationError(null); setDeleteTarget({ kind: 'account', account: existingAccount }) }}>删除账号</Button> : null}
            <Button variant="ghost" size="sm" disabled={saving} onClick={closeAccountDialog}>取消</Button>
            <Button variant="primary" size="sm" disabled={saving || Object.keys(accountValidation).length > 0} onClick={() => void saveAccount()}>{saving ? '保存中...' : '保存账号'}</Button>
          </>}
        >
          {mutationError ? <InlineFeedback tone="danger" message={mutationError} /> : null}
          <form className="a-form" onSubmit={(event) => event.preventDefault()}>
            <FormSection index={1} title="连接" description="图片账号只接入图片生成与图片编辑模型。">
              <LedgerField label="账号名称" required error={accountValidation.name}><input className="a-input" value={accountDialog.name} onChange={(event) => setAccountDialog({ ...accountDialog, name: event.target.value })} /></LedgerField>
              <LedgerField label="接入方式"><select className="a-select" value={accountDialog.adapterType} onChange={(event) => setAccountDialog({ ...accountDialog, adapterType: event.target.value })}><option value="openai_compatible">OpenAI 兼容</option><option value="openrouter">OpenRouter</option><option value="qwen_image">Qwen 原生图片</option><option value="seedream_image">Seedream 原生图片</option></select></LedgerField>
              <LedgerField label="Base URL" required error={accountValidation.baseUrl} hint={accountDialog.adapterType === 'seedream_image' ? '填写 Ark 根地址 https://ark.cn-beijing.volces.com；适配器自动添加 API 路径，不要重复填写版本。' : accountDialog.adapterType === 'qwen_image' ? '填写 DashScope workspace 服务根地址；适配器自动添加原生图片 API 路径。' : '代理或中转服务请填写到 /v1。'}><input className="a-input" value={accountDialog.baseUrl} onChange={(event) => setAccountDialog({ ...accountDialog, baseUrl: event.target.value })} placeholder={accountDialog.adapterType === 'seedream_image' ? 'https://ark.cn-beijing.volces.com' : accountDialog.adapterType === 'qwen_image' ? 'https://dashscope.aliyuncs.com' : 'https://api.openai.com/v1'} style={{ fontFamily: 'var(--font-admin-mono)', fontSize: 12.5 }} /></LedgerField>
              <LedgerField label="账号状态"><select className="a-select" value={accountDialog.status} onChange={(event) => setAccountDialog({ ...accountDialog, status: event.target.value })}><option value="enabled">启用</option><option value="disabled">停用</option><option value="error">异常</option></select></LedgerField>
            </FormSection>
            <FormSection index={2} title="鉴权" description="密钥只写不读，保存后显示掩码。">
              <LedgerField label="鉴权方式"><select className="a-select" value={accountDialog.authType} onChange={(event) => setAccountDialog({ ...accountDialog, authType: event.target.value })}><option value="api_key">API Key</option></select></LedgerField>
              <LedgerField label="API Key" hint={accountDialog.id ? '留空表示保持现有密钥。' : '首次创建账号时必须填写 API Key。'}><input className="a-input" type="password" value={accountDialog.apiKey} onChange={(event) => setAccountDialog({ ...accountDialog, apiKey: event.target.value })} placeholder={accountDialog.id ? '已配置 · 留空表示保持不变' : 'sk-...'} /></LedgerField>
            </FormSection>
            <FormSection index={3} title="调度">
              <div className="a-form__pair">
                <LedgerField label="优先级" hint="数值越小越优先作为候选账号。" error={accountValidation.priority}><input className="a-input" inputMode="numeric" value={accountDialog.priority} onChange={(event) => setAccountDialog({ ...accountDialog, priority: event.target.value })} /></LedgerField>
                <LedgerField label="权重" hint="同优先级内按权重分配。" error={accountValidation.weight}><input className="a-input" inputMode="numeric" value={accountDialog.weight} onChange={(event) => setAccountDialog({ ...accountDialog, weight: event.target.value })} /></LedgerField>
              </div>
              <div className="a-form__pair">
                <LedgerField label="并发上限" error={accountValidation.concurrencyLimit}><input className="a-input" inputMode="numeric" value={accountDialog.concurrencyLimit} onChange={(event) => setAccountDialog({ ...accountDialog, concurrencyLimit: event.target.value })} /></LedgerField>
                <LedgerField label="超时（毫秒）" error={accountValidation.timeoutMS}><input className="a-input" inputMode="numeric" value={accountDialog.timeoutMS} onChange={(event) => setAccountDialog({ ...accountDialog, timeoutMS: event.target.value })} /></LedgerField>
              </div>
              <SwitchField label="启用该账号" checked={accountDialog.status === 'enabled'} onChange={(checked) => setAccountDialog({ ...accountDialog, status: checked ? 'enabled' : 'disabled' })} hint="停用后不再参与候选调度；异常状态用于保留错误上下文。" />
            </FormSection>
            <ImageConfigurationFields extra={accountDialog.extra} adapter={accountDialog.adapterType} onChange={(extra) => setAccountDialog({ ...accountDialog, extra })} />
          </form>
        </Drawer>
      ) : null}

      {modelAccount ? (
        <Drawer
          wide
          title="图片模型配置"
          description={`${modelAccount.name} · ${modelAccountModels.length} 个图片真实模型 · ${providerAdapterLabel(modelAccount.adapter_type)}`}
          headerAside={<span className={cn('a-badge', modelAccount.status === 'enabled' ? 'a-badge--ok' : 'a-badge--warn')}><i />{modelAccount.status === 'enabled' ? '账号启用中' : '账号未启用'}</span>}
          onClose={() => { if (modelDialog) return; setModelAccountId(''); closeModelDialog() }}
          footer={<>
            <Button variant="ghost" size="sm" onClick={() => { setModelAccountId(''); closeModelDialog() }}>关闭</Button>
            <div style={{ flex: 1 }} />
            <Button variant="primary" size="sm" onClick={() => openModelDialog(newModelDraft(modelAccount))}>添加图片模型</Button>
          </>}
        >
          <div className="aa-summary">
            <div><span>图片模型</span><strong>{modelAccountModels.length} 个</strong></div>
            <div><span>已启用</span><strong>{modelAccountModels.filter((model) => model.enabled).length} 个</strong></div>
            <div><span>任务类型</span><strong>文生图 / 图片编辑</strong></div>
            <div><span>待完善</span><strong>{modelAccountModels.filter(modelNeedsAttention).length} 个</strong></div>
          </div>
          <div className="a-section-title" style={{ marginBottom: 0 }}>
            <div>
              <h3 style={{ fontSize: 15 }}>图片真实模型</h3>
              <p>能力字段包含基础分辨率、质量、尺寸模式、比例、像素尺寸、背景、压缩、输出格式与审核。</p>
            </div>
          </div>
          {!modelAccountModels.length ? (
            <div className="aa-empty"><strong>暂无图片真实模型</strong><p>为当前账号添加可请求的上游模型代码、能力与成本。</p><Button variant="primary" size="sm" onClick={() => openModelDialog(newModelDraft(modelAccount))}>添加图片模型</Button></div>
          ) : (
            <div className="aa-model-grid">
              {modelAccountModels.map((model) => (
                <article key={String(model.id)} className="aa-model">
                  <div className="aa-model__copy">
                    <strong>{model.display_name || model.model_code} <span className={cn('a-badge', model.enabled ? 'a-badge--ok' : 'a-badge--warn')} style={{ marginLeft: 6 }}><i />{model.enabled ? '可用' : '停用'}</span></strong>
                    <span>{model.model_code} · {model.task_types.map(adminTaskTypeLabel).join(' / ') || '未配置任务类型'} · {normalizeBaseResolution(model.base_resolution).join('–') || '未配置基础分辨率'}</span>
                    <div className="aa-model__tags">
                      {model.quality?.length ? <span className="a-badge">质量 {model.quality.join('/')}</span> : null}
                      {model.supported_ratios?.length ? <span className="a-badge">比例 {model.supported_ratios.join('/')}</span> : null}
                      {model.output_format?.length ? <span className="a-badge">输出 {model.output_format.join('/')}</span> : null}
                      {!['qwen_image', 'seedream_image'].includes(modelAccount.adapter_type) && model.cost_per_image ? <span className="a-badge">{model.cost_per_image} {model.currency}</span> : null}
                      {modelNeedsAttention(model) ? <span className="a-badge a-badge--warn"><i />待完善</span> : null}
                    </div>
                  </div>
                  <div className="aa-model__actions">
                    <button className="a-row-btn a-row-btn--primary" type="button" onClick={() => openModelDialog(editModelDraft(modelAccount, model))}>编辑</button>
                    <button className="a-row-btn" type="button" onClick={() => setTestDialog(newTestImageDialog(modelAccount, [model]))}>测试</button>
                    <button className="a-row-btn a-row-btn--danger" type="button" aria-label={`删除真实模型 ${model.model_code}`} onClick={() => { setMutationError(null); setDeleteTarget({ kind: 'model', account: modelAccount, model }) }}>删除</button>
                  </div>
                </article>
              ))}
            </div>
          )}
          <div className="a-banner a-banner--info">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.6" aria-hidden="true"><circle cx="12" cy="12" r="9" /><path d="M12 11v5M12 7.5h.01" /></svg>
            <div><strong style={{ fontSize: 13 }}>图片模型能力会参与路由与估价校验</strong><div className="a-hint">任务类型、基础分辨率、质量、尺寸模式、比例、像素尺寸、背景、压缩与输出格式缺失时，对应能力会被阻断。</div></div>
          </div>
        </Drawer>
      ) : null}

      {modelDialog ? (
        <Drawer
          stacked={Boolean(modelAccount)}
          wide
          title="图片真实模型配置"
          description={`${modelDialog.account.name} · ${modelDialog.displayName || '新增图片模型'}${modelDialog.modelCode ? ` · ${modelDialog.modelCode}` : ''}`}
          headerAside={<span className={cn('a-badge', modelDialog.enabled ? 'a-badge--ok' : 'a-badge--warn')}><i />{modelDialog.enabled ? '可用' : '停用'}</span>}
          onClose={closeModelDialog}
          footer={<>
            <span className={cn('a-badge', Object.keys(modelValidation).length ? 'a-badge--bad' : modelDirty ? 'a-badge--warn' : 'a-badge--ok')}><i />{Object.keys(modelValidation).length ? `${Object.keys(modelValidation).length} 项校验未通过` : modelDirty ? `${modelDirty} 项未保存` : '未修改'}</span>
            <div style={{ flex: 1 }} />
            <Button variant="ghost" size="sm" disabled={saving} onClick={closeModelDialog}>取消</Button>
            <Button variant="primary" size="sm" disabled={saving || Boolean(modelDialog.extraError) || Object.keys(modelValidation).length > 0} onClick={() => void saveModel()}>{saving ? '保存中...' : '保存模型'}</Button>
          </>}
        >
          {mutationError ? <InlineFeedback tone="danger" message={mutationError} /> : null}
          <form className="a-form" onSubmit={(event) => event.preventDefault()}>
            <FormSection index={1} title="模型标识" description="图片模型的任务类型只有文生图与图片编辑。">
              {['qwen_image', 'seedream_image'].includes(modelDialog.account.adapter_type) ? <LedgerField label="精确型号预设"><select className="a-select" value={String((modelDialog.extra?.native_image as Record<string, unknown> | undefined)?.model ?? '')} onChange={(event) => { const profile = nativeImageProfile(event.target.value); if (!profile) return; setModelDialog({ ...modelDialog, modelCode: profile.model, displayName: profile.model, taskTypes: ['text_to_image', 'image_edit'], maxReferenceImageCount: String(profile.max_reference_images), maxImageCount: String(profile.max_output_images), sizeModes: profile.adapter === 'qwen_image' ? ['auto', 'pixel'] : ['pixel'], supportedPixelSizes: profile.adapter === 'qwen_image' ? ['1024x1024', '1536x1024', '1024x1536', '2048x2048'] : ['1024x1024', '1536x1536', '2048x2048'], supportsCustomSize: true, minWidth: '16', minHeight: '16', maxWidth: '3840', maxHeight: '3840', supportedBackgrounds: [], quality: ['auto'], outputFormat: profile.output_formats, currency: 'CNY', enabled: false, extra: { ...modelDialog.extra, native_image: configValue(switchNativePreset(modelDialog.extra, profile), ['native_image']) } }) }}><option value="">选择型号（Endpoint ID 可随后填写）</option>{nativeImagePresets.filter((code) => nativeImageProfile(code)?.adapter === modelDialog.account.adapter_type).map((code) => <option key={code} value={code}>{code}</option>)}</select><small>切换实际型号会清除旧成本及拆层成本确认（旧 sale 扩展保留）；应用后默认禁用，请重新配置计费。Seedream 可将模型代码替换为真实 Endpoint ID，预设能力保留。</small></LedgerField> : null}
              <LedgerField label="模型代码" required error={modelValidation.modelCode}><input className="a-input" value={modelDialog.modelCode} onChange={(event) => setModelDialog({ ...modelDialog, modelCode: event.target.value })} placeholder="gpt-image-2" style={{ fontFamily: 'var(--font-admin-mono)' }} /></LedgerField>
              <LedgerField label="展示名称" error={modelValidation.displayName}><input className="a-input" value={modelDialog.displayName} onChange={(event) => setModelDialog({ ...modelDialog, displayName: event.target.value })} /></LedgerField>
              <LedgerField label="任务类型" required error={modelValidation.taskTypes}>
                <div className="cfg-checklist">
                  {adminTaskTypeOptions.map((option) => (
                    <label key={option.value} className="a-switch">
                      <input type="checkbox" checked={modelDialog.taskTypes.includes(option.value)} onChange={(event) => setModelDialog({ ...modelDialog, taskTypes: event.target.checked ? [...modelDialog.taskTypes, option.value] : modelDialog.taskTypes.filter((item) => item !== option.value) })} />
                      <span className="a-switch__track" />
                      <span>{option.label} <span className="a-hint">{option.value}</span></span>
                    </label>
                  ))}
                </div>
              </LedgerField>
              <SwitchField label="启用该模型" checked={modelDialog.enabled} onChange={(enabled) => setModelDialog({ ...modelDialog, enabled })} />
            </FormSection>

            <FormSection index={2} title="模型能力 · 尺寸" description="基础分辨率、质量、尺寸模式与像素尺寸共同决定用户可选参数。">
              <MultiSelectField label="基础分辨率" values={modelDialog.base_resolution} options={baseResolutionOptions} onChange={(base_resolution) => setModelDialog({ ...modelDialog, base_resolution })} customPlaceholder="自定义分辨率，如 8K" hint="用于路由价格档位与尺寸计算。" error={modelValidation.base_resolution} />
              <MultiSelectField label="质量参数" values={modelDialog.quality} options={qualityOptions} onChange={(quality) => setModelDialog({ ...modelDialog, quality })} customPlaceholder="自定义质量值" normalize={normalizeLowerEnum} />
              <ChecklistField label="尺寸模式" values={modelDialog.sizeModes} options={sizeModeOptions} onChange={(sizeModes) => setModelDialog(toggleSizeModes(modelDialog, sizeModes))} />
              {modelDialog.sizeModes.includes('ratio') ? (
                <>
                  <MultiSelectField label="支持比例" values={modelDialog.supportedRatios} options={defaultRatios} onChange={(supportedRatios) => setModelDialog({ ...modelDialog, supportedRatios })} customPlaceholder="自定义比例，如 3:2" normalize={normalizeRatio} error={modelValidation.supportedRatios} />
                  <SwitchField label="允许用户自定义比例" checked={modelDialog.supportsCustomRatio} onChange={(supportsCustomRatio) => setModelDialog({ ...modelDialog, supportsCustomRatio })} />
                </>
              ) : null}
              {modelDialog.sizeModes.includes('pixel') ? (
                <>
                  <MultiSelectField label="支持像素尺寸" values={modelDialog.supportedPixelSizes} options={defaultPixelSizes} onChange={(supportedPixelSizes) => setModelDialog({ ...modelDialog, supportedPixelSizes })} customPlaceholder="自定义像素，如 2048x2048" normalize={normalizePixelSize} error={modelValidation.supportedPixelSizes} />
                  <SwitchField label="允许用户自定义尺寸" checked={modelDialog.supportsCustomSize} onChange={(supportsCustomSize) => setModelDialog({ ...modelDialog, supportsCustomSize })} hint="启用后，用户输入的宽度和高度必须满足下方范围。" />
                  <div className="a-form__pair">
                    <LedgerField label="最小宽度" error={modelValidation.minWidth}><input className="a-input" type="number" min="16" max="3840" step="16" value={modelDialog.minWidth} onChange={(event) => setModelDialog({ ...modelDialog, minWidth: event.target.value })} /></LedgerField>
                    <LedgerField label="最大宽度" error={modelValidation.maxWidth}><input className="a-input" type="number" min="16" max="3840" step="16" value={modelDialog.maxWidth} onChange={(event) => setModelDialog({ ...modelDialog, maxWidth: event.target.value })} /></LedgerField>
                  </div>
                  <div className="a-form__pair">
                    <LedgerField label="最小高度" error={modelValidation.minHeight}><input className="a-input" type="number" min="16" max="3840" step="16" value={modelDialog.minHeight} onChange={(event) => setModelDialog({ ...modelDialog, minHeight: event.target.value })} /></LedgerField>
                    <LedgerField label="最大高度" error={modelValidation.maxHeight}><input className="a-input" type="number" min="16" max="3840" step="16" value={modelDialog.maxHeight} onChange={(event) => setModelDialog({ ...modelDialog, maxHeight: event.target.value })} /></LedgerField>
                  </div>
                </>
              ) : null}
            </FormSection>

            <FormSection index={3} title="模型能力 · 输出与审核">
              <MultiSelectField label="输出格式" values={modelDialog.outputFormat} options={outputFormatOptions} onChange={(outputFormat) => setModelDialog({ ...modelDialog, outputFormat })} customPlaceholder="自定义格式" normalize={normalizeLowerEnum} />
              <MultiSelectField label="支持背景" values={modelDialog.supportedBackgrounds} options={backgroundOptions} onChange={(supportedBackgrounds) => setModelDialog({ ...modelDialog, supportedBackgrounds })} customPlaceholder="自定义背景值" normalize={normalizeLowerEnum} />
              <SwitchField label="支持输出压缩质量" checked={modelDialog.supportsOutputCompression} onChange={(supportsOutputCompression) => setModelDialog({ ...modelDialog, supportsOutputCompression })} hint="仅在 JPEG / WebP 输出格式下开放 1–100 压缩质量。" />
              <MultiSelectField label="审核等级" values={modelDialog.moderation} options={moderationOptions} onChange={(moderation) => setModelDialog({ ...modelDialog, moderation })} customPlaceholder="自定义审核等级" normalize={normalizeLowerEnum} />
              <div className="a-form__pair">
                <LedgerField label="最大出图数" error={modelValidation.maxImageCount}><input className="a-input" type="number" min="1" max="10" value={modelDialog.maxImageCount} onChange={(event) => setModelDialog({ ...modelDialog, maxImageCount: event.target.value })} /></LedgerField>
                <LedgerField label="最大参考图" error={modelValidation.maxReferenceImageCount}><input className="a-input" type="number" min="0" max="64" value={modelDialog.maxReferenceImageCount} onChange={(event) => setModelDialog({ ...modelDialog, maxReferenceImageCount: event.target.value })} /></LedgerField>
              </div>
            </FormSection>

            <ImageConfigurationFields extra={modelDialog.extra} inherited={modelDialog.account.extra} adapter={modelDialog.account.adapter_type} profile={effectiveNativeProfile(modelDialog.extra, modelDialog.account.extra, modelDialog.modelCode)} onChange={(extra) => setModelDialog({ ...modelDialog, extra })} />
            {['qwen_image', 'seedream_image'].includes(modelDialog.account.adapter_type) ? <FormSection index={4} title="厂商原生成本" description="此处仅维护厂商人民币成本；启用需要完整成本。统一销售价格在路由模型配置，成本不能代替售价。">
              {(['pricing'] as const).map((field) => {
                const local = configValue(modelDialog.extra, ['native_image', field])
                const inherited = configValue(modelDialog.account.extra, ['native_image', field])
                const profile = effectiveNativeProfile(modelDialog.extra, modelDialog.account.extra, modelDialog.modelCode)
                return <div key={field}>
                  <p className="a-hint">{local === undefined ? '继承账号价格；编辑任一价格字段将创建本模型覆盖。' : local === null ? '当前为显式 null；编辑后建立价格覆盖。' : '本模型价格覆盖。'}</p>
                  <NativeImagePricingFields label="厂商原生成本" value={(local === undefined ? inherited : local) as NativeImagePricing | undefined} model={profile?.model ?? ''} profile={profile} enabled={modelDialog.enabled} onChange={(value) => setModelDialog({ ...modelDialog, extra: patchConfig(modelDialog.extra, ['native_image', field], value) })} />
                  {local !== undefined ? <button type="button" className="a-btn a-btn--sm" onClick={() => { if (window.confirm('移除本模型价格覆盖并继承账号价格？')) setModelDialog({ ...modelDialog, extra: patchConfig(modelDialog.extra, ['native_image', field], undefined) }) }}>恢复价格继承</button> : null}
                </div>
              })}
              <a className="a-btn a-btn--sm" href="#/routing?media=image">前往路由配置统一图片售价</a>
              {modelValidation.native ? <p className="a-field__error" role="alert">{modelValidation.native}</p> : null}
            </FormSection> : <FormSection index={4} title="成本">
              <div className="a-form__pair">
                <LedgerField label="单图成本" error={modelValidation.costPerImage}><input className="a-input" inputMode="decimal" value={modelDialog.costPerImage} onChange={(event) => setModelDialog({ ...modelDialog, costPerImage: event.target.value })} /></LedgerField>
                <LedgerField label="币种" error={modelValidation.currency}><input className="a-input" value={modelDialog.currency} onChange={(event) => setModelDialog({ ...modelDialog, currency: event.target.value })} placeholder="CNY" /></LedgerField>
              </div>
            </FormSection>}
          </form>
        </Drawer>
      ) : null}

      {testDialog ? (
        <Drawer
          title="测试图片模型"
          description={`${testDialog.account.name} · 使用真实账号发起一次上游请求`}
          onClose={() => setTestDialog(null)}
          footer={<><Button variant="ghost" size="sm" disabled={testing} onClick={() => setTestDialog(null)}>关闭</Button><div style={{ flex: 1 }} /><Button variant="primary" size="sm" disabled={testing || !testDialog.modelId || !testDialog.prompt.trim()} onClick={() => void runTestImage()}>{testing ? '测试中...' : '开始测试'}</Button></>}
        >
          <form className="a-form" onSubmit={(event) => event.preventDefault()}>
            <FormSection title="测试参数" description="测试不会创建用户任务，也不会扣减积分。">
              <LedgerField label="测试模型"><select className="a-select" value={testDialog.modelId} onChange={(event) => { const selected = (modelsByAccount[String(testDialog.account.id)] ?? []).find((model) => String(model.id) === event.target.value); if (selected) setTestDialog(rebuildTestImageDialog(testDialog, selected)) }}>{(modelsByAccount[String(testDialog.account.id)] ?? []).filter((model) => model.enabled).map((model) => <option key={String(model.id)} value={String(model.id)}>{model.display_name || model.model_code}</option>)}</select></LedgerField>
              <LedgerField label="来源模式"><select className="a-select" value={testDialog.sourceMode} onChange={(event) => setTestDialog({ ...testDialog, sourceMode: event.target.value, result: undefined, error: undefined })}><option value="images">Images API</option><option value="codex_responses">Codex Responses</option></select></LedgerField>
              <LedgerField label="尺寸模式"><select className="a-select" value={testDialog.sizeMode} onChange={(event) => setTestDialog({ ...testDialog, sizeMode: event.target.value, result: undefined, error: undefined })}>{selectedTestSizeModes.map((option) => <option key={option} value={option}>{sizeModeLabel(option)}</option>)}</select></LedgerField>
              {testDialog.sizeMode === 'ratio' ? <div className="a-form__pair"><LedgerField label="比例">{selectedTestModel?.supports_custom_ratio ? <input className="a-input" list="test-model-ratios" value={testDialog.aspectRatio} onChange={(event) => setTestDialog({ ...testDialog, aspectRatio: event.target.value, result: undefined, error: undefined })} /> : <select className="a-select" value={testDialog.aspectRatio} onChange={(event) => setTestDialog({ ...testDialog, aspectRatio: event.target.value, result: undefined, error: undefined })}>{(selectedTestModel?.supported_ratios ?? []).map((option) => <option key={option} value={option}>{option}</option>)}</select>}<datalist id="test-model-ratios">{(selectedTestModel?.supported_ratios ?? []).map((option) => <option key={option} value={option} />)}</datalist></LedgerField><LedgerField label="基础分辨率"><select className="a-select" value={testDialog.baseResolution} onChange={(event) => setTestDialog({ ...testDialog, baseResolution: event.target.value, result: undefined, error: undefined })}>{normalizeBaseResolution(selectedTestModel?.base_resolution ?? []).map((option) => <option key={option} value={option}>{option}</option>)}</select></LedgerField></div> : testDialog.sizeMode === 'pixel' ? <LedgerField label="像素尺寸">{selectedTestModel?.supports_custom_size ? <><input className="a-input" list="test-model-pixel-sizes" value={testDialog.requestedSize} onChange={(event) => setTestDialog({ ...testDialog, requestedSize: event.target.value, result: undefined, error: undefined })} /><datalist id="test-model-pixel-sizes">{(selectedTestModel?.supported_pixel_sizes ?? []).map((option) => <option key={option} value={option} />)}</datalist></> : <select className="a-select" value={testDialog.requestedSize} onChange={(event) => setTestDialog({ ...testDialog, requestedSize: event.target.value, result: undefined, error: undefined })}>{(selectedTestModel?.supported_pixel_sizes ?? []).map((option) => <option key={option} value={option}>{option}</option>)}</select>}</LedgerField> : null}
              <div className="a-form__pair">
                <LedgerField label="质量参数"><select className="a-select" value={testDialog.quality} onChange={(event) => setTestDialog({ ...testDialog, quality: event.target.value, result: undefined, error: undefined })}>{selectedTestQualities.map((option) => <option key={option} value={option}>{option}</option>)}</select></LedgerField>
                <LedgerField label="输出格式"><select className="a-select" value={testDialog.outputFormat} onChange={(event) => setTestDialog({ ...testDialog, outputFormat: event.target.value, result: undefined, error: undefined })}>{selectedTestFormats.map((option) => <option key={option} value={option}>{option}</option>)}</select></LedgerField>
              </div>
              <div className="a-form__pair">
                <LedgerField label="背景"><select className="a-select" value={testDialog.background} onChange={(event) => setTestDialog(changeTestImageBackground(testDialog, event.target.value, selectedTestFormats))}><option value="">不传</option>{selectedTestBackgrounds.map((option) => <option key={option} value={option}>{option}</option>)}</select></LedgerField>
                {selectedTestModel?.supports_output_compression && (testDialog.outputFormat === 'jpeg' || testDialog.outputFormat === 'webp') ? <LedgerField label="压缩质量"><input className="a-input" type="number" min="1" max="100" value={testDialog.outputCompression} onChange={(event) => setTestDialog({ ...testDialog, outputCompression: event.target.value, result: undefined, error: undefined })} /></LedgerField> : null}
              </div>
              <LedgerField label="审核等级"><select className="a-select" value={testDialog.moderation} onChange={(event) => setTestDialog({ ...testDialog, moderation: event.target.value, result: undefined, error: undefined })}>{selectedTestModeration.map((option) => <option key={option} value={option}>{option}</option>)}</select></LedgerField>
              <LedgerField label="提示词"><textarea className="a-textarea" value={testDialog.prompt} onChange={(event) => setTestDialog({ ...testDialog, prompt: event.target.value, result: undefined, error: undefined })} rows={4} /></LedgerField>
            </FormSection>
            {testing ? (
              <section className="cfg-test-media" aria-live="polite" aria-busy="true">
                <div className="a-skeleton cfg-test-media__frame" />
                <div className="cfg-test-media__status">
                  <div className="cfg-test-progress" role="progressbar" aria-label="正在生成测试图片"><i /></div>
                  <strong>正在请求真实上游模型...</strong>
                  <span className="a-hint">这通常需要数秒，完成后会在此显示图片结果。</span>
                </div>
              </section>
            ) : testDialog.error ? (
              <InlineFeedback tone="danger" message={testDialog.error} />
            ) : testDialog.result ? (
              <section className="cfg-test-media">
                {testDialog.result.image_url ? (
                  <img className="cfg-test-media__image" src={adminApi.modelAccountTestImageUrl(testDialog.result.image_url, accessToken)} alt="测试图片结果" />
                ) : (
                  <div className="cfg-test-media__missing">上游未返回图片结果</div>
                )}
                <div className="cfg-test-meta">
                  <Badge tone="success">{testDialog.result.status}</Badge>
                  <Badge>{testDialog.result.width ?? 0} × {testDialog.result.height ?? 0}</Badge>
                  <Badge>{testDialog.result.elapsed_ms} ms</Badge>
                  {testDialog.result.provider_request_id ? <Badge>请求 {testDialog.result.provider_request_id}</Badge> : null}
                </div>
                {testActualParams.length ? (
                  <div className="cfg-test-params">
                    {testActualParams.map(([key, value]) => <span key={key} className="a-badge">{key}: {value}</span>)}
                  </div>
                ) : null}
              </section>
            ) : (
              <section className="cfg-test-media__idle">
                <ImageIcon aria-hidden="true" />
                <strong>尚无测试结果</strong>
                <span className="a-hint">填写参数后开始测试，图片会显示在这里。</span>
              </section>
            )}
          </form>
        </Drawer>
      ) : null}

      {deleteTarget ? (
        <Modal
          title={deleteTarget.kind === 'account' ? '删除接入账号' : '删除真实模型'}
          detail={deleteTarget.kind === 'account' ? deleteTarget.account.name : `${deleteTarget.account.name} / ${deleteTarget.model.model_code}`}
          onClose={() => { if (!deleting) setDeleteTarget(null) }}
          footer={<><Button variant="ghost" disabled={deleting} onClick={() => setDeleteTarget(null)}>取消</Button><Button variant="danger" disabled={deleting} onClick={() => void confirmDelete()}>{deleting ? '删除中...' : '确认删除'}</Button></>}
        >
          {mutationError ? <InlineFeedback tone="danger" message={mutationError} /> : null}
          <p className="m-0 text-sm leading-6 text-[var(--a-ink-2)]">删除后该配置不再参与新请求，历史任务仍按已保存的模型快照展示。</p>
        </Modal>
      ) : null}
    </section>
  )
}

function accountColumns({ modelsByAccount, onEdit, onConfigureModels, onTest, onDelete }: {
  modelsByAccount: Record<string, ModelAccountModel[]>
  onEdit: (account: ModelAccount) => void
  onConfigureModels: (account: ModelAccount) => void
  onTest: (account: ModelAccount) => void
  onDelete: (account: ModelAccount) => void
}): ColumnDef<ModelAccount>[] {
  return [
    {
      key: 'account',
      title: '账号',
      width: 'minmax(220px,2fr)',
      render: (account) => (
        <span className="cfg-account-cell">
          <span className="cfg-account-cell__logo">{account.name.slice(0, 1).toUpperCase()}</span>
          <span className="a-cell-stack"><strong>{account.name}</strong><span>{providerAdapterLabel(account.adapter_type)} · {accountsSecondaryLabel(account)}</span></span>
        </span>
      ),
    },
    {
      key: 'adapter',
      title: '接入方式',
      width: 'minmax(140px,1fr)',
      render: (account) => <span className="a-cell-stack"><strong>{providerAdapterLabel(account.adapter_type)}</strong><span>{providerAuthLabel(account.auth_type)}</span></span>,
    },
    {
      key: 'baseUrl',
      title: 'Base URL',
      width: 'minmax(220px,2fr)',
      kind: 'code',
      render: (account) => <code className="a-mono text-[11.5px]" title={account.base_url}>{account.base_url}</code>,
    },
    {
      key: 'credentials',
      title: '密钥',
      width: 'minmax(100px,.8fr)',
      render: (account) => <Badge tone={account.credentials_status?.has_api_key ? 'success' : 'danger'}>{credentialsStatusLabel(account.credentials_status?.has_api_key)}</Badge>,
    },
    {
      key: 'models',
      title: '真实模型',
      width: 'minmax(90px,.7fr)',
      align: 'right',
      kind: 'number',
      render: (account) => <span className="a-num">{modelsByAccount[String(account.id)]?.length ?? 0}</span>,
    },
    {
      key: 'concurrency',
      title: '并发 / 超时',
      width: 'minmax(110px,.8fr)',
      align: 'right',
      kind: 'number',
      render: (account) => <span className="a-num">{account.concurrency_limit} / {Math.round(account.timeout_ms / 1000)}s</span>,
    },
    {
      key: 'status',
      title: '状态',
      width: 'minmax(90px,.7fr)',
      render: (account) => <Badge tone={modelAccountStatusTone(account.status)}>{modelAccountStatusLabel(account.status)}</Badge>,
    },
    {
      key: 'actions',
      title: '操作',
      width: 'minmax(220px,1.4fr)',
      align: 'right',
      render: (account) => (
        <span className="a-row-actions">
          <button className="a-row-btn" type="button" onClick={() => onEdit(account)}>编辑</button>
          <button className="a-row-btn a-row-btn--primary" type="button" onClick={() => onConfigureModels(account)}>{accountPrimaryActionLabel}</button>
          <button className="a-row-btn" type="button" onClick={() => onTest(account)}>测试</button>
          <button className="a-icon-btn" style={{ width: 32, height: 32 }} type="button" aria-label={`删除账号 ${account.name}`} title="删除账号" onClick={() => onDelete(account)}><Trash2 className="size-4" /></button>
        </span>
      ),
    },
  ]
}

function accountsSecondaryLabel(account: ModelAccount) {
  if (account.status === 'error') return '异常 · 查看账号配置'
  if (account.status === 'disabled') return '停用'
  return account.extra?.source_mode === 'codex_responses' ? 'Codex Responses' : '主账号'
}

function accountStatusTone(status: string) {
  if (status === 'enabled') return 'a-badge--ok'
  if (status === 'error') return 'a-badge--bad'
  return 'a-badge--warn'
}

function editAccountDraft(row: ModelAccount): AccountDraft {
  return { id: row.id, name: row.name, adapterType: row.adapter_type, authType: row.auth_type, baseUrl: row.base_url, apiKey: '', priority: String(row.priority), weight: String(row.weight), concurrencyLimit: String(row.concurrency_limit), timeoutMS: String(row.timeout_ms), status: row.status, sourceMode: sourceModeFromExtra(row.extra), extra: row.extra }
}

function newModelDraft(account: ModelAccount): ModelDraft {
  return { account, modelCode: '', displayName: '', taskTypes: ['text_to_image'], base_resolution: ['1K', '2K'], quality: ['auto'], maxReferenceImageCount: '5', maxImageCount: '1', sizeModes: ['ratio'], supportedRatios: defaultRatios, supportsCustomRatio: false, supportedPixelSizes: defaultPixelSizes, supportsCustomSize: false, minWidth: '512', maxWidth: '3840', minHeight: '512', maxHeight: '3840', supportedBackgrounds: ['auto'], outputFormat: ['png'], supportsOutputCompression: false, moderation: ['auto'], costPerImage: '0.00000', currency: 'USD', enabled: false }
}

function editModelDraft(account: ModelAccount, row: ModelAccountModel): ModelDraft {
  return { account, row, extra: row.extra, modelCode: row.model_code, displayName: row.display_name, taskTypes: row.task_types, base_resolution: normalizeBaseResolution(row.base_resolution), quality: normalizeLowerEnums(row.quality ?? ['auto']), maxReferenceImageCount: String(row.max_reference_image_count ?? 5), maxImageCount: String(row.max_image_count ?? 1), sizeModes: row.size_modes?.length ? row.size_modes : ['ratio'], supportedRatios: row.supported_ratios?.length ? row.supported_ratios : defaultRatios, supportsCustomRatio: Boolean(row.supports_custom_ratio), supportedPixelSizes: row.supported_pixel_sizes?.length ? row.supported_pixel_sizes : defaultPixelSizes, supportsCustomSize: Boolean(row.supports_custom_size), minWidth: String(row.min_width ?? 512), maxWidth: String(row.max_width ?? 3840), minHeight: String(row.min_height ?? 512), maxHeight: String(row.max_height ?? 3840), supportedBackgrounds: normalizeLowerEnums(row.supported_backgrounds ?? []), outputFormat: normalizeLowerEnums(row.output_format ?? ['png']), supportsOutputCompression: Boolean(row.supports_output_compression), moderation: normalizeLowerEnums(row.moderation ?? ['auto']), costPerImage: row.cost_per_image, currency: row.currency, enabled: row.enabled }
}

function modelPayload(draft: ModelDraft) {
  const extra: Record<string, unknown> = { ...draft.extra, media_type: 'image' }
  return {
    extra,
    model_code: draft.modelCode,
    display_name: draft.displayName,
    task_types: draft.taskTypes,
    base_resolution: draft.base_resolution,
    quality: draft.quality,
    max_reference_image_count: Number(draft.maxReferenceImageCount),
    max_image_count: Number(draft.maxImageCount),
    size_modes: draft.sizeModes,
    supported_ratios: draft.supportedRatios,
    supported_pixel_sizes: draft.supportedPixelSizes,
    supports_custom_ratio: draft.supportsCustomRatio,
    supported_backgrounds: draft.supportedBackgrounds,
    supports_custom_size: draft.supportsCustomSize,
    min_width: Number(draft.minWidth),
    max_width: Number(draft.maxWidth),
    min_height: Number(draft.minHeight),
    max_height: Number(draft.maxHeight),
    output_format: draft.outputFormat,
    supports_output_compression: draft.supportsOutputCompression,
    moderation: draft.moderation,
    cost_per_image: draft.costPerImage,
    currency: draft.currency,
    enabled: draft.enabled,
  }
}

function newTestImageDialog(account: ModelAccount, models: ModelAccountModel[]): TestImageDialog {
  const enabledModels = models.filter((model) => model.enabled)
  const selected = enabledModels.find((model) => model.model_code === 'gpt-image-2') ?? enabledModels[0] ?? models[0]
  return buildTestImageDialog(account, selected, defaultTestPrompt, sourceModeFromExtra(account.extra))
}

function rebuildTestImageDialog(current: TestImageDialog, selected: ModelAccountModel): TestImageDialog {
  return buildTestImageDialog(current.account, selected, current.prompt, current.sourceMode)
}

function buildTestImageDialog(account: ModelAccount, selected: ModelAccountModel | undefined, prompt: string, sourceMode: string): TestImageDialog {
  const modes = selected?.size_modes ?? []
  const sizeMode = modes.includes('auto') ? 'auto' : modes.includes('ratio') ? 'ratio' : 'pixel'
  const background = selected?.supported_backgrounds?.[0] ?? ''
  const configuredFormats = normalizeLowerEnums(selected?.output_format ?? ['png'])
  const outputFormat = background === 'transparent'
    ? configuredFormats.find((format) => format === 'png' || format === 'webp') ?? 'png'
    : configuredFormats[0] ?? 'png'
  return { account, modelId: selected ? String(selected.id) : '', prompt, sourceMode, sizeMode, requestedSize: selected?.supported_pixel_sizes?.[0] ?? '1024x1024', baseResolution: normalizeBaseResolution(selected?.base_resolution ?? ['1K'])[0] ?? '1K', quality: selected?.quality?.[0] ?? 'auto', outputFormat, background, outputCompression: String(selected?.output_compression ?? 100), moderation: selected?.moderation?.[0] ?? 'auto', aspectRatio: selected?.supported_ratios?.[0] ?? '1:1' }
}

function testModelSizeModes(model?: ModelAccountModel) {
  const modes = (model?.size_modes ?? []).filter((mode) => mode === 'auto' || mode === 'ratio' || mode === 'pixel')
  return modes.length ? modes : ['ratio']
}

function testModelOptions(values: string[] | undefined, fallback: string[]) {
  const options = normalizeLowerEnums(values ?? [])
  return options.length ? options : fallback
}

function sizeModeLabel(mode: string) {
  if (mode === 'auto') return '自动'
  if (mode === 'pixel') return '按像素'
  return '按比例'
}

function changeTestImageBackground(dialog: TestImageDialog, background: string, availableFormats: string[]): TestImageDialog {
  const outputFormat = background === 'transparent' && dialog.outputFormat !== 'png' && dialog.outputFormat !== 'webp'
    ? availableFormats.find((format) => format === 'png' || format === 'webp') ?? 'png'
    : dialog.outputFormat
  return { ...dialog, background, outputFormat, result: undefined, error: undefined }
}

function toggleSizeModes(draft: ModelDraft, next: string[]): ModelDraft {
  const sizeModes = next.length ? next : ['auto']
  return {
    ...draft,
    sizeModes,
    supportsCustomSize: sizeModes.includes('pixel') ? draft.supportsCustomSize : false,
    supportsCustomRatio: sizeModes.includes('ratio') ? draft.supportsCustomRatio : false,
  }
}

function validateAccountDraft(draft: AccountDraft): Record<string, string> {
  const errors: Record<string, string> = {}
  if (!draft.name.trim()) errors.name = '请填写账号名称。'
  if (!draft.baseUrl.trim()) errors.baseUrl = '请填写 Base URL。'
  else if (!/^https?:\/\//i.test(draft.baseUrl.trim())) errors.baseUrl = 'Base URL 必须以 http:// 或 https:// 开头。'
  if (!Number.isFinite(Number(draft.priority)) || Number(draft.priority) < 0) errors.priority = '优先级必须是非负整数。'
  if (!Number.isFinite(Number(draft.weight)) || Number(draft.weight) < 0) errors.weight = '权重必须是非负整数。'
  if (!Number.isInteger(Number(draft.concurrencyLimit)) || Number(draft.concurrencyLimit) < 1) errors.concurrencyLimit = '并发上限必须大于 0。'
  if (!Number.isInteger(Number(draft.timeoutMS)) || Number(draft.timeoutMS) < 1000) errors.timeoutMS = '超时至少为 1000 毫秒。'
  return errors
}

function accountDraftDirtyCount(draft: AccountDraft, existing?: ModelAccount): number {
  const baseline = existing ? editAccountDraft(existing) : blankAccount
  const fields: Array<[string, string]> = [
    ['name', draft.name], ['adapterType', draft.adapterType], ['authType', draft.authType], ['baseUrl', draft.baseUrl],
    ['priority', draft.priority], ['weight', draft.weight], ['concurrencyLimit', draft.concurrencyLimit], ['timeoutMS', draft.timeoutMS],
    ['status', draft.status], ['sourceMode', draft.sourceMode],
  ]
  let count = fields.filter(([key, value]) => String((baseline as unknown as Record<string, unknown>)[key] ?? '') !== String(value)).length
  if (JSON.stringify(draft.extra) !== JSON.stringify(baseline.extra)) count += 1
  if (draft.apiKey) count += 1
  return count
}

function validateModelDraft(draft: ModelDraft): Record<string, string> {
  const errors: Record<string, string> = {}
  if (!draft.modelCode.trim()) errors.modelCode = '请填写模型代码。'
  if (!draft.taskTypes.length) errors.taskTypes = '至少选择一种图片任务类型。'
  if (!draft.base_resolution.length) errors.base_resolution = '至少配置一个基础分辨率。'
  if (draft.sizeModes.includes('ratio') && !draft.supportedRatios.length) errors.supportedRatios = '按比例模式至少需要一种比例。'
  if (draft.sizeModes.includes('pixel') && !draft.supportedPixelSizes.length) errors.supportedPixelSizes = '按像素模式至少需要一个像素尺寸。'
  if (draft.sizeModes.includes('pixel')) {
    if (Number(draft.minWidth) < 16 || Number(draft.minWidth) > Number(draft.maxWidth)) errors.minWidth = '最小宽度需大于等于 16 且不超过最大宽度。'
    if (Number(draft.maxWidth) > 3840) errors.maxWidth = '最大宽度不能超过 3840。'
    if (Number(draft.minHeight) < 16 || Number(draft.minHeight) > Number(draft.maxHeight)) errors.minHeight = '最小高度需大于等于 16 且不超过最大高度。'
    if (Number(draft.maxHeight) > 3840) errors.maxHeight = '最大高度不能超过 3840。'
  }
  if (!Number.isInteger(Number(draft.maxImageCount)) || Number(draft.maxImageCount) < 1) errors.maxImageCount = '最大出图数至少为 1。'
  if (!Number.isInteger(Number(draft.maxReferenceImageCount)) || Number(draft.maxReferenceImageCount) < 0) errors.maxReferenceImageCount = '最大参考图不能为负数。'
  if (!['qwen_image', 'seedream_image'].includes(draft.account.adapter_type) && (!Number.isFinite(Number(draft.costPerImage)) || Number(draft.costPerImage) < 0)) errors.costPerImage = '单图成本必须是非负数字。'
  if (!draft.currency.trim()) errors.currency = '请填写币种。'
  if (['qwen_image', 'seedream_image'].includes(draft.account.adapter_type)) {
    const nativeErrors = nativeConfigurationErrors(draft.extra, draft.account.extra, draft.modelCode, draft.enabled)
    if (nativeErrors.length) errors.native = nativeErrors.join(' ')
  }
  return errors
}

function modelDraftDirtyCount(draft: ModelDraft): number {
  if (!draft.row) return modelPayload(draft).model_code ? 1 : 0
  const baseline = editModelDraft(draft.account, draft.row)
  return JSON.stringify(modelPayload(draft)) === JSON.stringify(modelPayload(baseline)) ? 0 : 1
}

function modelNeedsAttention(model: ModelAccountModel) {
  const hasBaseResolution = normalizeBaseResolution(model.base_resolution).length > 0
  const hasRatio = !model.size_modes?.includes('ratio') || Boolean(model.supported_ratios?.length)
  const hasPixel = !model.size_modes?.includes('pixel') || Boolean(model.supported_pixel_sizes?.length)
  return !hasBaseResolution || !hasRatio || !hasPixel
}

function normalizeBaseResolution(values?: string[] | null) {
  return (values ?? []).map(normalizeBaseResolutionValue).filter(Boolean)
}

function normalizeBaseResolutionValue(value: string) {
  const trimmed = value.trim()
  if (!trimmed) return ''
  if (trimmed.toLowerCase() === 'auto') return ''
  return trimmed.toUpperCase()
}

function normalizeLowerEnum(value: string) {
  return value.trim().toLowerCase()
}

function normalizeLowerEnums(values: string[]) {
  return values.map(normalizeLowerEnum).filter(Boolean)
}

function normalizeRatio(value: string) {
  const parts = value.trim().split(':').map((item) => Number(item))
  if (parts.length !== 2 || parts.some((item) => !Number.isFinite(item) || item <= 0)) return ''
  const divisor = gcd(parts[0], parts[1])
  return `${parts[0] / divisor}:${parts[1] / divisor}`
}

function normalizePixelSize(value: string) {
  const parts = value.trim().toLowerCase().replace('×', 'x').split('x').map((item) => Number(item))
  if (parts.length !== 2 || parts.some((item) => !Number.isInteger(item) || item <= 0)) return ''
  return `${parts[0]}x${parts[1]}`
}

function gcd(a: number, b: number): number {
  let left = Math.abs(Math.round(a))
  let right = Math.abs(Math.round(b))
  while (right) {
    const next = left % right
    left = right
    right = next
  }
  return left || 1
}

function sourceModeFromExtra(extra?: Record<string, unknown>) {
  const mode = String(extra?.source_mode ?? '').trim()
  if (mode === 'codex_responses') return 'codex_responses'
  if (extra?.gpt_image_2_codex_source === true) return 'codex_responses'
  return 'images'
}
