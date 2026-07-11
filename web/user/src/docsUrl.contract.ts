import { docsEntryPoints, docsUrl, openDocsEntry, openDocsSite, resolveDocsUrl } from './docsUrl'

if (docsUrl({ VITE_DOCS_URL: 'https://docs.example.com/' }) !== 'https://docs.example.com/') {
  throw new Error('configured documentation URL was not preserved')
}

if (docsUrl({}) !== 'http://localhost:5175/') {
  throw new Error('local docs fallback drifted')
}

if (resolveDocsUrl({ VITE_DOCS_URL: '   ' }, { VITE_DOCS_URL: 'https://build-docs.example.com/' }) !== 'https://build-docs.example.com/') {
  throw new Error('blank runtime documentation URL should fall through to the build environment')
}

if (resolveDocsUrl({}, { VITE_DOCS_URL: 'https://build-docs.example.com/' }) !== 'https://build-docs.example.com/') {
  throw new Error('missing runtime documentation URL should fall through to the build environment')
}

if (resolveDocsUrl({ VITE_DOCS_URL: '' }, { VITE_DOCS_URL: '' }) !== 'http://localhost:5175/') {
  throw new Error('blank runtime and build documentation URLs should use the local fallback')
}

const opened: Array<{ url?: string | URL; target?: string; features?: string }> = []
openDocsSite({
  runtimeEnv: { VITE_DOCS_URL: 'https://runtime-docs.example.com/' },
  buildEnv: { VITE_DOCS_URL: 'https://build-docs.example.com/' },
  open: (url, target, features) => {
    opened.push({ url, target, features })
    return null
  },
})

if (opened[0]?.url !== 'https://runtime-docs.example.com/') {
  throw new Error(`runtime documentation URL should take priority, got ${String(opened[0]?.url)}`)
}

if (opened[0]?.target !== '_blank' || opened[0]?.features !== 'noopener,noreferrer') {
  throw new Error(`documentation site must open safely in a new tab, got ${JSON.stringify(opened[0])}`)
}

const expectedEntryPoints = ['home', 'api-keys', 'account-menu', 'footer', 'legacy-route']
if (JSON.stringify(docsEntryPoints) !== JSON.stringify(expectedEntryPoints)) {
  throw new Error(`known documentation entry points drifted, got ${JSON.stringify(docsEntryPoints)}`)
}

const entryOpens: string[] = []
for (const entryPoint of docsEntryPoints) {
  openDocsEntry(entryPoint, {
    runtimeEnv: { VITE_DOCS_URL: 'https://docs.example.com/' },
    open: (url) => {
      entryOpens.push(String(url))
      return null
    },
  })
}
if (entryOpens.length !== docsEntryPoints.length || entryOpens.some((url) => url !== 'https://docs.example.com/')) {
  throw new Error(`every known documentation entry must open the external site directly, got ${JSON.stringify(entryOpens)}`)
}
