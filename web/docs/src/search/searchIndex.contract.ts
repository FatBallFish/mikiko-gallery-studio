import { guides } from '../content/guides'
import { buildSearchIndex, searchDocs } from './searchIndex'

const required = ['quickstart', 'authentication', 'native-api', 'openai-compatible', 'image-editing', 'reference-images', 'task-polling', 'capabilities', 'estimates', 'errors', 'rate-limits', 'troubleshooting']
if (required.some((id) => !guides.some((guide) => guide.id === id))) {
  throw new Error('documentation guide coverage is incomplete')
}
if (guides.some((guide) => !guide.title || !guide.summary || !guide.group || !guide.searchText)) {
  throw new Error('every guide needs navigation and search metadata')
}
const index = buildSearchIndex()
if (!searchDocs('OpenAI 兼容', index).some((item) => item.id === 'openai-compatible')) {
  throw new Error('search must normalize mixed Chinese and English terms')
}
if (!searchDocs('任务 状态', index).some((item) => item.id === 'task-polling')) {
  throw new Error('search must match Chinese guide intent')
}
