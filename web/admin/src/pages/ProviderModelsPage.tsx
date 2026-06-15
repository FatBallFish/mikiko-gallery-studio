import { useEffect, useMemo, useState, type KeyboardEvent } from 'react'
import type { ImageTaskType, ModelAccount, ModelAccountModel, ModelAccountTestImageResult } from '../../../shared/api-types'
import { adminApi } from '../../../shared/admin-api'
import { cn } from '../../../shared/classnames'
import { Badge, EmptyBlock, ErrorBlock, Field, InlineFeedback, LoadingBlock, Modal } from '../components'
import { adminButton, adminPage } from '../ui/classes'
import { adminDataGrid } from '../ui/dataGrid'
import { adminTaskTypeOptions } from './adminTaskTypes'
import {
  credentialsStatusLabel,
  modelAccountStatusLabel,
  modelAccountStatusTone,
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
const accountTextButtonClass = 'w-full min-w-0 bg-transparent text-left text-[var(--text)] hover:text-[var(--accent)]'
const accountTableClasses = {
  tableWrap: 'min-w-0 overflow-x-auto rounded-3xl border border-[var(--line)] bg-white/[0.01] shadow-[0_20px_70px_rgba(0,0,0,.18)] backdrop-blur-sm',
  toolbar: 'flex flex-wrap items-center justify-between gap-3',
  toolbarActions: 'flex flex-wrap gap-3',
  searchBox: 'min-h-10 w-64 rounded-xl border border-[var(--line)] bg-white/5 px-4 py-2 text-sm text-[var(--text)] placeholder:text-[var(--soft)] outline-none focus:border-[var(--accent)]/50 focus:ring-1 focus:ring-[var(--accent)]/40',
  table: 'w-full min-w-[1120px] border-collapse text-left',
  th: 'border-b border-[var(--line)] bg-white/[0.02] px-6 py-4 text-[11px] font-extrabold uppercase tracking-wider text-[var(--muted-strong)]',
  tr: 'border-b border-[var(--line)]/60 transition-colors last:border-b-0 hover:bg-white/[0.03]',
  trActive: 'bg-white/[0.025]',
  td: 'px-6 py-4 align-middle text-sm text-[var(--muted)]',
  identity: 'flex min-w-0 items-center gap-3',
  icon: 'grid size-9 shrink-0 place-items-center rounded-xl bg-white/5 text-[var(--accent)]',
  title: 'block truncate font-bold text-[var(--text)]',
  detail: 'mt-1 block truncate text-[11px] font-medium text-[var(--soft)]',
  stack: 'grid gap-1',
  configCode: 'block max-w-[220px] truncate font-mono text-[11px] text-[var(--muted)]',
  configMeta: 'text-[11px] text-[var(--soft)]',
  expandButton: 'inline-flex items-center gap-2 text-xs font-extrabold text-[var(--accent)] transition hover:text-[var(--text)]',
  actions: 'flex flex-wrap justify-end gap-2',
  actionLink: 'bg-transparent text-xs font-bold text-[var(--accent)] transition hover:text-[var(--text)]',
  subPanelCell: 'bg-black/10 px-6 py-5',
  subPanel: 'overflow-hidden rounded-2xl border border-[var(--line)] bg-white/[0.02]',
  subHeader: 'flex flex-wrap items-center justify-between gap-3 border-b border-[var(--line)] p-4',
  subTitle: 'text-[10px] font-extrabold uppercase tracking-[0.15em] text-[var(--muted-strong)]',
  subTable: 'w-full min-w-[760px] border-collapse text-left',
  subTh: 'border-b border-[var(--line)] px-4 py-3 text-[10px] font-extrabold uppercase tracking-wider text-[var(--muted-strong)]',
  subTd: 'px-4 py-3 align-middle text-xs text-[var(--muted)]',
  tagList: 'flex flex-wrap gap-1',
  modelTag: 'rounded-md border border-[var(--line)] bg-white/5 px-1.5 py-0.5 text-[9px] font-black uppercase text-[var(--muted)]',
  statusDot: 'size-2 rounded-full',
}
const tagInputClasses = {
  root: 'grid gap-2',
  list: 'flex flex-wrap gap-1.5',
  tag: 'inline-flex items-center gap-1.5 rounded-full border border-[var(--line)] bg-white/5 px-2 py-1 text-xs text-[var(--text)]',
  remove: 'ml-0.5 grid size-4 place-items-center rounded-full text-[var(--soft)] hover:bg-[rgba(184,95,84,.12)] hover:text-[var(--red)] focus-visible:bg-[rgba(184,95,84,.12)] focus-visible:text-[var(--red)] focus-visible:outline-none',
  inputRow: 'grid grid-cols-[minmax(0,1fr)_auto] gap-2',
}
const providerModelTaskTypeGridClass = 'grid max-h-[220px] gap-2 overflow-auto rounded-2xl border border-[var(--line)] bg-white/[0.02] p-2'
const providerModelTaskTypeOptionClass = 'grid grid-cols-[auto_minmax(0,1fr)] items-center gap-2 rounded-xl border border-[var(--line)] bg-white/5 p-2 text-sm has-[:checked]:border-[var(--accent)]/40 has-[:checked]:bg-[var(--accent)]/10'

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

  const load = async () => {
    setLoading(true)
    setError(null)
    try {
      const nextAccounts = await adminApi.listModelAccounts({ page_size: 100 })
      const modelPairs = await Promise.all(nextAccounts.map(async (account) => [String(account.id), await adminApi.listModelAccountModels(account.id)] as const))
      const nextModels = Object.fromEntries(modelPairs)
      setAccounts(nextAccounts)
      setModelsByAccount(nextModels)
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
      <section className={accountTableClasses.toolbar}>
        <div className={accountTableClasses.toolbarActions}>
          <button className={cn(adminButton.base, adminButton.primary)} type="button" onClick={() => setAccountDialog(blankAccount)}>添加账号</button>
          <button className={cn(adminButton.base, adminButton.ghost)} type="button" disabled title="批量操作暂未开放">批量操作</button>
        </div>
        <input className={accountTableClasses.searchBox} value={query} onChange={(event) => setQuery(event.target.value)} placeholder="搜索账号名称或适配器..." />
      </section>
      {!accounts.length ? <EmptyBlock title="暂无模型接入账号" detail="创建账号后再添加真实上游模型。" /> : null}
      {accounts.length ? (
        <div className={accountTableClasses.tableWrap}>
          <table className={accountTableClasses.table}>
                <thead>
                  <tr>
                    <th className="w-10 px-6 py-4"><input type="checkbox" className="accent-[var(--accent)]" aria-label="选择全部账号" disabled /></th>
                    <th className={accountTableClasses.th}>账号名称</th>
                    <th className={accountTableClasses.th}>接入/鉴权方式</th>
                    <th className={accountTableClasses.th}>配置信息</th>
                    <th className={accountTableClasses.th}>状态</th>
                    <th className={accountTableClasses.th}>支持模型</th>
                    <th className={cn(accountTableClasses.th, 'text-right')}>操作</th>
                  </tr>
                </thead>
                <tbody>
                  {filteredAccounts.map((row) => {
                    const rowId = String(row.id)
                    const rowModels = modelsByAccount[rowId] ?? []
                    const expanded = expandedAccountId === rowId
                    return (
                      <AccountRow
                        key={rowId}
                        account={row}
                        models={rowModels}
                        expanded={expanded}
                        onToggle={() => {
                          setExpandedAccountId((current) => current === rowId ? '' : rowId)
                        }}
                        onEditAccount={() => setAccountDialog(editAccountDraft(row))}
                        onAddModel={() => setModelDialog(newModelDraft(row))}
                        onTest={() => setTestDialog(newTestImageDialog(row, rowModels))}
                        onEditModel={(model) => setModelDialog(editModelDraft(row, model))}
                      />
                    )
                  })}
              </tbody>
            </table>
          </div>
      ) : null}
      {accounts.length && !filteredAccounts.length ? <EmptyBlock title="未找到接入账号" detail="换一个账号名称、适配器或 Base URL 关键词再试。" /> : null}
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
              <section className="col-span-full grid gap-3 rounded-3xl border border-[var(--line)] bg-white/[0.02] p-3">
                {testDialog.result.image_url ? <img className="max-h-[360px] w-full rounded-lg border border-[var(--line)] object-contain" src={adminApi.modelAccountTestImageUrl(testDialog.result.image_url, accessToken)} alt="" /> : null}
                <div className="grid grid-cols-[repeat(auto-fit,minmax(150px,1fr))] gap-2 text-sm">
                  <code className={adminDataGrid.code}>status: {testDialog.result.status}</code>
                  <code className={adminDataGrid.code}>size: {testDialog.result.width ?? 0}x{testDialog.result.height ?? 0}</code>
                  <code className={adminDataGrid.code}>elapsed: {testDialog.result.elapsed_ms}ms</code>
                  <code className={adminDataGrid.code}>request: {testDialog.result.provider_request_id || '-'}</code>
                </div>
                <pre className="max-h-[180px] overflow-auto rounded-2xl border border-[var(--line)] bg-white/5 p-3 text-xs">{JSON.stringify(testDialog.result.actual_params ?? {}, null, 2)}</pre>
              </section>
            ) : null}
          </div>
        </Modal>
      ) : null}
    </section>
  )
}

function AccountRow({
  account,
  models,
  expanded,
  onToggle,
  onEditAccount,
  onAddModel,
  onTest,
  onEditModel,
}: {
  account: ModelAccount
  models: ModelAccountModel[]
  expanded: boolean
  onToggle: () => void
  onEditAccount: () => void
  onAddModel: () => void
  onTest: () => void
  onEditModel: (model: ModelAccountModel) => void
}) {
  return (
    <>
      <tr className={cn(accountTableClasses.tr, expanded && accountTableClasses.trActive)}>
        <td className="px-6 py-4"><input type="checkbox" className="accent-[var(--accent)]" aria-label={`选择 ${account.name}`} disabled /></td>
        <td className={accountTableClasses.td}>
          <button type="button" className={accountTextButtonClass} onClick={onToggle}>
            <span className={accountTableClasses.identity}>
              <span className={accountTableClasses.icon}><CloudIcon /></span>
              <span className="min-w-0">
                <span className={accountTableClasses.title}>{account.name}</span>
                <span className={accountTableClasses.detail}>{credentialsStatusLabel(account.credentials_status?.has_api_key)}</span>
              </span>
            </span>
          </button>
        </td>
        <td className={accountTableClasses.td}>
          <span className={accountTableClasses.stack}>
            <span className="font-bold text-[var(--text)]">{providerAdapterLabel(account.adapter_type)}</span>
            <span className={accountTableClasses.detail}>{providerAuthLabel(account.auth_type)}</span>
          </span>
        </td>
        <td className={accountTableClasses.td}>
          <code className={accountTableClasses.configCode} title={account.base_url}>{account.base_url}</code>
          <span className={accountTableClasses.configMeta}>P:{account.priority} | W:{account.weight} | C:{account.concurrency_limit} | {account.timeout_ms}ms</span>
        </td>
        <td className={accountTableClasses.td}>
          <Badge tone={modelAccountStatusTone(account.status)}>{modelAccountStatusLabel(account.status)}</Badge>
        </td>
        <td className={accountTableClasses.td}>
          <button type="button" className={accountTableClasses.expandButton} onClick={onToggle}>
            {models.length} 个模型
            <ChevronIcon expanded={expanded} />
          </button>
        </td>
        <td className={cn(accountTableClasses.td, 'text-right')}>
          <div className={accountTableClasses.actions}>
            <button className={accountTableClasses.actionLink} type="button" onClick={onEditAccount}>编辑</button>
            <button className={accountTableClasses.actionLink} type="button" onClick={onAddModel}>加模型</button>
            <button className={accountTableClasses.actionLink} type="button" onClick={onTest}>测试</button>
          </div>
        </td>
      </tr>
      {expanded ? (
        <tr className="border-b border-[var(--line)]/60">
          <td colSpan={7} className={accountTableClasses.subPanelCell}>
            <div className={accountTableClasses.subPanel}>
              <div className={accountTableClasses.subHeader}>
                <h4 className={accountTableClasses.subTitle}>支持模型 / Supported Models</h4>
                <button className={cn(adminButton.base, adminButton.ghost, adminButton.small)} type="button" onClick={onAddModel}>添加模型</button>
              </div>
              {models.length ? (
                <table className={accountTableClasses.subTable}>
                  <thead>
                    <tr>
                      <th className={accountTableClasses.subTh}>模型代码 / 名称</th>
                      <th className={accountTableClasses.subTh}>任务类型</th>
                      <th className={accountTableClasses.subTh}>质量标准</th>
                      <th className={accountTableClasses.subTh}>单图成本</th>
                      <th className={accountTableClasses.subTh}>状态</th>
                      <th className={accountTableClasses.subTh}>操作</th>
                    </tr>
                  </thead>
                  <tbody>
                    {models.map((model) => (
                      <tr key={String(model.id)} className="border-b border-[var(--line)]/60 transition-colors last:border-b-0 hover:bg-white/[0.02]">
                        <td className={accountTableClasses.subTd}>
                          <span className={accountTableClasses.stack}>
                            <span className="font-bold text-[var(--text)]">{model.display_name || model.model_code}</span>
                            <code className="font-mono text-[10px] text-[var(--soft)]">{model.model_code}</code>
                          </span>
                        </td>
                        <td className={accountTableClasses.subTd}>
                          <span className={accountTableClasses.tagList}>{model.task_types.map((task) => <span key={task} className={accountTableClasses.modelTag}>{task}</span>)}</span>
                        </td>
                        <td className={accountTableClasses.subTd}>{normalizeQualities(model.qualities).join(', ') || '-'}</td>
                        <td className={accountTableClasses.subTd}>
                          <span className="font-bold text-emerald-400">{model.cost_per_image}</span>
                          <span className="ml-1 text-[10px] uppercase text-[var(--soft)]">{model.currency}</span>
                        </td>
                        <td className={accountTableClasses.subTd}>
                          <span className={cn(accountTableClasses.statusDot, model.enabled ? 'bg-emerald-500 shadow-[0_0_10px_rgba(16,185,129,0.45)]' : 'bg-white/20')} />
                        </td>
                        <td className={accountTableClasses.subTd}>
                          <button className={cn(adminButton.base, adminButton.ghost, adminButton.small)} type="button" onClick={() => onEditModel(model)}>编辑</button>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              ) : <EmptyBlock title="暂无真实模型" detail="为当前账号添加可请求的上游模型代码。" />}
            </div>
          </td>
        </tr>
      ) : null}
    </>
  )
}

function CloudIcon() {
  return (
    <svg className="size-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
      <path d="M17.5 19h.3a5.2 5.2 0 0 0 .2-10.4A6.6 6.6 0 0 0 5.4 9.8 4.8 4.8 0 0 0 6.1 19h11.4Z" />
      <path d="m10 13 2 2 4-4" />
    </svg>
  )
}

function ChevronIcon({ expanded }: { expanded: boolean }) {
  return (
    <svg className={cn('size-4 transition-transform', expanded && 'rotate-180')} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
      <path d="m6 9 6 6 6-6" />
    </svg>
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
