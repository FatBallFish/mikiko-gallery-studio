import { cn } from '../shared/classnames'

// Redesigned styles for high-end feel
export const rdShell = {
  shell: 'flex h-screen overflow-hidden bg-[var(--bg)] text-[var(--fg)] selection:bg-[var(--accent)]/30',
  sidebar: 'z-20 flex w-[100px] shrink-0 flex-col items-center border-r border-[var(--border)] bg-[color-mix(in_oklch,var(--bg)_96%,var(--accent)_4%)] py-6 transition-all duration-500 ease-[cubic-bezier(0.32,0.72,0,1)]',
  brand: 'mb-10 flex flex-col items-center gap-2 border-0 bg-transparent no-underline group cursor-pointer',
  brandOrb: 'relative grid size-12 place-items-center rounded-2xl bg-gradient-to-br from-[var(--accent)] to-[var(--accent-purple)] font-black text-white shadow-[0_0_30px_rgba(var(--accent-rgb),0.3)] transition-transform duration-500 group-hover:scale-110 group-hover:rotate-6',
  brandText: 'font-vault-mono text-[10px] uppercase tracking-[0.2em] text-[var(--muted)]',
  nav: 'flex w-full flex-col gap-2 px-2',
  navLink: 'group relative flex w-full flex-col items-center gap-1.5 rounded-2xl py-4 text-[var(--muted)] no-underline transition-all duration-300 hover:bg-[var(--accent)]/8 hover:text-[var(--fg)]',
  navLinkActive: 'bg-[var(--accent)]/12 text-[var(--accent)] shadow-[inset_0_0_20px_rgba(var(--accent-rgb),0.05)]',
  navLinkIndicator: 'absolute right-0 top-1/2 h-6 w-1 -translate-y-1/2 rounded-l-full bg-[var(--accent)] opacity-0 transition-all duration-300 group-hover:opacity-100',
  navLinkIndicatorActive: 'opacity-100',
  navShort: 'font-vault-mono text-[9px] tracking-[0.1em] opacity-50',
  navLabel: 'text-[11px] font-semibold',
  
  main: 'flex h-screen min-w-0 flex-1 flex-col overflow-y-auto bg-[radial-gradient(circle_at_50%_0%,var(--accent)/0.05,transparent_50%)]',
  topbar: 'sticky top-0 z-10 flex h-20 items-center justify-between gap-6 px-10 border-b border-[var(--border)] bg-[var(--bg)]/70 backdrop-blur-2xl transition-all duration-500',
  
  // New Header Menu Style
  menuList: 'flex items-center gap-8',
  menuItem: 'flex items-center gap-3 group cursor-pointer no-underline',
  menuIcon: 'grid size-10 place-items-center rounded-xl bg-gradient-to-br from-[var(--icon-color)]/20 to-[var(--icon-color)]/5 border border-[var(--icon-color)]/20 text-[var(--icon-color)] transition-all duration-300 group-hover:scale-110 group-hover:shadow-[0_0_15px_rgba(var(--icon-rgb),0.2)]',
  menuContent: 'flex flex-col',
  menuTitle: 'text-sm font-bold text-[var(--fg)] transition-colors group-hover:text-[var(--accent)]',
  menuLabel: 'text-[10px] text-[var(--muted)] uppercase tracking-wider',

  // Redesigned User Area
  userTools: 'flex items-center gap-4',
  balancePill: 'flex items-center gap-3 rounded-full border border-[var(--border)] bg-[var(--surface)] pl-4 pr-1.5 py-1.5 transition-all hover:border-[var(--accent)]/30',
  balanceText: 'text-sm font-medium text-[var(--muted)]',
  balanceValue: 'font-vault-mono font-bold text-[var(--accent)]',
  rechargeBtn: 'grid size-8 place-items-center rounded-full bg-[var(--accent)] text-white shadow-lg shadow-[var(--accent)]/20 hover:scale-110 transition-transform',
  
  avatarBtn: 'flex items-center gap-3 rounded-2xl border border-[var(--border)] bg-[var(--surface)] p-1.5 pr-4 transition-all hover:bg-[var(--accent)]/5 hover:border-[var(--accent)]/30 group',
  avatarImg: 'grid size-9 place-items-center rounded-xl bg-gradient-to-br from-[var(--accent)] to-[var(--accent-purple)] text-sm font-bold text-white shadow-inner',
  userName: 'text-sm font-bold text-[var(--fg)]',
  userChevron: 'size-4 text-[var(--muted)] transition-transform group-aria-expanded:rotate-180',
}

export const rdWorkspace = {
  root: 'flex h-[calc(100vh-80px)] overflow-hidden',
  sidebar: 'w-[400px] flex flex-col border-r border-[var(--border)] bg-[var(--surface)]/50 overflow-y-auto scrollbar-hide',
  sidebarSection: 'p-8 border-b border-[var(--border)]',
  
  sectionTitle: 'text-xs font-vault-mono uppercase tracking-[0.2em] text-[var(--muted)] mb-6 flex items-center gap-2 before:h-px before:w-4 before:bg-[var(--accent)]',
  
  // High-end Selectors
  grid: 'grid grid-cols-2 gap-3',
  grid3: 'grid grid-cols-3 gap-3',
  selectItem: 'group relative flex flex-col items-center gap-1 rounded-2xl border border-[var(--border)] bg-[var(--bg)]/50 p-4 transition-all duration-300 hover:border-[var(--accent)]/50 hover:bg-[var(--accent)]/5 active:scale-95',
  selectItemActive: 'border-[var(--accent)] bg-[var(--accent)]/10 shadow-[0_0_20px_rgba(var(--accent-rgb),0.1)] scale-[1.02]',
  itemLabel: 'text-sm font-bold text-[var(--fg)]',
  itemSub: 'text-[10px] font-vault-mono text-[var(--muted)] uppercase tracking-tight',
  
  // Model Select
  modelItem: 'flex w-full items-center justify-between gap-4 rounded-2xl border border-[var(--border)] bg-[var(--bg)]/50 p-4 transition-all duration-300 hover:border-[var(--accent)]/50 hover:bg-[var(--accent)]/5 active:scale-[0.98]',
  modelInfo: 'flex flex-col items-start',
  modelPoints: 'rounded-full bg-[var(--accent)]/10 px-2.5 py-1 text-[10px] font-bold text-[var(--accent)]',
  
  // Prompt Area
  promptWrapper: 'relative rounded-3xl border border-[var(--border)] bg-[var(--bg)]/50 p-1 focus-within:border-[var(--accent)]/50 transition-colors',
  textarea: 'w-full bg-transparent border-0 p-4 text-sm text-[var(--fg)] placeholder:text-[var(--muted)] focus:ring-0 resize-none min-h-[120px]',
  
  // Action Bar
  actionBar: 'mt-auto p-8 bg-[var(--bg)]/80 backdrop-blur-xl border-t border-[var(--border)]',
  priceRow: 'flex items-center justify-between mb-6',
  priceLabel: 'text-sm text-[var(--muted)]',
  priceValue: 'flex items-center gap-1.5 text-xl font-black text-[var(--accent)]',
  
  generateBtn: 'group relative w-full h-16 rounded-2xl bg-[var(--accent)] overflow-hidden transition-all hover:scale-[1.02] active:scale-[0.98] shadow-2xl shadow-[var(--accent)]/20 disabled:opacity-50 disabled:grayscale',
  btnGlow: 'absolute inset-0 bg-gradient-to-r from-transparent via-white/20 to-transparent -translate-x-full group-hover:animate-[shimmer_2s_infinite]',
  btnText: 'relative z-10 flex items-center justify-center gap-3 text-lg font-bold text-white',

  canvas: 'flex-1 overflow-y-auto p-10 bg-[var(--bg)] scroll-smooth',
  feed: 'mx-auto max-w-[1000px] flex flex-col gap-10 pb-20',
  
  // Double-Bezel Card
  cardShell: 'rounded-[2.5rem] bg-[var(--border)] p-1.5 shadow-2xl shadow-black/20 transition-all duration-500 hover:translate-y-[-4px]',
  cardInner: 'rounded-[calc(2.5rem-0.375rem)] bg-[var(--surface)] p-8 border border-white/5',
}

export const rdCommon = {
  glass: 'backdrop-blur-2xl bg-[var(--bg)]/60 border border-[var(--border)]',
  badge: 'inline-flex items-center gap-1.5 rounded-full px-3 py-1 text-[10px] font-bold uppercase tracking-widest',
  transition: 'transition-all duration-500 ease-[cubic-bezier(0.32,0.72,0,1)]',
}
