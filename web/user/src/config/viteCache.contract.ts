import assert from 'node:assert/strict'
import { fileURLToPath } from 'node:url'
import { resolve } from 'node:path'
import { resolveConfig } from 'vite'

async function main() {
  const root = fileURLToPath(new URL('../..', import.meta.url))
  const originalCwd = process.cwd()
  process.chdir(root)
  try {
    const configFile = resolve(root, 'vite.config.ts')
    const inspect = (port?: number, cacheDir?: string, command: 'serve' | 'build' = 'serve', strictPort?: boolean) =>
      resolveConfig({ root, configFile, server: { ...(port === undefined ? {} : { port }), ...(strictPort === undefined ? {} : { strictPort }) }, ...(cacheDir === undefined ? {} : { cacheDir }) }, command)
    const first = await inspect(5193, undefined, 'serve', true)
    const canonical = await inspect(5273, undefined, 'serve', true)
    const backup = await inspect(5293, undefined, 'serve', true)
    const defaultServer = await inspect()
    const explicit = await inspect(5193, 'explicit-cache')
    const explicitOtherPort = await inspect(5273, 'explicit-cache')
    const build = await inspect(undefined, undefined, 'build')
    for (const [label, config] of [['5193', first], ['5273', canonical], ['5293', backup], ['default', defaultServer], ['explicit', explicit], ['build', build]] as const) {
      console.log(`resolved ${label}: port=${config.server.port} cacheDir=${config.cacheDir}`)
    }
    assert.notEqual(first.cacheDir, canonical.cacheDir, 'CLI ports 5193 and 5273 must not share optimizer output')
    assert.equal(first.cacheDir, resolve(root, 'node_modules/.vite/dev-5193'))
    assert.equal(canonical.cacheDir, resolve(root, 'node_modules/.vite/dev-5273'))
    assert.equal(backup.cacheDir, resolve(root, 'node_modules/.vite/dev-5293'))
    assert.equal(defaultServer.cacheDir, resolve(root, `node_modules/.vite/dev-5173-pid-${process.pid}`))
    assert.equal(defaultServer.server.strictPort, false, 'Preserve Vite automatic port fallback')
    const nonStrict = await inspect(5193, undefined, 'serve', false)
    const automaticPort = await inspect(0)
    const strictAutomaticPort = await inspect(0, undefined, 'serve', true)
    for (const [label, config] of [['non-strict', nonStrict], ['port-zero', automaticPort], ['strict-port-zero', strictAutomaticPort]] as const) {
      console.log(`resolved ${label}: port=${config.server.port} strictPort=${config.server.strictPort} cacheDir=${config.cacheDir}`)
    }
    assert.equal(nonStrict.cacheDir, resolve(root, `node_modules/.vite/dev-5193-pid-${process.pid}`))
    assert.equal(nonStrict.server.strictPort, false)
    assert.notEqual(nonStrict.cacheDir, first.cacheDir)
    assert.equal(automaticPort.cacheDir, resolve(root, `node_modules/.vite/dev-0-pid-${process.pid}`))
    assert.equal(strictAutomaticPort.cacheDir, automaticPort.cacheDir, 'Port zero always needs process isolation')
    assert.equal((await inspect(0, 'explicit-zero-cache', 'serve', false)).cacheDir, resolve(root, 'explicit-zero-cache'))
    assert.equal(explicit.cacheDir, resolve(root, 'explicit-cache'))
    assert.equal(explicitOtherPort.cacheDir, explicit.cacheDir, 'Explicit cacheDir remains caller controlled')
    assert.equal(build.cacheDir, resolve(root, 'node_modules/.vite'), 'Build must retain Vite default')
    assert.equal((await inspect(5193, undefined, 'serve', true)).cacheDir, first.cacheDir, 'Repeated strict resolution must use a stable cache path')
    assert.equal((await inspect(5193, 'explicit-build-cache', 'build')).cacheDir, resolve(root, 'explicit-build-cache'))
  } finally {
    process.chdir(originalCwd)
  }
}
main().catch(error => { console.error(error); process.exitCode = 1 })
