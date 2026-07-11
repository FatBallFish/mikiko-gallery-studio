import { guides } from './guides'
import type { GuideId } from '../types'

const expectedIds: GuideId[] = [
  'quickstart',
  'authentication',
  'native-api',
  'openai-compatible',
  'image-editing',
  'reference-images',
  'task-polling',
  'capabilities',
  'estimates',
  'errors',
  'rate-limits',
  'troubleshooting',
]

const actualIds = guides.map((guide) => guide.id)
if (actualIds.length !== new Set(actualIds).size) throw new Error('guide IDs must be unique')

for (const id of expectedIds) {
  if (!actualIds.includes(id)) throw new Error(`missing guide: ${id}`)
}

for (const guide of guides) {
  if (!guide.title.trim()) throw new Error(`guide ${guide.id} is missing a title`)
  if (!guide.summary.trim()) throw new Error(`guide ${guide.id} is missing a summary`)
  if (!guide.group.trim()) throw new Error(`guide ${guide.id} is missing a navigation group`)
  if (!guide.searchText.trim()) throw new Error(`guide ${guide.id} is missing search text`)
}

console.log(`Docs guide contract passed (${guides.length} guides)`)
