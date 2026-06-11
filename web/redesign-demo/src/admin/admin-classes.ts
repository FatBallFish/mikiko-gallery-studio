import { cn } from '../../shared/classnames'

export const rdAdmin = {
  layout: 'flex h-screen overflow-hidden bg-[#050505] text-[#e0e0e0] font-sans selection:bg-[var(--accent)]/30',
  sidebar: 'z-20 flex w-72 shrink-0 flex-col border-r border-white/5 bg-[#0a0a0a] transition-all duration-500 ease-[cubic-bezier(0.32,0.72,0,1)]',
  brand: 'flex h-20 items-center gap-3 px-8 border-b border-white/5',
  brandOrb: 'size-8 rounded-lg bg-gradient-to-br from-[var(--accent)] to-[var(--accent-purple)] flex items-center justify-center font-black text-white text-sm',
  brandText: 'font-bold tracking-tight text-lg text-white',
  
  nav: 'flex-1 overflow-y-auto py-6 px-4 space-y-8 scrollbar-hide',
  navSection: 'space-y-1',
  navSectionTitle: 'px-4 mb-2 text-[10px] font-bold uppercase tracking-[0.2em] text-white/30',
  navLink: 'group flex items-center gap-3 rounded-xl px-4 py-2.5 text-sm font-medium text-white/50 transition-all hover:bg-white/5 hover:text-white',
  navLinkActive: 'bg-[var(--accent)]/10 text-[var(--accent)] hover:bg-[var(--accent)]/15 hover:text-[var(--accent)]',
  navIcon: 'size-5 opacity-70 group-hover:opacity-100 transition-opacity',
  navBadge: 'ml-auto rounded-full bg-white/10 px-2 py-0.5 text-[10px] font-bold text-white/50',

  main: 'flex flex-1 flex-col overflow-hidden',
  topbar: 'flex h-20 items-center justify-between px-10 border-b border-white/5 bg-[#0a0a0a]/50 backdrop-blur-xl',
  pageTitle: 'text-xl font-bold text-white tracking-tight',
  
  content: 'flex-1 overflow-y-auto p-10 space-y-10 bg-[radial-gradient(circle_at_50%_0%,var(--accent)/0.03,transparent_50%)]',
  
  // Dashboard Cards
  statGrid: 'grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-6',
  statCard: 'relative overflow-hidden rounded-3xl border border-white/5 bg-white/[0.02] p-6 transition-all hover:border-white/10 hover:bg-white/[0.04]',
  statLabel: 'text-xs font-medium text-white/40 uppercase tracking-wider mb-1',
  statValue: 'text-3xl font-black text-white tracking-tighter mb-2',
  statTrend: 'flex items-center gap-1.5 text-xs font-bold',
  statPositive: 'text-emerald-400',
  statNegative: 'text-rose-400',
  statChart: 'absolute bottom-0 left-0 right-0 h-16 opacity-30',

  // Section Header
  sectionHeader: 'flex items-end justify-between mb-6',
  sectionTitle: 'text-sm font-bold uppercase tracking-[0.15em] text-white/40 flex items-center gap-3 before:h-px before:w-6 before:bg-[var(--accent)]',
  
  // Tables
  tableWrapper: 'overflow-hidden rounded-3xl border border-white/5 bg-white/[0.01] backdrop-blur-sm',
  table: 'w-full text-left border-collapse',
  th: 'border-b border-white/5 px-6 py-4 text-[11px] font-bold uppercase tracking-wider text-white/30',
  td: 'px-6 py-4 text-sm text-white/70 border-b border-white/[0.02]',
  tr: 'transition-colors hover:bg-white/[0.02]',
  
  // Status & Badges
  badge: 'inline-flex items-center px-2.5 py-1 rounded-lg text-[10px] font-bold uppercase tracking-wider',
  badgeSuccess: 'bg-emerald-500/10 text-emerald-400 border border-emerald-500/20',
  badgeWarning: 'bg-amber-500/10 text-amber-400 border border-amber-500/20',
  badgeError: 'bg-rose-500/10 text-rose-400 border border-rose-500/20',
  badgeInfo: 'bg-[var(--accent)]/10 text-[var(--accent)] border border-[var(--accent)]/20',

  // Charts & Visuals
  chartContainer: 'rounded-3xl border border-white/5 bg-white/[0.02] p-8',
  pieGrid: 'grid grid-cols-1 lg:grid-cols-3 gap-8',
  
  // Monitoring specific
  healthGrid: 'grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-6',
  healthCard: 'rounded-3xl border border-white/5 bg-white/[0.02] p-6 flex items-center gap-5',
  healthIcon: 'size-12 rounded-2xl bg-white/5 flex items-center justify-center text-[var(--accent)]',
  healthContent: 'flex-1',
  healthLabel: 'text-sm font-bold text-white',
  healthValue: 'text-xs text-white/40 font-mono',
  healthStatus: 'size-2.5 rounded-full shadow-[0_0_10px_currentColor]',

  // SLA Indicators
  slaValue: 'text-4xl font-black tracking-tighter text-emerald-400',
  slaLabel: 'text-[10px] font-bold uppercase tracking-widest text-white/30 mt-1',
}

export const rdForm = {
  group: 'space-y-2',
  label: 'text-xs font-bold text-white/40 uppercase tracking-wider',
  input: 'w-full rounded-xl border border-white/10 bg-white/5 px-4 py-2.5 text-sm text-white placeholder:text-white/20 focus:border-[var(--accent)]/50 focus:outline-none focus:ring-1 focus:ring-[var(--accent)]/50 transition-all',
  button: 'inline-flex items-center justify-center rounded-xl px-6 py-2.5 text-sm font-bold transition-all active:scale-95',
  buttonPrimary: 'bg-[var(--accent)] text-white hover:opacity-90 shadow-lg shadow-[var(--accent)]/20',
  buttonSecondary: 'bg-white/5 text-white hover:bg-white/10 border border-white/10',
}
