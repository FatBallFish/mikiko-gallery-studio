// @ts-ignore contract scripts run in tsx/node; the admin app tsconfig does not include node types.
import { readdirSync, readFileSync } from 'node:fs'
// @ts-ignore contract scripts run in tsx/node; the admin app tsconfig does not include node types.
import { extname, join, relative } from 'node:path'
// @ts-ignore contract scripts run in tsx/node; the admin app tsconfig does not include node types.
import { fileURLToPath } from 'node:url'

const sourceRoot = fileURLToPath(new URL('.', import.meta.url))
const forbiddenPatterns = [
  'rounded-2xl',
  'rounded-3xl',
  'uppercase',
  'tracking-tight',
  'tracking-tighter',
  'tracking-wide',
  'tracking-wider',
  'tracking-widest',
  'tracking-[',
  'text-[10px]',
]

for (const file of sourceFiles(sourceRoot)) {
  const source = readFileSync(file, 'utf8')
  for (const forbidden of forbiddenPatterns) {
    if (source.includes(forbidden)) {
      throw new Error(`${relative(sourceRoot, file)} must remove visual drift ${forbidden}`)
    }
  }
}

function sourceFiles(directory: string): string[] {
  return readdirSync(directory, { withFileTypes: true }).flatMap((entry: { name: string; isDirectory: () => boolean }) => {
    const path = join(directory, entry.name)
    if (entry.isDirectory()) return sourceFiles(path)
    return extname(entry.name) === '.tsx' ? [path] : []
  })
}
