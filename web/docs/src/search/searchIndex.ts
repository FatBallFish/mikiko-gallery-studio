import { guides } from '../content/guides'
import type { SearchItem } from '../types'

const references: SearchItem[] = [
  { id: 'ref-capabilities', title: 'GET /api/open/v1/image-models', summary: '读取开放图片模型能力', keywords: 'capabilities model', href: '#/reference', kind: 'reference' },
  { id: 'ref-estimate', title: 'POST /api/open/v1/image-tasks/preflight', summary: '完整请求预检与报价', keywords: 'estimate preflight points', href: '#/reference', kind: 'reference' },
  { id: 'ref-task-create', title: 'POST /api/open/v1/image-tasks', summary: '创建持久图片任务', keywords: 'create task generation', href: '#/reference', kind: 'reference' },
  { id: 'ref-task-detail', title: 'GET /api/open/v1/image-tasks/{task_id}', summary: '查询异步任务状态', keywords: 'poll status task', href: '#/reference', kind: 'reference' },
  { id: 'ref-video', title: 'POST /api/open/v1/video-tasks', summary: '创建持久视频任务', keywords: 'video create task', href: '#/reference', kind: 'reference' },
  { id: 'ref-assets', title: 'GET /api/open/v1/assets', summary: '查询图片与视频资产', keywords: 'assets media', href: '#/reference', kind: 'reference' },
  { id: 'ref-mcp', title: '/mcp', summary: 'OAuth MCP Streamable HTTP', keywords: 'mcp oauth tools scopes', href: '#/guide/mcp', kind: 'reference' },
  { id: 'ref-openai', title: 'POST /v1/images/generations', summary: 'OpenAI 兼容文生图', keywords: 'openai compatible generations', href: '#/reference', kind: 'reference' },
]

export function buildSearchIndex(): SearchItem[] {
  return [
    ...guides.map((guide) => ({ id: guide.id, title: guide.title, summary: guide.summary, keywords: guide.searchText, href: `#/guide/${guide.id}`, kind: 'guide' as const })),
    ...references,
  ]
}

export function searchDocs(query: string, items = buildSearchIndex()) {
  const terms = query.trim().toLocaleLowerCase().split(/\s+/).filter(Boolean)
  if (!terms.length) return items.slice(0, 8)
  return items.filter((item) => {
    const haystack = `${item.title} ${item.summary} ${item.keywords}`.toLocaleLowerCase()
    return terms.every((term) => haystack.includes(term))
  }).slice(0, 12)
}
