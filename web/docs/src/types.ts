import type { ReactNode } from 'react'

export type GuideId = 'quickstart' | 'authentication' | 'native-api' | 'openai-compatible' | 'image-editing' | 'reference-images' | 'task-polling' | 'capabilities' | 'estimates' | 'errors' | 'rate-limits' | 'troubleshooting'

export type Guide = {
  id: GuideId
  group: '开始使用' | '图片生成' | '运行与排错'
  title: string
  summary: string
  searchText: string
  content: ReactNode
}

export type SearchItem = {
  id: string
  title: string
  summary: string
  keywords: string
  href: string
  kind: 'guide' | 'reference'
}
