export const overlayLayers = {
  modal: 'z-[110]',
  lightbox: 'z-[120]',
  zoom: 'z-[130]',
  zoomControls: 'z-[131]',
} as const

export const rdShell = {
  shellWrapper: 'flex justify-center bg-black min-h-screen w-full font-vault-body',
  shell: 'relative flex h-screen w-full overflow-hidden bg-[var(--bg)] text-[var(--fg)] selection:bg-[var(--accent)]/30',
  sidebar: 'hidden md:flex z-20 w-[100px] shrink-0 flex-col items-center border-r border-[var(--border)] bg-[var(--sidebar-bg)] py-6 transition-all duration-500',
  brand: 'mb-10 flex flex-col items-center gap-2 border-0 bg-transparent no-underline group cursor-pointer',
  brandMark: 'inline-flex items-center gap-2',
  brandLogo: 'block size-12 rounded-2xl object-cover shadow-[0_0_30px_rgba(var(--accent-rgb),0.22)]',
  brandName: 'text-sm font-black tracking-normal text-[var(--fg)]',
  brandOrb: 'relative grid size-12 place-items-center rounded-2xl bg-gradient-to-br from-[var(--accent)] to-[var(--accent-purple)] font-black text-white shadow-[0_0_30px_rgba(var(--accent-rgb),0.3)] transition-transform duration-500 group-hover:scale-110 group-hover:rotate-6',
  nav: 'flex w-full flex-col gap-2 px-2',
  navLink: 'group relative flex w-full flex-col items-center gap-1.5 rounded-2xl py-4 text-[var(--muted)] no-underline transition-all duration-300 hover:bg-[var(--accent)]/8 hover:text-[var(--fg)]',
  navLinkActive: 'bg-[var(--accent)]/12 text-[var(--accent)] shadow-[inset_0_0_20px_rgba(var(--accent-rgb),0.05)]',
  navLinkIndicator: 'absolute right-0 top-1/2 h-6 w-1 -translate-y-1/2 rounded-l-full bg-[var(--accent)] opacity-0 transition-all duration-300 group-hover:opacity-100',
  navLinkIndicatorActive: 'opacity-100',
  navShort: 'font-vault-mono text-[9px] tracking-[0.1em] opacity-50',
  navLabel: 'text-[11px] font-semibold',
  
  main: 'flex h-screen min-w-0 flex-1 flex-col overflow-y-auto bg-[radial-gradient(circle_at_50%_0%,var(--accent)/0.05,transparent_50%)] relative scroll-smooth',
  topbar: 'sticky top-0 z-10 flex h-20 shrink-0 items-center justify-center border-b border-[var(--border)] bg-[var(--bg)]/80 backdrop-blur-2xl transition-all duration-500',
  topbarInner: 'w-full max-w-[2560px] flex items-center justify-between gap-6 px-6 md:px-10',
  
  // Constrained Content Wrapper for 2560px max width
  contentConstrain: 'max-w-[2560px] mx-auto w-full flex-1 flex flex-col',
  
  // Mobile Nav (Bottom)
  mobileNav: 'md:hidden flex items-center justify-around fixed bottom-0 left-0 right-0 h-16 bg-[var(--bg)]/90 backdrop-blur-xl border-t border-[var(--border)] z-50 px-2',
  mobileNavLink: 'flex flex-col items-center gap-1 p-2 text-[var(--muted)]',
  mobileNavLinkActive: 'text-[var(--accent)]',
  
  userTools: 'flex items-center gap-3 md:gap-4 ml-auto',
  balancePill: 'hidden md:flex items-center gap-3 rounded-full border border-[var(--border)] bg-[var(--surface)] pl-4 pr-1.5 py-1.5 transition-all hover:border-[var(--accent)]/30',
  balanceText: 'text-sm font-medium text-[var(--muted)]',
  balanceValue: 'font-vault-mono font-bold text-[var(--accent)]',
  rechargeBtn: 'grid size-8 place-items-center rounded-full bg-[var(--accent)] text-white shadow-lg shadow-[var(--accent)]/20 hover:scale-110 transition-transform',
  
  avatarBtn: 'flex items-center gap-2 md:gap-3 rounded-2xl border border-[var(--border)] bg-[var(--surface)] p-1.5 md:pr-4 transition-all hover:bg-[var(--accent)]/5 hover:border-[var(--accent)]/30 group',
  avatarImg: 'grid size-8 md:size-9 place-items-center rounded-xl bg-gradient-to-br from-[var(--accent)] to-[var(--accent-purple)] text-sm font-bold text-white shadow-inner',
  userName: 'hidden md:block text-sm font-bold text-[var(--fg)]',
  userChevron: 'hidden md:block size-4 text-[var(--muted)] transition-transform group-aria-expanded:rotate-180',
  
  footer: 'mt-auto shrink-0 border-t border-[var(--border)] bg-[var(--bg)]/80 backdrop-blur-md py-6',
  footerContent: 'flex flex-col md:flex-row items-center justify-between gap-4 text-xs text-[var(--muted)] max-w-[2560px] mx-auto w-full px-6 md:px-10',
  footerLinks: 'flex items-center gap-6',
  footerLink: 'hover:text-[var(--fg)] transition-colors cursor-pointer',
}

export const rdHome = {
  hero: 'relative min-h-[500px] flex items-center justify-start overflow-hidden rounded-3xl bg-[var(--surface)] p-16 mb-16 group',
  heroImg: 'absolute inset-0 size-full object-cover opacity-60 transition-transform duration-1000 group-hover:scale-105',
  heroOverlay: 'absolute inset-0 bg-gradient-to-r from-[var(--bg)] via-[var(--bg)]/80 to-transparent',
  heroContent: 'relative z-10 max-w-2xl',
  heroBadge: 'inline-flex items-center gap-2 rounded-full bg-[var(--accent)]/10 px-4 py-2 text-[10px] font-bold uppercase tracking-[0.2em] text-[var(--accent)] mb-6',
  heroTitle: 'text-[clamp(3rem,6vw,5rem)] font-vault-display leading-[0.9] mb-6',
  heroText: 'text-lg text-[var(--muted)] mb-10 leading-relaxed',
  heroActions: 'flex items-center gap-6',

  statsGrid: 'grid grid-cols-4 gap-6 mb-16',
  statCard: 'relative overflow-hidden rounded-3xl border border-[var(--border)] bg-[var(--surface)] p-8 group hover:border-[var(--accent)]/30 transition-all',
  statLabel: 'text-[10px] font-vault-mono uppercase tracking-widest text-[var(--muted)] mb-2',
  statValue: 'text-3xl font-black text-[var(--fg)] group-hover:text-[var(--accent)] transition-colors',
  statGlow: 'absolute -right-4 -bottom-4 size-24 bg-[var(--accent)]/5 rounded-full blur-3xl opacity-0 group-hover:opacity-100 transition-opacity',
}

export const rdGallery = {
  toolbar: 'flex flex-col md:flex-row justify-between items-start md:items-center gap-4 bg-[var(--surface)]/50 border border-[var(--border)] p-4 rounded-2xl mb-8',
  filterGroup: 'flex flex-wrap items-center gap-3',
  filterSelectWrap: 'relative flex items-center',
  filterSelectBtn: 'flex items-center justify-between gap-3 h-10 px-4 rounded-xl border border-[var(--border)] bg-[var(--bg)]/50 text-xs font-bold text-[var(--fg)] hover:border-[var(--accent)]/50 hover:bg-[var(--surface)] transition-all cursor-pointer select-none outline-none',
  filterSelectDropdown: 'absolute top-[calc(100%+8px)] left-0 min-w-[160px] bg-[var(--surface)]/95 backdrop-blur-2xl border border-[var(--border)] rounded-2xl p-2 shadow-[0_10px_40px_rgba(0,0,0,0.3)] z-50 animate-in fade-in slide-in-from-top-2 duration-200 origin-top-left',
  filterOption: 'flex items-center px-4 py-2.5 rounded-xl text-xs font-bold text-[var(--muted)] cursor-pointer transition-colors hover:bg-[var(--accent)] hover:text-white',
  filterOptionActive: 'bg-[var(--accent)]/10 text-[var(--accent)]',
  
  batchBar: 'fixed bottom-8 left-1/2 -translate-x-1/2 z-50 flex items-center gap-2 bg-[var(--bg)]/95 backdrop-blur-2xl border border-[var(--accent)]/30 p-2.5 rounded-2xl shadow-[0_10px_40px_rgba(var(--accent-rgb),0.2)] animate-in slide-in-from-bottom-8 duration-300',
  batchCount: 'px-4 text-sm font-bold text-[var(--accent)] border-r border-[var(--border)]',
  batchBtn: 'flex items-center gap-1.5 px-4 py-2 rounded-xl hover:bg-[var(--surface)] text-xs font-medium transition-colors cursor-pointer text-[var(--fg)] hover:text-[var(--accent)]',
  
  masonry: 'w-full columns-1 sm:columns-2 lg:columns-3 xl:columns-4 2xl:columns-5 gap-8',
  item: 'mb-8 break-inside-avoid group relative',
  itemShell: 'relative rounded-3xl bg-[var(--border)] p-1 transition-all duration-500 hover:translate-y-[-8px] hover:shadow-2xl hover:shadow-[var(--accent)]/20 cursor-pointer',
  itemInner: 'relative overflow-hidden rounded-2xl bg-[var(--bg)]',
  itemImg: 'w-full h-full object-cover transition-transform duration-700 group-hover:scale-110',
  itemOverlay: 'absolute inset-0 [background:var(--image-overlay)] opacity-0 group-hover:opacity-100 transition-opacity duration-300 ease-out flex flex-col justify-between p-5',
  
  itemHeader: 'flex justify-between items-start',
  itemCheckbox: 'size-5 rounded border border-[var(--image-checkbox-border)] bg-[var(--image-checkbox-bg)] text-[var(--image-card-text)] flex items-center justify-center transition-colors hover:border-[var(--accent)]',
  itemCheckboxChecked: 'bg-[var(--accent)] border-[var(--accent)]',
  itemActionGroup: 'flex items-center gap-1 rounded-xl border border-[var(--image-action-border)] bg-[var(--image-action-bg)] p-1 backdrop-blur',
  itemActionBtn: 'rounded-xl p-1.5 text-[var(--image-action-text)] transition-colors hover:bg-[var(--image-action-hover-bg)] hover:text-[var(--image-action-hover-text)]',
  
  itemFooter: 'flex flex-col gap-1',
  itemTitle: 'text-sm font-bold text-[var(--image-card-text)] mb-0.5 line-clamp-2',
  itemMeta: 'text-[10px] font-vault-mono text-[var(--image-card-muted)] uppercase tracking-tight flex justify-between items-center',
  itemBadge: 'rounded-full bg-[var(--accent)]/20 text-[var(--accent)] border border-[var(--accent)]/30 px-2 py-0.5 text-[8px] font-bold',
}

export const rdBilling = {
  layout: 'grid grid-cols-1 gap-6 xl:grid-cols-[minmax(0,1fr)_380px]',
  card: 'rounded-2xl border border-[var(--border)] bg-[var(--surface)] p-5 sm:p-6',
  planGrid: 'grid grid-cols-1 gap-3 md:grid-cols-2',
  planItem: 'group relative flex flex-col gap-3 rounded-xl border border-[var(--border)] bg-[var(--bg)]/50 p-5 transition-colors hover:border-[var(--accent)] motion-reduce:transition-none',
  planActive: 'border-[var(--accent)] bg-[var(--accent)]/[0.03] ring-1 ring-[var(--accent)]',
  planPrice: 'text-4xl font-black text-[var(--fg)] group-hover:text-[var(--accent)] transition-colors',
  planPoints: 'text-sm text-[var(--muted)] font-vault-mono',

  orderPanel: 'flex flex-col gap-6 rounded-2xl border border-[var(--border)] bg-[var(--surface)] p-5 shadow-[var(--pg-shadow-md)] sm:p-6 xl:sticky xl:top-24',
  orderTitle: 'text-2xl font-black',
  orderRow: 'flex justify-between items-center py-4 border-b border-[var(--border)] last:border-0',
  orderTotal: 'text-3xl font-black text-[var(--accent)]',
}

export const rdWorkspace = {
  root: 'flex flex-col md:flex-row gap-8 w-full p-6 md:p-10 flex-1',
  
  // Card-style sidebar with original sizing
  sidebar: 'w-full md:w-[360px] lg:w-[400px] shrink-0 flex flex-col rounded-3xl border border-[var(--border)] bg-[var(--surface)]/60 shadow-xl overflow-hidden md:sticky md:top-28 md:h-[calc(100vh-140px)]',
  sidebarScroll: 'flex-1 overflow-y-auto scrollbar-hide',
  sidebarSection: 'p-5 md:p-6 border-b border-[var(--border)]',
  
  sectionTitle: 'text-xs font-vault-mono uppercase tracking-[0.2em] text-[var(--muted)] mb-4 flex items-center gap-2 before:h-px before:w-3 before:bg-[var(--accent)]',
  
  // Restored Selectors size
  grid: 'grid grid-cols-2 gap-2',
  grid3: 'grid grid-cols-3 gap-2',
  grid4: 'grid grid-cols-4 gap-2',
  selectItem: 'group relative flex flex-col items-center justify-center gap-1.5 rounded-xl border border-[var(--border)] bg-[var(--bg)]/50 p-3 transition-all duration-200 hover:border-[var(--accent)]/50 hover:bg-[var(--accent)]/5 hover:shadow-[0_0_15px_rgba(var(--accent-rgb),0.05)] active:scale-95 cursor-pointer',
  selectItemActive: 'border-[var(--accent)] bg-[color-mix(in_oklch,var(--accent)_18%,var(--surface))] text-[var(--lv-accent-contrast)] shadow-[0_0_20px_rgba(var(--accent-rgb),0.2)] ring-1 ring-[var(--accent)]/50 scale-[1.02] [&_*]:text-[var(--lv-accent-contrast)]',
  itemLabel: 'text-sm font-bold text-[var(--fg)] group-hover:text-[var(--accent)] transition-colors',
  itemSub: 'text-[10px] font-vault-mono text-[var(--muted)] uppercase tracking-tight',
  itemIcon: 'size-5 flex items-center justify-center text-[var(--muted)] group-hover:text-[var(--accent)] transition-colors',
  itemIconActive: 'text-white',
  
  // Restored Model Select size
  modelItem: 'group flex w-full items-center justify-between gap-3 rounded-xl border border-[var(--border)] bg-[var(--bg)]/50 p-3.5 transition-all duration-200 hover:border-[var(--accent)]/50 hover:bg-[var(--accent)]/5 hover:shadow-[0_0_15px_rgba(var(--accent-rgb),0.05)] active:scale-[0.98] cursor-pointer',
  modelItemActive: 'border-[var(--accent)] bg-[color-mix(in_oklch,var(--accent)_18%,var(--surface))] text-[var(--lv-accent-contrast)] ring-1 ring-[var(--accent)]/50 shadow-[0_0_20px_rgba(var(--accent-rgb),0.2)] [&_*]:text-[var(--lv-accent-contrast)]',
  modelInfo: 'flex flex-col items-start gap-0.5',
  modelPoints: 'rounded-full bg-[var(--accent)]/10 px-2.5 py-1 text-[10px] font-bold text-[var(--accent)] group-hover:bg-[color-mix(in_oklch,var(--accent)_18%,var(--surface))] group-hover:text-[var(--lv-accent-contrast)] transition-all',
  
  // Restored Upload Section size
  uploadSection: 'overflow-hidden transition-all duration-300',
  uploadTrigger: 'flex items-center justify-between w-full py-1 group cursor-pointer text-[var(--muted)] hover:text-[var(--accent)] transition-colors',
  uploadBox: 'mt-2 rounded-xl border-2 border-dashed border-[var(--border)] hover:border-[var(--accent)]/50 hover:bg-[var(--accent)]/5 transition-all p-2 cursor-pointer',
  uploadDashed: 'h-16 flex items-center justify-center gap-2',
  uploadGrid: 'grid grid-cols-5 gap-2 mt-3',
  uploadThumb: 'relative aspect-square overflow-hidden rounded-xl border border-[var(--border)] bg-[var(--bg)] group',
  uploadImg: 'w-full h-full object-cover',
  uploadRemove: 'absolute right-1 top-1 flex size-5 cursor-pointer items-center justify-center rounded-xl bg-[var(--image-action-bg)] text-[12px] text-[var(--image-action-text)] opacity-0 transition-opacity group-hover:opacity-100 hover:bg-[color-mix(in_oklch,var(--accent-coral)_18%,transparent)] hover:text-[var(--fg)]',
  
  // Prompt Area (Removed yellow border)
  promptWrapper: 'relative rounded-xl border border-[var(--border)] bg-[var(--bg)]/80 p-0 focus-within:border-[var(--accent)]/60 focus-within:shadow-[0_0_15px_rgba(var(--accent-rgb),0.15)] focus-within:bg-[var(--surface)] transition-all',
  textarea: 'w-full bg-transparent border-0 outline-none focus-visible:outline-none focus:outline-none focus:ring-0 focus:border-transparent p-4 text-sm text-[var(--fg)] placeholder:text-[var(--muted)] resize-none min-h-[90px] leading-relaxed',
  
  // Action Bar
  actionBar: 'mt-auto p-5 md:p-6 bg-[var(--bg)]/90 backdrop-blur-xl border-t border-[var(--border)] shrink-0 z-20',
  priceRow: 'flex items-center justify-between mb-4',
  priceLabel: 'text-sm text-[var(--muted)]',
  priceValue: 'flex items-center gap-1.5 text-lg font-black text-[var(--accent)]',
  
  generateBtn: 'group relative w-full h-14 rounded-xl bg-[var(--accent)] overflow-hidden transition-all hover:scale-[1.02] active:scale-[0.98] shadow-[0_5px_15px_rgba(var(--accent-rgb),0.3)] disabled:opacity-50 disabled:grayscale',
  btnGlow: 'absolute inset-0 bg-gradient-to-r from-transparent via-white/20 to-transparent -translate-x-full group-hover:animate-[shimmer_2s_infinite] motion-reduce:animate-none',
  btnText: 'relative z-10 flex items-center justify-center gap-2 text-base font-bold text-white',

  canvas: 'flex min-h-0 flex-1 flex-col w-full min-w-0',
  
  // Output Area
  outputPanel: 'w-full flex-1 flex min-h-0 max-h-[calc(100vh-140px)] flex-col justify-start rounded-3xl border border-[var(--border)] bg-[var(--canvas-bg)] shadow-2xl overflow-hidden relative p-6 md:p-10 min-h-[600px]',
  outputLoading: 'flex flex-col items-center justify-center w-full max-w-lg mx-auto',
  outputRing: 'relative size-20 mb-6',
  outputRingInner1: 'absolute inset-0 rounded-full border-2 border-[var(--accent)]/30 animate-ping motion-reduce:animate-none',
  outputRingInner2: 'absolute inset-2 rounded-full border border-[var(--accent)]/50 animate-pulse motion-reduce:animate-none',
  outputRingCore: 'absolute inset-4 rounded-full bg-[var(--accent)]/20 flex items-center justify-center border border-[var(--border)]',
  outputStage: 'text-sm font-semibold text-[var(--fg)] mb-2',
  outputStageText: 'text-[11px] font-vault-mono text-[var(--muted)] mb-5 h-4',
  outputProgressWrap: 'w-full bg-[var(--bg)] rounded-full h-1.5 overflow-hidden border border-[var(--border)]',
  outputProgressBar: 'bg-gradient-to-r from-[var(--accent)] to-[var(--accent-purple)] h-full transition-all duration-300 ease-out',
  
  outputImageWrap: 'relative w-full h-full flex flex-col items-center justify-center animate-in fade-in zoom-in-95 duration-500 motion-reduce:animate-none group',
  
  // Grid layout for multi-image
  outputGridSingle: 'max-h-[65vh] w-auto rounded-xl object-contain shadow-2xl border border-[var(--border)]',
  outputGridMultiple: 'grid grid-cols-1 sm:grid-cols-2 gap-4 w-full h-full content-center',
  outputGridImage: 'w-full h-full object-cover rounded-xl shadow-xl border border-[var(--border)] max-h-[40vh] hover:scale-[1.02] transition-transform cursor-pointer',
  
  outputActions: 'absolute bottom-6 left-1/2 -translate-x-1/2 bg-[var(--surface)]/90 backdrop-blur-2xl border border-[var(--border)] px-2 py-1.5 rounded-2xl shadow-2xl flex items-center gap-1.5 animate-in slide-in-from-bottom-4 duration-500 opacity-0 group-hover:opacity-100 transition-opacity',
  outputBtn: 'p-2 rounded-xl hover:bg-[var(--bg)] text-[var(--muted)] hover:text-[var(--fg)] transition-colors flex items-center gap-1.5 text-xs font-medium cursor-pointer',
  
  outputMetaRow: 'mt-auto pt-6 border-t border-[var(--border)]/30 flex flex-wrap items-center justify-between text-[10px] font-vault-mono text-[var(--muted)] gap-4 w-full',
}

export const rdCommon = {
  glass: 'backdrop-blur-2xl bg-[var(--bg)]/60 border border-[var(--border)]',
  badge: 'inline-flex items-center gap-1.5 rounded-full px-3 py-1 text-[10px] font-bold uppercase tracking-widest',
  transition: 'transition-all duration-[var(--motion-route)] ease-[cubic-bezier(0.32,0.72,0,1)]',
}

// Unified component primitives - replaces old `userButton`/`userForm`/`userState`/`userPill`/`userCard`/`userText`
export const button = {
  base: 'inline-flex items-center justify-center gap-2 rounded-xl border border-[var(--border)] bg-[var(--surface)] px-[18px] py-2.5 text-sm font-bold text-[var(--fg)] no-underline transition-[color,background-color,border-color,box-shadow,opacity,transform] duration-200 ease-out hover:-translate-y-px hover:border-[color-mix(in_oklch,var(--accent)_45%,var(--border))] hover:bg-[color-mix(in_oklch,var(--accent)_6%,transparent)] active:translate-y-0 active:scale-[0.98] disabled:opacity-50 disabled:pointer-events-none',
  primary: 'border-[var(--accent)] bg-[var(--accent)] text-[var(--bg)] hover:bg-[var(--accent)] hover:border-[var(--accent)] hover:shadow-[0_5px_20px_-5px_rgba(var(--accent-rgb),0.4)]',
  ghost: 'bg-transparent border-transparent hover:bg-[color-mix(in_oklch,var(--accent)_8%,transparent)]',
  danger: 'border-[color-mix(in_oklch,var(--accent-coral)_30%,var(--border))] text-[var(--accent-coral)] hover:border-[var(--accent-coral)] hover:bg-[color-mix(in_oklch,var(--accent-coral)_12%,transparent)]',
  icon: 'inline-grid size-10 place-items-center rounded-xl border border-[var(--border)] bg-[color-mix(in_oklch,var(--fg)_5%,transparent)] text-[var(--fg)] transition-[color,background-color,border-color,box-shadow,opacity,transform] duration-200 hover:border-[color-mix(in_oklch,var(--accent)_45%,var(--border))] hover:bg-[color-mix(in_oklch,var(--accent)_8%,transparent)] hover:text-[var(--accent)] active:scale-[0.98]',
  iconSm: 'inline-grid size-8 place-items-center rounded-xl border border-[var(--border)] bg-[color-mix(in_oklch,var(--fg)_5%,transparent)] text-[var(--fg)] transition-[color,background-color,border-color,box-shadow,opacity,transform] duration-200 hover:border-[color-mix(in_oklch,var(--accent)_45%,var(--border))] hover:text-[var(--accent)] active:scale-[0.98]',
}

export const form = {
  field: 'grid gap-2',
  fieldLabel: 'text-xs font-bold text-[var(--muted)]',
  input: 'w-full rounded-xl border border-[var(--border)] bg-[var(--surface)] px-3.5 py-2.5 text-sm text-[var(--fg)] outline-none transition-[color,background-color,border-color,box-shadow,opacity,transform] duration-200 placeholder:text-[var(--dim)] focus:border-[var(--accent)] focus:ring-2 focus:ring-[color-mix(in_oklch,var(--accent)_22%,transparent)]',
  textarea: 'w-full min-h-[120px] resize-y rounded-xl border border-[var(--border)] bg-[var(--surface)] px-3.5 py-2.5 text-sm text-[var(--fg)] outline-none transition-[color,background-color,border-color,box-shadow,opacity,transform] duration-200 placeholder:text-[var(--dim)] focus:border-[var(--accent)] focus:ring-2 focus:ring-[color-mix(in_oklch,var(--accent)_22%,transparent)]',
}

export const state = {
  spinner: 'size-3.5 animate-spin rounded-full border-2 border-current border-r-transparent motion-reduce:animate-none',
  stateLine: 'grid gap-2 rounded-2xl border border-[var(--border)] bg-[var(--surface)] p-6 text-[var(--muted)]',
  empty: 'grid place-items-center gap-3 rounded-3xl border border-[var(--border)] bg-[var(--surface)] p-12 text-center',
  emptyIcon: 'grid size-14 place-items-center rounded-2xl bg-[color-mix(in_oklch,var(--accent)_8%,transparent)] text-[var(--accent)]',
  emptyTitle: 'text-base font-bold text-[var(--fg)]',
  emptyDetail: 'text-sm text-[var(--muted)] max-w-[42ch]',
  skeleton: 'pg-skeleton rounded-xl',
  skeletonLine: 'pg-skeleton h-3.5 rounded-xl',
  toastStack: 'fixed right-5 top-5 z-[100] grid w-[min(380px,calc(100vw-40px))] gap-3',
  toast: 'grid grid-cols-[auto_minmax(0,1fr)] gap-3 rounded-2xl border border-[var(--border)] bg-[color-mix(in_oklch,var(--surface)_92%,black_8%)] p-3.5 text-[var(--fg)] shadow-[var(--pg-shadow-lg)] backdrop-blur-xl',
  modalBackdrop: `fixed inset-0 ${overlayLayers.modal} grid place-items-center bg-black/60 backdrop-blur-md p-6 animate-in fade-in duration-200`,
  modalCard: 'max-h-[90vh] w-[min(920px,100%)] overflow-auto rounded-3xl border border-[var(--border)] bg-[var(--surface)] p-6 shadow-[var(--pg-shadow-lg)] animate-in zoom-in-95 duration-200',
}

export const pill = {
  base: 'inline-flex w-fit items-center gap-1 rounded-full px-2.5 py-1 font-vault-mono text-[11px] font-bold',
  neutral: 'bg-[color-mix(in_oklch,var(--fg)_8%,transparent)] text-[var(--muted)]',
  good: 'bg-[color-mix(in_oklch,var(--accent-emerald)_18%,transparent)] text-[var(--accent-emerald)]',
  warn: 'bg-[color-mix(in_oklch,var(--accent)_18%,transparent)] text-[var(--accent)]',
  bad: 'bg-[color-mix(in_oklch,var(--accent-coral)_18%,transparent)] text-[var(--accent-coral)]',
}

export const card = {
  base: 'rounded-2xl border border-[var(--border)] bg-[var(--surface)]',
  padded: 'rounded-3xl border border-[var(--border)] bg-[var(--surface)] p-6',
  hero: 'rounded-3xl border border-[var(--border)] bg-[var(--surface)]',
}

export const text = {
  eyebrow: 'mb-4 font-vault-mono text-xs uppercase tracking-[0.15em] text-[var(--accent)]',
  mono: 'font-vault-mono',
  display: 'font-vault-display',
}
