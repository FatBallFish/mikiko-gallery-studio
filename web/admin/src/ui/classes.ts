export const adminTokens = {
  radius: {
    xs: '6px',
    sm: '8px',
    md: '10px',
    lg: '14px',
  },
  surface: {
    page: 'var(--bg)',
    panel: 'var(--surface)',
    panelSubtle: 'var(--surface-frost)',
  },
  focus: '0 0 0 3px color-mix(in oklch, var(--accent) 22%, transparent)',
}

export const adminShell = {
  root: 'flex h-screen overflow-hidden bg-[var(--bg)] text-[var(--fg)] selection:bg-[var(--accent)]/22 max-[920px]:h-auto max-[920px]:min-h-screen max-[920px]:flex-col',
  sidebar: 'z-20 flex w-[var(--pg-sidebar-admin-width)] shrink-0 flex-col border-r border-[var(--border)] bg-[var(--shell)] transition-all duration-300 ease-[var(--pg-ease-out)] max-[920px]:hidden',
  mobileTopbar: 'hidden min-h-14 items-center justify-between gap-3 border-b border-[var(--border)] bg-[var(--topbar)] px-4 py-3 backdrop-blur-xl max-[920px]:flex',
  mobileTitle: 'min-w-0 truncate text-sm font-bold text-[var(--fg)]',
  mobileDrawerBackdrop: 'fixed inset-0 z-[80] hidden bg-black/50 backdrop-blur-sm max-[920px]:block',
  mobileDrawer: 'fixed inset-y-0 left-0 z-[90] hidden w-[min(320px,86vw)] flex-col border-r border-[var(--border)] bg-[var(--shell)] shadow-[0_20px_80px_rgba(0,0,0,.32)] max-[920px]:flex',
  mobileDrawerHead: 'flex min-h-14 items-center justify-between gap-3 border-b border-[var(--border)] px-4',
  brand: 'flex h-[var(--pg-topbar-height)] items-center gap-3 border-b border-[var(--border)] px-6 text-[var(--fg)] no-underline max-[920px]:h-auto max-[920px]:px-4 max-[920px]:py-3',
  brandOrb: 'grid size-9 place-items-center rounded-xl bg-[var(--accent)]/12 text-sm font-black text-[var(--accent)]',
  brandText: 'font-[family-name:var(--font-admin-display)] text-[1.05rem] font-semibold tracking-tight text-[var(--fg)]',
  nav: 'flex-1 space-y-6 overflow-y-auto px-3 py-4',
  navGroup: 'space-y-1',
  navLabel: 'mb-2 px-3 text-[10px] font-bold uppercase tracking-[0.18em] text-[var(--dim)]',
  navLink: 'group flex w-full max-w-full items-center gap-3 rounded-xl border border-transparent px-3 py-2.5 text-sm font-medium text-[var(--muted)] no-underline transition-all hover:bg-[var(--surface)] hover:text-[var(--fg)]',
  navLinkActive: 'admin-nav-active text-[var(--fg)] hover:text-[var(--fg)]',
  navIcon: 'grid size-5 place-items-center',
  navBadge: 'ml-auto rounded-full bg-[var(--surface)] px-2 py-0.5 text-[10px] font-bold not-italic text-[var(--muted)]',
  sideNote: 'm-5 mt-auto grid gap-3 border-t border-[var(--border)] pt-5 max-[920px]:hidden',
  sideNoteIdentity: 'flex min-w-0 items-center gap-3',
  main: 'flex min-w-0 flex-1 flex-col overflow-hidden',
  topbar: 'flex h-[var(--pg-topbar-height)] shrink-0 items-center justify-between gap-4 border-b border-[var(--border)] bg-[var(--topbar)] px-8 backdrop-blur-xl max-[920px]:hidden',
  topbarMeta: 'flex min-w-0 items-center gap-2 text-[var(--muted)]',
  flexRow: 'flex flex-wrap items-center gap-2',
  metaRow: 'flex flex-wrap items-center justify-end gap-2',
  chip: 'inline-flex min-h-9 items-center gap-2 rounded-full border border-[var(--border)] bg-[var(--surface)] px-3 py-1.5 text-[11px] font-semibold text-[var(--muted)]',
  providerPill: 'inline-flex items-center gap-2 rounded-xl border border-[var(--border)] bg-[var(--surface)] px-3 py-2 text-[11px] font-bold uppercase tracking-[0.12em] text-[var(--muted)]',
  iconButton: 'grid size-10 place-items-center rounded-xl border border-[var(--border)] bg-[var(--surface)] text-[var(--muted)] transition-colors hover:border-[var(--border-strong)] hover:text-[var(--fg)]',
  avatarWidget: 'flex items-center gap-2.5',
  avatarOrb: 'grid size-10 place-items-center rounded-xl bg-[var(--surface)] text-sm font-bold text-[var(--muted)]',
  statusStrip: 'flex shrink-0 items-center gap-0 overflow-x-auto border-b border-[var(--border)] bg-[var(--canvas)]',
  statusCell: 'grid min-w-[180px] gap-1 border-r border-[var(--border)] px-4 py-3 last:border-r-0',
  statusLabel: 'block text-[10px] font-extrabold uppercase tracking-[.14em] text-[var(--dim)]',
  statusValue: 'block truncate text-sm font-semibold text-[var(--fg)]',
  content: 'flex-1 overflow-y-auto bg-[radial-gradient(circle_at_50%_0%,color-mix(in_oklch,var(--accent)_5%,transparent),transparent_48%)] p-8 max-[920px]:w-full max-[920px]:p-4',
}

export const adminButton = {
  base: 'inline-flex min-h-10 items-center justify-center gap-2 rounded-lg border border-[var(--border)] px-[18px] py-2.5 text-sm font-bold transition hover:-translate-y-px focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[color-mix(in_oklch,var(--accent)_28%,transparent)] disabled:pointer-events-none disabled:opacity-50',
  primary: 'border-[var(--accent)] bg-[var(--accent)] text-white hover:bg-[color-mix(in_oklch,var(--accent)_88%,black_12%)]',
  secondary: 'bg-[var(--surface-solid)] text-[var(--fg)] hover:border-[var(--border-strong)] hover:bg-[var(--elevated)]',
  ghost: 'bg-[var(--surface)] text-[var(--fg)] hover:border-[var(--border-strong)] hover:bg-[var(--elevated)]',
  danger: 'border-[color-mix(in_oklch,var(--red)_24%,transparent)] bg-[color-mix(in_oklch,var(--red)_10%,transparent)] text-[var(--red)]',
  text: 'border-transparent bg-transparent px-2 text-[var(--muted)] hover:bg-[var(--surface)] hover:text-[var(--fg)]',
  success: 'border-[color-mix(in_oklch,var(--green)_24%,transparent)] bg-[color-mix(in_oklch,var(--green)_10%,transparent)] text-[var(--green)]',
  small: 'min-h-8 px-2.5 py-1.5 text-xs',
}

export const adminSurface = {
  card: 'rounded-lg border border-[var(--border)] bg-[var(--surface-solid)]',
  lane: 'min-w-0 rounded-lg border border-[var(--border)] bg-[var(--surface-solid)]',
  panel: 'rounded-lg border border-[var(--border)] bg-[var(--surface-solid)] shadow-[var(--pg-shadow-sm)]',
  panelSubtle: 'rounded-lg border border-transparent bg-[var(--surface-solid)]',
}

export const adminPage = {
  stack: 'grid min-h-0 gap-6',
  scrollStack: 'grid min-h-0 gap-6',
  fullSurface: 'grid min-h-0 grid-cols-1 overflow-hidden rounded-lg border border-[var(--border)] bg-[var(--surface-solid)]',
  splitSurface: 'grid min-h-0 grid-cols-[minmax(0,1fr)_280px] gap-3 overflow-hidden rounded-3xl border border-[var(--border)] bg-[var(--surface-solid)] max-[1260px]:grid-cols-1',
  formGrid: 'grid grid-cols-2 gap-3 max-[620px]:grid-cols-1',
  filterBand: 'rounded-lg border border-[var(--border)] bg-[var(--surface-solid)] p-4 shadow-[var(--pg-shadow-sm)]',
  filterRow: 'flex flex-wrap items-center gap-2',
  mainLane: 'min-w-0 overflow-auto p-4',
  sideRail: 'grid min-w-0 content-start overflow-y-auto border-l border-[var(--border)] bg-[var(--surface-solid)] max-[1260px]:border-l-0 max-[1260px]:border-t',
  signalSection: 'grid gap-2 border-b border-[var(--border)] p-4 last:border-b-0',
  toolbar: 'mb-3 flex flex-wrap items-center justify-between gap-3',
  pagination: 'mt-3 flex flex-wrap items-center justify-between gap-3',
  microTabs: 'mb-3 flex flex-wrap items-center gap-2',
  microTab: 'min-h-8 rounded-full border border-[var(--border)] bg-[var(--surface)] px-3 py-1.5 text-xs font-extrabold uppercase tracking-[.08em] text-[var(--muted)] hover:border-[var(--border-strong)] hover:bg-[var(--elevated)]',
  microTabActive: 'border-[var(--accent)]/25 bg-[var(--accent)]/10 text-[var(--accent)]',
  mutedAction: 'text-xs font-extrabold text-[var(--soft)]',
  detailStack: 'grid gap-3',
  detailSection: 'grid min-w-0 gap-3',
  sectionHead: 'flex flex-wrap items-center justify-between gap-2 border-b border-[var(--border)] pb-3',
  sectionTitle: 'font-bold text-[var(--fg)]',
}

export const adminState = {
  block: 'grid min-h-[220px] place-items-center content-center gap-3 rounded-3xl border border-[var(--border)] bg-[var(--surface-solid)] p-8 text-center',
  iconWrap: 'grid size-14 place-items-center rounded-2xl bg-[var(--accent)]/10 text-[var(--accent)]',
  title: 'text-base font-bold text-[var(--fg)]',
  detail: 'max-w-[42ch] text-sm text-[var(--muted)]',
}

export const adminPill = {
  base: 'inline-flex items-center gap-2 rounded-full px-2.5 py-1 text-[11px] font-extrabold',
}
