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
const dockerignore = requireSource('.dockerignore')
const gitignore = requireSource('.gitignore')
const localRuntime = requireSource('config/runtime.local.env.example')
const localInstallState = JSON.parse(requireSource('config/install-state.local.json.example'))

if (!localCompose.includes('name: pic-gallery-local')) {
  throw new Error('the only local Compose project must be named pic-gallery-local')
}
if (!gitignore.split(/\r?\n/).includes('.secrets/')) throw new Error('Git must ignore the local image-generation credential directory')
for (const path of oldComposePaths) {
  if (existsSync(resolve(path))) throw new Error(`obsolete local Compose file must be removed: ${path}`)
}
for (const secretPath of [
  '.worktrees/',
  '.secrets/',
  'tmp/imagegen/',
  'config/runtime.env',
  'config/.runtime.env.tmp-*',
  'config/install-state.json',
  'config/.install-state.json.tmp-*',
  'config/install-state.json.lock',
]) {
  if (!dockerignore.split(/\r?\n/).includes(secretPath)) throw new Error(`Docker build context must exclude ${secretPath}`)
}
for (const service of ['postgres:', 'redis:', 'minio:', 'mailpit:', 'bootstrap-local:', 'api:', 'worker:', 'user-web:', 'docs-web:', 'admin-web:', 'nginx:']) {
  if (!localCompose.includes(`  ${service}`)) throw new Error(`unified local Compose is missing ${service}`)
}
if (localCompose.includes('  migrate:')) throw new Error('local migration must run inside the guarded bootstrap command')
if (existsSync(resolve('deployments/docker-compose/bootstrap-local.sql'))) {
  throw new Error('static local setup SQL must be removed')
}
if (!localRuntime.includes('CORS_ALLOWED_ORIGINS=http://localhost:8088,http://127.0.0.1:8088')) {
  throw new Error('unified local runtime must allow both fixed shared nginx origins')
}
if (!localRuntime.includes('PIC_GALLERY_DOCS_URL=/developer-docs/') || !localRuntime.includes('PIC_GALLERY_DOCS_PROBE_URL=http://nginx/developer-docs/')) {
  throw new Error('unified local runtime must separate the browser docs path from the API-reachable Compose probe target')
}
if (
  localInstallState.installation_id !== 'pic-gallery-local' ||
  localInstallState.phase !== 'completed' ||
  localInstallState.ever_completed !== true ||
  localInstallState.commit?.operation_id !== 'local-bootstrap' ||
  localInstallState.commit?.installation_id !== 'pic-gallery-local' ||
  localInstallState.commit?.config_revision !== 1
) {
  throw new Error('unified local install-state must match the completed local runtime identity')
}
for (const forbidden of ['${POSTGRES_DB', '${POSTGRES_USER', '${DEV_NGINX_PORT']) {
  if (localCompose.includes(forbidden)) throw new Error(`local Compose override conflicts with runtime.env: ${forbidden}`)
}
for (const published of ['127.0.0.1:8088:80', '127.0.0.1:${MINIO_API_PORT:-9000}:9000', '127.0.0.1:${MINIO_CONSOLE_PORT:-9001}:9001', '127.0.0.1:${MAILPIT_SMTP_PORT:-1025}:1025', '127.0.0.1:${MAILPIT_UI_PORT:-8025}:8025']) {
  if (!localCompose.includes(published)) throw new Error(`local service must bind only to loopback: ${published}`)
}

const devUp = read('scripts/dev/up.sh')
const devDown = read('scripts/dev/down.sh')
const e2eRunner = read('scripts/e2e/run-docker-e2e.sh')
const prepareRuntime = requireSource('scripts/dev/prepare-local-runtime.sh')
const bootstrapLifecycle = requireSource('scripts/dev/test-local-bootstrap.sh')
const localEntrypoint = requireSource('scripts/docker/local-runtime-entrypoint.sh')
const apiDockerfile = read('Dockerfile.api')
const workerDockerfile = read('Dockerfile.worker')
for (const [label, source] of [['dev up', devUp], ['dev down', devDown], ['E2E runner', e2eRunner]] as const) {
  if (!source.includes('docker-compose.local.yml')) throw new Error(`${label} must target the unified local Compose file`)
}
for (const required of [
  'entrypoint: ["mikiko-gallery-studio-local-bootstrap"]',
  '${PIC_GALLERY_LOCAL_CONFIG_DIR:-../../config}:/app/config',
  '${PIC_GALLERY_LOCAL_CONFIG_DIR:-../../config}:/run/pic-gallery-config:ro',
  'entrypoint: ["/usr/local/bin/mikiko-gallery-studio-local-entrypoint"]',
  'condition: service_completed_successfully',
]) {
  if (!localCompose.includes(required)) throw new Error(`unified local Compose runtime bootstrap is missing ${required}`)
}
if ((localCompose.match(/\$\{PIC_GALLERY_LOCAL_CONFIG_DIR:-\.\.\/\.\.\/config\}:\/run\/pic-gallery-config:ro/g) || []).length < 2) {
  throw new Error('API and Worker must read the same local config directory')
}
for (const required of ['/out/mikiko-gallery-studio-local-bootstrap ./cmd/local-bootstrap', '/usr/local/bin/mikiko-gallery-studio-local-bootstrap', 'su-exec', 'local-runtime-entrypoint.sh']) {
  if (!apiDockerfile.includes(required)) throw new Error(`the local API image is missing ${required}`)
}
for (const required of ['su-exec', 'local-runtime-entrypoint.sh']) {
  if (!workerDockerfile.includes(required)) throw new Error(`the local Worker image is missing ${required}`)
}
for (const required of ['runtime.env', 'install-state.json', 'install -m 600', 'su-exec picgallery']) {
  if (!localEntrypoint.includes(required)) throw new Error(`local runtime entrypoint is missing ${required}`)
}
for (const required of ['runtime.local.env.example', 'install-state.local.json.example', 'PIC_GALLERY_LOCAL_CONFIG_DIR', 'runtime.env', 'install-state.json', 'install -m 600']) {
  if (!prepareRuntime.includes(required)) throw new Error(`local runtime preparation is missing ${required}`)
}
for (const required of ['PIC_GALLERY_LOCAL_CONFIG_DIR', 'TEST_IMAGE_TAG', 'local-bootstrap-lifecycle-', 'admin@example.com', 'alternate-root@example.com', 'mikiko-gallery-studio-db-migrate', 'INSERT INTO admin_users', '--force-recreate bootstrap-local api', 'service_completed_successfully', 'stat -f', 'down -v']) {
  if (!bootstrapLifecycle.includes(required)) throw new Error(`local bootstrap lifecycle test is missing ${required}`)
}
if (bootstrapLifecycle.includes('pic-gallery-local_postgres-data') || bootstrapLifecycle.includes('pic-gallery-local_shared-storage')) {
  throw new Error('isolated bootstrap lifecycle test must never address shared local volumes')
}
for (const [label, source] of [['dev up', devUp], ['E2E runner', e2eRunner]] as const) {
  if (!source.includes('prepare-local-runtime.sh')) throw new Error(`${label} must prepare the shared local runtime files before Compose starts`)
}
if (e2eRunner.includes('DEV_NGINX_PORT')) throw new Error('shared E2E must use the fixed runtime/nginx port')
if (e2eRunner.includes('down -v')) throw new Error('E2E must never delete the shared local volumes')
for (const required of [
  'LOCAL_BASE_URL="http://127.0.0.1:8088"',
  'assert_local_url BASE_URL "$BASE_URL" "$LOCAL_BASE_URL"',
  'assert_local_url USER_WEB_URL "$USER_WEB_URL" "$LOCAL_BASE_URL"',
  'assert_local_url ADMIN_WEB_URL "$ADMIN_WEB_URL" "$LOCAL_BASE_URL/admin"',
  'acquire_e2e_lock',
  'release_e2e_lock',
  'readlink "$LOCK_FILE"',
  'kill -0 "$owner_pid"',
  'RECOVERY_MARKER',
  'write_recovery_marker',
  'recovery required before another shared E2E run',
  '--recover',
  'os.setsid()',
  'child_pgid',
  'kill -0 -- "-$recovery_child_pgid"',
  'stop_e2e_children',
  'could not terminate the Docker E2E process group',
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
for (const required of ['stop_writers', 'writers_are_stopped', 'stop_e2e_children', 'ps -a -q "$service"', 'failed to inspect writer state']) {
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
for (const required of ['pg_dump', 'pg_restore --list', 'database-manifest', 'minio-manifest', 'shared-storage-manifest', '--execute', 'stop_old_dev_writers', 'failed to list old writer containers', 'failed to inspect old writer state', 'failed to list the source PostgreSQL container', 'failed to inspect the source PostgreSQL container', 'old writer is still running', 'SOURCE_STARTED_BY_MIGRATION', 'docker volume rm pic-gallery-dev_postgres-data']) {
  if (!migration.includes(required)) throw new Error(`migration safety flow is missing ${required}`)
}
if (!migration.includes('prepare-local-runtime.sh') || migration.includes('DEV_NGINX_PORT')) {
  throw new Error('legacy dev migration must prepare the fixed-port local runtime before full Compose startup')
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
