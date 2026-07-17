import assert from 'node:assert/strict'
import { mkdtempSync, mkdirSync, rmSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { spawnSync } from 'node:child_process'
import test from 'node:test'

const contractScript = fileURLToPath(new URL('./landing-build-wiring.mjs', import.meta.url))
const expectedBuild = 'vite build && node ../../scripts/workflow/contracts/landing-build.mjs'
const copyLine = 'COPY scripts/workflow/contracts/landing-build.mjs ./scripts/workflow/contracts/landing-build.mjs'

function createWiringFixture({
  build = expectedBuild,
  contractCopy = copyLine,
  buildRun = 'RUN npm run build',
  fromInstruction = 'FROM',
  copyAfterBuild = false,
} = {}) {
  const root = mkdtempSync(join(tmpdir(), 'pic-gallery-landing-wiring-'))
  mkdirSync(join(root, 'web/user'), { recursive: true })
  writeFileSync(join(root, 'web/user/package.json'), JSON.stringify({ scripts: { build } }))

  const preamble = '# syntax=docker/dockerfile:1.7'
  const buildLines = copyAfterBuild
    ? [preamble, `${fromInstruction} node:22-alpine AS build`, buildRun, contractCopy]
    : [preamble, `${fromInstruction} node:22-alpine AS build`, contractCopy, buildRun]
  writeFileSync(join(root, 'Dockerfile.user-web'), [...buildLines, 'FROM nginx:1.27-alpine'].join('\n'))
  return root
}

function runContract(root) {
  return spawnSync(process.execPath, [contractScript], {
    encoding: 'utf8',
    env: { ...process.env, PIC_GALLERY_REPOSITORY_ROOT: root },
  })
}

test('accepts the exact build command and pre-build contract copy', (t) => {
  const root = createWiringFixture()
  t.after(() => rmSync(root, { recursive: true, force: true }))

  const result = runContract(root)

  assert.equal(result.status, 0, result.stderr)
})

test('accepts case-insensitive Docker stage instructions', (t) => {
  const root = createWiringFixture({ fromInstruction: 'from' })
  t.after(() => rmSync(root, { recursive: true, force: true }))

  const result = runContract(root)

  assert.equal(result.status, 0, result.stderr)
})

test('rejects build commands that only contain the contract command', (t) => {
  const root = createWiringFixture({ build: `vite build --mode production && ${expectedBuild.split(' && ')[1]}` })
  t.after(() => rmSync(root, { recursive: true, force: true }))

  const result = runContract(root)

  assert.notEqual(result.status, 0)
  assert.match(result.stderr, /user-web build must equal/)
})

test('rejects copying the contract after the Docker build command', (t) => {
  const root = createWiringFixture({ copyAfterBuild: true })
  t.after(() => rmSync(root, { recursive: true, force: true }))

  const result = runContract(root)

  assert.notEqual(result.status, 0)
  assert.match(result.stderr, /contract COPY must precede RUN npm run build/)
})

test('rejects a non-exact Docker contract copy', (t) => {
  const root = createWiringFixture({ contractCopy: `${copyLine} /tmp/landing-build.mjs` })
  t.after(() => rmSync(root, { recursive: true, force: true }))

  const result = runContract(root)

  assert.notEqual(result.status, 0)
  assert.match(result.stderr, /missing the exact landing-build\.mjs COPY/)
})

test('rejects a non-exact Docker build command', (t) => {
  const root = createWiringFixture({ buildRun: 'RUN npm run build -- --mode production' })
  t.after(() => rmSync(root, { recursive: true, force: true }))

  const result = runContract(root)

  assert.notEqual(result.status, 0)
  assert.match(result.stderr, /missing the exact RUN npm run build command/)
})
