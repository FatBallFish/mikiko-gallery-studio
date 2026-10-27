import { createServer } from 'vite'

// Use Vite's loader so the contract exercises the same raw Markdown imports as the app.
const server = await createServer({ server: { middlewareMode: true }, appType: 'custom' })
try {
  await server.ssrLoadModule('/src/content/markdown.contract.ts')
} finally {
  await server.close()
}
