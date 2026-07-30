// @ts-ignore contract scripts run in tsx/node; the browser app tsconfigs do not include node types.
import { readFileSync } from 'node:fs'

const root = new URL('../../', import.meta.url)
const read = (path: string) => readFileSync(new URL(path, root), 'utf8')
const key = 'PROMPT_OPTIMIZATION_QUOTE_SIGNING_KEY'

const envExample = read('config/runtime.env.example')
if (!envExample.includes(`${key}=`) || !envExample.includes('# [中文]') || !envExample.includes('# [English]')) {
  throw new Error('runtime environment template must document the prompt optimization quote signing key bilingually')
}
if (envExample.includes(`${key}=local-dev-prompt-optimization-quote-signing-key`)) {
  throw new Error('runtime environment template must not contain a usable prompt optimization quote signing key')
}

const localCompose = read('deployments/docker-compose/docker-compose.local.yml')
const localRuntime = read('config/runtime.local.env.example')
if ((localCompose.split('${PIC_GALLERY_LOCAL_CONFIG_DIR:-../../config}:/run/pic-gallery-config:ro').length - 1) < 2 || !localRuntime.includes(`${key}=local-dev-prompt-optimization-quote-signing-key`)) {
  throw new Error('local API and worker must load the quote signing key from the shared local runtime env')
}

const prodCompose = read('deployments/docker-compose/docker-compose.prod.yml')
if (prodCompose.includes(key) || (prodCompose.split('target: /app/config').length - 1) < 2) {
  throw new Error('production API and worker must load the quote signing key from mounted config/runtime.env')
}

const prodEnv = read('deployments/docker-compose/.env.prod.example')
if (prodEnv.includes(`${key}=`)) {
  throw new Error('legacy production env template must not duplicate runtime secrets')
}

const prepare = read('deployments/docker-compose/prepare.sh')
if (prepare.includes(key) || !prepare.includes('scripts/install.sh')) {
  throw new Error('deployment prepare script must delegate quote signing key generation to mgsctl')
}

const mgsctlRuntime = read('internal/mgsctl/runtime.go')
if (!mgsctlRuntime.includes(`"${key}":`) || !mgsctlRuntime.includes('derivedSecret(root, "prompt-quote-signing")')) {
  throw new Error('mgsctl must generate a purpose-separated prompt optimization quote signing key')
}
