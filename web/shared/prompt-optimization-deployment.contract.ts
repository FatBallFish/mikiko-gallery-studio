// @ts-ignore contract scripts run in tsx/node; the browser app tsconfigs do not include node types.
import { readFileSync } from 'node:fs'

const root = new URL('../../', import.meta.url)
const read = (path: string) => readFileSync(new URL(path, root), 'utf8')
const key = 'PROMPT_OPTIMIZATION_QUOTE_SIGNING_KEY'

const envExample = read('.env.example')
if (!envExample.includes(`${key}=local-dev-prompt-optimization-quote-signing-key`)) {
  throw new Error('root environment template must document the prompt optimization quote signing key')
}

for (const path of ['deployments/docker-compose/docker-compose.dev.yml', 'deployments/docker-compose/docker-compose.e2e.yml']) {
  const source = read(path)
  if ((source.match(new RegExp(key, 'g')) ?? []).length < 2) {
    throw new Error(`${path} must configure the quote signing key for API and worker runtimes`)
  }
}

const prodCompose = read('deployments/docker-compose/docker-compose.prod.yml')
const requiredExpression = `${key}: \${${key}:?set ${key} in the env file}`
if ((prodCompose.split(requiredExpression).length - 1) < 2) {
  throw new Error('production compose must require the quote signing key for API and worker runtimes')
}

const prodEnv = read('deployments/docker-compose/.env.prod.example')
if (!prodEnv.includes(`${key}=change_me_with_openssl_rand_hex_32`)) {
  throw new Error('production env template must include a strong quote signing key placeholder')
}

const prepare = read('deployments/docker-compose/prepare.sh')
if (!prepare.includes(`replace_env ${key} "$(generate_secret)"`)) {
  throw new Error('deployment prepare script must generate the quote signing key')
}
