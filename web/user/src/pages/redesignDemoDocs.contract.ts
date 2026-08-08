import { readFileSync } from 'node:fs'

const source = readFileSync(new URL('./RedesignDemo.tsx', import.meta.url), 'utf8')

if (!source.includes("openDocsEntry('account-menu')") || !source.includes("openDocsEntry('footer')")) {
  throw new Error('redesign demo documentation entries must open the deployed documentation site directly')
}

if (source.includes("| 'docs'") || source.includes("activeTab === 'docs'") || source.includes('function DocsView(')) {
  throw new Error('redesign demo must not retain a local documentation tab or view')
}

if (source.includes(`<span className={rdShell.footerLink}>API 文档</span>`)) {
  throw new Error('redesign demo footer documentation entry must be actionable')
}
