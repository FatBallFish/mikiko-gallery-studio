import { useEffect, useMemo, useState, type KeyboardEvent } from 'react'
import type { ImageTaskType, ModelAccount, ModelAccountModel, ModelAccountTestImageResult } from '../../../shared/api-types'
import { adminApi } from '../../../shared/admin-api'
import { cn } from '../../../shared/classnames'
import { Badge, EmptyBlock, ErrorBlock, Field, InlineFeedback, LoadingBlock, Modal, PageHeader, StatusCell, StatusStrip } from '../components'
import { adminButton, adminPage } from '../ui/classes'
import { adminDataGrid, adminGridCols } from '../ui/dataGrid'
import { adminTaskTypeOptions } from './adminTaskTypes'
import {
  credentialsStatusLabel,
  modelAccountStatusLabel,
  modelAccountStatusTone,
  modelCapabilitySummary,
  modelEnabledLabel,
  modelEnabledTone,
  providerAccountDialogDetail,
  providerAdapterLabel,
  providerAuthLabel,
} from './providerModelRows'

type AccountDraft = { id?: string | number; name: string; adapterType: string; authType: string; baseUrl: string; apiKey: string; priority: string; weight: string; concurrencyLimit: string; timeoutMS: string; status: string; sourceMode: string }
type ModelDraft = { account: ModelAccount; row?: ModelAccountModel; modelCode: string; displayName: string; taskTypes: ImageTaskType[]; qualities: string[]; qualityInput: string; costPerImage: string; currency: string; enabled: boolean }
type TestImageDialog = { account: ModelAccount; modelId: string; prompt: string; sourceMode: string; result?: ModelAccountTestImageResult; error?: string }

const qualityOptions = ['auto', '1K', '2K', '4K']
const blankAccount: AccountDraft = { name: '', adapterType: 'openai_compatible', authType: 'api_key', baseUrl: '', apiKey: '', priority: '1', weight: '100', concurrencyLimit: '5', timeoutMS: '120000', status: 'enabled', sourceMode: 'images' }
const defaultTestPrompt = 'A small product photo of a ceramic coffee cup on a clean desk'
const accountTextButtonClass = 'w-full min-w-0 bg-transparent text-left text-[var(--text)] hover:text-[var(--blue)]'
const tagInputClasses = {
  root: 'grid gap-2',
  list: 'flex flex-wrap gap-1.5',
  tag: 'inline-flex items-center gap-1.5 rounded-full border border-[var(--line)] bg-white px-2 py-1 text-xs text-[var(--text)]',
  remove: 'ml-0.5 grid size-4 place-items-center rounded-full text-[var(--soft)] hover:bg-[rgba(184,95,84,.12)] hover:text-[var(--red)] focus-visible:bg-[rgba(184,95,84,.12)] focus-visible:text-[var(--red)] focus-visible:outline-none',
  inputRow: 'grid grid-cols-[minmax(0,1fr)_auto] gap-2',
}
const providerModelTaskTypeGridClass = 'grid max-h-[220px] gap-2 overflow-auto rounded-[10px] border border-[var(--line)] bg-white/60 p-2'
const providerModelTaskTypeOptionClass = 'grid grid-cols-[auto_minmax(0,1fr)] items-center gap-2 rounded-lg border border-[var(--line)] bg-white/70 p-2 text-sm has-[:checked]:border-[var(--blue)] has-[:checked]:bg-[rgba(87,117,185,.08)]'

export function ProviderModelsPage({ accessToken }: { accessToken?: string }) {
  const [accounts, setAccounts] = useState<ModelAccount[]>([])
  const [modelsByAccount, setModelsByAccount] = useState<Record<string, ModelAccountModel[]>>({})
  const [selectedAccountId, setSelectedAccountId] = useState<string>('')
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [notice, setNotice] = useState('账号响应只展示密钥状态，不返回明文凭据。')
  const [accountDialog, setAccountDialog] = useState<AccountDraft | null>(null)
  const [modelDialog, setModelDialog] = useState<ModelDraft | null>(null)
  const [testDialog, setTestDialog] = useState<TestImageDialog | null>(null)
  const [saving, setSaving] = useState(false)
  const [testing, setTesting] = useState(false)

  const load = async () => {
    setLoading(true)
    setError(null)
    try {
      const nextAccounts = await adminApi.listModelAccounts({ page_size: 100 })
      const modelPairs = await Promise.all(nextAccounts.map(async (account) => [String(account.id), await adminApi.listModelAccountModels(account.id)] as const))
      const nextModels = Object.fromEntries(modelPairs)
      setAccounts(nextAccounts)
      setModelsByAccount(nextModels)
      setSelectedAccountId((current) => current || String(nextAccounts[0]?.id ?? ''))
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : '模型接入载入失败')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => { void load() }, [])

  const selectedAccount = accounts.find((account) => String(account.id) === selectedAccountId) ?? accounts[0]
  const selectedModels = selectedAccount ? modelsByAccount[String(selectedAccount.id)] ?? [] : []
  const totals = useMemo(() => ({
    enabledAccounts: accounts.filter((item) => item.status === 'enabled').length,
    enabledModels: Object.values(modelsByAccount).flat().filter((item) => item.enabled).length,
  }), [accounts, modelsByAccount])

  async function saveAccount() {
    if (!accountDialog) return
    setSaving(true)
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
      if (accountDialog.id) await adminApi.updateModelAccount(accountDialog.id, payload)
      else await adminApi.createModelAccount(payload)
      setAccountDialog(null)
      setNotice(`${accountDialog.name || '模型账号'} 已保存。`)
      await load()
    } finally {
      setSaving(false)
    }
  }

  async function saveModel() {
    if (!modelDialog) return
    setSaving(true)
    try {
      const payload = {
        model_code: modelDialog.modelCode,
        display_name: modelDialog.displayName,
        task_types: modelDialog.taskTypes,
        qualities: modelDialog.qualities,
        cost_per_image: modelDialog.costPerImage,
        currency: modelDialog.currency,
        enabled: modelDialog.enabled,
      }
      if (modelDialog.row) await adminApi.updateModelAccountModel(modelDialog.account.id, modelDialog.row.id, payload)
      else await adminApi.createModelAccountModel(modelDialog.account.id, payload)
      setModelDialog(null)
      setNotice(`${payload.model_code} 已保存。`)
      await load()
    } finally {
      setSaving(false)
    }
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
      })
      setTestDialog((current) => current ? { ...current, result } : current)
      setNotice(`${testDialog.account.name} 测试出图完成。`)
    } catch (caught) {
      setTestDialog((current) => current ? { ...current, error: caught instanceof Error ? caught.message : '测试出图失败' } : current)
    } finally {
      setTesting(false)
    }
  }

  if (loading) return <LoadingBlock label="载入模型接入" />
  if (error) return <ErrorBlock message={error} onRetry={load} />

  return (
    <section className={adminPage.stack}>
      <PageHeader eyebrow="Model Accounts" title="模型接入" detail="维护上游账号、端点、密钥状态，以及账号下真实可请求模型。" actions={<><button className={cn(adminButton.base, adminButton.ghost)} type="button" onClick={() => void load()}>刷新</button><button className={cn(adminButton.base, adminButton.primary)} type="button" onClick={() => setAccountDialog(blankAccount)}>新增账号</button></>} />
      <StatusStrip columns={4}>
        <StatusCell label="接入账号" value={accounts.length} />
        <StatusCell label="启用账号" value={totals.enabledAccounts} />
        <StatusCell label="真实模型" value={Object.values(modelsByAccount).flat().length} />
        <StatusCell label="启用模型" value={totals.enabledModels} />
      </StatusStrip>
      <section className={adminPage.fullSurface}>
        <section className={adminPage.mainLane}>
          <InlineFeedback tone="neutral" message={notice} />
          {!accounts.length ? <EmptyBlock title="暂无模型接入账号" detail="创建账号后再添加真实上游模型。" /> : (
            <div className={cn(adminDataGrid.root, adminGridCols.account)}>
              <div className={cn(adminDataGrid.head, adminGridCols.account)}><span>账号</span><span>接入方式</span><span>Base URL</span><span>调度</span><span>状态</span><span>操作</span></div>
              {accounts.map((row) => (
                <div key={String(row.id)} className={cn(adminDataGrid.row, adminGridCols.account)}>
                  <button type="button" className={accountTextButtonClass} onClick={() => setSelectedAccountId(String(row.id))}><strong>{row.name}</strong><p className={adminDataGrid.detail}>{credentialsStatusLabel(row.credentials_status?.has_api_key)}</p></button>
                  <span>{providerAdapterLabel(row.adapter_type)} / {providerAuthLabel(row.auth_type)}</span>
                  <code className={adminDataGrid.code}>{row.base_url}</code>
                  <span>优先级 {row.priority} · 权重 {row.weight}</span>
                  <Badge tone={modelAccountStatusTone(row.status)}>{modelAccountStatusLabel(row.status)}</Badge>
                  <div className={adminDataGrid.actions}>
                    <button className={cn(adminButton.base, adminButton.ghost, adminButton.small)} type="button" onClick={() => setAccountDialog(editAccountDraft(row))}>编辑</button>
                    <button className={cn(adminButton.base, adminButton.ghost, adminButton.small)} type="button" onClick={() => setModelDialog(newModelDraft(row))}>加模型</button>
                    <button className={cn(adminButton.base, adminButton.ghost, adminButton.small)} type="button" onClick={() => setTestDialog(newTestImageDialog(row, modelsByAccount[String(row.id)] ?? []))}>测试</button>
                  </div>
                </div>
              ))}
            </div>
          )}
        </section>
        <aside className={adminPage.sideRail}>
          <section className={adminPage.signalSection}>
            <strong>{selectedAccount?.name ?? '真实模型'}</strong>
            <p>{selectedAccount ? `${selectedModels.length} 个上游模型挂载在此账号下。` : '选择账号查看模型。'}</p>
          </section>
          {selectedModels.length ? selectedModels.map((model) => (
            <section key={String(model.id)} className={adminPage.signalSection}>
              <strong>{model.display_name || model.model_code}</strong>
              <p>{model.model_code} · {modelCapabilitySummary(model)}</p>
              <div className="flex flex-wrap items-center gap-2"><Badge tone={modelEnabledTone(model.enabled)}>{modelEnabledLabel(model.enabled)}</Badge><button className={cn(adminButton.base, adminButton.ghost, adminButton.small)} type="button" onClick={() => selectedAccount && setModelDialog(editModelDraft(selectedAccount, model))}>编辑</button></div>
            </section>
          )) : <EmptyBlock title="暂无真实模型" detail="为当前账号添加 gpt-image-1 等上游模型代码。" />}
        </aside>
      </section>
      {accountDialog ? (
        <Modal title={accountDialog.id ? '编辑模型账号' : '新增模型账号'} detail={providerAccountDialogDetail()} onClose={() => setAccountDialog(null)} footer={<><button className={cn(adminButton.base, adminButton.ghost)} type="button" disabled={saving} onClick={() => setAccountDialog(null)}>取消</button><button className={cn(adminButton.base, adminButton.primary)} type="button" disabled={saving || !accountDialog.name || !accountDialog.baseUrl} onClick={() => void saveAccount()}>{saving ? '保存中...' : '保存'}</button></>}>
          <div className={adminPage.formGrid}>
            <Field label="账号名称"><input value={accountDialog.name} onChange={(event) => setAccountDialog({ ...accountDialog, name: event.target.value })} /></Field>
            <Field label="接入方式"><select value={accountDialog.adapterType} onChange={(event) => setAccountDialog({ ...accountDialog, adapterType: event.target.value })}><option value="openai_compatible">OpenAI 兼容</option><option value="openrouter">OpenRouter</option></select></Field>
            <Field label="鉴权方式"><select value={accountDialog.authType} onChange={(event) => setAccountDialog({ ...accountDialog, authType: event.target.value })}><option value="api_key">API Key</option></select></Field>
            <Field label="Base URL"><input value={accountDialog.baseUrl} onChange={(event) => setAccountDialog({ ...accountDialog, baseUrl: event.target.value })} placeholder="https://api.openai.com" /></Field>
            <Field label="API Key"><input type="password" value={accountDialog.apiKey} onChange={(event) => setAccountDialog({ ...accountDialog, apiKey: event.target.value })} placeholder={accountDialog.id ? '留空则保持原密钥' : 'sk-...'} /></Field>
            <Field label="状态"><select value={accountDialog.status} onChange={(event) => setAccountDialog({ ...accountDialog, status: event.target.value })}><option value="enabled">启用</option><option value="disabled">停用</option><option value="error">异常</option></select></Field>
            <Field label="gpt-image-2 来源模式" hint="Codex 来源会强制 quality=auto，并按清晰度与比例计算 size。"><select value={accountDialog.sourceMode} onChange={(event) => setAccountDialog({ ...accountDialog, sourceMode: event.target.value })}><option value="images">Images API</option><option value="codex_responses">Codex Responses</option></select></Field>
            <Field label="优先级" hint="数值越小越优先作为候选账号；同优先级时再看权重。"><input type="number" min="1" value={accountDialog.priority} onChange={(event) => setAccountDialog({ ...accountDialog, priority: event.target.value })} /></Field>
            <Field label="权重" hint="同优先级账号的分流权重，100 表示默认满权重。"><input type="number" min="0" value={accountDialog.weight} onChange={(event) => setAccountDialog({ ...accountDialog, weight: event.target.value })} /></Field>
            <Field label="并发限制" hint="该账号同时处理的最大请求数。"><input type="number" min="1" value={accountDialog.concurrencyLimit} onChange={(event) => setAccountDialog({ ...accountDialog, concurrencyLimit: event.target.value })} /></Field>
            <Field label="超时毫秒" hint="调用上游接口的单次请求超时时间。"><input type="number" min="1000" value={accountDialog.timeoutMS} onChange={(event) => setAccountDialog({ ...accountDialog, timeoutMS: event.target.value })} /></Field>
          </div>
        </Modal>
      ) : null}
      {modelDialog ? (
        <Modal title={modelDialog.row ? '编辑真实模型' : '新增真实模型'} detail={modelDialog.account.name} onClose={() => setModelDialog(null)} footer={<><button className={cn(adminButton.base, adminButton.ghost)} type="button" disabled={saving} onClick={() => setModelDialog(null)}>取消</button><button className={cn(adminButton.base, adminButton.primary)} type="button" disabled={saving || !modelDialog.modelCode} onClick={() => void saveModel()}>{saving ? '保存中...' : '保存'}</button></>}>
          <div className={adminPage.formGrid}>
            <Field label="模型代码"><input value={modelDialog.modelCode} onChange={(event) => setModelDialog({ ...modelDialog, modelCode: event.target.value })} placeholder="gpt-image-1" /></Field>
            <Field label="展示名称"><input value={modelDialog.displayName} onChange={(event) => setModelDialog({ ...modelDialog, displayName: event.target.value })} /></Field>
            <Field label="任务类型"><div className={providerModelTaskTypeGridClass}>{adminTaskTypeOptions.map((option) => <label key={option.value} className={providerModelTaskTypeOptionClass}><input type="checkbox" checked={modelDialog.taskTypes.includes(option.value)} onChange={(event) => setModelDialog({ ...modelDialog, taskTypes: event.target.checked ? [...modelDialog.taskTypes, option.value] : modelDialog.taskTypes.filter((item) => item !== option.value) })} /><span>{option.label}</span></label>)}</div></Field>
            <Field label="质量列表"><QualityTagInput draft={modelDialog} onChange={setModelDialog} /></Field>
            <Field label="单图成本"><input value={modelDialog.costPerImage} onChange={(event) => setModelDialog({ ...modelDialog, costPerImage: event.target.value })} /></Field>
            <Field label="币种"><input value={modelDialog.currency} onChange={(event) => setModelDialog({ ...modelDialog, currency: event.target.value })} /></Field>
            <Field label="状态"><select value={modelDialog.enabled ? 'enabled' : 'disabled'} onChange={(event) => setModelDialog({ ...modelDialog, enabled: event.target.value === 'enabled' })}><option value="enabled">启用</option><option value="disabled">停用</option></select></Field>
          </div>
        </Modal>
      ) : null}
      {testDialog ? (
        <Modal title="测试模型账号" detail={testDialog.account.name} onClose={() => setTestDialog(null)} footer={<><button className={cn(adminButton.base, adminButton.ghost)} type="button" disabled={testing} onClick={() => setTestDialog(null)}>关闭</button><button className={cn(adminButton.base, adminButton.primary)} type="button" disabled={testing || !testDialog.modelId || !testDialog.prompt.trim()} onClick={() => void runTestImage()}>{testing ? '测试中...' : '开始测试'}</button></>}>
          <div className={adminPage.formGrid}>
            <Field label="测试模型"><select value={testDialog.modelId} onChange={(event) => setTestDialog({ ...testDialog, modelId: event.target.value, result: undefined, error: undefined })}>{(modelsByAccount[String(testDialog.account.id)] ?? []).filter((model) => model.enabled).map((model) => <option key={String(model.id)} value={String(model.id)}>{model.display_name || model.model_code}</option>)}</select></Field>
            <Field label="来源模式"><select value={testDialog.sourceMode} onChange={(event) => setTestDialog({ ...testDialog, sourceMode: event.target.value, result: undefined, error: undefined })}><option value="images">Images API</option><option value="codex_responses">Codex Responses</option></select></Field>
            <Field label="提示词"><textarea value={testDialog.prompt} onChange={(event) => setTestDialog({ ...testDialog, prompt: event.target.value, result: undefined, error: undefined })} rows={4} /></Field>
            {testDialog.error ? <InlineFeedback tone="danger" message={testDialog.error} /> : null}
            {testDialog.result ? (
              <section className="col-span-full grid gap-3 rounded-[var(--pg-radius-sm)] border border-[var(--line)] bg-white p-3">
                {testDialog.result.image_url ? <img className="max-h-[360px] w-full rounded-lg border border-[var(--line)] object-contain" src={adminApi.modelAccountTestImageUrl(testDialog.result.image_url, accessToken)} alt="" /> : null}
                <div className="grid grid-cols-[repeat(auto-fit,minmax(150px,1fr))] gap-2 text-sm">
                  <code className={adminDataGrid.code}>status: {testDialog.result.status}</code>
                  <code className={adminDataGrid.code}>size: {testDialog.result.width ?? 0}x{testDialog.result.height ?? 0}</code>
                  <code className={adminDataGrid.code}>elapsed: {testDialog.result.elapsed_ms}ms</code>
                  <code className={adminDataGrid.code}>request: {testDialog.result.provider_request_id || '-'}</code>
                </div>
                <pre className="max-h-[180px] overflow-auto rounded-lg bg-[var(--pg-admin-bg-subtle)] p-3 text-xs">{JSON.stringify(testDialog.result.actual_params ?? {}, null, 2)}</pre>
              </section>
            ) : null}
          </div>
        </Modal>
      ) : null}
    </section>
  )
}

function editAccountDraft(row: ModelAccount): AccountDraft {
  return { id: row.id, name: row.name, adapterType: row.adapter_type, authType: row.auth_type, baseUrl: row.base_url, apiKey: '', priority: String(row.priority), weight: String(row.weight), concurrencyLimit: String(row.concurrency_limit), timeoutMS: String(row.timeout_ms), status: row.status, sourceMode: sourceModeFromExtra(row.extra) }
}

function newModelDraft(account: ModelAccount): ModelDraft {
  return { account, modelCode: '', displayName: '', taskTypes: ['text_to_image'], qualities: ['auto', '1K', '2K'], qualityInput: '', costPerImage: '0.00000', currency: 'USD', enabled: true }
}

function editModelDraft(account: ModelAccount, row: ModelAccountModel): ModelDraft {
  return { account, row, modelCode: row.model_code, displayName: row.display_name, taskTypes: row.task_types, qualities: normalizeQualities(row.qualities), qualityInput: '', costPerImage: row.cost_per_image, currency: row.currency, enabled: row.enabled }
}

function newTestImageDialog(account: ModelAccount, models: ModelAccountModel[]): TestImageDialog {
  const enabledModels = models.filter((model) => model.enabled)
  const selected = enabledModels.find((model) => model.model_code === 'gpt-image-2') ?? enabledModels[0] ?? models[0]
  return { account, modelId: selected ? String(selected.id) : '', prompt: defaultTestPrompt, sourceMode: sourceModeFromExtra(account.extra) }
}

function QualityTagInput({ draft, onChange }: { draft: ModelDraft; onChange: (next: ModelDraft) => void }) {
  const addQuality = (raw: string) => {
    const quality = normalizeQuality(raw)
    if (!quality || draft.qualities.some((item) => item.toLowerCase() === quality.toLowerCase())) {
      onChange({ ...draft, qualityInput: '' })
      return
    }
    onChange({ ...draft, qualities: [...draft.qualities, quality], qualityInput: '' })
  }
  const removeQuality = (quality: string) => onChange({ ...draft, qualities: draft.qualities.filter((item) => item !== quality) })
  const onKeyDown = (event: KeyboardEvent<HTMLInputElement>) => {
    if (event.key !== 'Enter') return
    event.preventDefault()
    addQuality(draft.qualityInput)
  }

  return (
    <div className={tagInputClasses.root}>
      <div className={tagInputClasses.list}>
        {draft.qualities.map((quality) => (
          <span key={quality} className={tagInputClasses.tag}>{quality}<span className={tagInputClasses.remove} role="button" tabIndex={0} aria-label={`删除 ${quality}`} onClick={() => removeQuality(quality)} onKeyDown={(event) => { if (event.key === 'Enter' || event.key === ' ') removeQuality(quality) }}>x</span></span>
        ))}
      </div>
      <div className={tagInputClasses.inputRow}>
        <input list="model-quality-options" value={draft.qualityInput} onChange={(event) => onChange({ ...draft, qualityInput: event.target.value })} onKeyDown={onKeyDown} placeholder="选择或输入质量，回车添加" />
        <button className={cn(adminButton.base, adminButton.ghost, adminButton.small)} type="button" onClick={() => addQuality(draft.qualityInput)}>添加</button>
      </div>
      <datalist id="model-quality-options">{qualityOptions.map((quality) => <option key={quality} value={quality} />)}</datalist>
    </div>
  )
}

function normalizeQualities(values: string[]) {
  return values.map(normalizeQuality).filter(Boolean)
}

function normalizeQuality(value: string) {
  const trimmed = value.trim()
  if (!trimmed) return ''
  if (trimmed.toLowerCase() === 'auto') return 'auto'
  return trimmed.toUpperCase()
}

function sourceModeFromExtra(extra?: Record<string, unknown>) {
  const mode = String(extra?.source_mode ?? '').trim()
  if (mode === 'codex_responses') return 'codex_responses'
  if (extra?.gpt_image_2_codex_source === true) return 'codex_responses'
  return 'images'
}
