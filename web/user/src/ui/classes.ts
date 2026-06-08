export const userShell = {
  shell: 'flex h-screen overflow-hidden bg-[var(--bg)] text-[var(--fg)] max-[760px]:block max-[760px]:h-auto max-[760px]:min-h-screen max-[760px]:overflow-visible',
  sidebar: 'z-20 flex w-[108px] shrink-0 flex-col items-center border-r border-[var(--border)] bg-[color-mix(in_oklch,var(--surface)_94%,black_6%)] py-5 max-[760px]:sticky max-[760px]:top-0 max-[760px]:h-auto max-[760px]:w-full max-[760px]:flex-row max-[760px]:overflow-x-auto max-[760px]:border-r-0 max-[760px]:border-b max-[760px]:p-2',
  brand: 'mb-7 grid w-full place-items-center gap-1.5 border-0 bg-transparent text-2xl text-[var(--accent)] no-underline max-[760px]:mb-0 max-[760px]:mr-2 max-[760px]:w-auto max-[760px]:min-w-16',
  brandOrb: 'grid size-[42px] place-items-center rounded-full bg-[radial-gradient(circle_at_35%_30%,#f9d9a6,#c8734b_52%,#5a3e94_100%)] font-extrabold text-[#190f0a] shadow-[0_0_28px_rgba(212,157,94,.34)]',
  brandText: 'font-vault-body text-xs text-[var(--fg)]',
  nav: 'flex w-full flex-col gap-1 max-[760px]:w-auto max-[760px]:flex-row',
  navBottom: 'mt-auto border-t border-[var(--border)] pt-3.5 max-[760px]:mt-0 max-[760px]:border-l max-[760px]:border-t-0 max-[760px]:pt-0 max-[760px]:pl-2.5',
  navLink: 'flex w-full flex-col items-center gap-1 border-r-2 border-transparent bg-transparent py-3.5 text-xs text-[var(--muted)] no-underline transition duration-200 ease-out hover:bg-[color-mix(in_oklch,var(--accent)_8%,transparent)] hover:text-[var(--fg)] max-[760px]:min-w-[72px] max-[760px]:border-r-0 max-[760px]:border-b-2 max-[760px]:p-2',
  navLinkActive: 'border-r-[var(--accent)] bg-[color-mix(in_oklch,var(--accent)_8%,transparent)] text-[var(--accent)] max-[760px]:border-b-[var(--accent)] max-[760px]:border-r-transparent',
  navShort: 'font-vault-mono text-[10px] tracking-[.08em]',
  main: 'flex h-screen min-w-0 flex-1 flex-col overflow-y-auto max-[760px]:h-auto max-[760px]:min-h-screen max-[760px]:overflow-visible',
  topbar: 'sticky top-0 z-10 flex min-h-[76px] items-center justify-end gap-4 border-b border-[var(--border)] bg-[color-mix(in_oklch,var(--bg)_80%,transparent)] px-10 backdrop-blur-[14px] max-[760px]:min-h-0 max-[760px]:flex-wrap max-[760px]:justify-start max-[760px]:px-3.5 max-[760px]:py-2.5',
  quickLinks: 'mr-auto flex items-center gap-3 max-[760px]:order-2 max-[760px]:w-full max-[760px]:overflow-x-auto',
  quickLinkButton: 'inline-flex min-h-9 cursor-pointer items-center gap-2 rounded-full border border-[var(--border)] bg-[var(--surface)] px-3.5 py-1.5 text-sm',
  topbarTools: 'flex items-center gap-2.5 max-[420px]:flex-wrap max-[420px]:gap-2',
  chip: 'inline-flex min-h-9 items-center gap-2 rounded-full border border-[var(--border)] bg-[var(--surface)] px-3.5 py-1.5 text-sm no-underline',
  avatarInitial: 'grid size-7 place-items-center rounded-full bg-[radial-gradient(circle_at_35%_30%,#f9d9a6,#c8734b_52%,#5a3e94_100%)] text-xs font-extrabold text-[#190f0a] shadow-[0_0_28px_rgba(212,157,94,.34)]',
  avatarMenuWrap: 'relative',
  avatarMenu: 'absolute right-0 top-[calc(100%+10px)] z-[60] w-[184px] rounded-xl border border-[var(--border)] bg-[color-mix(in_oklch,var(--surface)_94%,black_6%)] p-2 shadow-[var(--pg-shadow-lg)]',
  avatarMenuItem: 'flex min-h-[38px] w-full cursor-pointer items-center gap-2.5 rounded-lg border-0 bg-transparent px-2.5 py-2 text-left text-[var(--fg)] hover:bg-[color-mix(in_oklch,var(--accent)_10%,transparent)]',
  avatarMenuDanger: 'text-[oklch(74%_.16_35)]',
  avatarMenuDivider: 'my-1.5 mx-1 h-px border-0 bg-[var(--border)]',
  routeSurface: 'min-h-0',
  content: 'mx-auto w-full max-w-[1200px] p-10 max-[760px]:p-5 max-[420px]:p-4',
}

export const userButton = {
  base: 'inline-flex items-center justify-center gap-2 rounded-full border border-[var(--border)] bg-[var(--surface)] px-[18px] py-2.5 text-[var(--fg)] no-underline transition duration-200 ease-out hover:-translate-y-px hover:border-[color-mix(in_oklch,var(--accent)_45%,var(--border))]',
  primary: 'border-[var(--accent)] bg-[var(--accent)] font-extrabold text-[var(--bg)]',
  ghost: 'bg-transparent text-[var(--fg)]',
  danger: 'text-[oklch(70%_.17_30)]',
  icon: 'inline-grid size-10 place-items-center rounded-xl border border-[var(--border)] bg-[color-mix(in_oklch,var(--fg)_7%,transparent)] text-[var(--fg)]',
}

export const userForm = {
  field: 'grid gap-2',
  fieldLabel: 'text-xs text-[var(--muted)]',
  input: 'w-full rounded-[10px] border border-[var(--border)] bg-[var(--surface)] px-3 py-2.5 text-[var(--fg)] outline-none focus:border-[var(--accent)] focus:ring-3 focus:ring-[color-mix(in_oklch,var(--accent)_18%,transparent)]',
  textarea: 'w-full min-h-[120px] resize-y rounded-[10px] border border-[var(--border)] bg-[var(--surface)] px-3 py-2.5 text-[var(--fg)] outline-none focus:border-[var(--accent)] focus:ring-3 focus:ring-[color-mix(in_oklch,var(--accent)_18%,transparent)]',
}

export const userState = {
  spinner: 'size-3.5 animate-spin rounded-full border-2 border-current border-r-transparent',
  stateLine: 'grid gap-2 rounded-[var(--radius)] border border-dashed border-[var(--border)] p-6 text-[var(--muted)]',
  empty: 'grid gap-2 rounded-[var(--radius)] border border-dashed border-[var(--border)] p-6 text-[var(--muted)]',
  toastStack: 'fixed right-5 top-5 z-[100] grid w-[min(380px,calc(100vw-40px))] gap-3',
  toast: 'grid grid-cols-[auto_minmax(0,1fr)] gap-3 rounded-xl border border-[var(--toast-ring,var(--border))] bg-[color-mix(in_oklch,var(--surface)_92%,black_8%)] p-3 text-[var(--fg)] shadow-[var(--pg-shadow-lg)] backdrop-blur',
  modalBackdrop: 'fixed inset-0 z-[80] grid place-items-center bg-black/60 p-6',
  modalCard: 'max-h-[90vh] w-[min(920px,100%)] overflow-auto rounded-3xl border border-[var(--border)] bg-[var(--surface)] p-6',
}

export const userPill = {
  base: 'inline-flex w-fit items-center gap-1 rounded-full bg-[color-mix(in_oklch,var(--fg)_8%,transparent)] px-2 py-1 font-vault-mono text-[11px] text-[var(--muted)]',
  neutral: 'bg-[color-mix(in_oklch,var(--fg)_8%,transparent)] text-[var(--muted)]',
  good: 'bg-[color-mix(in_oklch,var(--accent-emerald)_18%,transparent)] text-[oklch(86%_.1_160)]',
  public: 'bg-[color-mix(in_oklch,var(--accent-emerald)_18%,transparent)] text-[oklch(86%_.1_160)]',
  success: 'bg-[color-mix(in_oklch,var(--accent-emerald)_18%,transparent)] text-[oklch(86%_.1_160)]',
  warn: 'bg-[color-mix(in_oklch,var(--accent)_18%,transparent)] text-[var(--accent)]',
  warning: 'bg-[color-mix(in_oklch,var(--accent)_18%,transparent)] text-[var(--accent)]',
  bad: 'bg-[color-mix(in_oklch,var(--accent-coral)_18%,transparent)] text-[oklch(76%_.14_35)]',
  danger: 'bg-[color-mix(in_oklch,var(--accent-coral)_18%,transparent)] text-[oklch(76%_.14_35)]',
}

export const userCard = {
  base: 'rounded-[var(--radius)] border border-[var(--border)] bg-[var(--surface)]',
  padded: 'rounded-[var(--radius)] border border-[var(--border)] bg-[var(--surface)] p-6',
}

export const userText = {
  eyebrow: 'mb-4 font-vault-mono text-xs uppercase tracking-[.15em] text-[var(--accent)]',
}
