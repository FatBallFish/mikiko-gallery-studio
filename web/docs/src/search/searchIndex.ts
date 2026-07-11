import { guides } from '../content/guides'
import type { SearchItem } from '../types'

const references: SearchItem[] = [
  { id: 'ref-capabilities', title: 'GET /api/open/image/v1/capabilities', summary: '读取开放图片模型能力', keywords: 'capabilities model', href: '#/reference', kind: 'reference' },
  { id: 'ref-estimate', title: 'GET /api/open/image/v1/estimate', summary: '获取任务积分预估', keywords: 'estimate points', href: '#/reference', kind: 'reference' },
  { id: 'ref-task-create', title: 'POST /api/open/image/v1/tasks', summary: '创建开放图片任务', keywords: 'create task generation', href: '#/reference', kind: 'reference' },
  { id: 'ref-task-detail', title: 'GET /api/open/image/v1/tasks/{task_id}', summary: '查询异步任务状态', keywords: 'poll status task', href: '#/reference', kind: 'reference' },
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
