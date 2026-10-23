import { useEffect, useMemo, useState } from 'react'
import { Plus, PlugZap, Star, Trash2 } from 'lucide-react'
import { adminApi } from '../../../shared/admin-api'
import type { TextModel, TextModelAccount } from '../../../shared/api-types'
import { cn } from '../../../shared/classnames'
import { Badge, Button, Drawer, EmptyBlock, ErrorBlock, InlineFeedback, LoadingBlock, Modal, RefreshIconButton, TooltipIconButton } from '../components'
import { DataTable, FilterToolbar, type ColumnDef } from '../ui/dataTable'
import { FormSection, LedgerField } from './AdminMediaTabs'
import {
  accountDraftFromView,
  emptyTextModelAccountDraft,
  modelDraftFromView,
  textModelAccountRequest,
  textModelModelRequest,
  textModelReadiness,
  validateTextModelAccountDraft,
  validateTextModelDraft,
  type TextModelAccountDraft,
  type TextModelDraft,
} from './textModelRows'

type DeleteTarget =
  | { kind: 'account'; account: TextModelAccount }
  | { kind: 'model'; account: TextModelAccount; model: TextModel }

const emptyModelDraft = (): TextModelDraft => ({ modelCode: '', displayName: '', inputPrice: '0.000000', outputPrice: '0.000000', currency: 'USD', enabled: false, isDefault: false })

export function TextModelsPage({ onFeedback, onDirtyChange, onBusyChange, onAccountsChanged }: {
  onFeedback: (title: string, detail?: string) => void
  onDirtyChange?: (dirty: boolean) => void
  onBusyChange?: (busy: boolean) => void
  onAccountsChanged?: () => void
}) {
  const [accounts, setAccounts] = useState<TextModelAccount[]>([])
  const [modelsByAccount, setModelsByAccount] = useState<Record<string, TextModel[]>>({})
  const [query, setQuery] = useState('')
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState(false)
  const [deleting, setDeleting] = useState(false)
  const [error, setError] = useState('')
  const [mutationError, setMutationError] = useState('')
  const [probe, setProbe] = useState('')
  const [accountDraft, setAccountDraft] = useState<TextModelAccountDraft | null>(null)
  const [modelAccountID, setModelAccountID] = useState('')
  const [modelDraft, setModelDraft] = useState<TextModelDraft | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<DeleteTarget | null>(null)

  const modelAccount = useMemo(() => accounts.find((item) => String(item.id) === modelAccountID), [accounts, modelAccountID])
  const selectedModels = useMemo(() => (modelAccount ? modelsByAccount[String(modelAccount.id)] ?? [] : []), [modelAccount, modelsByAccount])
  const allModels = useMemo(() => Object.values(modelsByAccount).flat(), [modelsByAccount])
  const readiness = useMemo(() => textModelReadiness(accounts, allModels), [accounts, allModels])
  const enabledAccounts = accounts.filter((account) => account.enabled).length
  const missingSecretAccounts = accounts.filter((account) => !account.secret_status.has_secret).length
  const pendingCount = missingSecretAccounts + allModels.filter((model) => !model.enabled).length

  const existingAccount = accountDraft?.id ? accounts.find((account) => String(account.id) === String(accountDraft.id)) : undefined
  const accountValidation = accountDraft ? validateTextModelAccountDraft(accountDraft) : ''
  const accountDirty = accountDraft ? accountDraftDirtyCount(accountDraft, existingAccount) : 0
  const existingModels = existingAccount ? modelsByAccount[String(existingAccount.id)]?.length ?? 0 : 0

  const modelBaseline = modelDraft?.id ? selectedModels.find((model) => String(model.id) === String(modelDraft.id)) : undefined
  const modelValidation = modelDraft ? validateTextModelDraft(modelDraft) : ''
  const modelDirty = modelDraft ? modelDraftDirtyCount(modelDraft, modelBaseline) : 0
  const dirty = Boolean(accountDraft && accountDirty) || Boolean(modelDraft && modelDirty)

  const filteredAccounts = useMemo(() => {
    const keyword = query.trim().toLowerCase()
    if (!keyword) return accounts
    return accounts.filter((account) => [
      account.name,
      account.api_style,
      account.base_url,
      account.enabled ? 'enabled' : 'disabled',
    ].some((value) => String(value ?? '').toLowerCase().includes(keyword)))
  }, [accounts, query])

  async function loadAccounts() {
    setLoading(true)
    setError('')
    try {
      const next = await adminApi.listTextModelAccounts()
      const entries = await Promise.all(next.map(async (account) => [String(account.id), await adminApi.listTextModels(account.id)] as const))
      const nextModels = Object.fromEntries(entries)
      setAccounts(next)
      setModelsByAccount(nextModels)
      setModelAccountID((current) => (next.some((account) => String(account.id) === current) ? current : ''))
    } catch (cause) {
      setError(errorText(cause))
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => { void loadAccounts() }, [])
  useEffect(() => { onBusyChange?.(busy) }, [busy, onBusyChange])
  useEffect(() => { onDirtyChange?.(dirty) }, [dirty, onDirtyChange])

  function openAccountEditor(draft: TextModelAccountDraft) {
    setMutationError('')
    setProbe('')
    setModelDraft(null)
    setModelAccountID('')
    setAccountDraft(draft)
  }

  function openModelDrawer(account: TextModelAccount) {
    setMutationError('')
    setProbe('')
    setModelDraft(null)
    setModelAccountID(String(account.id))
  }

  function openModelEditor(draft: TextModelDraft) {
    setMutationError('')
    setModelDraft(draft)
  }

  async function saveAccount() {
    if (!accountDraft) return
    const validation = validateTextModelAccountDraft(accountDraft)
    if (validation) {
      setMutationError(validation)
      return
    }
    setBusy(true)
    setMutationError('')
    try {
      const creating = !accountDraft.id
      const saved = accountDraft.id
        ? await adminApi.updateTextModelAccount(accountDraft.id, textModelAccountRequest(accountDraft))
        : await adminApi.createTextModelAccount(textModelAccountRequest(accountDraft))
      setAccountDraft(null)
      await loadAccounts()
      if (creating) setModelAccountID(String(saved.id))
      onAccountsChanged?.()
      onFeedback(creating ? '文本模型账号已创建' : '文本模型账号已保存', saved.name)
    } catch (cause) {
      setMutationError(errorText(cause))
    } finally {
      setBusy(false)
    }
  }

  async function saveModel() {
    if (!modelDraft || !modelAccount) return
    const validation = validateTextModelDraft(modelDraft)
    if (validation) {
      setMutationError(validation)
      return
    }
    setBusy(true)
    setMutationError('')
    try {
      const creating = !modelDraft.id
      if (modelDraft.id) await adminApi.updateTextModel(modelDraft.id, textModelModelRequest(modelDraft))
      else await adminApi.createTextModel(modelAccount.id, textModelModelRequest(modelDraft))
      const accountID = String(modelAccount.id)
      setModelDraft(null)
      await loadAccounts()
      setModelAccountID(accountID)
      onFeedback(creating ? '文本模型已创建' : '文本模型已保存', modelDraft.displayName || modelDraft.modelCode)
    } catch (cause) {
      setMutationError(errorText(cause))
    } finally {
      setBusy(false)
    }
  }

  async function runModelAction(action: 'test' | 'default' | 'delete', model: TextModel) {
    if (!modelAccount) return
    if (action === 'delete') {
      setMutationError('')
      setDeleteTarget({ kind: 'model', account: modelAccount, model })
      return
    }
    setBusy(true)
    setMutationError('')
    setProbe('')
    try {
      if (action === 'test') {
        const result = await adminApi.testTextModel(model.id)
        setProbe(`${result.model_code} 连接测试通过 · ${result.latency_ms}ms；不会更改默认优化模型`)
      } else {
        await adminApi.setDefaultTextModel(model.id)
        onFeedback('默认优化模型已更新', model.display_name)
      }
      await loadAccounts()
    } catch (cause) {
      setMutationError(errorText(cause))
    } finally {
      setBusy(false)
    }
  }

  async function confirmDelete() {
    if (!deleteTarget) return
    if (deleteTarget.kind === 'account' && (modelsByAccount[String(deleteTarget.account.id)]?.length ?? 0) > 0) {
      setMutationError('账号下仍有模型，请先删除模型后再删除账号。')
      return
    }
    setDeleting(true)
    setMutationError('')
    try {
      if (deleteTarget.kind === 'account') {
        await adminApi.deleteTextModelAccount(deleteTarget.account.id)
        setAccountDraft(null)
        setModelAccountID('')
        setDeleteTarget(null)
        await loadAccounts()
        onAccountsChanged?.()
        onFeedback('文本模型账号已删除', deleteTarget.account.name)
      } else {
        await adminApi.deleteTextModel(deleteTarget.model.id)
        if (modelDraft?.id && String(modelDraft.id) === String(deleteTarget.model.id)) setModelDraft(null)
        setDeleteTarget(null)
        await loadAccounts()
        setModelAccountID(String(deleteTarget.account.id))
        onFeedback('文本模型已删除', deleteTarget.model.display_name)
      }
    } catch (cause) {
      setMutationError(errorText(cause))
    } finally {
      setDeleting(false)
    }
  }

  if (loading && !accounts.length) return <LoadingBlock label="正在加载文本模型配置..." />
  if (error && !accounts.length) return <ErrorBlock message={error} onRetry={() => void loadAccounts()} />

  const readinessTitle = readiness.status === 'ready' ? '提示词优化已就绪' : '提示词优化尚未就绪'
  const readinessDetail = readiness.status === 'ready'
    ? `默认模型：${readiness.defaultModel?.display_name ?? readiness.defaultModel?.model_code} · ${readiness.defaultAccount?.name ?? '未知账号'}`
    : readiness.status === 'selection_required'
      ? `当前有 ${readiness.eligibleCount} 个可用模型，请选择一个默认优化模型。`
      : '请先启用一个已配置密钥的账号和模型。'

  return (
    <section className="a-page-stack">
      <section className="a-metrics" aria-label="文本模型概况">
        <div className="a-metric"><div className="a-metric__label">文本账号</div><div className="a-metric__value">{accounts.length}</div><div className="a-metric__foot">{enabledAccounts} 启用 · {missingSecretAccounts} 缺密钥</div></div>
        <div className="a-metric"><div className="a-metric__label">文本真实模型</div><div className="a-metric__value">{allModels.length}</div><div className="a-metric__foot">{allModels.filter((model) => model.enabled).length} 启用</div></div>
        <div className="a-metric"><div className="a-metric__label">默认优化模型</div><div className="a-metric__value cfg-metric-value--text">{readiness.defaultModel ? readiness.defaultModel.display_name || readiness.defaultModel.model_code : '未设置'}</div><div className="a-metric__foot">{readiness.defaultAccount?.name ?? '提示词优化'}</div></div>
        <div className="a-metric"><div className="a-metric__label">待完善</div><div className="a-metric__value" style={{ color: pendingCount ? 'var(--a-danger)' : undefined }}>{pendingCount}</div><div className="a-metric__foot">缺密钥账号 / 停用模型</div></div>
      </section>

      <section className={cn('flex min-w-0 items-start gap-3 rounded-md border px-3 py-2.5', readiness.status === 'ready' ? 'border-[color-mix(in_oklch,var(--a-success)_24%,transparent)] bg-[var(--a-success-soft)]' : 'border-[color-mix(in_oklch,var(--a-warn)_24%,transparent)] bg-[var(--a-warn-soft)]')} role="status">
        <span className={cn('mt-1 size-2 shrink-0 rounded-full', readiness.status === 'ready' ? 'bg-[var(--a-success)]' : 'bg-[var(--a-warn)]')} />
        <span className="min-w-0"><strong className="block text-sm">{readinessTitle}</strong><span className="mt-0.5 block text-xs leading-5 text-[var(--a-ink-3)]">{readinessDetail}</span></span>
      </section>

      {error ? <InlineFeedback tone="danger" message={error} /> : null}
      {mutationError && !accountDraft && !modelDraft && !deleteTarget ? <InlineFeedback tone="danger" message={mutationError} /> : null}
      {probe ? <InlineFeedback tone="success" message={probe} /> : null}

      {accounts.length ? (
        <FilterToolbar
          fields={[{
            key: 'query',
            label: '搜索账号',
            primary: true,
            minWidth: '220px',
            maxWidth: '420px',
            control: <input value={query} onChange={(event) => setQuery(event.target.value)} className="a-input" placeholder="账号名称 / 接口格式 / Base URL" aria-label="搜索文本模型账号" />,
          }]}
          resultSummary={`共 ${accounts.length} 个文本账号 · 当前显示 ${filteredAccounts.length} 个`}
        />
      ) : null}

      <section className="a-panel a-panel--flush">
        <div className="a-panel__head">
          <div>
            <h2>文本模型账号</h2>
            <p>OpenAI 兼容接口；密钥只写不读。</p>
          </div>
          <div className="a-row-actions">
            <RefreshIconButton label="刷新文本模型账号" refreshing={loading} onClick={() => void loadAccounts()} />
            <Button variant="primary" size="sm" onClick={() => openAccountEditor(emptyTextModelAccountDraft())}>添加文本账号</Button>
          </div>
        </div>
        {!accounts.length ? (
          <div className="a-panel__body">
            <EmptyBlock title="暂无文本模型账号" detail="新增一个 OpenAI 兼容账号后，再配置文本模型与价格。" action={<Button variant="primary" size="sm" onClick={() => openAccountEditor(emptyTextModelAccountDraft())}>添加文本账号</Button>} />
          </div>
        ) : (
          <DataTable
            className="cfg-account-table"
            minWidth={1080}
            columns={textAccountColumns({
              modelsByAccount,
              busy,
              onEdit: (account) => openAccountEditor(accountDraftFromView(account)),
              onConfigureModels: (account) => openModelDrawer(account),
              onDelete: (account) => { setMutationError(''); setDeleteTarget({ kind: 'account', account }) },
            })}
            rows={filteredAccounts}
            rowKey={(account) => account.id}
            empty={<EmptyBlock title="未找到文本账号" detail="换一个账号名称、接口格式或 Base URL 关键词再试。" />}
          />
        )}
      </section>

      {accountDraft ? (
        <Drawer
          wide
          title={accountDraft.id ? '编辑文本模型账号' : '新增文本模型账号'}
          description={`${accountDraft.name || '新账号'} · ${accountDraft.apiStyle === 'responses' ? 'Responses' : 'Chat Completions'}${accountDraft.id ? ` · ${existingModels} 个文本模型` : ' · 填写连接与鉴权参数'}`}
          headerAside={<span className={cn('a-badge', accountDraft.enabled ? 'a-badge--ok' : 'a-badge--warn')}><i />{accountDraft.enabled ? '启用' : '停用'}</span>}
          onClose={() => { if (!busy) setAccountDraft(null) }}
          footer={<>
            <span className={cn('a-badge', accountValidation ? 'a-badge--bad' : accountDirty ? 'a-badge--warn' : 'a-badge--ok')}><i />{accountValidation ? '1 项校验未通过' : accountDirty ? `${accountDirty} 项未保存` : '未修改'}</span>
            <div style={{ flex: 1 }} />
            {accountDraft.id ? <Button variant="danger" size="sm" disabled={busy || existingModels > 0} title={existingModels > 0 ? '请先删除账号下的模型' : undefined} onClick={() => { if (existingAccount) { setMutationError(''); setDeleteTarget({ kind: 'account', account: existingAccount }) } }}>删除账号</Button> : null}
            <Button variant="ghost" size="sm" disabled={busy} onClick={() => setAccountDraft(null)}>取消</Button>
            <Button variant="primary" size="sm" disabled={busy || Boolean(accountValidation)} onClick={() => void saveAccount()}>{busy ? '保存中...' : '保存账号'}</Button>
          </>}
        >
          {mutationError ? <InlineFeedback tone="danger" message={mutationError} /> : null}
          <form className="a-form" onSubmit={(event) => event.preventDefault()}>
            <FormSection index={1} title="连接">
              <LedgerField label="账号名称" required><input className="a-input" value={accountDraft.name} onChange={(event) => setAccountDraft({ ...accountDraft, name: event.target.value })} /></LedgerField>
              <LedgerField label="平台类型"><select className="a-select" value={accountDraft.platformType} disabled><option value="openai_compatible">OpenAI Compatible</option></select></LedgerField>
              <LedgerField label="接口格式"><div className="grid grid-cols-2 gap-1 rounded-md border border-[var(--a-line)] p-1">{(['responses', 'chat_completions'] as const).map((value) => <button key={value} type="button" className={cn('min-h-9 rounded px-2 text-xs font-semibold', accountDraft.apiStyle === value ? 'bg-[var(--a-accent)] text-white' : 'text-[var(--a-ink-3)] hover:bg-[var(--a-surface-2)]')} onClick={() => setAccountDraft({ ...accountDraft, apiStyle: value })}>{value === 'responses' ? 'Responses' : 'Chat'}</button>)}</div></LedgerField>
              <LedgerField label="Base URL" required hint="仅支持无凭据的 HTTP(S) 地址。"><input className="a-input" value={accountDraft.baseUrl} placeholder="https://api.example.com" onChange={(event) => setAccountDraft({ ...accountDraft, baseUrl: event.target.value })} /></LedgerField>
            </FormSection>
            <FormSection index={2} title="鉴权" description="密钥只写不读，保存后显示掩码。">
              <LedgerField label="API 密钥"><div className="flex gap-2"><select className="a-select" style={{ maxWidth: 112 }} value={accountDraft.secretMode} onChange={(event) => setAccountDraft({ ...accountDraft, secretMode: event.target.value as TextModelAccountDraft['secretMode'], apiKey: '' })}><option value="keep">保持</option><option value="replace">替换</option><option value="clear">清除</option></select><input className="a-input" type="password" disabled={accountDraft.secretMode !== 'replace'} value={accountDraft.apiKey} placeholder={accountDraft.hasSecret ? `已配置 ${accountDraft.fingerprint}` : '未配置'} onChange={(event) => setAccountDraft({ ...accountDraft, apiKey: event.target.value })} /></div></LedgerField>
              <LedgerField label="账号状态"><select className="a-select" value={accountDraft.enabled ? 'enabled' : 'disabled'} onChange={(event) => setAccountDraft({ ...accountDraft, enabled: event.target.value === 'enabled' })}><option value="enabled">启用</option><option value="disabled">停用</option></select></LedgerField>
            </FormSection>
          </form>
        </Drawer>
      ) : null}

      {modelAccount ? (
        <Drawer
          wide
          title="文本模型配置"
          description={`${modelAccount.name} · ${selectedModels.length} 个文本模型 · ${modelAccount.api_style === 'responses' ? 'Responses' : 'Chat Completions'}`}
          headerAside={<span className={cn('a-badge', modelAccount.enabled ? 'a-badge--ok' : 'a-badge--warn')}><i />{modelAccount.enabled ? '账号启用中' : '账号未启用'}</span>}
          onClose={() => { if (modelDraft) return; setModelAccountID('') }}
          footer={<>
            <Button variant="ghost" size="sm" onClick={() => { setModelAccountID(''); setModelDraft(null) }}>关闭</Button>
            <div style={{ flex: 1 }} />
            <Button variant="primary" size="sm" disabled={busy} onClick={() => openModelEditor(emptyModelDraft())}><Plus size={15} />添加文本模型</Button>
          </>}
        >
          <div className="aa-summary">
            <div><span>文本模型</span><strong>{selectedModels.length} 个</strong></div>
            <div><span>已启用</span><strong>{selectedModels.filter((model) => model.enabled).length} 个</strong></div>
            <div><span>默认优化</span><strong>{selectedModels.find((model) => model.is_default)?.display_name ?? '未设置'}</strong></div>
            <div><span>待完善</span><strong>{selectedModels.filter((model) => !model.enabled).length} 个</strong></div>
          </div>
          <div className="a-section-title" style={{ marginBottom: 0 }}>
            <div>
              <h3 style={{ fontSize: 15 }}>真实文本模型</h3>
              <p>价格单位为每百万 Token；默认优化模型必须启用后才能参与提示词优化。</p>
            </div>
          </div>
          {mutationError && !modelDraft ? <InlineFeedback tone="danger" message={mutationError} /> : null}
          {probe ? <InlineFeedback tone="success" message={probe} /> : null}
          {!selectedModels.length ? (
            <div className="aa-empty"><strong>暂无文本模型</strong><p>为当前账号添加模型代码、展示名称与每百万 Token 价格。</p><Button variant="primary" size="sm" disabled={busy} onClick={() => openModelEditor(emptyModelDraft())}>添加文本模型</Button></div>
          ) : (
            <DataTable
              className="cfg-account-table"
              minWidth={820}
              columns={textModelColumns({
                account: modelAccount,
                busy,
                onEdit: (model) => openModelEditor(modelDraftFromView(model)),
                onTest: (model) => void runModelAction('test', model),
                onDefault: (model) => void runModelAction('default', model),
                onDelete: (model) => { setMutationError(''); setDeleteTarget({ kind: 'model', account: modelAccount, model }) },
              })}
              rows={selectedModels}
              rowKey={(model) => model.id}
              empty={<EmptyBlock title="暂无文本模型" detail="添加模型后即可参与提示词优化。" />}
            />
          )}
          <div className="a-banner a-banner--info">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.6" aria-hidden="true"><circle cx="12" cy="12" r="9" /><path d="M12 11v5M12 7.5h.01" /></svg>
            <div><strong style={{ fontSize: 13 }}>文本模型不参与图片/视频路由</strong><div className="a-hint">默认优化模型只用于提示词优化；测试连接不会更改默认模型，也不会扣减积分。</div></div>
          </div>
        </Drawer>
      ) : null}

      {modelDraft && modelAccount ? (
        <Drawer
          stacked={Boolean(modelAccount)}
          wide
          title="文本模型编辑"
          description={`${modelAccount.name} · ${modelDraft.displayName || '新增文本模型'}${modelDraft.modelCode ? ` · ${modelDraft.modelCode}` : ''}`}
          headerAside={<span className={cn('a-badge', modelDraft.enabled ? 'a-badge--ok' : 'a-badge--warn')}><i />{modelDraft.enabled ? '启用' : '停用'}</span>}
          onClose={() => { if (!busy) setModelDraft(null) }}
          footer={<>
            <span className={cn('a-badge', modelValidation ? 'a-badge--bad' : modelDirty ? 'a-badge--warn' : 'a-badge--ok')}><i />{modelValidation ? '1 项校验未通过' : modelDirty ? `${modelDirty} 项未保存` : '未修改'}</span>
            <div style={{ flex: 1 }} />
            <Button variant="ghost" size="sm" disabled={busy} onClick={() => setModelDraft(null)}>取消</Button>
            <Button variant="primary" size="sm" disabled={busy || Boolean(modelValidation)} onClick={() => void saveModel()}>{busy ? '保存中...' : '保存模型'}</Button>
          </>}
        >
          {mutationError ? <InlineFeedback tone="danger" message={mutationError} /> : null}
          <form className="a-form" onSubmit={(event) => event.preventDefault()}>
            <FormSection index={1} title="模型标识">
              <LedgerField label="模型代码" required><input className="a-input" value={modelDraft.modelCode} placeholder="model-id" onChange={(event) => setModelDraft({ ...modelDraft, modelCode: event.target.value })} style={{ fontFamily: 'var(--font-admin-mono)' }} /></LedgerField>
              <LedgerField label="展示名称"><input className="a-input" value={modelDraft.displayName} placeholder="展示名称" onChange={(event) => setModelDraft({ ...modelDraft, displayName: event.target.value })} /></LedgerField>
            </FormSection>
            <FormSection index={2} title="价格" description="每百万 Token 的输入与输出价格。">
              <div className="a-form__pair">
                <LedgerField label="输入价" required><input className="a-input" inputMode="decimal" value={modelDraft.inputPrice} onChange={(event) => setModelDraft({ ...modelDraft, inputPrice: event.target.value })} /></LedgerField>
                <LedgerField label="输出价" required><input className="a-input" inputMode="decimal" value={modelDraft.outputPrice} onChange={(event) => setModelDraft({ ...modelDraft, outputPrice: event.target.value })} /></LedgerField>
              </div>
              <LedgerField label="币种" required><input className="a-input" value={modelDraft.currency} placeholder="USD" onChange={(event) => setModelDraft({ ...modelDraft, currency: event.target.value })} /></LedgerField>
            </FormSection>
            <FormSection index={3} title="状态">
              <LedgerField label="模型状态"><select className="a-select" value={modelDraft.enabled ? 'enabled' : 'disabled'} onChange={(event) => setModelDraft({ ...modelDraft, enabled: event.target.value === 'enabled' })}><option value="enabled">启用</option><option value="disabled">停用</option></select></LedgerField>
            </FormSection>
          </form>
        </Drawer>
      ) : null}

      {deleteTarget ? (
        <Modal
          title={deleteTarget.kind === 'account' ? '删除文本模型账号' : '删除文本模型'}
          detail={deleteTarget.kind === 'account' ? deleteTarget.account.name : `${deleteTarget.account.name} / ${deleteTarget.model.model_code}`}
          onClose={() => { if (!deleting) setDeleteTarget(null) }}
          footer={<><Button variant="ghost" disabled={deleting} onClick={() => setDeleteTarget(null)}>取消</Button><Button variant="danger" disabled={deleting} onClick={() => void confirmDelete()}>{deleting ? '删除中...' : '确认删除'}</Button></>}
        >
          {mutationError ? <InlineFeedback tone="danger" message={mutationError} /> : null}
          <p className="m-0 text-sm leading-6 text-[var(--a-ink-2)]">{deleteTarget.kind === 'account' ? '删除后该账号不再参与提示词优化；请先删除账号下的模型。' : '删除后该模型不再参与提示词优化；历史结果不受影响。'}</p>
        </Modal>
      ) : null}
    </section>
  )
}

function textAccountColumns({ modelsByAccount, busy, onEdit, onConfigureModels, onDelete }: {
  modelsByAccount: Record<string, TextModel[]>
  busy: boolean
  onEdit: (account: TextModelAccount) => void
  onConfigureModels: (account: TextModelAccount) => void
  onDelete: (account: TextModelAccount) => void
}): ColumnDef<TextModelAccount>[] {
  return [
    {
      key: 'account',
      title: '账号',
      width: 'minmax(220px,2fr)',
      render: (account) => (
        <span className="cfg-account-cell">
          <span className="cfg-account-cell__logo">{account.name.slice(0, 1).toUpperCase()}</span>
          <span className="a-cell-stack"><strong>{account.name}</strong><span>OpenAI Compatible · {account.api_style === 'responses' ? 'Responses' : 'Chat Completions'}</span></span>
        </span>
      ),
    },
    { key: 'apiStyle', title: '接口格式', width: 'minmax(140px,1fr)', render: (account) => <Badge tone="primary">{account.api_style === 'responses' ? 'Responses' : 'Chat Completions'}</Badge> },
    { key: 'baseUrl', title: 'Base URL', width: 'minmax(220px,2fr)', kind: 'code', render: (account) => <code className="a-mono text-[11.5px]" title={account.base_url}>{account.base_url}</code> },
    { key: 'secret', title: '密钥', width: 'minmax(100px,.8fr)', render: (account) => <Badge tone={account.secret_status.has_secret ? 'success' : 'danger'}>{account.secret_status.has_secret ? '已配置' : '未配置'}</Badge> },
    { key: 'models', title: '文本模型', width: 'minmax(90px,.7fr)', align: 'right', kind: 'number', render: (account) => <span className="a-num">{modelsByAccount[String(account.id)]?.length ?? 0}</span> },
    { key: 'default', title: '默认优化', width: 'minmax(140px,1.1fr)', render: (account) => { const current = (modelsByAccount[String(account.id)] ?? []).find((model) => model.is_default); return current ? <Badge tone="success">{current.display_name || current.model_code}</Badge> : <span className="a-hint">未设置</span> } },
    { key: 'status', title: '状态', width: 'minmax(90px,.7fr)', render: (account) => <Badge tone={account.enabled ? 'success' : 'warning'}>{account.enabled ? '启用' : '停用'}</Badge> },
    {
      key: 'actions',
      title: '操作',
      width: 'minmax(220px,1.4fr)',
      align: 'right',
      render: (account) => {
        const modelCount = modelsByAccount[String(account.id)]?.length ?? 0
        return (
          <span className="a-row-actions">
            <button className="a-row-btn" type="button" onClick={() => onEdit(account)}>编辑</button>
            <button className="a-row-btn a-row-btn--primary" type="button" onClick={() => onConfigureModels(account)}>模型配置</button>
            <TooltipIconButton label={`删除账号 ${account.name}`} disabled={busy || modelCount > 0} disabledReason={modelCount > 0 ? '请先删除账号下的模型' : undefined} onClick={() => onDelete(account)}><Trash2 /></TooltipIconButton>
          </span>
        )
      },
    },
  ]
}

function textModelColumns({ account, busy, onEdit, onTest, onDefault, onDelete }: {
  account: TextModelAccount
  busy: boolean
  onEdit: (model: TextModel) => void
  onTest: (model: TextModel) => void
  onDefault: (model: TextModel) => void
  onDelete: (model: TextModel) => void
}): ColumnDef<TextModel>[] {
  return [
    {
      key: 'model',
      title: '模型',
      width: 'minmax(220px,1.8fr)',
      render: (model) => (
        <span className="a-cell-stack">
          <strong>{model.display_name || model.model_code} {model.is_default ? <span className="a-badge a-badge--ok" style={{ marginLeft: 6 }}><i />默认</span> : null}</strong>
          <span>{model.model_code}</span>
        </span>
      ),
    },
    { key: 'inputPrice', title: '输入价 / 百万 Token', width: 'minmax(140px,1fr)', align: 'right', kind: 'number', render: (model) => <span className="a-num">{model.input_price_per_million_tokens} {model.currency}</span> },
    { key: 'outputPrice', title: '输出价 / 百万 Token', width: 'minmax(140px,1fr)', align: 'right', kind: 'number', render: (model) => <span className="a-num">{model.output_price_per_million_tokens} {model.currency}</span> },
    { key: 'status', title: '状态', width: 'minmax(80px,.6fr)', render: (model) => <Badge tone={model.enabled ? 'success' : 'warning'}>{model.enabled ? '启用' : '停用'}</Badge> },
    {
      key: 'actions',
      title: '操作',
      width: 'minmax(180px,1.2fr)',
      align: 'right',
      render: (model) => {
        const testDisabledReason = busy ? '操作处理中' : !account.enabled ? '请先启用当前账号' : !model.enabled ? '请先启用该模型' : undefined
        const defaultDisabledReason = busy ? '操作处理中' : !account.enabled ? '请先启用当前账号' : model.is_default ? '当前已是默认优化模型' : !model.enabled ? '请先启用该模型' : undefined
        return (
          <span className="a-row-actions">
            <button className="a-row-btn a-row-btn--primary" type="button" disabled={busy} onClick={() => onEdit(model)}>编辑</button>
            <TooltipIconButton label="测试连接" disabled={Boolean(testDisabledReason)} disabledReason={testDisabledReason} onClick={() => onTest(model)}><PlugZap /></TooltipIconButton>
            <TooltipIconButton label="设为默认优化模型" disabled={Boolean(defaultDisabledReason)} disabledReason={defaultDisabledReason} onClick={() => onDefault(model)}><Star /></TooltipIconButton>
            <TooltipIconButton label="删除模型" disabled={busy} disabledReason="操作处理中" onClick={() => onDelete(model)}><Trash2 /></TooltipIconButton>
          </span>
        )
      },
    },
  ]
}

function accountDraftDirtyCount(draft: TextModelAccountDraft, baseline?: TextModelAccount) {
  const current = accountDraftSignature(draft)
  const base = baseline ? accountDraftSignature(accountDraftFromView(baseline)) : accountDraftSignature(emptyTextModelAccountDraft())
  return JSON.stringify(current) === JSON.stringify(base) ? 0 : 1
}

function accountDraftSignature(draft: TextModelAccountDraft) {
  return {
    name: draft.name,
    apiStyle: draft.apiStyle,
    baseUrl: draft.baseUrl,
    enabled: draft.enabled,
    secretMode: draft.secretMode,
    apiKey: draft.secretMode === 'replace' ? draft.apiKey : '',
  }
}

function modelDraftDirtyCount(draft: TextModelDraft, baseline?: TextModel) {
  const current = textModelModelRequest(draft)
  const base = baseline ? textModelModelRequest(modelDraftFromView(baseline)) : textModelModelRequest(emptyModelDraft())
  return JSON.stringify(current) === JSON.stringify(base) ? 0 : 1
}

function errorText(error: unknown) {
  return error instanceof Error ? error.message : '操作失败，请稍后重试'
}