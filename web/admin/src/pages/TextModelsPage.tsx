import { useEffect, useMemo, useState } from 'react'
import { KeyRound, Plus, PlugZap, Save, Star, Trash2 } from 'lucide-react'
import { adminApi } from '../../../shared/admin-api'
import type { TextModel, TextModelAccount } from '../../../shared/api-types'
import { cn } from '../../../shared/classnames'
import { EmptyBlock, InlineFeedback, LoadingBlock } from '../components'
import { adminButton, adminPage } from '../ui/classes'
import {
  accountDraftFromView,
  emptyTextModelAccountDraft,
  modelDraftFromView,
  textModelAccountRequest,
  textModelModelRequest,
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
  const [modelDrafts, setModelDrafts] = useState<TextModelDraft[]>([])
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState(false)
  const [dirty, setDirty] = useState(false)
  const [error, setError] = useState('')
  const [probe, setProbe] = useState('')
  const selected = useMemo(() => accounts.find((item) => String(item.id) === selectedID), [accounts, selectedID])

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
      setAccounts(next)
      const nextID = preferredID ?? (next[0] ? String(next[0].id) : 'new')
      setSelectedID(nextID)
      const account = next.find((item) => String(item.id) === nextID)
      setAccountDraft(account ? accountDraftFromView(account) : emptyTextModelAccountDraft())
      if (account) {
        const accountModels = await adminApi.listTextModels(account.id)
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
        setProbe(`${result.model_code} 连接成功 · ${result.latency_ms}ms`)
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

  return (
    <div className="grid min-w-0 gap-4 xl:grid-cols-[260px_minmax(0,1fr)]">
      <aside className="min-w-0 border-r border-[var(--border)] pr-4 max-xl:border-r-0 max-xl:border-b max-xl:pb-4 max-xl:pr-0">
        <div className="mb-3 flex items-center justify-between gap-2">
          <strong className="text-sm">模型账号</strong>
          <button type="button" className={cn(adminButton.base, adminButton.ghost, 'size-9 px-0')} title="新增文本模型账号" disabled={busy} onClick={() => { setSelectedID('new'); setAccountDraft(emptyTextModelAccountDraft()); setModels([]); setModelDrafts([]); markDirty() }}><Plus size={16} /></button>
        </div>
        <div className="grid gap-1.5">
          {accounts.map((account) => (
            <button key={account.id} type="button" className={cn('flex min-h-12 min-w-0 w-full items-center justify-between gap-3 rounded-md border px-3 text-left text-sm', selectedID === String(account.id) ? 'border-[var(--accent)] bg-[color-mix(in_oklch,var(--accent)_10%,transparent)]' : 'border-transparent hover:border-[var(--border)]')} onClick={() => void selectAccount(account)}>
              <span className="flex min-w-0 flex-1 overflow-hidden"><span className="min-w-0 flex-1"><span className="block truncate font-semibold">{account.name}</span><span className="block truncate text-xs text-[var(--muted)]">{account.api_style === 'responses' ? 'Responses' : 'Chat Completions'}</span></span></span>
              <span className={cn('size-2 shrink-0 rounded-full', account.enabled ? 'bg-emerald-500' : 'bg-[var(--muted)]')} />
            </button>
          ))}
          {!accounts.length ? <EmptyBlock title="暂无账号" detail="新增一个 OpenAI 兼容账号。" /> : null}
        </div>
      </aside>

      <section className="min-w-0">
        {error ? <InlineFeedback tone="danger" message={error} /> : null}
        {probe ? <InlineFeedback tone="success" message={probe} /> : null}
        <div className="grid gap-4 md:grid-cols-2">
          <Field label="账号名称"><input value={accountDraft.name} onChange={(event) => updateAccountDraft({ name: event.target.value })} /></Field>
          <Field label="平台类型"><select value={accountDraft.platformType} disabled><option value="openai_compatible">OpenAI Compatible</option></select></Field>
          <Field label="接口格式"><div className="grid grid-cols-2 gap-1 rounded-md border border-[var(--border)] p-1">{(['responses', 'chat_completions'] as const).map((value) => <button key={value} type="button" className={cn('min-h-9 rounded px-2 text-xs font-semibold', accountDraft.apiStyle === value ? 'bg-[var(--accent)] text-white' : 'text-[var(--muted)] hover:bg-[var(--surface)]')} onClick={() => updateAccountDraft({ apiStyle: value })}>{value === 'responses' ? 'Responses' : 'Chat'}</button>)}</div></Field>
          <Field label="Base URL"><input value={accountDraft.baseUrl} placeholder="https://api.example.com" onChange={(event) => updateAccountDraft({ baseUrl: event.target.value })} /></Field>
          <Field label="API 密钥"><div className="flex gap-2"><select className="max-w-28" value={accountDraft.secretMode} onChange={(event) => updateAccountDraft({ secretMode: event.target.value as TextModelAccountDraft['secretMode'], apiKey: '' })}><option value="keep">保持</option><option value="replace">替换</option><option value="clear">清除</option></select><input type="password" disabled={accountDraft.secretMode !== 'replace'} value={accountDraft.apiKey} placeholder={accountDraft.hasSecret ? `已配置 ${accountDraft.fingerprint}` : '未配置'} onChange={(event) => updateAccountDraft({ apiKey: event.target.value })} /></div></Field>
          <Field label="账号状态"><label className="flex min-h-10 items-center gap-2"><input type="checkbox" checked={accountDraft.enabled} onChange={(event) => updateAccountDraft({ enabled: event.target.checked })} /><span>{accountDraft.enabled ? '启用' : '停用'}</span></label></Field>
        </div>
        <div className="mt-4 flex flex-wrap justify-end gap-2">
          {accountDraft.id ? <button type="button" className={cn(adminButton.base, adminButton.ghost)} disabled={busy || models.length > 0} onClick={() => void deleteAccount()}><Trash2 size={15} />删除账号</button> : null}
          <button type="button" className={cn(adminButton.base, adminButton.primary)} disabled={busy} onClick={() => void saveAccount()}><Save size={15} />保存账号</button>
        </div>

        {selected ? (
          <div className="mt-7 border-t border-[var(--border)] pt-5">
            <div className="mb-3 flex items-center justify-between gap-3"><div><h3 className="m-0 text-base">支持模型</h3><p className="m-0 mt-1 text-xs text-[var(--muted)]">价格单位为每百万 Token，未公开别名保持 0。</p></div><button type="button" className={cn(adminButton.base, adminButton.ghost)} disabled={busy} onClick={() => { setModelDrafts((items) => [...items, emptyModelDraft()]); markDirty() }}><Plus size={15} />新增模型</button></div>
            <div className="overflow-x-auto border-y border-[var(--border)]">
              <table className="w-full min-w-[850px] border-collapse text-sm"><thead><tr className="text-left text-xs text-[var(--muted)]"><th className="p-2">模型</th><th className="p-2">输入价</th><th className="p-2">输出价</th><th className="p-2">币种</th><th className="p-2">启用</th><th className="p-2 text-right">操作</th></tr></thead><tbody>{modelDrafts.map((draft, index) => { const persisted = models.find((item) => String(item.id) === String(draft.id)); return <tr key={String(draft.id ?? `new-${index}`)} className="border-t border-[var(--border)] align-top"><td className="p-2"><input value={draft.modelCode} placeholder="model-id" onChange={(event) => updateModelDraft(index, { modelCode: event.target.value })} /><input className="mt-1" value={draft.displayName} placeholder="展示名称" onChange={(event) => updateModelDraft(index, { displayName: event.target.value })} />{draft.isDefault ? <span className="mt-1 inline-flex items-center gap-1 text-xs text-[var(--accent)]"><Star size={12} fill="currentColor" />默认优化模型</span> : null}</td><td className="p-2"><input inputMode="decimal" value={draft.inputPrice} onChange={(event) => updateModelDraft(index, { inputPrice: event.target.value })} /></td><td className="p-2"><input inputMode="decimal" value={draft.outputPrice} onChange={(event) => updateModelDraft(index, { outputPrice: event.target.value })} /></td><td className="p-2"><input className="w-20" value={draft.currency} onChange={(event) => updateModelDraft(index, { currency: event.target.value })} /></td><td className="p-2"><input type="checkbox" checked={draft.enabled} onChange={(event) => updateModelDraft(index, { enabled: event.target.checked })} /></td><td className="p-2"><div className="flex justify-end gap-1"><IconButton title="保存模型" disabled={busy} onClick={() => void saveModel(index)}><Save size={15} /></IconButton>{persisted ? <><IconButton title="测试连接" disabled={busy || !persisted.enabled || !selected.enabled} onClick={() => void modelAction('test', persisted)}><PlugZap size={15} /></IconButton><IconButton title="设为默认优化模型" disabled={busy || !persisted.enabled || persisted.is_default} onClick={() => void modelAction('default', persisted)}><Star size={15} /></IconButton><IconButton title="删除模型" disabled={busy} onClick={() => void modelAction('delete', persisted)}><Trash2 size={15} /></IconButton></> : null}</div></td></tr> })}</tbody></table>
            </div>
          </div>
        ) : <div className="mt-6 flex items-center gap-2 text-sm text-[var(--muted)]"><KeyRound size={16} />请先保存账号，再配置模型。</div>}
      </section>
    </div>
  )
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return <label className="grid gap-1.5 text-sm text-[var(--muted)]"><span className="font-semibold text-[var(--fg)]">{label}</span>{children}</label>
}

function IconButton({ title, disabled, onClick, children }: { title: string; disabled?: boolean; onClick: () => void; children: React.ReactNode }) {
  return <button type="button" className={cn(adminButton.base, adminButton.ghost, 'size-9 px-0')} title={title} aria-label={title} disabled={disabled} onClick={onClick}>{children}</button>
}

function errorText(error: unknown) {
  return error instanceof Error ? error.message : '操作失败，请稍后重试'
}
