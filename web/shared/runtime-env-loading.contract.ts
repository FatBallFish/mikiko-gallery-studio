// @ts-ignore contract scripts run in tsx/node; the browser app tsconfigs do not include node types.
import { readFileSync } from 'node:fs'
// @ts-ignore contract scripts run in tsx/node; the browser app tsconfigs do not include node types.
import { spawnSync } from 'node:child_process'

const root = new URL('../../', import.meta.url)
const read = (path: string) => readFileSync(new URL(path, root), 'utf8')

const gitignore = read('.gitignore')
if (!gitignore.split(/\r?\n/).includes('/config/runtime.env')) {
  throw new Error('.gitignore must explicitly ignore only the generated /config/runtime.env file')
}

const git = (args: string[]) => spawnSync('git', args, { cwd: root, encoding: 'utf8' })
if (git(['check-ignore', '-q', 'config/runtime.env']).status !== 0) {
  throw new Error('config/runtime.env must be ignored by git')
}
if (git(['check-ignore', '-q', 'config/runtime.env.example']).status === 0) {
  throw new Error('config/runtime.env.example must remain trackable')
}

for (const path of ['README.md', 'README.zh-CN.md']) {
  const source = read(path)
  const installExample =
    'powershell -ExecutionPolicy Bypass -File scripts/service/manage.ps1 install -Components "api,worker" -EnvFile "config/runtime.env"'
  if (!source.includes(installExample)) {
    throw new Error(`${path} must install Windows services with config/runtime.env`)
  }
  if (source.includes('-EnvFile ".env"')) {
    throw new Error(`${path} must not reference the retired root .env runtime path`)
  }
}

const smoke = read('scripts/test/api_contract_smoke.sh')
for (const required of [
  'SETUP_COMPLETED=true',
  'POSTGRES_CONTAINER=',
  'REDIS_CONTAINER=',
  'postgres:16-alpine',
  'redis:7-alpine',
  'docker rm -f "$POSTGRES_CONTAINER" "$REDIS_CONTAINER"',
]) {
  if (!smoke.includes(required)) {
    throw new Error(`API contract smoke must include isolated runtime prerequisite: ${required}`)
  }
}
for (const retired of ['DATABASE_URL=file:', 'import sqlite3', 'DB_PATH=']) {
  if (smoke.includes(retired)) {
    throw new Error(`API contract smoke must not retain SQLite runtime setup: ${retired}`)
  }
}
