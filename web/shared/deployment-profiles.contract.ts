// @ts-ignore contract scripts run in Node; browser tsconfigs do not include node types.
import { readFileSync } from 'node:fs'
// @ts-ignore contract scripts run in Node; browser tsconfigs do not include node types.
import { createRequire } from 'node:module'

const root = new URL('../../', import.meta.url)
const read = (path: string) => readFileSync(new URL(path, root), 'utf8')
const require = createRequire(new URL('../docs/package.json', import.meta.url))
const YAML = require('yaml') as { parse: (source: string) => any }
const compose = YAML.parse(read('deployments/docker-compose/docker-compose.prod.yml'))
const services = compose.services ?? {}

for (const service of ['postgres', 'redis', 'minio', 'minio-init', 'api', 'worker', 'user-web', 'admin-web', 'docs-web', 'gateway']) {
  if (!services[service]) throw new Error(`production Compose is missing ${service}`)
}

for (const service of ['postgres', 'redis', 'minio', 'minio-init']) {
  const profiles = services[service].profiles ?? []
  if (!profiles.includes('full')) throw new Error(`${service} must participate in the Docker full profile`)
}
for (const service of ['api', 'worker', 'user-web', 'admin-web', 'docs-web', 'gateway']) {
  const profiles = services[service].profiles ?? []
  if (!profiles.includes('full') || !profiles.includes('core') || !profiles.includes(service)) {
    throw new Error(`${service} must participate in full, core, and its component profile`)
  }
}

for (const service of ['api', 'worker']) {
  if (services[service].working_dir !== '/app') throw new Error(`${service} must resolve ./config/runtime.env from /app`)
  if (services[service].environment) throw new Error(`${service} must load runtime values only from mounted config/runtime.env`)
  const configMount = (services[service].volumes ?? []).find((volume: any) => volume?.target === '/app/config')
  if (!configMount || configMount.type !== 'bind' || !String(configMount.source).includes('MGSCTL_RUNTIME_DIR')) {
    throw new Error(`${service} must bind the portable runtime config directory`)
  }
  if (service === 'api' && configMount.read_only) throw new Error('API config mount must be writable for setup commit')
  if (service === 'worker' && !configMount.read_only) throw new Error('Worker config mount must be read-only')
}

const apiHealth = JSON.stringify(services.api.healthcheck?.test ?? [])
if (!apiHealth.includes('/healthz') || apiHealth.includes('/readyz')) {
  throw new Error('API Docker health must use liveness so setup mode can start')
}
if (services.gateway.depends_on?.api?.condition !== 'service_healthy') {
  throw new Error('Gateway must start from the API liveness-backed container health')
}

const redisCommand = JSON.stringify(services.redis.command ?? [])
const redisHealth = JSON.stringify(services.redis.healthcheck?.test ?? [])
if (!redisCommand.includes('requirepass') || !redisHealth.includes('REDIS_PASSWORD')) {
  throw new Error('managed Redis must require and probe with authentication')
}

if (services.minio.command?.[0] !== 'server' || !services['minio-init'].depends_on?.minio) {
  throw new Error('Docker full must include MinIO and an idempotent bucket/user initializer')
}
for (const dependency of ['postgres', 'redis', 'minio-init']) {
  if (services.api.depends_on?.[dependency]) {
    throw new Error(`API managed dependency ${dependency} must be staged by mgsctl before application startup`)
  }
}
const minioInitMounts = JSON.stringify(services['minio-init'].volumes ?? [])
if (!minioInitMounts.includes('minio-init.sh')) throw new Error('MinIO initializer script must be mounted read-only')
const postgresInitMounts = JSON.stringify(services.postgres.volumes ?? [])
if (!postgresInitMounts.includes('postgres-init.sh') || !postgresInitMounts.includes('/opt/deploy/')) {
  throw new Error('managed PostgreSQL must install the least-privilege application-role initializer')
}
const postgresEntrypoint = JSON.stringify(services.postgres.entrypoint ?? [])
for (const marker of ['cp ', 'chown postgres:postgres', 'chmod 0700', '/docker-entrypoint-initdb.d/10-app-role.sh', 'docker-entrypoint.sh postgres']) {
  if (!postgresEntrypoint.includes(marker)) throw new Error(`PostgreSQL initializer bootstrap is missing: ${marker}`)
}
const postgresInit = read('deployments/docker-compose/postgres-init.sh')
for (const marker of ['CREATE ROLE', 'ALTER DATABASE', 'ALTER ROLE CURRENT_USER PASSWORD NULL', 'NOSUPERUSER', 'NOCREATEDB', 'NOCREATEROLE', 'NOREPLICATION']) {
  if (!postgresInit.includes(marker)) throw new Error(`PostgreSQL initializer is missing least-privilege role setup: ${marker}`)
}
const minioInit = read('deployments/docker-compose/minio-init.sh')
for (const marker of ['mc mb --ignore-existing', 'mc admin user info', 'mc admin policy create', 'arn:aws:s3:::$STORAGE_S3_BUCKET/*']) {
  if (!minioInit.includes(marker)) throw new Error(`MinIO initializer is missing idempotent bucket-scoped setup: ${marker}`)
}

const nginx = read('deployments/nginx/default.conf')
for (const route of ['location = /setup', 'location /api/', 'location = /healthz', 'location = /readyz']) {
  if (!nginx.includes(route)) throw new Error(`Gateway is missing route: ${route}`)
}

const envExample = read('deployments/docker-compose/.env.prod.example')
for (const retired of ['DATABASE_URL=', 'AUTH_ACCESS_TOKEN_SECRET=', 'POSTGRES_PASSWORD=change_me']) {
  if (envExample.includes(retired)) throw new Error(`legacy Docker env template still duplicates runtime config: ${retired}`)
}

const prepare = read('deployments/docker-compose/prepare.sh')
if (!prepare.includes('scripts/install.sh') || prepare.includes('replace_env ') || prepare.includes('generate_secret')) {
  throw new Error('legacy Docker prepare wrapper must delegate policy and secret generation to mgsctl')
}
