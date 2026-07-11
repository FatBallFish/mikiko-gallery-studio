import { useEffect, useRef } from 'react'
import { docsSiteUrl, openDocsEntry } from '../docsUrl'
import { ExternalLink } from '../ui/icons'

export const docsRedirectCopy = {
  title: '正在打开开发者文档',
  detail: '文档已迁移至独立站点。如果浏览器阻止了新标签页，请使用下方按钮继续。',
  action: '打开开发者文档',
} as const

export function DocsPage() {
  const openedRef = useRef(false)
  const destination = docsSiteUrl()

  useEffect(() => {
    if (openedRef.current) return
    openedRef.current = true
    openDocsEntry('legacy-route')
  }, [])

  return (
    <main className="grid min-h-screen place-items-center overflow-hidden bg-[var(--bg)] p-6 text-[var(--fg)]">
      <section className="relative grid w-full max-w-xl gap-6 overflow-hidden rounded-2xl border border-[var(--border)] bg-[var(--surface)] p-8 shadow-[var(--pg-shadow-lg)] sm:p-10" role="status" aria-live="polite">
        <span className="absolute inset-y-0 left-0 w-1 bg-[var(--accent)]" aria-hidden="true" />
        <div className="grid gap-3">
          <h1 className="m-0 text-2xl font-semibold sm:text-3xl">{docsRedirectCopy.title}</h1>
          <p className="m-0 leading-7 text-[var(--muted)]">{docsRedirectCopy.detail}</p>
          <code className="max-w-full overflow-hidden text-ellipsis whitespace-nowrap rounded-lg border border-[var(--border)] bg-[var(--bg)] px-3 py-2 text-xs text-[var(--dim)]" title={destination}>{destination}</code>
        </div>
        <button
          type="button"
          className="inline-flex min-h-11 w-fit items-center justify-center gap-2 rounded-xl border border-[var(--accent)] bg-[var(--accent)] px-5 py-2.5 text-sm font-bold text-[var(--bg)] transition-transform duration-[var(--motion-fast)] hover:-translate-y-px active:translate-y-0 motion-reduce:transition-none"
          onClick={() => openDocsEntry('legacy-route')}
        >
          {docsRedirectCopy.action}
          <ExternalLink size={17} strokeWidth={1.5} aria-hidden="true" />
        </button>
      </section>
    </main>
  )
}
