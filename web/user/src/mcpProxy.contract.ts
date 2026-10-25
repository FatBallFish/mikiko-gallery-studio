import assert from 'node:assert/strict'
import { createServer as createHTTPServer } from 'node:http'
import { once } from 'node:events'
import { fileURLToPath } from 'node:url'
import { createServer } from 'vite'

// Exercise the real Vite config on disposable ports, without touching a running
// Studio/API or requiring credentials. JSON/SSE must never become SPA HTML.
const requests: { url: string; host: string | undefined; body: string }[] = []
let releaseStream: (() => void) | undefined
const upstream = createHTTPServer(async (req, res) => {
  let body = ''
  for await (const chunk of req) body += chunk
  requests.push({ url: req.url!, host: req.headers.host, body })
  if (req.url === '/mcp' && req.method === 'POST') {
    res.writeHead(200, { 'Content-Type': 'text/event-stream' })
    res.write('event: message\ndata: {"phase":"first"}\n\n')
    await new Promise<void>(resolve => { releaseStream = resolve })
    res.end('event: message\ndata: {"phase":"last"}\n\n')
  } else {
    res.writeHead(req.url === '/mcp?unauthorized=1' ? 401 : 200, {
      'Content-Type': 'application/json',
      'WWW-Authenticate': 'Bearer resource_metadata="http://localhost:5273/.well-known/oauth-protected-resource/mcp"',
    })
    res.end(JSON.stringify({ path: req.url, method: req.method, body }))
  }
})
upstream.listen(0, '127.0.0.1')
await once(upstream, 'listening')
const address = upstream.address()
assert(address && typeof address !== 'string')
const previousTarget = process.env.VITE_API_PROXY_TARGET
process.env.VITE_API_PROXY_TARGET = `http://127.0.0.1:${address.port}`
let vite: Awaited<ReturnType<typeof createServer>> | undefined
try {
  vite = await createServer({
    configFile: fileURLToPath(new URL('../vite.config.ts', import.meta.url)),
    root: fileURLToPath(new URL('..', import.meta.url)),
    server: { host: '127.0.0.1', port: 0, open: false, watch: null },
    logLevel: 'error',
  })
  await vite.listen()
  const viteAddress = vite.httpServer!.address()
  assert(viteAddress && typeof viteAddress !== 'string')
  const origin = `http://127.0.0.1:${viteAddress.port}`
  const routes = ['/mcp', '/.well-known/oauth-authorization-server', '/.well-known/oauth-authorization-server/mcp', '/.well-known/oauth-protected-resource', '/.well-known/oauth-protected-resource/mcp', '/oauth/authorize?client_id=test&resource=http%3A%2F%2Flocalhost%3A5273%2Fmcp', '/oauth/token', '/oauth/register', '/oauth/revoke', '/api/v1/oauth/consent?request_id=keep', '/docs']
  for (const path of routes) {
    const method = /\/(token|register|revoke)$/.test(path) ? 'POST' : 'GET'
    const body = method === 'POST' ? 'client_id=test&resource=http%3A%2F%2Flocalhost%3A5273%2Fmcp' : undefined
    const response = await fetch(origin + path, { method, body, headers: { Accept: 'application/json', ...(body ? { 'Content-Type': 'application/x-www-form-urlencoded' } : {}) } })
    assert.equal(response.status, 200, path)
    assert.match(response.headers.get('content-type')!, /application\/json/, path)
    assert.deepEqual(await response.json(), { path, method, body: body ?? '' })
    assert.equal(requests[requests.length - 1]!.host, new URL(origin).host, 'preserve public Host')
  }
  const denied = await fetch(origin + '/mcp?unauthorized=1')
  assert.equal(denied.status, 401)
  assert.match(denied.headers.get('www-authenticate')!, /localhost:5273/)
  await denied.text()

  const stream = await fetch(origin + '/mcp', { method: 'POST', headers: { Accept: 'application/json, text/event-stream', 'Content-Type': 'application/json' }, body: '{"jsonrpc":"2.0","id":1,"method":"initialize"}', signal: AbortSignal.timeout(10000) })
  assert.match(stream.headers.get('content-type')!, /text\/event-stream/)
  const reader = stream.body!.getReader()
  const first = await reader.read()
  assert.equal(first.done, false)
  assert.match(new TextDecoder().decode(first.value), /"phase":"first"/)
  assert.match(requests[requests.length - 1]!.body, /"method":"initialize"/)
  releaseStream!()
  let rest = ''
  for (;;) { const part = await reader.read(); if (part.done) break; rest += new TextDecoder().decode(part.value) }
  assert.match(rest, /"phase":"last"/)

  for (const path of ['/?oauth_request=req_123#/oauth-consent', '/?oauth_request=req_123#/login?returnTo=oauth-consent', '/oauth-consent', '/mcp-extra', '/mcp/extra', '/oauth/token-extra', '/.well-known/oauth-protected-resource/mcp/extra', '/.well-known/oauth-authorization-server-extra']) {
    const count = requests.length
    const response = await fetch(origin + path, { headers: { Accept: 'text/html' } })
    assert.match(response.headers.get('content-type')!, /text\/html/, path)
    assert.match(await response.text(), /<html/, path)
    assert.equal(requests.length, count, `SPA route must not be proxied: ${path}`)
  }
  console.log('OK: real Vite canonical MCP/OAuth proxy, streaming and SPA boundaries')
} finally {
  releaseStream?.()
  await vite?.close()
  upstream.closeAllConnections()
  await new Promise<void>(resolve => upstream.close(() => resolve()))
  if (previousTarget === undefined) delete process.env.VITE_API_PROXY_TARGET
  else process.env.VITE_API_PROXY_TARGET = previousTarget
}
