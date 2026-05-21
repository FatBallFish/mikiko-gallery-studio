import { useMemo, useState } from 'react'
import type { EndpointDoc } from '../../../shared/api-types'
import { API_PATHS } from '../../../shared/api-types'
import { mockApi } from '../../../shared/mock-api'
import { CopyButton, EmptyState, ErrorState, LoadingState } from '../components'
import { useMockResource } from '../useMockResource'

const groups = ['All', 'Agent API', 'Open API', 'OpenAI Compat', 'Ops API']
const errors = [
  ['insufficient_balance', '积分余额不足，降低输出质量或充值后重试。'],
  ['invalid_signature', 'Open API 签名校验失败，请检查 AK/SK 与时间戳。'],
  ['provider_unavailable', '上游模型暂不可用，系统会按错误策略重试或降级。'],
  ['rate_limit_exceeded', 'RPM 或并发限制命中，请降低调用频率。'],
]

const methodByKey: Record<string, EndpointDoc['method']> = {
  sendEmailCode: 'POST',
  loginEmailCode: 'POST',
  refreshSession: 'POST',
  logout: 'POST',
  profile: 'GET',
  preferences: 'PUT',
  balance: 'GET',
  ledger: 'GET',
  estimate: 'POST',
  redeemCode: 'POST',
  capabilities: 'GET',
  referenceAssets: 'GET',
  uploadSessions: 'POST',
  tasks: 'POST',
  historyTasks: 'GET',
  publishImage: 'PUT',
  galleryImages: 'GET',
  generations: 'POST',
  edits: 'POST',
  models: 'GET',
  login: 'POST',
  users: 'GET',
  modelProviders: 'GET',
  modelRoutes: 'PUT',
  errorPolicies: 'PUT',
  configTabs: 'GET',
  imageReviews: 'GET',
  dashboard: 'GET',
}

const titleByKey: Record<string, string> = {
  sendEmailCode: '发送邮箱验证码',
  loginEmailCode: '邮箱验证码登录/注册',
  refreshSession: '刷新用户会话',
  logout: '退出登录',
  profile: '获取个人资料',
  preferences: '更新生成偏好',
  balance: '获取积分余额',
  ledger: '查询积分流水',
  estimate: '估算生成消耗',
  redeemCode: '兑换积分码',
  capabilities: '读取模型能力',
  referenceAssets: '管理参考素材',
  uploadSessions: '创建参考图上传会话',
  tasks: '创建或查询图片任务',
  historyTasks: '查询历史任务',
  publishImage: '提交公开图审核',
  galleryImages: '查询公开图片广场',
  generations: 'OpenAI 兼容文生图',
  edits: 'OpenAI 兼容图片编辑',
  models: 'OpenAI 兼容模型列表',
  login: '管理员登录',
  users: '用户管理列表',
  modelProviders: '模型供应商健康',
  modelRoutes: '更新模型路由',
  errorPolicies: '更新错误策略',
  configTabs: '配置中心数据',
  imageReviews: '图片公开审核',
  dashboard: '运营指标看板',
}

function docsFromPaths(): EndpointDoc[] {
  const sections: Array<[EndpointDoc['group'], Record<string, string>, string]> = [
    ['Agent API', API_PATHS.agent, 'Access Token'],
    ['Open API', API_PATHS.open, 'X-Access-Key + X-Signature'],
    ['OpenAI Compat', API_PATHS.compat, 'Bearer sk-*'],
    ['Ops API', API_PATHS.ops, 'Admin Token'],
  ]
  return sections.flatMap(([group, paths, auth]) => Object.entries(paths).map(([key, path]) => {
    const method = methodByKey[key] ?? (path.includes('{') ? 'PUT' : 'GET')
    const requestExample = method === 'GET'
      ? `curl https://api.picgallery.ai${path} \\\n  -H "Authorization: Bearer $TOKEN"`
      : `curl -X ${method} https://api.picgallery.ai${path} \\\n  -H "Authorization: Bearer $TOKEN" \\\n  -H "Content-Type: application/json" \\\n  -d '{"request_id":"req_demo"}'`
    return {
      group,
      method,
      path,
      title: titleByKey[key] ?? key,
      auth: group === 'Agent API' && key.startsWith('login') || key === 'sendEmailCode' ? 'none' : auth,
      requestExample,
      responseExample: '{"code":"ok","message":"success","data":{},"request_id":"req_demo"}',
    }
  }))
}

function mergeDocs(mockDocs: EndpointDoc[]) {
  const byPath = new Map<string, EndpointDoc>()
  docsFromPaths().forEach((doc) => byPath.set(`${doc.method}-${doc.path}`, doc))
  mockDocs.forEach((doc) => byPath.set(`${doc.method}-${doc.path}`, doc))
  return Array.from(byPath.values()).sort((a, b) => `${a.group}-${a.path}`.localeCompare(`${b.group}-${b.path}`))
}

export function DocsPage() {
  const docs = useMockResource(() => mockApi.listEndpointDocs(), [])
  const [query, setQuery] = useState('')
  const [group, setGroup] = useState('All')
  const catalog = useMemo(() => mergeDocs(docs.data ?? []), [docs.data])
  const filtered = useMemo(() => catalog.filter((item) => {
    const text = `${item.group} ${item.method} ${item.path} ${item.title} ${item.auth}`.toLowerCase()
    return (group === 'All' || item.group === group) && (!query || text.includes(query.toLowerCase()))
  }), [catalog, group, query])

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
            <span>Mock catalog expanded from API_PATHS</span>
          </div>
          {docs.loading ? <LoadingState /> : null}
          {docs.error ? <ErrorState message={docs.error} onRetry={docs.reload} /> : null}
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
          {errors.map(([code, detail]) => (
            <article key={code}>
              <code>{code}</code>
              <span>{detail}</span>
            </article>
          ))}
          <div className="code-sample small">
            <div><strong>统一响应</strong><CopyButton text={'{"code":"ok","message":"success","data":{},"request_id":"req_x"}'} /></div>
            <pre>{'{\n  "code": "ok",\n  "message": "success",\n  "data": {},\n  "request_id": "req_x"\n}'}</pre>
          </div>
        </aside>
      </section>
    </div>
  )
}
