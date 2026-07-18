import assert from 'node:assert/strict'
import { mkdtempSync, mkdirSync, rmSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { spawnSync } from 'node:child_process'
import test from 'node:test'

const contractScript = fileURLToPath(new URL('./landing-build.mjs', import.meta.url))

function createBuildFixture({ includeManifest = true, secondaryCode = 'secondary shell' } = {}) {
  const root = mkdtempSync(join(tmpdir(), 'pic-gallery-landing-build-'))
  const assetsDir = join(root, 'web/user/dist/assets')
  const manifestDir = join(root, 'web/user/dist/.vite')
  mkdirSync(assetsDir, { recursive: true })
  mkdirSync(manifestDir, { recursive: true })

  writeFileSync(join(assetsDir, 'app-shell.js'), 'authenticated entry')
  writeFileSync(join(assetsDir, 'shared-runtime.js'), 'authenticated shell')
  writeFileSync(join(assetsDir, 'secondary-entry.js'), 'secondary entry')
  writeFileSync(join(assetsDir, 'secondary-bridge.js'), 'secondary bridge')
  writeFileSync(join(assetsDir, 'deep-shared.js'), secondaryCode)
  writeFileSync(join(assetsDir, 'public-route.js'), 'public route')
  writeFileSync(join(assetsDir, 'landing-motion.js'), 'ScrollTrigger landing motion')

  if (includeManifest) {
    writeFileSync(
      join(manifestDir, 'manifest.json'),
      JSON.stringify({
        'src/main.tsx': {
          file: 'assets/app-shell.js',
          isEntry: true,
          imports: ['_shared-runtime.js'],
          dynamicImports: ['src/pages/LandingPage.tsx'],
        },
        '_shared-runtime.js': {
          file: 'assets/shared-runtime.js',
        },
        'src/secondary.tsx': {
          file: 'assets/secondary-entry.js',
          isEntry: true,
          imports: ['_secondary-bridge.js'],
        },
        '_secondary-bridge.js': {
          file: 'assets/secondary-bridge.js',
          imports: ['_deep-shared.js'],
        },
        '_deep-shared.js': {
          file: 'assets/deep-shared.js',
        },
        'src/pages/LandingPage.tsx': {
          file: 'assets/public-route.js',
          isDynamicEntry: true,
          imports: ['_landing-motion.js'],
        },
        '_landing-motion.js': {
          file: 'assets/landing-motion.js',
        },
      }),
    )
  }

  return root
}

function runContract(root) {
  return spawnSync(process.execPath, [contractScript], {
    encoding: 'utf8',
    env: { ...process.env, PIC_GALLERY_REPOSITORY_ROOT: root },
  })
}

test('accepts ScrollTrigger only in the manifest landing chunk', (t) => {
  const root = createBuildFixture()
  t.after(() => rmSync(root, { recursive: true, force: true }))

  const result = runContract(root)

  assert.equal(result.status, 0, result.stderr)
})

test('rejects ScrollTrigger in a nested dependency of a second entry', (t) => {
  const root = createBuildFixture({ secondaryCode: 'ScrollTrigger leaked runtime' })
  t.after(() => rmSync(root, { recursive: true, force: true }))

  const result = runContract(root)

  assert.notEqual(result.status, 0)
  assert.match(result.stderr, /ScrollTrigger leaked into authenticated entry graph: assets\/deep-shared\.js/)
})

test('rejects builds without the Vite manifest', (t) => {
  const root = createBuildFixture({ includeManifest: false })
  t.after(() => rmSync(root, { recursive: true, force: true }))

  const result = runContract(root)

  assert.notEqual(result.status, 0)
  assert.match(result.stderr, /user-web build manifest is missing/)
})
