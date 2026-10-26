import React from 'react'

function inline(text: string): React.ReactNode[] {
  return text.split(/(`[^`]+`|\[[^\]]+\]\([^)]+\))/g).map((part, index) => {
    if (part.startsWith('`')) return <code key={index}>{part.slice(1, -1)}</code>
    const link = part.match(/^\[([^\]]+)\]\(([^)]+)\)$/)
    if (link) {
      const href = link[2].endsWith('developer-field-map.md') ? '#/guide/inventory' : link[2].endsWith('mcp.md') ? '#/guide/mcp' : link[2].includes('openapi.yaml') ? '#/reference' : link[2]
      return <a key={index} href={href}>{link[1]}</a>
    }
    return part
  })
}

// Small static renderer for owned guide sources; HTML is always escaped by React.
export function DetailedGuide({ source }: { source: string }) {
  const lines = source.split('\n')
  const blocks: React.ReactNode[] = []
  for (let i = 0; i < lines.length; i++) {
    const line = lines[i]
    if (!line.trim()) continue
    if (line.startsWith('```')) {
      const code: string[] = []
      while (++i < lines.length && !lines[i].startsWith('```')) code.push(lines[i])
      blocks.push(<pre key={i} style={{ overflowX: 'auto' }}><code>{code.join('\n')}</code></pre>)
    } else if (line.startsWith('|')) {
      const rows: string[][] = []
      do { if (!/^\|[\s:|-]+\|$/.test(lines[i])) rows.push(lines[i].split('|').slice(1, -1).map(cell => cell.trim())) } while (++i < lines.length && lines[i].startsWith('|'))
      i--
      blocks.push(<div key={i} style={{ overflowX: 'auto' }}><table><thead><tr>{rows[0]?.map((cell, j) => <th key={j}>{inline(cell)}</th>)}</tr></thead><tbody>{rows.slice(1).map((row, j) => <tr key={j}>{row.map((cell, k) => <td key={k}>{inline(cell)}</td>)}</tr>)}</tbody></table></div>)
    } else if (/^#{1,3} /.test(line)) {
      blocks.push(line.startsWith('### ') ? <h3 key={i}>{line.slice(4)}</h3> : <h2 key={i}>{line.replace(/^#+ /, '')}</h2>)
    } else blocks.push(<p key={i}>{inline(line)}</p>)
  }
  return <>{blocks}</>
}
