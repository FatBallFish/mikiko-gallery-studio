import React, { useEffect, useState } from 'react'
import { Check, Circle, LoaderCircle, TriangleAlert } from 'lucide-react'
import { cn } from '../../../shared/classnames'
import type { WorkspaceTaskView } from './workspaceViewModel'

const toneClass = {
  idle: 'border-[var(--border)] bg-[var(--surface)] text-[var(--dim)]',
  active: 'border-[var(--accent)] bg-[color-mix(in_oklch,var(--accent)_14%,var(--surface))] text-[var(--accent)] shadow-[0_0_20px_rgba(var(--accent-rgb),0.12)]',
  done: 'border-[color-mix(in_oklch,var(--accent-emerald)_46%,var(--border))] bg-[color-mix(in_oklch,var(--accent-emerald)_12%,var(--surface))] text-[var(--accent-emerald)]',
  failed: 'border-[color-mix(in_oklch,var(--accent-coral)_54%,var(--border))] bg-[color-mix(in_oklch,var(--accent-coral)_12%,var(--surface))] text-[var(--accent-coral)]',
}

function RailIcon({ status }: { status: 'idle' | 'active' | 'done' | 'failed' }) {
  if (status === 'done') return <Check size={14} strokeWidth={2} aria-hidden="true" />
  if (status === 'failed') return <TriangleAlert size={14} strokeWidth={1.8} aria-hidden="true" />
  if (status === 'active') return <LoaderCircle className="animate-spin motion-reduce:animate-none" size={14} strokeWidth={1.8} aria-hidden="true" />
  return <Circle size={10} strokeWidth={1.5} aria-hidden="true" />
}

function elapsedLabel(startedAt: string | undefined, finishedAt: string | undefined, now: number) {
  const started = Date.parse(startedAt ?? '')
  if (!Number.isFinite(started)) return null
  const finished = Date.parse(finishedAt ?? '')
  const elapsedSeconds = Math.max(0, Math.floor(((Number.isFinite(finished) ? finished : now) - started) / 1000))
  const minutes = Math.floor(elapsedSeconds / 60)
  const seconds = elapsedSeconds % 60
  return minutes > 0 ? `${minutes} 分 ${seconds} 秒` : `${seconds} 秒`
}

export function WorkspaceStatusRail({ task, startedAt, finishedAt }: { task: WorkspaceTaskView; startedAt?: string; finishedAt?: string }) {
  const active = task.state === 'queued' || task.state === 'running'
  const [now, setNow] = useState(() => Date.now())
  useEffect(() => {
    if (!active || !startedAt) return undefined
    const timer = window.setInterval(() => setNow(Date.now()), 1_000)
    return () => window.clearInterval(timer)
  }, [active, startedAt])
  const elapsed = elapsedLabel(startedAt, active ? undefined : finishedAt, now)
  if (!task.rail.length) return null
  const resultLabel = task.requestedCount > 0 && (task.state === 'partial' || task.state === 'success')
    ? `${task.resultCount} / ${task.requestedCount}`
    : null
  return (
    <section
      className="grid gap-3 border-y border-[var(--border)] bg-[color-mix(in_oklch,var(--surface)_58%,transparent)] px-4 py-3 backdrop-blur-xl md:px-5"
      role="status"
      aria-live="polite"
      aria-atomic="true"
      aria-busy={task.state === 'queued' || task.state === 'running'}
      aria-label="创作进度"
      data-status={task.state === 'failure' ? 'failed' : task.state}
    >
      <div className="flex min-w-0 items-start justify-between gap-4">
        <div className="min-w-0">
          <strong className="block text-sm text-[var(--fg)]">{task.title}</strong>
          <span className="mt-0.5 block text-xs leading-5 text-[var(--muted)]">{task.detail}</span>
        </div>
        <span className="flex shrink-0 flex-col items-end gap-0.5 font-vault-mono text-xs text-[var(--muted)]">
          {elapsed ? <span>已用时 {elapsed}</span> : null}
          {resultLabel ? <strong className="font-bold text-[var(--fg)]">{resultLabel}</strong> : null}
        </span>
      </div>
      <ol className="m-0 grid list-none grid-cols-[repeat(auto-fit,minmax(92px,1fr))] gap-1.5 p-0">
        {task.rail.map((item) => (
          <li
            key={item.phase}
            className={cn('flex min-w-0 items-center gap-1.5 rounded-lg border px-2 py-1.5 text-[11px] font-semibold transition-colors duration-[var(--motion-fast)] motion-reduce:transition-none', toneClass[item.status])}
            aria-current={item.status === 'active' ? 'step' : undefined}
          >
            <RailIcon status={item.status} />
            <span className="truncate">{item.label}</span>
            <span className="sr-only">{item.status === 'done' ? '已完成' : item.status === 'active' ? '进行中' : item.status === 'failed' ? '失败' : '等待中'}</span>
          </li>
        ))}
      </ol>
    </section>
  )
}
