import { useEffect, useMemo, useState, type KeyboardEvent } from 'react'
import type { ImageTaskType, ModelAccount, ModelAccountModel, ModelAccountTestImageResult } from '../../../shared/api-types'
import { adminApi } from '../../../shared/admin-api'
import { cn } from '../../../shared/classnames'
import { Trash2 } from 'lucide-react'
import { ActionMenu, Badge, Drawer, EmptyBlock, ErrorBlock, Field, InlineFeedback, LoadingBlock, Modal, PageHeader, TooltipIconButton } from '../components'
import { adminButton, adminPage } from '../ui/classes'
import { adminDataGrid } from '../ui/dataGrid'
import { DataTable, FilterToolbar, ListPage, type ColumnDef } from '../ui/dataTable'
import { AccessAccountsIcon } from '../ui/icons'
import { adminTaskTypeOptions } from './adminTaskTypes'
import { modelLifecycleErrorMessage } from './adminModelLifecycle'
import {
  credentialsStatusLabel,
  modelAccountStatusLabel,
  modelAccountStatusTone,
  providerAccountDialogDetail,
  providerAdapterLabel,
  providerAuthLabel,
} from './providerModelRows'

type AccountDraft = { id?: string | number; name: string; adapterType: string; authType: string; baseUrl: string; apiKey: string; priority: string; weight: string; concurrencyLimit: string; timeoutMS: string; status: string; sourceMode: string }
type ModelDraft = { account: ModelAccount; row?: ModelAccountModel; modelCode: string; displayName: string; taskTypes: ImageTaskType[]; base_resolution: string[]; baseResolutionInput: string; quality: string[]; qualityInput: string; maxReferenceImageCount: string; maxImageCount: string; sizeModes: string[]; supportedRatios: string[]; ratioInput: string; supportsCustomRatio: boolean; supportedPixelSizes: string[]; pixelInput: string; supportsCustomSize: boolean; minWidth: string; maxWidth: string; minHeight: string; maxHeight: string; supportedBackgrounds: string[]; outputFormat: string[]; outputFormatInput: string; supportsOutputCompression: boolean; moderation: string[]; moderationInput: string; costPerImage: string; currency: string; enabled: boolean }
type TestImageDialog = { account: ModelAccount; modelId: string; prompt: string; sourceMode: string; sizeMode: string; requestedSize: string; baseResolution: string; quality: string; outputFormat: string; background: string; outputCompression: string; moderation: string; aspectRatio: string; result?: ModelAccountTestImageResult; error?: string }
type DeleteTarget = { kind: 'account'; account: ModelAccount } | { kind: 'model'; account: ModelAccount; model: ModelAccountModel }

const baseResolutionOptions = ['1K', '2K', '4K']
const qualityOptions = ['auto', 'low', 'medium', 'high']
const outputFormatOptions = ['png', 'jpeg', 'webp']
const backgroundOptions = ['auto', 'opaque', 'transparent']
const moderationOptions = ['auto', 'low']
const defaultRatios = ['1:1', '16:9', '9:16', '4:3', '3:4']
const defaultPixelSizes = ['1024x1024', '1536x1024', '1024x1536', '1280x720', '720x1280', '1024x768', '768x1024']
const blankAccount: AccountDraft = { name: '', adapterType: 'openai_compatible', authType: 'api_key', baseUrl: '', apiKey: '', priority: '1', weight: '100', concurrencyLimit: '5', timeoutMS: '120000', status: 'enabled', sourceMode: 'images' }
const defaultTestPrompt = 'A small product photo of a ceramic coffee cup on a clean desk'
const accountPrimaryActionLabel = '查看模型'
const accountTableClasses = {
  identity: 'flex min-w-0 items-center gap-3',
  icon: 'grid size-9 shrink-0 place-items-center rounded-lg bg-[var(--canvas)] text-[var(--accent)]',
  title: 'block truncate font-semibold text-[var(--text)]',
  detail: 'mt-0.5 block truncate text-xs text-[var(--soft)]',
  stack: 'grid gap-1',
  configCode: 'block max-w-[220px] truncate font-mono text-[11px] text-[var(--muted)]',
  configMeta: 'mt-0.5 block text-[11px] text-[var(--soft)]',
  actions: 'flex flex-wrap items-center justify-end gap-2',
  detailPanel: 'grid gap-3 rounded-lg bg-[var(--surface-solid)] p-3',
  detailHeader: 'flex flex-wrap items-center justify-between gap-3 border-b border-[var(--border)] pb-3',
  detailTitle: 'text-base font-semibold text-[var(--text)]',
  detailSummary: 'mt-1 text-xs text-[var(--soft)]',
  modelCode: 'font-mono text-[11px] text-[var(--soft)]',
  modelCapability: 'max-w-[34rem] text-xs leading-5 text-[var(--muted)]',
  machineValue: 'font-mono text-xs font-semibold text-[var(--text)]',
}
const tagInputClasses = {
  root: 'grid gap-2',
  list: 'flex flex-wrap gap-1.5',
  tag: 'inline-flex items-center gap-1.5 rounded-full border border-[var(--line)] bg-white/5 px-2 py-1 text-xs text-[var(--text)]',
  remove: 'ml-0.5 grid size-4 place-items-center rounded-full text-[var(--soft)] hover:bg-[rgba(184,95,84,.12)] hover:text-[var(--red)] focus-visible:bg-[rgba(184,95,84,.12)] focus-visible:text-[var(--red)] focus-visible:outline-none',
  inputRow: 'grid grid-cols-[minmax(0,1fr)_auto] gap-2',
}
const providerModelTaskTypeGridClass = 'grid max-h-[220px] gap-2 overflow-auto rounded-lg border border-[var(--line)] bg-white/[0.02] p-2'
const providerModelTaskTypeOptionClass = 'grid grid-cols-[auto_minmax(0,1fr)] items-center gap-2 rounded-md border border-[var(--line)] bg-white/5 p-2 text-sm has-[:checked]:border-[var(--accent)]/40 has-[:checked]:bg-[var(--accent)]/10'

export function ProviderModelsPage({ accessToken }: { accessToken?: string }) {
  const [accounts, setAccounts] = useState<ModelAccount[]>([])
  const [modelsByAccount, setModelsByAccount] = useState<Record<string, ModelAccountModel[]>>({})
  const [expandedAccountId, setExpandedAccountId] = useState<string>('')
  const [query, setQuery] = useState('')
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [accountDialog, setAccountDialog] = useState<AccountDraft | null>(null)
  const [modelDialog, setModelDialog] = useState<ModelDraft | null>(null)
  const [testDialog, setTestDialog] = useState<TestImageDialog | null>(null)
  const [saving, setSaving] = useState(false)
  const [testing, setTesting] = useState(false)
  const [mutationError, setMutationError] = useState<string | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<DeleteTarget | null>(null)
  const [deleting, setDeleting] = useState(false)

  const load = async (preferredAccountId?: string) => {
    setLoading(true)
    setError(null)
    try {
      const nextAccounts = await adminApi.listModelAccounts({ page_size: 100 })
      const modelPairs = await Promise.all(nextAccounts.map(async (account) => [String(account.id), await adminApi.listModelAccountModels(account.id)] as const))
      const nextModels = Object.fromEntries(modelPairs)
      setAccounts(nextAccounts)
      setModelsByAccount(nextModels)
      setExpandedAccountId((current) => {
        const preferred = preferredAccountId || current
        return nextAccounts.some((account) => String(account.id) === preferred) ? preferred : String(nextAccounts[0]?.id ?? '')
      })
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : '模型接入载入失败')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => { void load() }, [])

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

  useEffect(() => {
    if (filteredAccounts.length && !filteredAccounts.some((account) => String(account.id) === expandedAccountId)) {
      setExpandedAccountId(String(filteredAccounts[0].id))
    }
  }, [expandedAccountId, filteredAccounts])

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
        extra: { source_mode: accountDialog.sourceMode },
      }
      const creating = !accountDialog.id
      const saved = accountDialog.id ? await adminApi.updateModelAccount(accountDialog.id, payload) : await adminApi.createModelAccount(payload)
      setAccountDialog(null)
      await load(String(saved.id))
      if (creating) setModelDialog(newModelDraft(saved))
    } catch (caught) {
      setMutationError(caught instanceof Error ? caught.message : '模型账号保存失败')
    } finally {
      setSaving(false)
    }
  }

  async function saveModel() {
    if (!modelDialog) return
    setSaving(true)
    setMutationError(null)
    try {
      const payload = {
        model_code: modelDialog.modelCode,
        display_name: modelDialog.displayName,
        task_types: modelDialog.taskTypes,
        base_resolution: modelDialog.base_resolution,
        quality: modelDialog.quality,
        max_reference_image_count: Number(modelDialog.maxReferenceImageCount),
        max_image_count: Number(modelDialog.maxImageCount),
        size_modes: modelDialog.sizeModes,
		supported_ratios: modelDialog.supportedRatios,
		supported_pixel_sizes: modelDialog.supportedPixelSizes,
		supports_custom_ratio: modelDialog.supportsCustomRatio,
		supported_backgrounds: modelDialog.supportedBackgrounds,
		supports_custom_size: modelDialog.supportsCustomSize,
		min_width: Number(modelDialog.minWidth),
		max_width: Number(modelDialog.maxWidth),
		min_height: Number(modelDialog.minHeight),
		max_height: Number(modelDialog.maxHeight),
        output_format: modelDialog.outputFormat,
        supports_output_compression: modelDialog.supportsOutputCompression,
        moderation: modelDialog.moderation,
        cost_per_image: modelDialog.costPerImage,
        currency: modelDialog.currency,
        enabled: modelDialog.enabled,
      }
      if (modelDialog.row) await adminApi.updateModelAccountModel(modelDialog.account.id, modelDialog.row.id, payload)
      else await adminApi.createModelAccountModel(modelDialog.account.id, payload)
      setModelDialog(null)
      await load()
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
      setDeleteTarget(null)
      await load()
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

  if (loading) return <LoadingBlock label="载入模型接入" />
  if (error) return <ErrorBlock message={error} onRetry={load} />

  const selectedAccount = filteredAccounts.find((account) => String(account.id) === expandedAccountId)
  const selectedModels = selectedAccount ? modelsByAccount[String(selectedAccount.id)] ?? [] : []
	const selectedTestModel = testDialog
		? (modelsByAccount[String(testDialog.account.id)] ?? []).find((model) => String(model.id) === testDialog.modelId)
		: undefined
	const selectedTestSizeModes = testModelSizeModes(selectedTestModel)
	const selectedTestQualities = testModelOptions(selectedTestModel?.quality, ['auto'])
	const selectedTestFormats = testModelOptions(selectedTestModel?.output_format, ['png'])
		.filter((format) => testDialog?.background !== 'transparent' || format === 'png' || format === 'webp')
	const selectedTestBackgrounds = testModelOptions(selectedTestModel?.supported_backgrounds, [])
	const selectedTestModeration = testModelOptions(selectedTestModel?.moderation, ['auto'])

  return (
    <section className={adminPage.stack}>
      <PageHeader
        title="接入账号"
        description="管理上游模型账号、真实模型能力、健康测试和密钥轮换入口。"
        actions={<button className={cn(adminButton.base, adminButton.primary)} type="button" onClick={() => openAccountDialog(blankAccount)}>添加账号</button>}
      />
      {!accounts.length ? <EmptyBlock title="暂无模型接入账号" detail="创建账号后再添加真实上游模型。" /> : null}
      {accounts.length ? (
        <ListPage
          filters={(
            <FilterToolbar
              fields={[{
                key: 'query',
                label: '搜索账号',
                primary: true,
                minWidth: '220px',
                maxWidth: '420px',
                control: <input value={query} onChange={(event) => setQuery(event.target.value)} placeholder="账号名称 / 适配器 / Base URL" />,
              }]}
              resultSummary={`共 ${accounts.length} 个账号 · 当前显示 ${filteredAccounts.length} 个`}
            />
          )}
        >
          <DataTable
            columns={accountColumns({
              modelsByAccount,
              selectedAccountId: expandedAccountId,
              onSelect: (account) => setExpandedAccountId(String(account.id)),
              onEdit: (account) => openAccountDialog(editAccountDraft(account)),
              onAddModel: (account) => openModelDialog(newModelDraft(account)),
              onTest: (account) => setTestDialog(newTestImageDialog(account, modelsByAccount[String(account.id)] ?? [])),
              onDelete: (account) => { setMutationError(null); setDeleteTarget({ kind: 'account', account }) },
            })}
            rows={filteredAccounts}
            rowKey={(account) => account.id}
            empty={<EmptyBlock title="未找到接入账号" detail="换一个账号名称、适配器或 Base URL 关键词再试。" />}
          />
        </ListPage>
      ) : null}
      {selectedAccount ? (
        <section className={accountTableClasses.detailPanel} aria-label={`${selectedAccount.name} 真实模型`}>
          <header className={accountTableClasses.detailHeader}>
            <div>
              <h2 className={accountTableClasses.detailTitle}>{selectedAccount.name} · 真实模型</h2>
              <p className={accountTableClasses.detailSummary}>共 {selectedModels.length} 个模型；能力与成本在账号维度独立维护。</p>
            </div>
            <button className={cn(adminButton.base, adminButton.primary, adminButton.small)} type="button" onClick={() => openModelDialog(newModelDraft(selectedAccount))}>添加模型</button>
          </header>
          <DataTable
            columns={modelColumns(
              (model) => openModelDialog(editModelDraft(selectedAccount, model)),
              (model) => { setMutationError(null); setDeleteTarget({ kind: 'model', account: selectedAccount, model }) },
            )}
            rows={selectedModels}
            rowKey={(model) => model.id}
            empty={<EmptyBlock title="暂无真实模型" detail="为当前账号添加可请求的上游模型代码。" />}
          />
        </section>
      ) : null}
      {accountDialog ? (
        <Drawer
          title={accountDialog.id ? '编辑模型账号' : '新增模型账号'}
          description={providerAccountDialogDetail()}
          onClose={closeAccountDialog}
          footer={<><button className={cn(adminButton.base, adminButton.ghost)} type="button" disabled={saving} onClick={closeAccountDialog}>取消</button><button className={cn(adminButton.base, adminButton.primary)} type="button" disabled={saving || !accountDialog.name || !accountDialog.baseUrl} onClick={() => void saveAccount()}>{saving ? '保存中...' : '保存'}</button></>}
        >
          {mutationError ? <InlineFeedback tone="danger" message={mutationError} /> : null}
          <div className={adminPage.formGrid}>
            <Field label="账号名称"><input value={accountDialog.name} onChange={(event) => setAccountDialog({ ...accountDialog, name: event.target.value })} /></Field>
            <Field label="接入方式"><select value={accountDialog.adapterType} onChange={(event) => setAccountDialog({ ...accountDialog, adapterType: event.target.value })}><option value="openai_compatible">OpenAI 兼容</option><option value="openrouter">OpenRouter</option></select></Field>
            <Field label="鉴权方式"><select value={accountDialog.authType} onChange={(event) => setAccountDialog({ ...accountDialog, authType: event.target.value })}><option value="api_key">API Key</option></select></Field>
            <Field label="Base URL"><input value={accountDialog.baseUrl} onChange={(event) => setAccountDialog({ ...accountDialog, baseUrl: event.target.value })} placeholder="https://api.openai.com" /></Field>
            <Field label="API Key"><input type="password" value={accountDialog.apiKey} onChange={(event) => setAccountDialog({ ...accountDialog, apiKey: event.target.value })} placeholder={accountDialog.id ? '留空则保持原密钥' : 'sk-...'} /></Field>
            <Field label="状态"><select value={accountDialog.status} onChange={(event) => setAccountDialog({ ...accountDialog, status: event.target.value })}><option value="enabled">启用</option><option value="disabled">停用</option><option value="error">异常</option></select></Field>
            <Field label="gpt-image-2 来源模式" hint="Codex 来源会强制 quality=auto，并按基础分辨率与比例计算 size。"><select value={accountDialog.sourceMode} onChange={(event) => setAccountDialog({ ...accountDialog, sourceMode: event.target.value })}><option value="images">Images API</option><option value="codex_responses">Codex Responses</option></select></Field>
            <Field label="优先级" hint="数值越小越优先作为候选账号；同优先级时再看权重。"><input type="number" min="1" value={accountDialog.priority} onChange={(event) => setAccountDialog({ ...accountDialog, priority: event.target.value })} /></Field>
            <Field label="权重" hint="同优先级账号的分流权重，100 表示默认满权重。"><input type="number" min="0" value={accountDialog.weight} onChange={(event) => setAccountDialog({ ...accountDialog, weight: event.target.value })} /></Field>
            <Field label="并发限制" hint="该账号同时处理的最大请求数。"><input type="number" min="1" value={accountDialog.concurrencyLimit} onChange={(event) => setAccountDialog({ ...accountDialog, concurrencyLimit: event.target.value })} /></Field>
            <Field label="超时毫秒" hint="调用上游接口的单次请求超时时间。"><input type="number" min="1000" value={accountDialog.timeoutMS} onChange={(event) => setAccountDialog({ ...accountDialog, timeoutMS: event.target.value })} /></Field>
          </div>
        </Drawer>
      ) : null}
      {modelDialog ? (
        <Drawer
          title={modelDialog.row ? '编辑真实模型' : '新增真实模型'}
          description={modelDialog.account.name}
          onClose={closeModelDialog}
          footer={<><button className={cn(adminButton.base, adminButton.ghost)} type="button" disabled={saving} onClick={closeModelDialog}>取消</button><button className={cn(adminButton.base, adminButton.primary)} type="button" disabled={saving || !modelDialog.modelCode} onClick={() => void saveModel()}>{saving ? '保存中...' : '保存'}</button></>}
        >
          {mutationError ? <InlineFeedback tone="danger" message={mutationError} /> : null}
          <div className={adminPage.formGrid}>
            <Field label="模型代码"><input value={modelDialog.modelCode} onChange={(event) => setModelDialog({ ...modelDialog, modelCode: event.target.value })} placeholder="gpt-image-1" /></Field>
            <Field label="展示名称"><input value={modelDialog.displayName} onChange={(event) => setModelDialog({ ...modelDialog, displayName: event.target.value })} /></Field>
            <Field label="任务类型"><div className={providerModelTaskTypeGridClass}>{adminTaskTypeOptions.map((option) => <label key={option.value} className={providerModelTaskTypeOptionClass}><input type="checkbox" checked={modelDialog.taskTypes.includes(option.value)} onChange={(event) => setModelDialog({ ...modelDialog, taskTypes: event.target.checked ? [...modelDialog.taskTypes, option.value] : modelDialog.taskTypes.filter((item) => item !== option.value) })} /><span>{option.label}</span></label>)}</div></Field>
            <Field label="基础分辨率"><BaseResolutionTagInput draft={modelDialog} onChange={setModelDialog} /></Field>
            <Field label="质量参数"><TagInput values={modelDialog.quality} input={modelDialog.qualityInput} placeholder="auto / low / medium / high" options={qualityOptions} normalize={normalizeLowerEnum} onInput={(qualityInput) => setModelDialog({ ...modelDialog, qualityInput })} onChange={(quality, qualityInput = '') => setModelDialog({ ...modelDialog, quality, qualityInput })} /></Field>
            <Field label="最大参考图"><input type="number" min="0" max="64" value={modelDialog.maxReferenceImageCount} onChange={(event) => setModelDialog({ ...modelDialog, maxReferenceImageCount: event.target.value })} /></Field>
			<Field label="最大出图数"><input type="number" min="1" max="10" value={modelDialog.maxImageCount} onChange={(event) => setModelDialog({ ...modelDialog, maxImageCount: event.target.value })} /></Field>
			<Field label="尺寸模式">
			  <div className={providerModelTaskTypeGridClass}>
				<label className={providerModelTaskTypeOptionClass}><input type="checkbox" checked={modelDialog.sizeModes.includes('auto')} onChange={(event) => setModelDialog(toggleSizeMode(modelDialog, 'auto', event.target.checked))} /><span>支持自动尺寸</span></label>
				<label className={providerModelTaskTypeOptionClass}><input type="checkbox" checked={modelDialog.sizeModes.includes('ratio')} onChange={(event) => setModelDialog(toggleSizeMode(modelDialog, 'ratio', event.target.checked))} /><span>支持图片比例</span></label>
                <label className={providerModelTaskTypeOptionClass}><input type="checkbox" checked={modelDialog.sizeModes.includes('pixel')} onChange={(event) => setModelDialog(toggleSizeMode(modelDialog, 'pixel', event.target.checked))} /><span>支持像素大小</span></label>
              </div>
            </Field>
			{modelDialog.sizeModes.includes('ratio') ? <Field label="支持比例"><TagInput values={modelDialog.supportedRatios} input={modelDialog.ratioInput} placeholder="例如 3:2，回车添加" options={defaultRatios} normalize={normalizeRatio} onInput={(ratioInput) => setModelDialog({ ...modelDialog, ratioInput })} onChange={(supportedRatios, ratioInput = '') => setModelDialog({ ...modelDialog, supportedRatios, ratioInput })} /></Field> : null}
			{modelDialog.sizeModes.includes('ratio') ? <Field label="允许自定义比例"><label className={providerModelTaskTypeOptionClass}><input type="checkbox" checked={modelDialog.supportsCustomRatio} onChange={(event) => setModelDialog({ ...modelDialog, supportsCustomRatio: event.target.checked })} /><span>{modelDialog.supportsCustomRatio ? '允许' : '不允许'}</span></label></Field> : null}
			{modelDialog.sizeModes.includes('pixel') ? <Field label="支持像素"><TagInput values={modelDialog.supportedPixelSizes} input={modelDialog.pixelInput} placeholder="例如 2048x2048，回车添加" options={defaultPixelSizes} normalize={normalizePixelSize} onInput={(pixelInput) => setModelDialog({ ...modelDialog, pixelInput })} onChange={(supportedPixelSizes, pixelInput = '') => setModelDialog({ ...modelDialog, supportedPixelSizes, pixelInput })} /></Field> : null}
			{modelDialog.sizeModes.includes('pixel') ? <Field label="允许用户自定义尺寸" hint="启用后，用户输入的 Width 和 Height 必须严格满足以下范围及平台尺寸规则。"><label className={providerModelTaskTypeOptionClass}><input type="checkbox" checked={modelDialog.supportsCustomSize} onChange={(event) => setModelDialog({ ...modelDialog, supportsCustomSize: event.target.checked })} /><span>{modelDialog.supportsCustomSize ? '允许' : '不允许'}</span></label></Field> : null}
			{modelDialog.sizeModes.includes('pixel') ? <Field label="像素宽度范围"><div className="grid grid-cols-2 gap-2"><input aria-label="最小宽度" type="number" min="16" max="3840" step="16" value={modelDialog.minWidth} onChange={(event) => setModelDialog({ ...modelDialog, minWidth: event.target.value })} /><input aria-label="最大宽度" type="number" min="16" max="3840" step="16" value={modelDialog.maxWidth} onChange={(event) => setModelDialog({ ...modelDialog, maxWidth: event.target.value })} /></div></Field> : null}
			{modelDialog.sizeModes.includes('pixel') ? <Field label="像素高度范围"><div className="grid grid-cols-2 gap-2"><input aria-label="最小高度" type="number" min="16" max="3840" step="16" value={modelDialog.minHeight} onChange={(event) => setModelDialog({ ...modelDialog, minHeight: event.target.value })} /><input aria-label="最大高度" type="number" min="16" max="3840" step="16" value={modelDialog.maxHeight} onChange={(event) => setModelDialog({ ...modelDialog, maxHeight: event.target.value })} /></div></Field> : null}
			<Field label="支持背景"><div className={providerModelTaskTypeGridClass}>{backgroundOptions.map((value) => <label key={value} className={providerModelTaskTypeOptionClass}><input type="checkbox" checked={modelDialog.supportedBackgrounds.includes(value)} onChange={(event) => setModelDialog({ ...modelDialog, supportedBackgrounds: event.target.checked ? [...modelDialog.supportedBackgrounds, value] : modelDialog.supportedBackgrounds.filter((item) => item !== value) })} /><span>{value}</span></label>)}</div></Field>
            <Field label="输出格式"><TagInput values={modelDialog.outputFormat} input={modelDialog.outputFormatInput} placeholder="png / jpeg / webp" options={outputFormatOptions} normalize={normalizeLowerEnum} onInput={(outputFormatInput) => setModelDialog({ ...modelDialog, outputFormatInput })} onChange={(outputFormat, outputFormatInput = '') => setModelDialog({ ...modelDialog, outputFormat, outputFormatInput })} /></Field>
            <Field label="是否支持压缩质量" hint="启用后，用户可在 JPEG/WebP 输出格式下配置 1-100 的压缩质量。"><label className={providerModelTaskTypeOptionClass}><input type="checkbox" checked={modelDialog.supportsOutputCompression} onChange={(event) => setModelDialog({ ...modelDialog, supportsOutputCompression: event.target.checked })} /><span>{modelDialog.supportsOutputCompression ? '支持' : '不支持'}</span></label></Field>
            <Field label="审核等级"><TagInput values={modelDialog.moderation} input={modelDialog.moderationInput} placeholder="auto / low" options={moderationOptions} normalize={normalizeLowerEnum} onInput={(moderationInput) => setModelDialog({ ...modelDialog, moderationInput })} onChange={(moderation, moderationInput = '') => setModelDialog({ ...modelDialog, moderation, moderationInput })} /></Field>
            <Field label="单图成本"><input value={modelDialog.costPerImage} onChange={(event) => setModelDialog({ ...modelDialog, costPerImage: event.target.value })} /></Field>
            <Field label="币种"><input value={modelDialog.currency} onChange={(event) => setModelDialog({ ...modelDialog, currency: event.target.value })} /></Field>
            <Field label="状态"><select value={modelDialog.enabled ? 'enabled' : 'disabled'} onChange={(event) => setModelDialog({ ...modelDialog, enabled: event.target.value === 'enabled' })}><option value="enabled">启用</option><option value="disabled">停用</option></select></Field>
          </div>
        </Drawer>
      ) : null}
      {testDialog ? (
        <Modal title="测试模型账号" detail={testDialog.account.name} onClose={() => setTestDialog(null)} footer={<><button className={cn(adminButton.base, adminButton.ghost)} type="button" disabled={testing} onClick={() => setTestDialog(null)}>关闭</button><button className={cn(adminButton.base, adminButton.primary)} type="button" disabled={testing || !testDialog.modelId || !testDialog.prompt.trim()} onClick={() => void runTestImage()}>{testing ? '测试中...' : '开始测试'}</button></>}>
          <div className={adminPage.formGrid}>
            <Field label="测试模型"><select value={testDialog.modelId} onChange={(event) => { const selected = (modelsByAccount[String(testDialog.account.id)] ?? []).find((model) => String(model.id) === event.target.value); if (selected) setTestDialog(rebuildTestImageDialog(testDialog, selected)) }}>{(modelsByAccount[String(testDialog.account.id)] ?? []).filter((model) => model.enabled).map((model) => <option key={String(model.id)} value={String(model.id)}>{model.display_name || model.model_code}</option>)}</select></Field>
            <Field label="来源模式"><select value={testDialog.sourceMode} onChange={(event) => setTestDialog({ ...testDialog, sourceMode: event.target.value, result: undefined, error: undefined })}><option value="images">Images API</option><option value="codex_responses">Codex Responses</option></select></Field>
			<Field label="尺寸模式"><select value={testDialog.sizeMode} onChange={(event) => setTestDialog({ ...testDialog, sizeMode: event.target.value, result: undefined, error: undefined })}>{selectedTestSizeModes.map((option) => <option key={option} value={option}>{sizeModeLabel(option)}</option>)}</select></Field>
			{testDialog.sizeMode === 'ratio' ? <><Field label="比例">{selectedTestModel?.supports_custom_ratio ? <><input list="test-model-ratios" value={testDialog.aspectRatio} onChange={(event) => setTestDialog({ ...testDialog, aspectRatio: event.target.value, result: undefined, error: undefined })} /><datalist id="test-model-ratios">{(selectedTestModel.supported_ratios ?? []).map((option) => <option key={option} value={option} />)}</datalist></> : <select value={testDialog.aspectRatio} onChange={(event) => setTestDialog({ ...testDialog, aspectRatio: event.target.value, result: undefined, error: undefined })}>{(selectedTestModel?.supported_ratios ?? []).map((option) => <option key={option} value={option}>{option}</option>)}</select>}</Field><Field label="基础分辨率"><select value={testDialog.baseResolution} onChange={(event) => setTestDialog({ ...testDialog, baseResolution: event.target.value, result: undefined, error: undefined })}>{normalizeBaseResolution(selectedTestModel?.base_resolution ?? []).map((option) => <option key={option} value={option}>{option}</option>)}</select></Field></> : testDialog.sizeMode === 'pixel' ? <Field label="像素尺寸">{selectedTestModel?.supports_custom_size ? <><input list="test-model-pixel-sizes" value={testDialog.requestedSize} onChange={(event) => setTestDialog({ ...testDialog, requestedSize: event.target.value, result: undefined, error: undefined })} /><datalist id="test-model-pixel-sizes">{(selectedTestModel.supported_pixel_sizes ?? []).map((option) => <option key={option} value={option} />)}</datalist></> : <select value={testDialog.requestedSize} onChange={(event) => setTestDialog({ ...testDialog, requestedSize: event.target.value, result: undefined, error: undefined })}>{(selectedTestModel?.supported_pixel_sizes ?? []).map((option) => <option key={option} value={option}>{option}</option>)}</select>}</Field> : null}
            <Field label="质量参数"><select value={testDialog.quality} onChange={(event) => setTestDialog({ ...testDialog, quality: event.target.value, result: undefined, error: undefined })}>{selectedTestQualities.map((option) => <option key={option} value={option}>{option}</option>)}</select></Field>
			<Field label="输出格式"><select value={testDialog.outputFormat} onChange={(event) => setTestDialog({ ...testDialog, outputFormat: event.target.value, result: undefined, error: undefined })}>{selectedTestFormats.map((option) => <option key={option} value={option}>{option}</option>)}</select></Field>
			<Field label="背景"><select value={testDialog.background} onChange={(event) => setTestDialog(changeTestImageBackground(testDialog, event.target.value, selectedTestFormats))}><option value="">不传</option>{selectedTestBackgrounds.map((option) => <option key={option} value={option}>{option}</option>)}</select></Field>
            {selectedTestModel?.supports_output_compression && (testDialog.outputFormat === 'jpeg' || testDialog.outputFormat === 'webp') ? <Field label="压缩质量"><input type="number" min="1" max="100" value={testDialog.outputCompression} onChange={(event) => setTestDialog({ ...testDialog, outputCompression: event.target.value, result: undefined, error: undefined })} /></Field> : null}
            <Field label="审核等级"><select value={testDialog.moderation} onChange={(event) => setTestDialog({ ...testDialog, moderation: event.target.value, result: undefined, error: undefined })}>{selectedTestModeration.map((option) => <option key={option} value={option}>{option}</option>)}</select></Field>
            <Field label="提示词"><textarea value={testDialog.prompt} onChange={(event) => setTestDialog({ ...testDialog, prompt: event.target.value, result: undefined, error: undefined })} rows={4} /></Field>
            {testDialog.error ? <InlineFeedback tone="danger" message={testDialog.error} /> : null}
            {testDialog.result ? (
              <section className="col-span-full grid gap-3 rounded-lg border border-[var(--line)] bg-white/[0.02] p-3">
                {testDialog.result.image_url ? <img className="max-h-[360px] w-full rounded-lg border border-[var(--line)] object-contain" src={adminApi.modelAccountTestImageUrl(testDialog.result.image_url, accessToken)} alt="" /> : null}
                <div className="grid grid-cols-[repeat(auto-fit,minmax(150px,1fr))] gap-2 text-sm">
                  <code className={adminDataGrid.code}>status: {testDialog.result.status}</code>
                  <code className={adminDataGrid.code}>size: {testDialog.result.width ?? 0}x{testDialog.result.height ?? 0}</code>
                  <code className={adminDataGrid.code}>elapsed: {testDialog.result.elapsed_ms}ms</code>
                  <code className={adminDataGrid.code}>request: {testDialog.result.provider_request_id || '-'}</code>
                </div>
                <pre className="max-h-[180px] overflow-auto rounded-lg border border-[var(--line)] bg-white/5 p-3 text-xs">{JSON.stringify(testDialog.result.actual_params ?? {}, null, 2)}</pre>
              </section>
            ) : null}
          </div>
        </Modal>
      ) : null}
      {deleteTarget ? (
        <Modal
          title={deleteTarget.kind === 'account' ? '删除接入账号' : '删除真实模型'}
          detail={deleteTarget.kind === 'account' ? deleteTarget.account.name : `${deleteTarget.account.name} / ${deleteTarget.model.model_code}`}
          onClose={() => { if (!deleting) setDeleteTarget(null) }}
          footer={<><button className={cn(adminButton.base, adminButton.ghost)} type="button" disabled={deleting} onClick={() => setDeleteTarget(null)}>取消</button><button className={cn(adminButton.base, adminButton.danger)} type="button" disabled={deleting} onClick={() => void confirmDelete()}>{deleting ? '删除中...' : '确认删除'}</button></>}
        >
          {mutationError ? <InlineFeedback tone="danger" message={mutationError} /> : null}
          <p className="m-0 text-sm leading-6 text-[var(--muted)]">删除后该配置不再参与新请求，历史任务仍按已保存的模型快照展示。</p>
        </Modal>
      ) : null}
    </section>
  )
}

function accountColumns({
  modelsByAccount,
  selectedAccountId,
  onSelect,
  onEdit,
  onAddModel,
  onTest,
  onDelete,
}: {
  modelsByAccount: Record<string, ModelAccountModel[]>
  selectedAccountId: string
  onSelect: (account: ModelAccount) => void
  onEdit: (account: ModelAccount) => void
  onAddModel: (account: ModelAccount) => void
  onTest: (account: ModelAccount) => void
  onDelete: (account: ModelAccount) => void
}): ColumnDef<ModelAccount>[] {
  return [
    {
      key: 'account',
      title: '账号',
      width: 'minmax(220px,2fr)',
      render: (account) => (
        <span className={accountTableClasses.identity}>
          <span className={accountTableClasses.icon}><AccessAccountsIcon className="size-4" aria-hidden="true" /></span>
          <span className="min-w-0">
            <span className={accountTableClasses.title}>{account.name}</span>
            <span className={accountTableClasses.detail}>{credentialsStatusLabel(account.credentials_status?.has_api_key)}</span>
          </span>
        </span>
      ),
    },
    {
      key: 'adapter',
      title: '接入方式',
      width: 'minmax(130px,1fr)',
      render: (account) => <span className={accountTableClasses.stack}><span className="font-semibold text-[var(--text)]">{providerAdapterLabel(account.adapter_type)}</span><span className={accountTableClasses.detail}>{providerAuthLabel(account.auth_type)}</span></span>,
    },
    {
      key: 'configuration',
      title: '配置',
      width: 'minmax(220px,2fr)',
      kind: 'code',
      render: (account) => <span><code className={accountTableClasses.configCode} title={account.base_url}>{account.base_url}</code><span className={accountTableClasses.configMeta}>P:{account.priority} · W:{account.weight} · C:{account.concurrency_limit} · {account.timeout_ms}ms</span></span>,
    },
    {
      key: 'status',
      title: '状态',
      width: 'minmax(90px,.7fr)',
      render: (account) => <Badge tone={modelAccountStatusTone(account.status)}>{modelAccountStatusLabel(account.status)}</Badge>,
    },
    {
      key: 'models',
      title: '真实模型',
      width: 'minmax(100px,.8fr)',
      kind: 'number',
      render: (account) => <span className={accountTableClasses.machineValue}>{modelsByAccount[String(account.id)]?.length ?? 0}</span>,
    },
    {
      key: 'actions',
      title: '操作',
      width: 'minmax(190px,1.2fr)',
      align: 'right',
      render: (account) => {
        const selected = selectedAccountId === String(account.id)
        return (
          <span className={accountTableClasses.actions}>
            <button className={cn(adminButton.base, selected ? adminButton.ghost : adminButton.primary, adminButton.small)} type="button" aria-pressed={selected} onClick={() => onSelect(account)}>{accountPrimaryActionLabel}</button>
            <ActionMenu actions={[
              { id: 'edit-account', label: '编辑账号', run: () => onEdit(account) },
              { id: 'add-model', label: '添加真实模型', run: () => onAddModel(account) },
              { id: 'test-account', label: '测试模型账号', run: () => onTest(account) },
              { id: 'delete-account', label: '删除账号', tone: 'danger', run: () => onDelete(account) },
            ]} />
          </span>
        )
      },
    },
  ]
}

function modelColumns(onEdit: (model: ModelAccountModel) => void, onDelete: (model: ModelAccountModel) => void): ColumnDef<ModelAccountModel>[] {
  return [
    {
      key: 'model',
      title: '真实模型',
      width: 'minmax(190px,1.5fr)',
      render: (model) => <span className={accountTableClasses.stack}><span className={accountTableClasses.title}>{model.display_name || model.model_code}</span><code className={accountTableClasses.modelCode}>{model.model_code}</code></span>,
    },
    {
      key: 'capability',
      title: '任务与尺寸能力',
      width: 'minmax(280px,2.5fr)',
      render: (model) => <span className={accountTableClasses.modelCapability}>{model.task_types.join(' / ') || '未配置任务类型'}<br />{normalizeBaseResolution(model.base_resolution).join(' / ') || '未配置基础分辨率'} · {capabilitySummary(model)}</span>,
    },
    {
      key: 'output',
      title: '输出能力',
      width: 'minmax(150px,1.2fr)',
      render: (model) => <span className={accountTableClasses.stack}><span>{model.output_format?.join(' / ') || '-'}</span><Badge tone={model.supports_output_compression ? 'success' : 'neutral'}>{model.supports_output_compression ? '支持压缩质量' : '固定质量'}</Badge></span>,
    },
    {
      key: 'cost',
      title: '单图成本',
      width: 'minmax(110px,.8fr)',
      kind: 'number',
      align: 'right',
      render: (model) => <span className={accountTableClasses.machineValue}>{model.cost_per_image} {model.currency}</span>,
    },
    {
      key: 'status',
      title: '状态',
      width: 'minmax(84px,.6fr)',
      render: (model) => <Badge tone={model.enabled ? 'success' : 'warning'}>{model.enabled ? '启用' : '停用'}</Badge>,
    },
    {
      key: 'actions',
      title: '操作',
      width: 'minmax(90px,.7fr)',
      align: 'right',
      render: (model) => <span className={accountTableClasses.actions}><button className={cn(adminButton.base, adminButton.primary, adminButton.small)} type="button" onClick={() => onEdit(model)}>编辑</button><TooltipIconButton label={`删除 ${model.model_code}`} onClick={() => onDelete(model)}><Trash2 /></TooltipIconButton></span>,
    },
  ]
}

function editAccountDraft(row: ModelAccount): AccountDraft {
  return { id: row.id, name: row.name, adapterType: row.adapter_type, authType: row.auth_type, baseUrl: row.base_url, apiKey: '', priority: String(row.priority), weight: String(row.weight), concurrencyLimit: String(row.concurrency_limit), timeoutMS: String(row.timeout_ms), status: row.status, sourceMode: sourceModeFromExtra(row.extra) }
}

function newModelDraft(account: ModelAccount): ModelDraft {
  return { account, modelCode: '', displayName: '', taskTypes: ['text_to_image'], base_resolution: ['1K', '2K'], baseResolutionInput: '', quality: ['auto'], qualityInput: '', maxReferenceImageCount: '5', maxImageCount: '1', sizeModes: ['ratio'], supportedRatios: defaultRatios, ratioInput: '', supportsCustomRatio: false, supportedPixelSizes: defaultPixelSizes, pixelInput: '', supportsCustomSize: false, minWidth: '512', maxWidth: '3840', minHeight: '512', maxHeight: '3840', supportedBackgrounds: ['auto'], outputFormat: ['png'], outputFormatInput: '', supportsOutputCompression: false, moderation: ['auto'], moderationInput: '', costPerImage: '0.00000', currency: 'USD', enabled: true }
}

function editModelDraft(account: ModelAccount, row: ModelAccountModel): ModelDraft {
  return { account, row, modelCode: row.model_code, displayName: row.display_name, taskTypes: row.task_types, base_resolution: normalizeBaseResolution(row.base_resolution), baseResolutionInput: '', quality: normalizeLowerEnums(row.quality ?? ['auto']), qualityInput: '', maxReferenceImageCount: String(row.max_reference_image_count ?? 5), maxImageCount: String(row.max_image_count ?? 1), sizeModes: row.size_modes?.length ? row.size_modes : ['ratio'], supportedRatios: row.supported_ratios?.length ? row.supported_ratios : defaultRatios, ratioInput: '', supportsCustomRatio: Boolean(row.supports_custom_ratio), supportedPixelSizes: row.supported_pixel_sizes?.length ? row.supported_pixel_sizes : defaultPixelSizes, pixelInput: '', supportsCustomSize: Boolean(row.supports_custom_size), minWidth: String(row.min_width ?? 512), maxWidth: String(row.max_width ?? 3840), minHeight: String(row.min_height ?? 512), maxHeight: String(row.max_height ?? 3840), supportedBackgrounds: normalizeLowerEnums(row.supported_backgrounds ?? []), outputFormat: normalizeLowerEnums(row.output_format ?? ['png']), outputFormatInput: '', supportsOutputCompression: Boolean(row.supports_output_compression), moderation: normalizeLowerEnums(row.moderation ?? ['auto']), moderationInput: '', costPerImage: row.cost_per_image, currency: row.currency, enabled: row.enabled }
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

function toggleSizeMode(draft: ModelDraft, mode: 'auto' | 'ratio' | 'pixel', checked: boolean): ModelDraft {
  const next = checked ? Array.from(new Set([...draft.sizeModes, mode])) : draft.sizeModes.filter((item) => item !== mode)
  if (mode === 'pixel' && checked) {
    return { ...draft, sizeModes: next, supportedPixelSizes: draft.supportedPixelSizes.length ? draft.supportedPixelSizes : defaultPixelSizes }
  }
  if (mode === 'pixel' && !checked) {
    return { ...draft, sizeModes: next.length ? next : ['pixel'], supportsCustomSize: false }
  }
  if (mode === 'ratio' && !checked) {
    return { ...draft, sizeModes: next.length ? next : ['ratio'], supportsCustomRatio: false }
  }
  return { ...draft, sizeModes: next.length ? next : ['auto'] }
}

function TagInput({
  values,
  input,
  placeholder,
  options,
  normalize,
  onInput,
  onChange,
}: {
  values: string[]
  input: string
  placeholder: string
  options: string[]
  normalize: (value: string) => string
  onInput: (value: string) => void
  onChange: (values: string[], input?: string) => void
}) {
  const addValue = (raw: string) => {
    const value = normalize(raw)
    if (!value || values.some((item) => item.toLowerCase() === value.toLowerCase())) {
      onChange(values, '')
      return
    }
    onChange([...values, value], '')
  }
  const removeValue = (value: string) => onChange(values.filter((item) => item !== value), input)
  return (
    <div className={tagInputClasses.root}>
      <div className={tagInputClasses.list}>
        {values.map((value) => <span key={value} className={tagInputClasses.tag}>{value}<span className={tagInputClasses.remove} role="button" tabIndex={0} aria-label={`删除 ${value}`} onClick={() => removeValue(value)} onKeyDown={(event) => { if (event.key === 'Enter' || event.key === ' ') removeValue(value) }}>x</span></span>)}
      </div>
      <div className={tagInputClasses.inputRow}>
        <input list={`tag-options-${placeholder}`} value={input} onChange={(event) => onInput(event.target.value)} onKeyDown={(event) => { if (event.key === 'Enter') { event.preventDefault(); addValue(input) } }} placeholder={placeholder} />
        <button className={cn(adminButton.base, adminButton.ghost, adminButton.small)} type="button" onClick={() => addValue(input)}>添加</button>
      </div>
      <datalist id={`tag-options-${placeholder}`}>{options.map((value) => <option key={value} value={value} />)}</datalist>
    </div>
  )
}

function BaseResolutionTagInput({ draft, onChange }: { draft: ModelDraft; onChange: (next: ModelDraft) => void }) {
  const addBaseResolution = (raw: string) => {
    const baseResolution = normalizeBaseResolutionValue(raw)
    if (!baseResolution || draft.base_resolution.some((item) => item.toLowerCase() === baseResolution.toLowerCase())) {
      onChange({ ...draft, baseResolutionInput: '' })
      return
    }
    onChange({ ...draft, base_resolution: [...draft.base_resolution, baseResolution], baseResolutionInput: '' })
  }
  const removeBaseResolution = (baseResolution: string) => onChange({ ...draft, base_resolution: draft.base_resolution.filter((item) => item !== baseResolution) })
  const onKeyDown = (event: KeyboardEvent<HTMLInputElement>) => {
    if (event.key !== 'Enter') return
    event.preventDefault()
    addBaseResolution(draft.baseResolutionInput)
  }

  return (
    <div className={tagInputClasses.root}>
      <div className={tagInputClasses.list}>
        {draft.base_resolution.map((baseResolution) => (
          <span key={baseResolution} className={tagInputClasses.tag}>{baseResolution}<span className={tagInputClasses.remove} role="button" tabIndex={0} aria-label={`删除 ${baseResolution}`} onClick={() => removeBaseResolution(baseResolution)} onKeyDown={(event) => { if (event.key === 'Enter' || event.key === ' ') removeBaseResolution(baseResolution) }}>x</span></span>
        ))}
      </div>
      <div className={tagInputClasses.inputRow}>
        <input list="model-base-resolution-options" value={draft.baseResolutionInput} onChange={(event) => onChange({ ...draft, baseResolutionInput: event.target.value })} onKeyDown={onKeyDown} placeholder="选择或输入基础分辨率，回车添加" />
        <button className={cn(adminButton.base, adminButton.ghost, adminButton.small)} type="button" onClick={() => addBaseResolution(draft.baseResolutionInput)}>添加</button>
      </div>
      <datalist id="model-base-resolution-options">{baseResolutionOptions.map((baseResolution) => <option key={baseResolution} value={baseResolution} />)}</datalist>
    </div>
  )
}

function normalizeBaseResolution(values: string[]) {
  return values.map(normalizeBaseResolutionValue).filter(Boolean)
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

function capabilitySummary(model: ModelAccountModel) {
  const modes = model.size_modes?.length ? model.size_modes : ['ratio']
  const parts = [`参考图 ${model.max_reference_image_count ?? 5}`]
  if (modes.includes('ratio')) parts.push(`比例 ${model.supported_ratios?.join('/') || '-'}`)
  if (modes.includes('pixel')) parts.push(`像素 ${model.supported_pixel_sizes?.join('/') || '-'}`)
  return parts.join(' · ')
}

function sourceModeFromExtra(extra?: Record<string, unknown>) {
  const mode = String(extra?.source_mode ?? '').trim()
  if (mode === 'codex_responses') return 'codex_responses'
  if (extra?.gpt_image_2_codex_source === true) return 'codex_responses'
  return 'images'
}
