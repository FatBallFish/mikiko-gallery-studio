// @ts-ignore contract scripts run in Node; browser app tsconfigs do not include Node types.
import { readFileSync } from 'node:fs'
// @ts-ignore contract scripts run in Node; browser app tsconfigs do not include Node types.
import { resolve } from 'node:path'

// @ts-ignore contract scripts run in Node 20+ where import.meta.dirname is available.
const root = resolve(import.meta.dirname, '../..')
const setupRunner = readFileSync(resolve(root, 'scripts/e2e/setup-docker-e2e.sh'), 'utf8')
const clusterRunner = readFileSync(resolve(root, 'scripts/e2e/cluster-docker-e2e.sh'), 'utf8')
const browserRunner = readFileSync(resolve(root, 'scripts/e2e/setup-browser.py'), 'utf8')
const library = readFileSync(resolve(root, 'scripts/e2e/deployment-e2e-lib.sh'), 'utf8')
const productionCompose = readFileSync(resolve(root, 'deployments/docker-compose/docker-compose.prod.yml'), 'utf8')
const businessRunner = readFileSync(resolve(root, 'scripts/e2e/docker-e2e.mjs'), 'utf8')
const clusterProxy = readFileSync(resolve(root, 'scripts/e2e/cluster-http-proxy.py'), 'utf8')
const verifyRunner = readFileSync(resolve(root, 'scripts/workflow/verify.sh'), 'utf8')

for (const [name, source] of [['setup', setupRunner], ['cluster', clusterRunner]] as const) {
	const completeSource = `${source}\n${library}`
  for (const required of [
    'pic-gallery-${name}-e2e-',
    'mktemp',
    'E2E_EVIDENCE_DIR',
    'docker compose',
    '--project-name',
    'trap cleanup EXIT',
    'runtime.env',
  ]) {
    const expected = required.replace('${name}', name)
		if (!completeSource.includes(expected)) throw new Error(`${name} deployment E2E is missing ${expected}`)
  }
  for (const forbidden of [
    'docker-compose.local.yml',
    'pic-gallery-local',
    'pic-gallery-local_postgres-data',
    'pic-gallery-local_minio-data',
    'docker volume prune',
    'docker system prune',
    'down -v --remove-orphans',
    'set -x',
  ]) {
    if (source.includes(forbidden)) throw new Error(`${name} deployment E2E contains unsafe shared-state operation ${forbidden}`)
  }
}

for (const required of [
  'desktop-setup.png',
  'mobile-setup.png',
  'deployctl setup token show',
  'deployctl setup token reset',
  'setup-model',
  'request_urls',
  'E2E_APPLY_SETUP',
  'INVALID_CONFIGURATION',
	'restart-countdown',
	'completion-panel',
	'wait_for_url',
	'E2E_EXPECT_INTERRUPTION',
	'DIRECT_USER_WEB_URL',
	'DIRECT_ADMIN_WEB_URL',
	'DIRECT_DOCS_WEB_URL',
	'GATEWAY_DOCS_WEB_URL',
	'expected_setup_url',
]) {
  if (!browserRunner.includes(required)) throw new Error(`setup browser E2E is missing ${required}`)
}
for (const required of ['deployment_e2e_assert_frontend', 'missing-e2e.js', 'application/javascript', 'text/css']) {
	if (!`${setupRunner}\n${library}`.includes(required)) throw new Error(`setup deployment E2E is missing frontend HTTP contract ${required}`)
}
if (!setupRunner.includes('DEPLOYMENT_E2E_PROFILES')) {
	throw new Error('setup deployment E2E must support selecting the full profile for full/single acceptance')
}
for (const required of ['pg_advisory_lock', "locktype = 'advisory'", 'recovery-operation-id.txt']) {
	if (!setupRunner.includes(required)) throw new Error(`setup deployment E2E is missing interrupted-setup recovery control ${required}`)
}
if (!setupRunner.includes('E2E_APPLY_SETUP=true')) {
  throw new Error('setup deployment E2E must drive final apply through the API-hosted browser UI')
}
if (!verifyRunner.includes('native-package-contract.sh')) {
  throw new Error('repository verification must cross-build and inspect Linux/Windows native release bundles')
}
if (!clusterRunner.includes('register_pending_runtime')) {
  throw new Error('cluster deployment E2E must register real node runtimes before deployctl can partially create resources')
}
if (!clusterRunner.includes('local path=$1 minimum=$2 watched_pid=$3 timeout=${4:-240}\n  local deadline=$((SECONDS + timeout)) lines')) {
  throw new Error('cluster deployment E2E must initialize timeout before expanding it under set -u')
}
if (!clusterRunner.includes('kill -0 "$watched_pid"') || !clusterRunner.includes('wait_for_file_lines "$MARKER" 1 "$BUSINESS_PID" 300')) {
  throw new Error('cluster deployment E2E must stop waiting when its business runner exits before the provider marker')
}
if (!clusterRunner.includes('f"http://127.0.0.1:{gateway_port},http://localhost:{gateway_port}"')) {
  throw new Error('cluster setup must allow both loopback browser origins exercised by the business E2E')
}
if (!setupRunner.includes('SETUP_CORS_ALLOWED_ORIGINS="http://127.0.0.1:${gateway_port},http://localhost:${gateway_port},http://127.0.0.1:${user_port},http://127.0.0.1:${admin_port}"')) {
  throw new Error('setup deployment E2E must allow Gateway and direct frontend browser origins')
}

if (!setupRunner.includes('run_profile full') || !setupRunner.includes('run_profile core')) {
  throw new Error('setup deployment E2E must cover both full and core profiles')
}
if (!setupRunner.includes('if [[ "$profile" == core ]]; then\n    configure_local_e2e_runtime "$runtime" "$project" "$api_port"')) {
  throw new Error('Docker full setup must remain in production mode; only the core HTTP fixture may use local mode before setup')
}
if (!setupRunner.includes('if [[ "$profile" == full && "${E2E_RUN_BUSINESS:-true}" == true ]]; then\n    configure_local_e2e_runtime "$runtime" "$project" "$api_port"')) {
  throw new Error('Docker full business E2E may enable local test auth only after production-mode setup has recovered')
}
for (const [name, source, localMarker] of [
  ['setup', setupRunner, 'deployment_e2e_set_env_value "$env_file" PIC_GALLERY_ENV local'],
  ['cluster', clusterRunner, 'PIC_GALLERY_ENV=local'],
] as const) {
  if (!source.includes(localMarker) || source.includes('PIC_GALLERY_ENV=test')) {
    throw new Error(`${name} deployment E2E must use the supported local environment for its HTTP object storage fixture`)
  }
}
for (const [name, source] of [['setup', setupRunner], ['cluster', clusterRunner]] as const) {
  for (const required of ['POSTGRES_USER=postgres', 'APP_POSTGRES_USER=app', 'postgres-init.sh:/opt/deploy/postgres-init.sh:ro']) {
    if (!source.includes(required)) throw new Error(`${name} PostgreSQL E2E fixture is missing least-privilege setup ${required}`)
  }
}

const runtimeRegistration = setupRunner.indexOf('RUNTIMES+=("$runtime")')
const installInvocation = setupRunner.indexOf('"$DEPLOYCTL" install')
if (runtimeRegistration < 0 || installInvocation < 0 || runtimeRegistration > installInvocation) {
  throw new Error('setup deployment E2E must register the runtime before deployctl can partially create resources')
}
for (const required of ['E2E_INSTALL_ATTEMPTS', 'deployment_e2e_remove_project "$project"']) {
  if (!setupRunner.includes(required)) throw new Error(`setup deployment E2E is missing bounded install recovery ${required}`)
}

for (const required of ['cluster join', 'issue_token api', 'issue_token worker', 'encrypted enrollment', 'exactly-once', 'lease recovery']) {
  if (!clusterRunner.includes(required)) throw new Error(`cluster deployment E2E is missing ${required}`)
}
if (clusterRunner.includes('cluster scenario is not implemented yet')) {
  throw new Error('cluster deployment E2E still contains its unimplemented sentinel')
}
for (const required of ['E2E_IMAGE_PROVIDER_DELAY_MS', 'E2E_IMAGE_PROVIDER_MARKER', 'E2E_SKIP_MIDDLEWARE_HEALTH']) {
  if (!businessRunner.includes(required)) throw new Error(`Docker business E2E is missing cluster control ${required}`)
}
if (!businessRunner.includes("template !== '/setup' && !template.startsWith('/api/setup/')") ||
    (businessRunner.match(/normalModeOpenAPIPaths\(openapi\)/g) || []).length < 2) {
  throw new Error('normal-mode Docker business sweeps must exclude setup-only OpenAPI paths')
}
if (!businessRunner.includes(".replace('{token_id}', '00000000-0000-0000-0000-000000000000')") ||
    !businessRunner.includes("template.includes('/cluster/tokens/{token_id}')")) {
  throw new Error('Docker business route sweep must materialize missing cluster tokens as expected semantic 404s')
}
for (const required of ['ThreadingHTTPServer', '--upstream', '--capture-file', '--upstream-log']) {
  if (!clusterProxy.includes(required)) throw new Error(`cluster HTTP proxy is missing ${required}`)
}
if (!setupRunner.includes('docker rm -fv') || !clusterRunner.includes('docker rm -fv')) {
  if (!library.includes('deployment_e2e_remove_project')) {
    throw new Error('deployment E2E cleanup must remove only its recorded containers and anonymous volumes')
  }
}
for (const source of [setupRunner, clusterRunner]) {
  if (!source.includes('deployment_e2e_remove_project "$project"')) {
    throw new Error('deployment E2E runner must use label-scoped project cleanup fallback')
  }
}

for (const [service, nextService] of [['api', 'worker'], ['worker', 'user-web']] as const) {
  const start = productionCompose.indexOf(`  ${service}:`)
  const end = productionCompose.indexOf(`  ${nextService}:`, start + 1)
  const definition = productionCompose.slice(start, end)
  if (start < 0 || end < 0 || !definition.includes('extra_hosts:') || !definition.includes('host.docker.internal:host-gateway')) {
    throw new Error(`${service} must resolve host.docker.internal on portable Docker Engine installations`)
  }
}
