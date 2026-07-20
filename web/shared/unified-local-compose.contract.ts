// @ts-ignore contract scripts run in tsx/node; browser app tsconfigs do not include node types.
import { existsSync, readFileSync } from 'node:fs'

const root = new URL('../../', import.meta.url)
const resolve = (path: string) => new URL(path, root)
const read = (path: string) => readFileSync(resolve(path), 'utf8')
const requireSource = (path: string) => {
  if (!existsSync(resolve(path))) throw new Error(`required unified local environment file is missing: ${path}`)
  return read(path)
}

const localComposePath = 'deployments/docker-compose/docker-compose.local.yml'
const oldComposePaths = [
  'deployments/docker-compose/docker-compose.dev.yml',
  'deployments/docker-compose/docker-compose.e2e.yml',
  'deployments/docker-compose/docker-compose-middileware.yml',
]
const localCompose = requireSource(localComposePath)

if (!localCompose.includes('name: pic-gallery-local')) {
  throw new Error('the only local Compose project must be named pic-gallery-local')
}
for (const path of oldComposePaths) {
  if (existsSync(resolve(path))) throw new Error(`obsolete local Compose file must be removed: ${path}`)
}
for (const service of ['postgres:', 'redis:', 'minio:', 'mailpit:', 'api:', 'worker:', 'user-web:', 'docs-web:', 'admin-web:', 'nginx:']) {
  if (!localCompose.includes(`  ${service}`)) throw new Error(`unified local Compose is missing ${service}`)
}

const devUp = read('scripts/dev/up.sh')
const devDown = read('scripts/dev/down.sh')
const e2eRunner = read('scripts/e2e/run-docker-e2e.sh')
for (const [label, source] of [['dev up', devUp], ['dev down', devDown], ['E2E runner', e2eRunner]] as const) {
  if (!source.includes('docker-compose.local.yml')) throw new Error(`${label} must target the unified local Compose file`)
}
if (e2eRunner.includes('down -v')) throw new Error('E2E must never delete the shared local volumes')
for (const required of [
  'BASE_URL:-http://127.0.0.1:8088',
  'USER_WEB_URL:-http://127.0.0.1:8088',
  'ADMIN_WEB_URL:-http://127.0.0.1:8088/admin',
  'snapshot',
  'restore',
  'trap',
]) {
  if (!e2eRunner.includes(required)) throw new Error(`shared-environment E2E runner is missing ${required}`)
}

const stateHelper = requireSource('scripts/e2e/local-state.sh')
for (const required of ['pg_dump', 'pg_restore', 'minio-data', 'shared-storage', 'FLUSHDB']) {
  if (!stateHelper.includes(required)) throw new Error(`local state helper is missing ${required}`)
}
if (/docker volume rm|down\s+-v/.test(stateHelper)) throw new Error('local state helper must not delete persistent local volumes')

const migration = requireSource('scripts/dev/migrate-unified-local.sh')
for (const required of ['pg_dump', 'pg_restore --list', 'database-manifest', 'minio-manifest', 'shared-storage-manifest', '--execute']) {
  if (!migration.includes(required)) throw new Error(`migration safety flow is missing ${required}`)
}
if (!migration.includes('old volumes retained')) throw new Error('migration must retain old volumes until separate final cleanup')

if (!devDown.includes('--confirm-destroy-local-data')) {
  throw new Error('local volume deletion must require an explicit destructive confirmation token')
}
