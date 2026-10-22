import { useEffect, useMemo, useState } from 'react'
import { KeyRound, Plus, PlugZap, Save, Star, Trash2 } from 'lucide-react'
import { adminApi } from '../../../shared/admin-api'
import type { TextModel, TextModelAccount } from '../../../shared/api-types'
import { cn } from '../../../shared/classnames'
import { Button, EmptyBlock, InlineFeedback, LoadingBlock, TooltipIconButton } from '../components'
import { FormSaveBar, FormSection, LedgerField } from './AdminMediaTabs'
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

const emptyModelDraft = (): TextModelDraft => ({ modelCode: '', displayName: '', inputPrice: '0.000000', outputPrice: '0.000000', currency: 'USD', enabled: false, isDefault: false })

export function TextModelsPage({ onFeedback, onDirtyChange, onBusyChange }: {
  onFeedback: (title: string, detail?: string) => void
  onDirtyChange?: (dirty: boolean) => void
  onBusyChange?: (busy: boolean) => void
}) {
  const [accounts, setAccounts] = useState<TextModelAccount[]>([])
  const [selectedID, setSelectedID] = useState<string>('new')
  const [accountDraft, setAccountDraft] = useState<TextModelAccountDraft>(emptyTextModelAccountDraft)
  const [models, setModels] = useState<TextModel[]>([])
  const [allModels, setAllModels] = useState<TextModel[]>([])
  const [modelDrafts, setModelDrafts] = useState<TextModelDraft[]>([])
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState(false)
  const [dirty, setDirty] = useState(false)
  const [error, setError] = useState('')
  const [probe, setProbe] = useState('')
  const selected = useMemo(() => accounts.find((item) => String(item.id) === selectedID), [accounts, selectedID])
  const readiness = useMemo(() => textModelReadiness(accounts, allModels), [accounts, allModels])
  const accountValidation = validateTextModelAccountDraft(accountDraft)

  const markDirty = () => { setDirty(true); onDirtyChange?.(true) }
  const updateAccountDraft = (patch: Partial<TextModelAccountDraft>) => {
    setAccountDraft((current) => ({ ...current, ...patch }))
    markDirty()
  }
  const updateModelDraft = (index: number, patch: Partial<TextModelDraft>) => {
    setModelDrafts((current) => current.map((item, itemIndex) => itemIndex === index ? { ...item, ...patch } : item))
    markDirty()
  }

  async function loadAccounts(preferredID?: string) {
    setLoading(true)
    try {
      const next = await adminApi.listTextModelAccounts()
      const modelEntries = await Promise.all(next.map(async (account) => [String(account.id), await adminApi.listTextModels(account.id)] as const))
      const modelsByAccount = new Map(modelEntries)
      setAllModels(modelEntries.flatMap(([, accountModels]) => accountModels))
      setAccounts(next)
      const nextID = preferredID ?? (next[0] ? String(next[0].id) : 'new')
      setSelectedID(nextID)
      const account = next.find((item) => String(item.id) === nextID)
      setAccountDraft(account ? accountDraftFromView(account) : emptyTextModelAccountDraft())
      if (account) {
        const accountModels = modelsByAccount.get(String(account.id)) ?? []
        setModels(accountModels)
        setModelDrafts(accountModels.map(modelDraftFromView))
      } else {
        setModels([])
        setModelDrafts([])
      }
      onDirtyChange?.(false)
      setDirty(false)
    } catch (cause) {
      setError(errorText(cause))
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => { void loadAccounts() }, [])
  useEffect(() => onBusyChange?.(busy), [busy, onBusyChange])

  async function selectAccount(account: TextModelAccount) {
    if (busy || (dirty && selectedID !== String(account.id) && !window.confirm('切换账号会放弃当前未保存修改，确定继续吗？'))) return
    setSelectedID(String(account.id))
    setAccountDraft(accountDraftFromView(account))
    setLoading(true)
    try {
      const nextModels = await adminApi.listTextModels(account.id)
      setModels(nextModels)
      setModelDrafts(nextModels.map(modelDraftFromView))
      setError('')
      onDirtyChange?.(false)
      setDirty(false)
    } catch (cause) {
      setError(errorText(cause))
    } finally {
      setLoading(false)
    }
  }

  async function saveAccount() {
    const validation = validateTextModelAccountDraft(accountDraft)
    if (validation) return setError(validation)
    setBusy(true)
    setError('')
    try {
      const saved = accountDraft.id
        ? await adminApi.updateTextModelAccount(accountDraft.id, textModelAccountRequest(accountDraft))
        : await adminApi.createTextModelAccount(textModelAccountRequest(accountDraft))
      onFeedback('文本模型账号已保存', saved.name)
      await loadAccounts(String(saved.id))
    } catch (cause) {
      setError(errorText(cause))
    } finally {
      setBusy(false)
    }
  }

  async function saveModel(index: number) {
    const draft = modelDrafts[index]
    if (!draft || !selected) return
    const validation = validateTextModelDraft(draft)
    if (validation) return setError(validation)
    setBusy(true)
    setError('')
    try {
      if (draft.id) await adminApi.updateTextModel(draft.id, textModelModelRequest(draft))
      else await adminApi.createTextModel(selected.id, textModelModelRequest(draft))
      onFeedback('文本模型已保存', draft.displayName || draft.modelCode)
      await loadAccounts(String(selected.id))
    } catch (cause) {
      setError(errorText(cause))
    } finally {
      setBusy(false)
    }
  }

  async function modelAction(action: 'test' | 'default' | 'delete', model: TextModel) {
    if (action === 'delete' && !window.confirm(`确定删除模型 ${model.display_name} 吗？`)) return
    setBusy(true)
    setError('')
    setProbe('')
    try {
      if (action === 'test') {
        const result = await adminApi.testTextModel(model.id)
        setProbe(`${result.model_code} 连接测试通过 · ${result.latency_ms}ms；不会更改默认优化模型`)
      } else if (action === 'default') {
        await adminApi.setDefaultTextModel(model.id)
        onFeedback('默认优化模型已更新', model.display_name)
      } else {
        await adminApi.deleteTextModel(model.id)
        onFeedback('文本模型已删除', model.display_name)
      }
      if (selected) await loadAccounts(String(selected.id))
    } catch (cause) {
      setError(errorText(cause))
    } finally {
      setBusy(false)
    }
  }

  async function deleteAccount() {
    if (!accountDraft.id || models.length || !window.confirm(`确定删除账号 ${accountDraft.name} 吗？`)) return
    setBusy(true)
    try {
      await adminApi.deleteTextModelAccount(accountDraft.id)
      onFeedback('文本模型账号已删除', accountDraft.name)
      await loadAccounts()
    } catch (cause) {
      setError(errorText(cause))
    } finally {
      setBusy(false)
    }
  }

  if (loading && !accounts.length && selectedID !== 'new') return <LoadingBlock label="正在加载文本模型配置..." />
  const readinessTitle = readiness.status === 'ready' ? '提示词优化已就绪' : '提示词优化尚未就绪'
  const readinessDetail = readiness.status === 'ready'
    ? `默认模型：${readiness.defaultModel?.display_name ?? readiness.defaultModel?.model_code} · ${readiness.defaultAccount?.name ?? '未知账号'}`
    : readiness.status === 'selection_required'
      ? `当前有 ${readiness.eligibleCount} 个可用模型，请选择一个默认优化模型。`
      : '请先启用一个已配置密钥的账号和模型。'

  return (
    <div className="grid min-w-0 gap-4 xl:grid-cols-[260px_minmax(0,1fr)]">
      <aside className="min-w-0 border-r border-[var(--a-line)] pr-4 max-xl:border-r-0 max-xl:border-b max-xl:pb-4 max-xl:pr-0">
        <div className="mb-3 flex items-center justify-between gap-2">
          <strong className="text-sm">模型账号</strong>
          <button type="button" className="a-icon-btn" title="新增文本模型账号" aria-label="新增文本模型账号" disabled={busy} onClick={() => { setSelectedID('new'); setAccountDraft(emptyTextModelAccountDraft()); setModels([]); setModelDrafts([]); markDirty() }}><Plus size={16} /></button>
        </div>
        <div className="grid gap-1.5">
          {accounts.map((account) => (
            <button key={account.id} type="button" className={cn('flex min-h-12 min-w-0 w-full items-center justify-between gap-3 rounded-md border px-3 text-left text-sm', selectedID === String(account.id) ? 'border-[var(--a-accent)] bg-[var(--a-accent-soft)]' : 'border-transparent hover:border-[var(--a-line)]')} onClick={() => void selectAccount(account)}>
              <span className="flex min-w-0 flex-1 overflow-hidden"><span className="min-w-0 flex-1"><span className="block truncate font-semibold">{account.name}</span><span className="block truncate text-xs text-[var(--a-ink-3)]">{account.api_style === 'responses' ? 'Responses' : 'Chat Completions'}</span></span></span>
              <span className={cn('size-2 shrink-0 rounded-full', account.enabled ? 'bg-[var(--a-success)]' : 'bg-[var(--a-ink-3)]')} />
            </button>
          ))}
          {!accounts.length ? <EmptyBlock title="暂无账号" detail="新增一个 OpenAI 兼容账号。" variant="inline" /> : null}
        </div>
      </aside>

      <section className="min-w-0">
        <section className={cn('mb-3 flex min-w-0 items-start gap-3 rounded-md border px-3 py-2.5', readiness.status === 'ready' ? 'border-[color-mix(in_oklch,var(--a-success)_24%,transparent)] bg-[var(--a-success-soft)]' : 'border-[color-mix(in_oklch,var(--a-warn)_24%,transparent)] bg-[var(--a-warn-soft)]')} role="status">
          <span className={cn('mt-1 size-2 shrink-0 rounded-full', readiness.status === 'ready' ? 'bg-[var(--a-success)]' : 'bg-[var(--a-warn)]')} />
          <span className="min-w-0"><strong className="block text-sm">{readinessTitle}</strong><span className="mt-0.5 block text-xs leading-5 text-[var(--a-ink-3)]">{readinessDetail}</span></span>
        </section>
        {error ? <InlineFeedback tone="danger" message={error} /> : null}
        {probe ? <InlineFeedback tone="success" message={probe} /> : null}

        <section className="a-panel a-panel--flush">
          <div className="a-panel__head"><div><h2>文本模型账号</h2><p>OpenAI 兼容接口；密钥只写不读。</p></div></div>
          <form className="a-form" onSubmit={(event) => event.preventDefault()}>
            <FormSection index={1} title="连接">
              <LedgerField label="账号名称"><input className="a-input" value={accountDraft.name} onChange={(event) => updateAccountDraft({ name: event.target.value })} /></LedgerField>
              <LedgerField label="平台类型"><select className="a-select" value={accountDraft.platformType} disabled><option value="openai_compatible">OpenAI Compatible</option></select></LedgerField>
              <LedgerField label="接口格式"><div className="grid grid-cols-2 gap-1 rounded-md border border-[var(--a-line)] p-1">{(['responses', 'chat_completions'] as const).map((value) => <button key={value} type="button" className={cn('min-h-9 rounded px-2 text-xs font-semibold', accountDraft.apiStyle === value ? 'bg-[var(--a-accent)] text-white' : 'text-[var(--a-ink-3)] hover:bg-[var(--a-surface-2)]')} onClick={() => updateAccountDraft({ apiStyle: value })}>{value === 'responses' ? 'Responses' : 'Chat'}</button>)}</div></LedgerField>
              <LedgerField label="Base URL"><input className="a-input" value={accountDraft.baseUrl} placeholder="https://api.example.com" onChange={(event) => updateAccountDraft({ baseUrl: event.target.value })} /></LedgerField>
            </FormSection>
            <FormSection index={2} title="鉴权">
              <LedgerField label="API 密钥"><div className="flex gap-2"><select className="a-select" style={{ maxWidth: 112 }} value={accountDraft.secretMode} onChange={(event) => updateAccountDraft({ secretMode: event.target.value as TextModelAccountDraft['secretMode'], apiKey: '' })}><option value="keep">保持</option><option value="replace">替换</option><option value="clear">清除</option></select><input className="a-input" type="password" disabled={accountDraft.secretMode !== 'replace'} value={accountDraft.apiKey} placeholder={accountDraft.hasSecret ? `已配置 ${accountDraft.fingerprint}` : '未配置'} onChange={(event) => updateAccountDraft({ apiKey: event.target.value })} /></div></LedgerField>
              <LedgerField label="账号状态"><select className="a-select" value={accountDraft.enabled ? 'enabled' : 'disabled'} onChange={(event) => updateAccountDraft({ enabled: event.target.value === 'enabled' })}><option value="enabled">启用</option><option value="disabled">停用</option></select></LedgerField>
            </FormSection>
          </form>
          <FormSaveBar
            dirtyCount={dirty ? 1 : 0}
            errorCount={accountValidation ? 1 : 0}
            busy={busy}
            note={accountValidation || '文本模型不参与图片/视频路由候选；默认模型用于提示词优化。'}
            onReset={() => { const account = accounts.find((item) => String(item.id) === selectedID); setAccountDraft(account ? accountDraftFromView(account) : emptyTextModelAccountDraft()); setDirty(false); onDirtyChange?.(false) }}
            onSave={() => void saveAccount()}
            saveLabel="保存账号"
            extraActions={accountDraft.id ? <Button variant="danger" size="sm" disabled={busy || models.length > 0} onClick={() => void deleteAccount()}>删除账号</Button> : null}
          />
        </section>

        {selected ? (
          <section className="a-panel a-panel--flush" style={{ marginTop: 16 }}>
            <div className="a-panel__head"><div><h3 style={{ fontSize: 15 }}>支持模型</h3><p>价格单位为每百万 Token，未公开别名保持 0。</p></div><Button variant="secondary" size="sm" disabled={busy} onClick={() => { setModelDrafts((items) => [...items, emptyModelDraft()]); markDirty() }}><Plus size={15} />新增模型</Button></div>
            <div className="a-table-wrap">
              <table className="a-table" style={{ minWidth: 850 }}>
                <thead><tr><th>模型</th><th>输入价</th><th>输出价</th><th>币种</th><th>启用</th><th className="is-actions">操作</th></tr></thead>
                <tbody>{modelDrafts.map((draft, index) => {
                  const persisted = models.find((item) => String(item.id) === String(draft.id))
                  const testDisabledReason = busy ? '操作处理中' : !selected.enabled ? '请先启用当前账号' : persisted && !persisted.enabled ? '请先启用该模型' : undefined
                  const defaultDisabledReason = busy ? '操作处理中' : !selected.enabled ? '请先启用当前账号' : persisted?.is_default ? '当前已是默认优化模型' : persisted && !persisted.enabled ? '请先启用该模型' : undefined
                  return (
                    <tr key={String(draft.id ?? `new-${index}`)}>
                      <td><input className="a-input" value={draft.modelCode} placeholder="model-id" onChange={(event) => updateModelDraft(index, { modelCode: event.target.value })} /><input className="a-input" style={{ marginTop: 6 }} value={draft.displayName} placeholder="展示名称" onChange={(event) => updateModelDraft(index, { displayName: event.target.value })} />{draft.isDefault ? <span className="mt-1 inline-flex items-center gap-1 text-xs text-[var(--a-accent)]"><Star size={12} fill="currentColor" />默认优化模型</span> : null}</td>
                      <td><input className="a-input cfg-table-input" inputMode="decimal" value={draft.inputPrice} onChange={(event) => updateModelDraft(index, { inputPrice: event.target.value })} /></td>
                      <td><input className="a-input cfg-table-input" inputMode="decimal" value={draft.outputPrice} onChange={(event) => updateModelDraft(index, { outputPrice: event.target.value })} /></td>
                      <td><input className="a-input cfg-table-input" style={{ width: 80 }} value={draft.currency} onChange={(event) => updateModelDraft(index, { currency: event.target.value })} /></td>
                      <td><input type="checkbox" checked={draft.enabled} onChange={(event) => updateModelDraft(index, { enabled: event.target.checked })} /></td>
                      <td className="is-actions">
                        <div className="a-row-actions">
                          <TooltipIconButton label="保存模型" disabled={busy} disabledReason="操作处理中" onClick={() => void saveModel(index)}><Save /></TooltipIconButton>
                          {persisted ? <>
                            <TooltipIconButton label="测试连接" disabled={Boolean(testDisabledReason)} disabledReason={testDisabledReason} onClick={() => void modelAction('test', persisted)}><PlugZap /></TooltipIconButton>
                            <TooltipIconButton label="设为默认优化模型" disabled={Boolean(defaultDisabledReason)} disabledReason={defaultDisabledReason} onClick={() => void modelAction('default', persisted)}><Star /></TooltipIconButton>
                            <TooltipIconButton label="删除模型" disabled={busy} disabledReason="操作处理中" onClick={() => void modelAction('delete', persisted)}><Trash2 /></TooltipIconButton>
                          </> : null}
                        </div>
                      </td>
                    </tr>
                  )
                })}</tbody>
              </table>
            </div>
            {!modelDrafts.length ? <div className="p-4"><EmptyBlock title="暂无文本模型" detail="新增一个模型代码与展示名称后保存。" variant="inline" /></div> : null}
          </section>
        ) : <div className="mt-6 flex items-center gap-2 text-sm text-[var(--a-ink-3)]"><KeyRound size={16} />请先保存账号，再配置模型。</div>}
      </section>
    </div>
  )
}

function errorText(error: unknown) {
  return error instanceof Error ? error.message : '操作失败，请稍后重试'
}