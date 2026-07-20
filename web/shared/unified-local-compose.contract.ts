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
if (!localCompose.includes('CORS_ALLOWED_ORIGINS: http://localhost:${DEV_NGINX_PORT:-8088},http://127.0.0.1:${DEV_NGINX_PORT:-8088}')) {
  throw new Error('unified local API must allow both localhost aliases for the shared nginx origin')
}

const devUp = read('scripts/dev/up.sh')
const devDown = read('scripts/dev/down.sh')
const e2eRunner = read('scripts/e2e/run-docker-e2e.sh')
for (const [label, source] of [['dev up', devUp], ['dev down', devDown], ['E2E runner', e2eRunner]] as const) {
  if (!source.includes('docker-compose.local.yml')) throw new Error(`${label} must target the unified local Compose file`)
}
if (e2eRunner.includes('down -v')) throw new Error('E2E must never delete the shared local volumes')
for (const required of [
  'DEV_NGINX_PORT:-8088',
  'LOCAL_BASE_URL="http://127.0.0.1:${DEV_NGINX_PORT}"',
  'assert_local_url BASE_URL "$BASE_URL" "$LOCAL_BASE_URL"',
  'assert_local_url USER_WEB_URL "$USER_WEB_URL" "$LOCAL_BASE_URL"',
  'assert_local_url ADMIN_WEB_URL "$ADMIN_WEB_URL" "$LOCAL_BASE_URL/admin"',
  'acquire_e2e_lock',
  'release_e2e_lock',
  'readlink "$LOCK_FILE"',
  'kill -0 "$owner_pid"',
  'if stop_writers; then',
  'snapshot',
  'restore',
  'trap',
]) {
  if (!e2eRunner.includes(required)) throw new Error(`shared-environment E2E runner is missing ${required}`)
}
if (e2eRunner.includes('PIC_GALLERY_E2E_LOCKED')) {
  throw new Error('shared E2E lock must not be bypassable through an environment sentinel')
}
if (!e2eRunner.includes('local-runner-state.sh')) {
  throw new Error('shared E2E runner must load the tested writer state helper')
}
const runnerState = requireSource('scripts/e2e/local-runner-state.sh')
for (const required of ['stop_writers', 'writers_are_stopped', 'ps -a -q "$service"', 'failed to inspect writer state']) {
  if (!runnerState.includes(required)) throw new Error(`shared E2E writer state helper is missing ${required}`)
}

const dockerE2E = read('scripts/e2e/docker-e2e.mjs')
if (!dockerE2E.includes('new URL(url).origin')) {
  throw new Error('Docker E2E must normalize frontend URLs to browser origins before CORS checks')
}

const stateHelper = requireSource('scripts/e2e/local-state.sh')
for (const required of ['pg_dump', 'pg_restore', 'minio-data', 'shared-storage', 'FLUSHDB']) {
  if (!stateHelper.includes(required)) throw new Error(`local state helper is missing ${required}`)
}
if (/docker volume rm|down\s+-v/.test(stateHelper)) throw new Error('local state helper must not delete persistent local volumes')

const migration = requireSource('scripts/dev/migrate-unified-local.sh')
for (const required of ['pg_dump', 'pg_restore --list', 'database-manifest', 'minio-manifest', 'shared-storage-manifest', '--execute', 'stop_old_dev_writers', 'old writer is still running', 'SOURCE_STARTED_BY_MIGRATION', 'DEV_NGINX_PORT:-8088', 'docker volume rm pic-gallery-dev_postgres-data']) {
  if (!migration.includes(required)) throw new Error(`migration safety flow is missing ${required}`)
}
if (!migration.includes('old volumes retained')) throw new Error('migration must retain old volumes until separate final cleanup')
if (migration.indexOf('stop_old_dev_writers') > migration.indexOf('"$STATE_HELPER" snapshot')) {
  throw new Error('migration must stop old dev writers before taking the source snapshot')
}

if (!devDown.includes('--confirm-destroy-local-data')) {
  throw new Error('local volume deletion must require an explicit destructive confirmation token')
}

const verifyScript = read('scripts/workflow/verify.sh')
if (!verifyScript.includes('run-docker-e2e.contract.sh')) {
  throw new Error('repository verification must run the shared local E2E runner safety contract')
}
