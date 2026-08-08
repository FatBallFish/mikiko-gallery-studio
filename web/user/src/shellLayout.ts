import { cn } from '../../shared/classnames'
import type { RouteId } from './types'
import { routeTransitionClass } from './ui/motion'

export type ShellScrollMode = 'app' | 'document'

const stableShell = 'relative flex h-screen w-full overflow-hidden bg-[var(--bg)] text-[var(--fg)] selection:bg-[var(--accent)]/30'
const stableMain = 'relative flex h-screen min-w-0 flex-1 flex-col overflow-y-auto bg-[radial-gradient(circle_at_50%_0%,color-mix(in_oklch,var(--accent)_7%,transparent),transparent_48%)]'

export const shellChromeClasses = {
  wrapper: 'min-h-screen w-full bg-[var(--bg)] font-vault-body',
  sidebar: 'relative z-20 hidden w-[108px] shrink-0 flex-col items-center border-r border-[var(--border)] bg-[color-mix(in_oklch,var(--surface)_86%,var(--bg))] py-5 md:flex',
  brand: 'mb-7 flex flex-col items-center gap-2 border-0 bg-transparent no-underline group cursor-pointer',
  nav: 'relative flex w-full flex-col gap-1.5 px-2',
  navIndicator: 'pointer-events-none absolute right-0 top-0 h-[58px] w-0.5 rounded-l-full bg-[var(--accent)] shadow-[0_0_20px_color-mix(in_oklch,var(--accent)_72%,transparent)] transition-transform duration-[var(--motion-route)] ease-[var(--pg-ease-out)] motion-reduce:transition-none',
  navLink: 'group relative z-[1] flex h-[58px] w-full flex-col items-center justify-center gap-1 rounded-xl text-[var(--muted)] no-underline transition-colors duration-[var(--motion-fast)] hover:bg-[color-mix(in_oklch,var(--accent)_7%,transparent)] hover:text-[var(--fg)] focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-[-2px] focus-visible:outline-[var(--focus-ring)]',
  navLinkActive: 'bg-[color-mix(in_oklch,var(--accent)_11%,transparent)] text-[var(--accent)]',
  navLabel: 'text-[11px] font-semibold',
  topbar: 'sticky top-0 z-10 flex h-[76px] shrink-0 items-center justify-center border-b border-[var(--border)] bg-[color-mix(in_oklch,var(--bg)_84%,transparent)] backdrop-blur-2xl',
  topbarInner: 'flex w-full max-w-[2560px] items-center justify-between gap-6 px-4 sm:px-6 md:px-10',
  contentConstrain: 'mx-auto flex w-full max-w-[2560px] flex-1 flex-col pb-16 md:pb-0',
  mobileNav: 'fixed inset-x-0 bottom-0 z-50 grid h-16 grid-cols-7 items-stretch border-t border-[var(--border)] bg-[color-mix(in_oklch,var(--bg)_90%,transparent)] px-1 pb-[env(safe-area-inset-bottom)] backdrop-blur-2xl md:hidden',
  mobileNavLink: 'flex min-w-0 flex-col items-center justify-center gap-1 border-0 bg-transparent px-1 text-[var(--muted)] transition-colors focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-[-2px] focus-visible:outline-[var(--focus-ring)]',
  mobileNavLinkActive: 'text-[var(--accent)]',
} as const

export function shellActiveNavIndex(route: RouteId, items: ReadonlyArray<{ route: RouteId }>): number {
  return items.findIndex((item) => item.route === route)
}

export function resetShellScroll(
  mode: ShellScrollMode,
  target: { scrollTop: number } | null,
  documentScroller: { scrollTo: (x: number, y: number) => void } | null,
): void {
  if (mode === 'document') {
    documentScroller?.scrollTo(0, 0)
    return
  }
  if (target) target.scrollTop = 0
}

export function shellLayoutClasses(mode: ShellScrollMode = 'app', reducedMotion?: boolean) {
  if (mode === 'document') {
    return {
      shell: cn(stableShell, 'h-auto min-h-screen overflow-visible'),
      main: cn(stableMain, 'h-auto min-h-screen overflow-visible'),
      content: routeTransitionClass(reducedMotion),
    }
  }

  return {
    shell: stableShell,
    main: stableMain,
    content: routeTransitionClass(reducedMotion),
  }
}
