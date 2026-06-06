import { useMemo, useState } from 'react'
import type { EndpointDoc } from '../../../shared/api-types'
import type { DocsErrorCode, DocsExample } from '../../../shared/open-api-docs'
import { docsCopyableExamplesText } from '../../../shared/open-api-docs'
import { openApi } from '../../../shared/open-api'
import { CopyButton, EmptyState, LoadingState } from '../components'
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
    <div className="content docs-page" style={{ padding: 40 }}>
      <div className="header" style={{ marginBottom: 48, display: 'flex', justifyContent: 'space-between', alignItems: 'flex-end', gap: 20, flexWrap: 'wrap' }}>
        <div>
          <p className="eyebrow">{docsSectionLabels.eyebrow}</p>
          <h1 style={{ fontSize: 48, margin: 0 }}>开发文档</h1>
        </div>
        <div className="filters" style={{ display: 'flex', gap: 12, flexWrap: 'wrap' }}>
          <input value={query} onChange={(event) => setQuery(event.target.value)} placeholder={docsSearchPlaceholder} style={{ width: 320, borderRadius: 8 }} />
          <select value={group} onChange={(event) => setGroup(event.target.value)} style={{ width: 180, borderRadius: 8 }}>{groupOptions.map((item) => <option key={item.value} value={item.value}>{item.label}</option>)}</select>
        </div>
      </div>

      <section className="docs-layout">
        <div className="endpoint-list card" style={{ background: 'var(--vault-panel)', borderRadius: 12, border: '1px solid var(--vault-line)', padding: 24 }}>
          <div style={{ display: 'flex', justifyContent: 'space-between', gap: 12, marginBottom: 16, color: 'var(--vault-muted)' }}>
            <span>{docsEndpointCountLabel(filtered.length, catalog.length)}</span>
            <span>{docsSectionLabels.realtimeCatalog}</span>
          </div>
          {docs.loading ? <LoadingState label="正在读取 OpenAPI 文档..." /> : null}
          {!docs.loading && endpointUnavailable ? <EmptyState title="文档接口不可用" detail={docsError ?? '请稍后重试，或联系管理员检查 /docs/openapi.json。'} /> : null}
          {!docs.loading && !endpointUnavailable && !filtered.length ? <EmptyState title="没有匹配端点" detail="尝试搜索 tasks、images、balance 或 OpenAI。" /> : null}
          {filtered.map((doc) => (
            <article key={`${doc.method}-${doc.path}`} className="endpoint-card" style={{ marginBottom: 14 }}>
              <div className="endpoint-head">
                <span className="status-pill neutral">{docsGroupTagLabel(doc.group)}</span>
                <b>{doc.method}</b>
                <code>{doc.path}</code>
              </div>
              <h2>{doc.title}</h2>
              <span>{docsSectionLabels.authPrefix}: {docsAuthLabel(doc.auth)}</span>
              <div className="endpoint-examples">
                <div>
                  <div><strong>{docsSectionLabels.request}</strong><CopyButton text={doc.requestExample} /></div>
                  <pre>{doc.requestExample}</pre>
                </div>
                <div>
                  <div><strong>{docsSectionLabels.response}</strong><CopyButton text={doc.responseExample} /></div>
                  <pre>{doc.responseExample}</pre>
                </div>
              </div>
            </article>
          ))}
        </div>

        <aside className="docs-aside card" style={{ background: 'var(--vault-panel)', borderRadius: 12, border: '1px solid var(--vault-line)' }}>
          <p className="eyebrow">{docsSectionLabels.errors}</p>
          <h2>{docsSectionLabels.errors}</h2>
          {docsError && !docs.error ? <EmptyState title="文档接口不可用" detail={docsError} /> : null}
          {errorRowModels.map((row) => (
            <article key={row.code}>
              <code>{row.statusLabel} {row.code}</code>
              <span>{row.message}</span>
              <small>{row.retryableLabel} · {row.recoveryHint}</small>
            </article>
          ))}
          <div className="code-sample small">
            <div><strong>{docsSectionLabels.examples}</strong><CopyButton text={examplesText} /></div>
            <pre>{examplesText}</pre>
          </div>
        </aside>
      </section>
    </div>
  )
}
