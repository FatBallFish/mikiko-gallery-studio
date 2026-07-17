import { readFileSync } from 'node:fs'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const repositoryRoot = process.env.PIC_GALLERY_REPOSITORY_ROOT
  ? resolve(process.env.PIC_GALLERY_REPOSITORY_ROOT)
  : resolve(dirname(fileURLToPath(import.meta.url)), '../../..')
const packageJSON = JSON.parse(readFileSync(join(repositoryRoot, 'web/user/package.json'), 'utf8'))
const dockerfile = readFileSync(join(repositoryRoot, 'Dockerfile.user-web'), 'utf8')
const errors = []

const expectedBuildCommand = 'vite build && node ../../scripts/workflow/contracts/landing-build.mjs'
const buildScript = packageJSON.scripts?.build ?? ''
if (buildScript !== expectedBuildCommand) {
  errors.push(`user-web build must equal ${expectedBuildCommand}; received: ${buildScript || '<missing>'}`)
}

const buildStage = dockerfile
  .split(/^FROM\s+/im)
  .find((stage) => /\s+AS\s+build\s*$/im.test(stage.split('\n', 1)[0] ?? ''))
if (!buildStage) {
  errors.push('Dockerfile.user-web is missing the user-web build stage')
}

if (buildStage) {
  const buildLines = buildStage.split('\n').map((line) => line.trim())
  const contractCopy = 'COPY scripts/workflow/contracts/landing-build.mjs ./scripts/workflow/contracts/landing-build.mjs'
  const buildRun = 'RUN npm run build'
  const contractCopyIndex = buildLines.indexOf(contractCopy)
  const buildRunIndex = buildLines.indexOf(buildRun)

  if (contractCopyIndex === -1) {
    errors.push('user-web Docker build stage is missing the exact landing-build.mjs COPY')
  }
  if (buildRunIndex === -1) {
    errors.push('user-web Docker build stage is missing the exact RUN npm run build command')
  }
  if (contractCopyIndex !== -1 && buildRunIndex !== -1 && contractCopyIndex >= buildRunIndex) {
    errors.push('user-web Docker build stage contract COPY must precede RUN npm run build')
  }
}

if (errors.length > 0) {
  throw new Error(`landing build wiring contract failed:\n- ${errors.join('\n- ')}`)
}

console.log('OK: landing build wiring contract passed')
