import { Maximize2, Sparkles, Undo2 } from 'lucide-react'
import type { ReferenceAsset } from '../../../shared/api-types'
import { cn } from '../../../shared/classnames'
import { userApi } from '../../../shared/user-api'
import { Button, Modal } from '../components'
import type { PromptOptimizationState } from './workspacePromptOptimization'

export function PromptEditorActions({ optimizing, canUndo, onExpand, onOptimize, onUndo }: {
  optimizing: boolean
  canUndo: boolean
  onExpand?: () => void
  onOptimize: () => void
  onUndo: () => void
}) {
  return <div className="flex items-center gap-1">{canUndo ? <IconAction label="撤销提示词优化" onClick={onUndo}><Undo2 size={15} /></IconAction> : null}{onExpand ? <IconAction label="展开提示词编辑器" onClick={onExpand}><Maximize2 size={15} /></IconAction> : null}<IconAction label="优化提示词" disabled={optimizing} onClick={onOptimize}><Sparkles size={15} /></IconAction></div>
}

export function PromptEditorDialog({ prompt, assets, accessToken, optimization, onPromptChange, onClose, onOptimize, onConfirm, onApply, onCancel, onUndo }: {
  prompt: string
  assets: ReferenceAsset[]
  accessToken?: string | null
  optimization: PromptOptimizationState
  onPromptChange: (prompt: string) => void
  onClose: () => void
  onOptimize: () => void
  onConfirm: () => void
  onApply: () => void
  onCancel: () => void
  onUndo: () => void
}) {
  const busy = optimization.stage === 'estimating' || optimization.stage === 'optimizing'
  return <Modal title="提示词编辑器" onClose={onClose} className="max-[600px]:h-[calc(100dvh-1rem)] max-[600px]:max-h-none max-[600px]:w-[calc(100vw-1rem)] max-[600px]:rounded-lg max-[600px]:p-4">
    <div className="mt-5 grid gap-5 lg:grid-cols-[minmax(0,1fr)_240px]">
      <div className="min-w-0">
        <div className="mb-2 flex items-center justify-between gap-3"><label className="text-sm font-bold" htmlFor="expanded-prompt-editor">提示词</label><PromptEditorActions optimizing={busy} canUndo={optimization.stage === 'applied'} onOptimize={onOptimize} onUndo={onUndo} /></div>
        <textarea id="expanded-prompt-editor" autoFocus maxLength={4000} className="min-h-[48vh] w-full resize-y rounded-md border border-[var(--border)] bg-[var(--bg)] p-4 text-sm leading-7 text-[var(--fg)] outline-none focus:border-[var(--accent)]" value={prompt} disabled={busy} onChange={(event) => onPromptChange(event.target.value)} />
        <p className="m-0 mt-2 text-right text-xs text-[var(--muted)]">{Array.from(prompt).length} / 4000</p>
        {optimization.stage !== 'idle' && optimization.stage !== 'applied' ? <PromptOptimizationPanel state={optimization} onConfirm={onConfirm} onApply={onApply} onCancel={onCancel} /> : null}
      </div>
      <aside className="min-w-0 border-l border-[var(--border)] pl-5 max-lg:border-l-0 max-lg:border-t max-lg:pl-0 max-lg:pt-4">
        <h3 className="m-0 text-sm">图片编辑来源</h3>
        <p className="m-0 mt-1 text-xs text-[var(--muted)]">这些图片仅用于当前图片任务，不会发送给文本模型。</p>
        <div className="mt-3 grid grid-cols-2 gap-2 lg:grid-cols-1">
          {assets.map((asset) => { const src = userApi.imageAssetUrl(asset.preview_url || asset.download_url || '', accessToken); return <div key={asset.id || src} className="grid grid-cols-[52px_minmax(0,1fr)] items-center gap-2 rounded-md border border-[var(--border)] p-2"><div className="size-[52px] overflow-hidden rounded bg-[var(--bg)]">{src ? <img className="size-full object-cover" src={src} alt="" /> : null}</div><div className="min-w-0"><strong className="block truncate text-xs">{asset.name || '编辑图片'}</strong><span className="block truncate text-[11px] text-[var(--muted)]">{asset.width && asset.height ? `${asset.width} × ${asset.height}` : asset.mime_type || '图片'}</span></div></div> })}
          {!assets.length ? <div className="col-span-full rounded-md border border-dashed border-[var(--border)] p-4 text-center text-xs text-[var(--muted)]">当前没有图片编辑来源</div> : null}
        </div>
      </aside>
    </div>
  </Modal>
}

export function PromptOptimizationPanel({ state, onConfirm, onApply, onCancel }: {
  state: PromptOptimizationState
  onConfirm: () => void
  onApply: () => void
  onCancel: () => void
}) {
  if (state.stage === 'estimating' || state.stage === 'optimizing') return <div className="mt-4 rounded-md border border-[var(--border)] bg-[var(--surface)] p-4 text-sm">{state.stage === 'estimating' ? '正在计算预估费用...' : '正在优化提示词...'}</div>
  if (state.stage === 'error') return <div className="mt-4 rounded-md border border-[var(--accent-coral)] p-4"><p className="m-0 text-sm text-[var(--accent-coral)]">{state.error}</p><div className="mt-3 flex justify-end"><Button tone="ghost" onClick={onCancel}>关闭</Button></div></div>
  if (state.stage === 'confirming' && state.estimate) return <div className="mt-4 rounded-md border border-[var(--border)] bg-[var(--surface)] p-4"><div className="flex flex-wrap items-center justify-between gap-3"><div><strong className="block text-sm">确认优化提示词</strong><span className="text-xs text-[var(--muted)]">{state.estimate.model.display_name} · 预计 {state.estimate.estimated_points} 积分</span></div><div className="flex gap-2"><Button tone="ghost" onClick={onCancel}>取消</Button><Button onClick={onConfirm}>确认优化</Button></div></div></div>
  if (state.stage === 'comparing' && state.result) return <div className="mt-4 border-t border-[var(--border)] pt-4"><div className="grid gap-3 md:grid-cols-2"><PromptCompare label="原提示词" value={state.originalPrompt} /><PromptCompare label="优化后" value={state.result.optimized_prompt} accent /></div><div className="mt-3 flex justify-end gap-2"><Button tone="ghost" onClick={onCancel}>保留原文</Button><Button onClick={onApply}>应用优化</Button></div></div>
  return null
}

function PromptCompare({ label, value, accent = false }: { label: string; value: string; accent?: boolean }) {
  return <div className={cn('rounded-md border p-3', accent ? 'border-[var(--accent)] bg-[color-mix(in_oklch,var(--accent)_8%,transparent)]' : 'border-[var(--border)]')}><strong className="mb-2 block text-xs">{label}</strong><p className="m-0 max-h-44 overflow-auto whitespace-pre-wrap text-sm leading-6">{value}</p></div>
}

function IconAction({ label, disabled, onClick, children }: { label: string; disabled?: boolean; onClick: () => void; children: React.ReactNode }) {
  return <button type="button" className="grid size-8 place-items-center rounded border border-[var(--border)] bg-[var(--surface)] text-[var(--muted)] transition hover:border-[var(--accent)] hover:text-[var(--accent)] disabled:cursor-not-allowed disabled:opacity-45" title={label} aria-label={label} disabled={disabled} onClick={onClick}>{children}</button>
}
