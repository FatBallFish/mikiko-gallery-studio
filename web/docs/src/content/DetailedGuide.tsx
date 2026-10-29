import { isValidElement, memo } from 'react'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import { CodeBlock } from '../components/CodeBlock'
import type { GuideId } from '../types'

interface MarkdownNode {
  type: string
  value?: string
  depth?: number
  children?: MarkdownNode[]
  data?: { hProperties?: Record<string, string> }
}
function textOf(node: MarkdownNode): string {
  return node.value ?? node.children?.map(textOf).join('') ?? ''
}
export function guideHeadings() {
  return (tree: MarkdownNode) => {
    const used = new Set<string>()
    tree.children = tree.children?.filter((node) => !(node.type === 'heading' && node.depth === 1))
    for (const node of tree.children ?? []) {
      if (node.type !== 'heading') continue
      const last = node.children?.[node.children.length - 1]
      const explicit = last?.value?.match(/\s*\{#([\w-]+)\}$/)
      if (explicit && last?.value) last.value = last.value.slice(0, explicit.index)
      const base = explicit?.[1] ?? (textOf(node).normalize('NFKC').toLowerCase().replace(/[^\p{L}\p{N}\s_-]/gu, '').trim().replace(/\s+/g, '-') || 'section')
      let id = base
      let suffix = 1
      while (used.has(id)) id = `${base}-${suffix++}`
      used.add(id)
      node.data = { ...node.data, hProperties: { ...node.data?.hProperties, id } }
    }
  }
}
const sourceRoutes: Record<string, string> = {
  'mcp.md': 'mcp', 'developer-field-map.md': 'inventory', 'sdk-examples.md': 'sdk-examples',
  'migration.md': 'migration', 'quickstart.md': 'quickstart', 'authentication.md': 'authentication',
  'native-api.md': 'native-api', 'openai-compatible.md': 'openai-compatible',
  'image-editing.md': 'image-editing', 'reference-images.md': 'reference-images',
  'task-polling.md': 'task-polling', 'capabilities.md': 'capabilities', 'estimates.md': 'estimates',
  'errors.md': 'errors', 'rate-limits.md': 'rate-limits', 'troubleshooting.md': 'troubleshooting',
}
export function guideHref(href: string | undefined, id: GuideId) {
  if (!href) return undefined
  if (href.startsWith('#/')) return href
  if (href.startsWith('#')) return `#/guide/${id}${href}`
  if (/^(?:https?:|mailto:)/i.test(href)) return href
  const [path, anchor] = href.split('#')
  if (path.endsWith('openapi.yaml')) return '#/reference'
  const route = sourceRoutes[path.split('/').pop() ?? '']
  return route ? `#/guide/${route}${anchor ? `#${anchor}` : ''}` : href
}
export const DetailedGuide = memo(function DetailedGuide({ source, id }: { source: string; id: GuideId }) {
  return <ReactMarkdown remarkPlugins={[remarkGfm, guideHeadings]} skipHtml components={{
    a: ({ href, children }) => <a href={guideHref(href, id)}>{children}</a>,
    table: ({ children }) => <div className="guide-table-scroll" tabIndex={0} role="region" aria-label="可横向滚动的表格"><table>{children}</table></div>,
    pre: ({ children }) => {
      if (!isValidElement<{ className?: string; children?: string }>(children)) return <pre>{children}</pre>
      const language = /language-([^\s]+)/.exec(children.props.className ?? '')?.[1] ?? 'text'
      return <CodeBlock language={language} code={String(children.props.children ?? '').replace(/\n$/, '')} />
    },
    code: ({ children }) => <code>{children}</code>,
  }}>{source}</ReactMarkdown>
})
