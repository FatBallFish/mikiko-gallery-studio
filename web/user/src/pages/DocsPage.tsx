import { useMemo, useState } from 'react'
import type { EndpointDoc } from '../../../shared/api-types'
import { openApi } from '../../../shared/open-api'
import { CopyButton, EmptyState, LoadingState } from '../components'
import { useApiResource } from '../useApiResource'

const groups = ['All', 'Agent API', 'Open API', 'OpenAI Compat', 'Ops API']
const fallbackErrors = [
  ['insufficient_balance', '积分余额不足，降低输出质量或充值后重试。'],
  ['invalid_signature', 'Open API 签名校验失败，请检查 AK/SK 与时间戳。'],
  ['provider_unavailable', '上游模型暂不可用，系统会按错误策略重试或降级。'],
  ['rate_limit_exceeded', 'RPM 或并发限制命中，请降低调用频率。'],
]

export function DocsPage() {
  const docs = useApiResource(() => openApi.listEndpointDocs(), [])
  const examples = useApiResource(() => openApi.getExamples(), [])
  const errors = useApiResource(() => openApi.getErrors(), [])
  const [query, setQuery] = useState('')
  const [group, setGroup] = useState('All')
  const catalog = useMemo(() => docs.data ?? [], [docs.data])
  const filtered = useMemo(() => catalog.filter((item: EndpointDoc) => {
    const text = `${item.group} ${item.method} ${item.path} ${item.title} ${item.auth}`.toLowerCase()
    return (group === 'All' || item.group === group) && (!query || text.includes(query.toLowerCase()))
  }), [catalog, group, query])
  const errorRows = Array.isArray(errors.data) ? errors.data : fallbackErrors

  return (
    <div className="content docs-page" style={{ padding: 40 }}>
      <div className="header" style={{ marginBottom: 48, display: 'flex', justifyContent: 'space-between', alignItems: 'flex-end', gap: 20, flexWrap: 'wrap' }}>
        <div>
          <p className="eyebrow">DEVELOPER PORTAL</p>
          <h1 style={{ fontSize: 48, margin: 0 }}>开发文档</h1>
        </div>
        <div className="filters" style={{ display: 'flex', gap: 12, flexWrap: 'wrap' }}>
          <input value={query} onChange={(event) => setQuery(event.target.value)} placeholder="搜索 endpoint / auth / title" style={{ width: 320, borderRadius: 8 }} />
          <select value={group} onChange={(event) => setGroup(event.target.value)} style={{ width: 180, borderRadius: 8 }}>{groups.map((item) => <option key={item}>{item}</option>)}</select>
        </div>
      </div>

      <section className="docs-layout">
        <div className="endpoint-list card" style={{ background: 'var(--vault-panel)', borderRadius: 12, border: '1px solid var(--vault-line)', padding: 24 }}>
          <div style={{ display: 'flex', justifyContent: 'space-between', gap: 12, marginBottom: 16, color: 'var(--vault-muted)' }}>
            <span>{filtered.length} / {catalog.length} endpoints</span>
            <span>OpenAPI 实时目录</span>
          </div>
          {docs.loading ? <LoadingState label="正在读取 OpenAPI 文档..." /> : null}
          {!docs.loading && !filtered.length ? <EmptyState title="没有匹配端点" detail="尝试搜索 tasks、images、balance 或 OpenAI。" /> : null}
          {filtered.map((doc) => (
            <article key={`${doc.method}-${doc.path}`} className="endpoint-card" style={{ marginBottom: 14 }}>
              <div className="endpoint-head">
                <span className="status-pill neutral">{doc.group}</span>
                <b>{doc.method}</b>
                <code>{doc.path}</code>
              </div>
              <h2>{doc.title}</h2>
              <span>Auth: {doc.auth}</span>
              <div className="endpoint-examples">
                <div>
                  <div><strong>Request</strong><CopyButton text={doc.requestExample} /></div>
                  <pre>{doc.requestExample}</pre>
                </div>
                <div>
                  <div><strong>Response</strong><CopyButton text={doc.responseExample} /></div>
                  <pre>{doc.responseExample}</pre>
                </div>
              </div>
            </article>
          ))}
        </div>

        <aside className="docs-aside card" style={{ background: 'var(--vault-panel)', borderRadius: 12, border: '1px solid var(--vault-line)' }}>
          <p className="eyebrow">Error Codes</p>
          <h2>错误码示例</h2>
          {errorRows.map((row: any) => {
            const code = Array.isArray(row) ? row[0] : row.code
            const detail = Array.isArray(row) ? row[1] : row.message ?? row.detail
            return <article key={code}><code>{code}</code><span>{detail}</span></article>
          })}
          <div className="code-sample small">
            <div><strong>接口示例</strong><CopyButton text={JSON.stringify(examples.data ?? {}, null, 2)} /></div>
            <pre>{JSON.stringify(examples.data ?? { code: 'ok', message: 'success', data: {}, request_id: 'req_x' }, null, 2)}</pre>
          </div>
        </aside>
      </section>
    </div>
  )
}
