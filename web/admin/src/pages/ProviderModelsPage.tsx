import { useEffect, useMemo, useState, type KeyboardEvent } from 'react'
import type { ImageTaskType, ModelAccount, ModelAccountModel } from '../../../shared/api-types'
import { adminApi } from '../../../shared/admin-api'
import { Badge, EmptyBlock, ErrorBlock, Field, InlineFeedback, LoadingBlock, Modal, PageHeader } from '../components'

type AccountDraft = { id?: string | number; name: string; adapterType: string; authType: string; baseUrl: string; apiKey: string; priority: string; weight: string; concurrencyLimit: string; timeoutMS: string; status: string }
type ModelDraft = { account: ModelAccount; row?: ModelAccountModel; modelCode: string; displayName: string; taskTypes: ImageTaskType[]; qualities: string[]; qualityInput: string; costPerImage: string; currency: string; enabled: boolean }

const taskTypes: ImageTaskType[] = ['text_to_image', 'reference_to_image', 'image_edit']
const qualityOptions = ['auto', '1K', '2K', '4K']
const blankAccount: AccountDraft = { name: '', adapterType: 'openai_compatible', authType: 'api_key', baseUrl: '', apiKey: '', priority: '1', weight: '100', concurrencyLimit: '5', timeoutMS: '120000', status: 'enabled' }

export function ProviderModelsPage() {
  const [accounts, setAccounts] = useState<ModelAccount[]>([])
  const [modelsByAccount, setModelsByAccount] = useState<Record<string, ModelAccountModel[]>>({})
  const [selectedAccountId, setSelectedAccountId] = useState<string>('')
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [notice, setNotice] = useState('账号响应只展示密钥状态，不返回明文凭据。')
  const [accountDialog, setAccountDialog] = useState<AccountDraft | null>(null)
  const [modelDialog, setModelDialog] = useState<ModelDraft | null>(null)
  const [saving, setSaving] = useState(false)

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

  if (loading) return <LoadingBlock label="载入模型接入" />
  if (error) return <ErrorBlock message={error} onRetry={load} />

  return (
    <section className="page-stack">
      <PageHeader eyebrow="Model Accounts" title="模型接入" detail="维护上游账号、端点、密钥状态，以及账号下真实可请求模型。" actions={<><button className="ghost" type="button" onClick={() => void load()}>刷新</button><button className="btn primary" type="button" onClick={() => setAccountDialog(blankAccount)}>新增账号</button></>} />
      <section className="ops-status-strip compact-strip">
        <div className="status-cell"><label>接入账号</label><strong>{accounts.length}</strong></div>
        <div className="status-cell"><label>启用账号</label><strong>{totals.enabledAccounts}</strong></div>
        <div className="status-cell"><label>真实模型</label><strong>{Object.values(modelsByAccount).flat().length}</strong></div>
        <div className="status-cell"><label>启用模型</label><strong>{totals.enabledModels}</strong></div>
      </section>
      <section className="pg-admin-card ops-surface full-main">
        <section className="main-lane table-lane">
          <InlineFeedback tone="neutral" message={notice} />
          {!accounts.length ? <EmptyBlock title="暂无模型接入账号" detail="创建账号后再添加真实上游模型。" /> : (
            <>
              <div className="table-head account-grid"><span>账号</span><span>Adapter</span><span>Base URL</span><span>权重</span><span>状态</span><span>操作</span></div>
              {accounts.map((row) => (
                <div key={String(row.id)} className="table-row account-grid">
                  <button type="button" className="text-button" onClick={() => setSelectedAccountId(String(row.id))}><strong>{row.name}</strong><p>{row.credentials_status?.has_api_key ? 'API Key 已配置' : '未配置密钥'}</p></button>
                  <span>{row.adapter_type} / {row.auth_type}</span>
                  <code>{row.base_url}</code>
                  <span>{row.priority} / {row.weight}</span>
                  <Badge tone={row.status === 'enabled' ? 'success' : row.status === 'error' ? 'danger' : 'warning'}>{row.status}</Badge>
                  <div className="row-actions buttons">
                    <button className="ghost small" type="button" onClick={() => setAccountDialog({ id: row.id, name: row.name, adapterType: row.adapter_type, authType: row.auth_type, baseUrl: row.base_url, apiKey: '', priority: String(row.priority), weight: String(row.weight), concurrencyLimit: String(row.concurrency_limit), timeoutMS: String(row.timeout_ms), status: row.status })}>编辑</button>
                    <button className="ghost small" type="button" onClick={() => setModelDialog(newModelDraft(row))}>加模型</button>
                  </div>
                </div>
              ))}
            </>
          )}
        </section>
        <aside className="signal-rail">
          <section className="signal-section">
            <strong>{selectedAccount?.name ?? '真实模型'}</strong>
            <p>{selectedAccount ? `${selectedModels.length} 个上游模型挂载在此账号下。` : '选择账号查看模型。'}</p>
          </section>
          {selectedModels.length ? selectedModels.map((model) => (
            <section key={String(model.id)} className="signal-section">
              <strong>{model.display_name || model.model_code}</strong>
              <p>{model.model_code} · {model.qualities.join('/')} · {model.cost_per_image} {model.currency}</p>
              <div className="row-actions buttons"><Badge tone={model.enabled ? 'success' : 'warning'}>{model.enabled ? '启用' : '停用'}</Badge><button className="ghost small" type="button" onClick={() => selectedAccount && setModelDialog(editModelDraft(selectedAccount, model))}>编辑</button></div>
            </section>
          )) : <EmptyBlock title="暂无真实模型" detail="为当前账号添加 gpt-image-1 等上游模型代码。" />}
        </aside>
      </section>
      {accountDialog ? (
        <Modal title={accountDialog.id ? '编辑模型账号' : '新增模型账号'} detail="仅 api_key auth 可启用，其他 auth 仅作为预留字段。" onClose={() => setAccountDialog(null)} footer={<><button className="ghost" type="button" disabled={saving} onClick={() => setAccountDialog(null)}>取消</button><button className="btn primary" type="button" disabled={saving || !accountDialog.name || !accountDialog.baseUrl} onClick={() => void saveAccount()}>{saving ? '保存中...' : '保存'}</button></>}>
          <div className="form-grid">
            <Field label="账号名称"><input value={accountDialog.name} onChange={(event) => setAccountDialog({ ...accountDialog, name: event.target.value })} /></Field>
            <Field label="Adapter"><select value={accountDialog.adapterType} onChange={(event) => setAccountDialog({ ...accountDialog, adapterType: event.target.value })}><option value="openai_compatible">openai_compatible</option><option value="openrouter">openrouter</option></select></Field>
            <Field label="Auth"><select value={accountDialog.authType} onChange={(event) => setAccountDialog({ ...accountDialog, authType: event.target.value })}><option value="api_key">api_key</option></select></Field>
            <Field label="Base URL"><input value={accountDialog.baseUrl} onChange={(event) => setAccountDialog({ ...accountDialog, baseUrl: event.target.value })} placeholder="https://api.openai.com" /></Field>
            <Field label="API Key"><input type="password" value={accountDialog.apiKey} onChange={(event) => setAccountDialog({ ...accountDialog, apiKey: event.target.value })} placeholder={accountDialog.id ? '留空则保持原密钥' : 'sk-...'} /></Field>
            <Field label="状态"><select value={accountDialog.status} onChange={(event) => setAccountDialog({ ...accountDialog, status: event.target.value })}><option value="enabled">启用</option><option value="disabled">停用</option><option value="error">错误</option></select></Field>
            <Field label="优先级" hint="数值越小越优先作为候选账号；同优先级时再看权重。"><input type="number" min="1" value={accountDialog.priority} onChange={(event) => setAccountDialog({ ...accountDialog, priority: event.target.value })} /></Field>
            <Field label="权重" hint="同优先级账号的分流权重，100 表示默认满权重。"><input type="number" min="0" value={accountDialog.weight} onChange={(event) => setAccountDialog({ ...accountDialog, weight: event.target.value })} /></Field>
            <Field label="并发限制" hint="该账号同时处理的最大请求数。"><input type="number" min="1" value={accountDialog.concurrencyLimit} onChange={(event) => setAccountDialog({ ...accountDialog, concurrencyLimit: event.target.value })} /></Field>
            <Field label="超时毫秒" hint="调用上游接口的单次请求超时时间。"><input type="number" min="1000" value={accountDialog.timeoutMS} onChange={(event) => setAccountDialog({ ...accountDialog, timeoutMS: event.target.value })} /></Field>
          </div>
        </Modal>
      ) : null}
      {modelDialog ? (
        <Modal title={modelDialog.row ? '编辑真实模型' : '新增真实模型'} detail={modelDialog.account.name} onClose={() => setModelDialog(null)} footer={<><button className="ghost" type="button" disabled={saving} onClick={() => setModelDialog(null)}>取消</button><button className="btn primary" type="button" disabled={saving || !modelDialog.modelCode} onClick={() => void saveModel()}>{saving ? '保存中...' : '保存'}</button></>}>
          <div className="form-grid">
            <Field label="模型代码"><input value={modelDialog.modelCode} onChange={(event) => setModelDialog({ ...modelDialog, modelCode: event.target.value })} placeholder="gpt-image-1" /></Field>
            <Field label="展示名称"><input value={modelDialog.displayName} onChange={(event) => setModelDialog({ ...modelDialog, displayName: event.target.value })} /></Field>
            <Field label="任务类型"><div className="check-grid-scroll">{taskTypes.map((type) => <label key={type} className="check-option"><input type="checkbox" checked={modelDialog.taskTypes.includes(type)} onChange={(event) => setModelDialog({ ...modelDialog, taskTypes: event.target.checked ? [...modelDialog.taskTypes, type] : modelDialog.taskTypes.filter((item) => item !== type) })} /><span>{type}</span></label>)}</div></Field>
            <Field label="质量列表"><QualityTagInput draft={modelDialog} onChange={setModelDialog} /></Field>
            <Field label="单图成本"><input value={modelDialog.costPerImage} onChange={(event) => setModelDialog({ ...modelDialog, costPerImage: event.target.value })} /></Field>
            <Field label="币种"><input value={modelDialog.currency} onChange={(event) => setModelDialog({ ...modelDialog, currency: event.target.value })} /></Field>
            <Field label="状态"><select value={modelDialog.enabled ? 'enabled' : 'disabled'} onChange={(event) => setModelDialog({ ...modelDialog, enabled: event.target.value === 'enabled' })}><option value="enabled">启用</option><option value="disabled">停用</option></select></Field>
          </div>
        </Modal>
      ) : null}
    </section>
  )
}

function newModelDraft(account: ModelAccount): ModelDraft {
  return { account, modelCode: '', displayName: '', taskTypes: ['text_to_image'], qualities: ['auto', '1K', '2K'], qualityInput: '', costPerImage: '0.00000', currency: 'USD', enabled: true }
}

function editModelDraft(account: ModelAccount, row: ModelAccountModel): ModelDraft {
  return { account, row, modelCode: row.model_code, displayName: row.display_name, taskTypes: row.task_types, qualities: normalizeQualities(row.qualities), qualityInput: '', costPerImage: row.cost_per_image, currency: row.currency, enabled: row.enabled }
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
    <div className="tag-input-wrap">
      <div className="tag-list">
        {draft.qualities.map((quality) => (
          <span key={quality} className="input-tag">{quality}<span role="button" tabIndex={0} aria-label={`删除 ${quality}`} onClick={() => removeQuality(quality)} onKeyDown={(event) => { if (event.key === 'Enter' || event.key === ' ') removeQuality(quality) }}>x</span></span>
        ))}
      </div>
      <div className="combo-input-row">
        <input list="model-quality-options" value={draft.qualityInput} onChange={(event) => onChange({ ...draft, qualityInput: event.target.value })} onKeyDown={onKeyDown} placeholder="选择或输入质量，回车添加" />
        <button className="ghost small" type="button" onClick={() => addQuality(draft.qualityInput)}>添加</button>
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
