import React, { type ReactNode } from 'react'
import { cn } from '../../../shared/classnames'
import { useApp } from '../components'
import type { RouteId } from '../types'
import { CreditCard, FolderKanban, KeyRound, Palette, User } from './icons'

export type SettingsSectionId = 'profile' | 'projects' | 'billing' | 'api-keys' | 'appearance'

export const settingsWorkspaceSections: Array<{ id: SettingsSectionId; route: RouteId; label: string; detail: string }> = [
  { id: 'profile', route: 'profile', label: '个人资料', detail: '账户、积分与资料' },
  { id: 'projects', route: 'projects', label: '项目管理', detail: '资产与创作归属' },
  { id: 'billing', route: 'checkout', label: '积分与充值', detail: '套餐、充值与订单' },
  { id: 'api-keys', route: 'api-keys', label: 'API 密钥', detail: '凭据、额度与调用限制' },
  { id: 'appearance', route: 'settings', label: '外观偏好', detail: '主题模式与强调色' },
]

const icons: Record<SettingsSectionId, ReactNode> = {
  profile: <User size={18} strokeWidth={1.6} aria-hidden="true" />,
  projects: <FolderKanban size={18} strokeWidth={1.6} aria-hidden="true" />,
  billing: <CreditCard size={18} strokeWidth={1.6} aria-hidden="true" />,
  'api-keys': <KeyRound size={18} strokeWidth={1.6} aria-hidden="true" />,
  appearance: <Palette size={18} strokeWidth={1.6} aria-hidden="true" />,
}

export function SettingsWorkspace({ active, title, detail, action, children }: {
  active: SettingsSectionId
  title: string
  detail: string
  action?: ReactNode
  children: ReactNode
}) {
  const app = useApp()
  return (
    <div className="w-full flex-1 px-4 py-6 sm:px-6 md:px-10 md:py-8">
      <header className="mb-8 flex flex-col gap-5 border-b border-[var(--border)] pb-7 lg:flex-row lg:items-end lg:justify-between">
        <div className="min-w-0">
          <h1 className="m-0 text-[clamp(2rem,4vw,3.25rem)] font-black leading-none">{title}</h1>
          <p className="mt-3 max-w-3xl text-sm leading-relaxed text-[var(--muted)] md:text-base">{detail}</p>
        </div>
        {action ? <div className="flex shrink-0 flex-wrap gap-3">{action}</div> : null}
      </header>

      <div className="grid min-w-0 gap-7 lg:grid-cols-[220px_minmax(0,1fr)]">
        <nav className="flex gap-2 overflow-x-auto pb-1 lg:sticky lg:top-24 lg:h-fit lg:flex-col lg:overflow-visible" aria-label="账户设置">
          {settingsWorkspaceSections.map((section) => (
            <button
              key={section.id}
              type="button"
              aria-current={active === section.id ? 'page' : undefined}
              className={cn(
                'group flex min-w-44 items-center gap-3 rounded-xl border px-3 py-3 text-left transition-colors motion-reduce:transition-none lg:min-w-0',
                active === section.id
                  ? 'border-[color-mix(in_oklch,var(--accent)_45%,var(--border))] bg-[color-mix(in_oklch,var(--accent)_10%,var(--surface))] text-[var(--fg)]'
                  : 'border-transparent text-[var(--muted)] hover:border-[var(--border)] hover:bg-[var(--surface)] hover:text-[var(--fg)]',
              )}
              onClick={() => app.navigate(section.route)}
            >
              <span className={cn('grid size-9 shrink-0 place-items-center rounded-xl border border-[var(--border)] bg-[var(--surface)]', active === section.id && 'text-[var(--accent)]')}>{icons[section.id]}</span>
              <span className="min-w-0">
                <strong className="block text-sm">{section.label}</strong>
                <small className="mt-0.5 block whitespace-nowrap text-[11px] text-[var(--dim)]">{section.detail}</small>
              </span>
            </button>
          ))}
        </nav>
        <main className="min-w-0" data-settings-workspace-content>{children}</main>
      </div>
    </div>
  )
}
