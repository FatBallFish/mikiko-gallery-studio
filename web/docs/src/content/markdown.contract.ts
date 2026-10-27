import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { DetailedGuide, guideHref } from './DetailedGuide'
import { guides } from './guides'
import './guides.contract'
import '../search/searchIndex.contract'

function assert(value: unknown, message: string): asserts value {
  if (!value) throw new Error(message)
}
for (const guide of guides) {
  assert(typeof guide.content === 'string' && guide.content.includes('## '), `${guide.id}: Markdown sections required`)
  assert(!/studio\.example\.com|api\.example\.com|localhost:5273/.test(guide.content), `${guide.id}: placeholder endpoint`)
  const html = renderToStaticMarkup(createElement(DetailedGuide, { source: guide.content, id: guide.id }))
  const headings = [...html.matchAll(/<h[23] id="([^"]+)"/g)].map(match => match[1])
  assert(headings.length && headings.length === new Set(headings).size, `${guide.id}: unique heading IDs`)
  assert(!html.includes('{#'), `${guide.id}: explicit anchor markers leaked`)
  assert(!html.includes('<h1'), `${guide.id}: shell owns page title`)
}
const fixture = renderToStaticMarkup(createElement(DetailedGuide, { id: 'quickstart', source: `## 标题
### 标题
## 标题-1

普通段落的第一行
第二行仍在同一段。

\`inline\`

\`\`\`json
{"ok":true}
\`\`\`

\`\`\`
one line
\`\`\`

| 字段 | 说明 |
| --- | --- |
| id | 标识 |

<script>alert(1)</script>

[unsafe](javascript:alert%281%29)` }))
assert(fixture.includes('id="标题"') && fixture.includes('id="标题-1"') && fixture.includes('id="标题-1-1"'), 'collision-safe Chinese heading IDs')
assert(fixture.includes('第一行\n第二行'), 'soft line breaks form one paragraph')
assert(fixture.includes('data-language="json"') && fixture.includes('data-language="text"'), 'all fenced code uses copy component')
assert(fixture.includes('guide-table-scroll') && fixture.includes('<table>'), 'GFM table wrapper')
assert(!fixture.includes('<script>') && !fixture.includes('href="javascript:'), 'unsafe content blocked')
assert(guideHref('#标题', 'quickstart') === '#/guide/quickstart#标题', 'same-page route')
assert(guideHref('mcp.md', 'quickstart') === '#/guide/mcp', 'source link route')
assert(guideHref('../../web/docs/public/openapi/openapi.yaml', 'quickstart') === '#/reference', 'reference route preserved')
for (const id of ['quickstart', 'authentication', 'sdk-examples', 'migration']) {
  const source = guides.find(guide => guide.id === id)!.content
  assert(source.includes('sk-') && source.includes('ak-') && source.includes('MIKIKO_API_KEY'), `${id}: SK versus AK guidance`)
  assert(source.includes('https://mikiko.studio'), `${id}: production endpoint`)
}
const mcp = guides.find(guide => guide.id === 'mcp')!.content
assert(mcp.includes('https://mikiko.studio/mcp') && mcp.includes('OAuth'), 'MCP OAuth endpoint')
console.log(`Markdown rendering, routes, safety, endpoints and credential contracts passed (${guides.length} guides)`)
