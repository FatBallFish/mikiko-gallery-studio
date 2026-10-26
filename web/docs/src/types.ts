import type { ReactNode } from 'react'

export type GuideId = 'quickstart' | 'authentication' | 'native-api' | 'openai-compatible' | 'image-editing' | 'reference-images' | 'task-polling' | 'capabilities' | 'estimates' | 'errors' | 'rate-limits' | 'troubleshooting' | 'migration' | 'sdk-examples' | 'mcp' | 'inventory'

export type Guide = {
  id: GuideId
  group: '开始使用' | '生成与资产' | '运行与排错' | 'MCP 与 OAuth'
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
