import { readFileSync } from 'node:fs'

const workspace = readFileSync(new URL('./WorkspacePage.tsx', import.meta.url), 'utf8')
const theme = readFileSync(new URL('../../../shared/user-theme.css', import.meta.url), 'utf8')

if (!theme.includes('--lv-accent-contrast:')) {
  throw new Error('user theme must expose a semantic accent contrast token')
}

if (workspace.includes('var(--accent-contrast)')) {
  throw new Error('workspace option states must use the declared --lv-accent-contrast token')
}

for (const className of ['selectItemActive', 'modelButtonActive', 'modelMetaActive']) {
  const expression = new RegExp(`${className}:\\s*['\"][^'\"]*text-white`)
  if (expression.test(workspace)) {
    throw new Error(`${className} must not hard-code white text in the light theme`)
  }
}

if (workspace.includes('[&_*]:text-white')) {
  throw new Error('workspace option states must not force every descendant to white')
}
