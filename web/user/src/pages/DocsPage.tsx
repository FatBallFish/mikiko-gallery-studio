import { useMemo, useState } from 'react'
import type { EndpointDoc } from '../../../shared/api-types'
import { cn } from '../../../shared/classnames'
import type { DocsErrorCode, DocsExample } from '../../../shared/open-api-docs'
import { docsCopyableExamplesText } from '../../../shared/open-api-docs'
import { openApi } from '../../../shared/open-api'
import { CopyButton, EmptyState, LoadingState } from '../components'
import { userPill } from '../ui/classes'
import { useApiResource } from '../useApiResource'
import {
  docsAuthLabel,
  docsEndpointCountLabel,
  docsErrorRows,
  docsGroupOptions,
  docsGroupTagLabel,
  docsSearchPlaceholder,
  docsSectionLabels,
  filterDocsEndpoints,
} from './docsPageModel'

const fallbackErrors = [
  ['insufficient_balance', '积分余额不足，降低输出质量或充值后重试。'],
  ['invalid_signature', 'Open API 签名校验失败，请检查 AK/SK 与时间戳。'],
  ['provider_unavailable', '上游模型暂不可用，系统会按错误策略重试或降级。'],
  ['rate_limit_exceeded', 'RPM 或并发限制命中，请降低调用频率。'],
] satisfies Array<[string, string]>

const docsClasses = {
  content: 'docs-page w-full flex-1 p-6 md:p-10',
  header: 'mb-12 flex flex-col gap-6 lg:flex-row lg:items-end lg:justify-between',
  title: 'm-0 text-4xl font-black leading-none md:text-6xl',
  filters: 'flex flex-wrap gap-3',
  search: 'h-12 w-full rounded-xl border border-[var(--border)] bg-[var(--surface)] px-4 text-sm text-[var(--fg)] sm:w-80',
  groupSelect: 'h-12 w-full rounded-xl border border-[var(--border)] bg-[var(--surface)] px-4 text-sm text-[var(--fg)] sm:w-45',
  layout: 'grid gap-6 lg:grid-cols-[minmax(0,1fr)_320px]',
  endpointList: 'rounded-3xl border border-[var(--border)] bg-[var(--surface)] p-6 md:p-8',
  listMeta: 'mb-4 flex justify-between gap-3 text-[var(--muted)]',
  endpointCard: 'mb-3.5 rounded-2xl border border-[var(--border)] bg-[var(--bg)]/50 p-[18px]',
  endpointHead: 'flex items-start justify-between gap-3',
  endpointPath: 'min-w-0 [overflow-wrap:anywhere] font-mono text-[var(--accent)]',
  endpointTitle: 'my-3 text-2xl font-black leading-tight',
  authLine: 'text-sm text-[var(--muted)]',
  examples: 'mt-3 grid gap-3 md:grid-cols-2',
  examplePanel: 'min-w-0 rounded-xl border border-[var(--border)] bg-[var(--bg)] p-3',
  exampleHead: 'mb-2 flex items-center justify-between gap-2',
  examplePre: 'm-0 max-h-[360px] overflow-auto whitespace-pre-wrap font-mono text-xs leading-5 text-[var(--muted)]',
  aside: 'grid gap-4 rounded-3xl border border-[var(--border)] bg-[var(--surface)] p-6 md:p-8',
  asideTitle: 'm-0 text-2xl font-black leading-tight',
  errorRow: 'grid gap-1 border-b border-[var(--border)] pb-3 last:border-b-0',
  errorCode: '[overflow-wrap:anywhere] font-mono text-sm text-[var(--accent)]',
  errorMessage: 'text-sm text-[var(--fg)]',
  errorHint: 'text-xs text-[var(--muted)]',
  codeSample: 'overflow-x-auto rounded-2xl border border-[var(--border)] bg-[var(--bg)] p-[18px] text-[var(--fg)]',
  codeSampleHead: 'mb-3 flex items-center justify-between gap-2',
  codeSamplePre: 'm-0 whitespace-pre-wrap font-mono text-xs leading-5',
}

export function DocsPage() {
  const docs = useApiResource(() => openApi.listEndpointDocs(), [])
  const examples = useApiResource(() => openApi.getExamples(), [])
  const errors = useApiResource(() => openApi.getErrors(), [])
  const [query, setQuery] = useState('')
  const [group, setGroup] = useState('All')
  const catalog = useMemo(() => docs.data ?? [], [docs.data])
  const groupOptions = useMemo(() => docsGroupOptions(), [])
  const filtered = useMemo(() => filterDocsEndpoints(catalog, group, query), [catalog, group, query])
  const docsError = docs.error || examples.error || errors.error
  const endpointUnavailable = Boolean(docs.error)
  const errorRows: Array<DocsErrorCode | [string, string]> = errors.data?.length ? errors.data : fallbackErrors
  const errorRowModels = useMemo(() => docsErrorRows(errorRows), [errorRows])
  const exampleRows: DocsExample[] = examples.data ?? []
  const examplesText = docsCopyableExamplesText(exampleRows)

  return (
    <div className={docsClasses.content}>
      <div className={docsClasses.header}>
        <div>
          <h1 className={docsClasses.title}>开发文档</h1>
        </div>
        <div className={docsClasses.filters}>
          <input className={docsClasses.search} value={query} onChange={(event) => setQuery(event.target.value)} placeholder={docsSearchPlaceholder} />
          <select className={docsClasses.groupSelect} value={group} onChange={(event) => setGroup(event.target.value)}>{groupOptions.map((item) => <option key={item.value} value={item.value}>{item.label}</option>)}</select>
        </div>
      </div>

      <section className={docsClasses.layout}>
        <div className={docsClasses.endpointList}>
          <div className={docsClasses.listMeta}>
            <span>{docsEndpointCountLabel(filtered.length, catalog.length)}</span>
            <span>{docsSectionLabels.realtimeCatalog}</span>
          </div>
          {docs.loading ? <LoadingState label="正在读取 OpenAPI 文档..." /> : null}
          {!docs.loading && endpointUnavailable ? <EmptyState title="文档接口不可用" detail={docsError ?? '请稍后重试，或联系管理员检查 /docs/openapi.json。'} /> : null}
          {!docs.loading && !endpointUnavailable && !filtered.length ? <EmptyState title="没有匹配端点" detail="尝试搜索 tasks、images、balance 或 OpenAI。" /> : null}
          {filtered.map((doc) => (
            <article key={`${doc.method}-${doc.path}`} className={docsClasses.endpointCard}>
              <div className={docsClasses.endpointHead}>
                <span className={cn(userPill.base, userPill.neutral)}>{docsGroupTagLabel(doc.group)}</span>
                <b>{doc.method}</b>
                <code className={docsClasses.endpointPath}>{doc.path}</code>
              </div>
              <h2 className={docsClasses.endpointTitle}>{doc.title}</h2>
              <span className={docsClasses.authLine}>{docsSectionLabels.authPrefix}: {docsAuthLabel(doc.auth)}</span>
              <div className={docsClasses.examples}>
                <div className={docsClasses.examplePanel}>
                  <div className={docsClasses.exampleHead}><strong>{docsSectionLabels.request}</strong><CopyButton text={doc.requestExample} /></div>
                  <pre className={docsClasses.examplePre}>{doc.requestExample}</pre>
                </div>
                <div className={docsClasses.examplePanel}>
                  <div className={docsClasses.exampleHead}><strong>{docsSectionLabels.response}</strong><CopyButton text={doc.responseExample} /></div>
                  <pre className={docsClasses.examplePre}>{doc.responseExample}</pre>
                </div>
              </div>
            </article>
          ))}
        </div>

        <aside className={docsClasses.aside}>
          <h2 className={docsClasses.asideTitle}>{docsSectionLabels.errors}</h2>
          {docsError && !docs.error ? <EmptyState title="文档接口不可用" detail={docsError} /> : null}
          {errorRowModels.map((row) => (
            <article key={row.code} className={docsClasses.errorRow}>
              <code className={docsClasses.errorCode}>{row.statusLabel} {row.code}</code>
              <span className={docsClasses.errorMessage}>{row.message}</span>
              <small className={docsClasses.errorHint}>{row.retryableLabel} · {row.recoveryHint}</small>
            </article>
          ))}
          <div className={docsClasses.codeSample}>
            <div className={docsClasses.codeSampleHead}><strong>{docsSectionLabels.examples}</strong><CopyButton text={examplesText} /></div>
            <pre className={docsClasses.codeSamplePre}>{examplesText}</pre>
          </div>
        </aside>
      </section>
    </div>
  )
}
