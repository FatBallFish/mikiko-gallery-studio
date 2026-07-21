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
  'SETUP_COMPLETED=$setup_completed',
	'SETUP_TOKEN=$setup_token',
  'SETUP_TOKEN_VERSION=1',
  'POSTGRES_CONTAINER=',
  'REDIS_CONTAINER=',
  'postgres:16-alpine',
  'redis:7-alpine',
  'docker rm -f "$POSTGRES_CONTAINER" "$REDIS_CONTAINER"',
	'assert_ordinary_startup_does_not_migrate',
	'run_setup_initialization',
	'/api/setup/v1/session',
	'/api/setup/v1/apply',
]) {
  if (!smoke.includes(required)) {
    throw new Error(`API contract smoke must include isolated runtime prerequisite: ${required}`)
  }
}
for (const retired of ['MIGRATE_BINARY=', 'go build -o "$MIGRATE_BINARY"', '"$MIGRATE_BINARY"']) {
	if (smoke.includes(retired)) {
		throw new Error(`API contract smoke must initialize through setup instead of db-migrate: ${retired}`)
	}
}
const setupInitializationIndex = smoke.indexOf('\nrun_setup_initialization\n')
const normalAPIStartIndex = smoke.indexOf(
	'\nAPP_ENV_FILE="$SMOKE_ENV_PATH" \\\n"$API_BINARY" >"$SERVER_LOG" 2>&1 &',
	setupInitializationIndex,
)
if (setupInitializationIndex < 0 || normalAPIStartIndex < 0 || setupInitializationIndex > normalAPIStartIndex) {
	throw new Error('API contract smoke must complete setup initialization before ordinary API startup')
}
for (const retired of ['DATABASE_URL=file:', 'import sqlite3', 'DB_PATH=']) {
  if (smoke.includes(retired)) {
    throw new Error(`API contract smoke must not retain SQLite runtime setup: ${retired}`)
  }
}

const smokeDocs = [
  'README.md',
  'README.zh-CN.md',
  'docs/ops/api-contract-smoke-test.md',
  'docs/org/workflow/DEVELOPMENT_WORKFLOW.md',
  'AGENTS.md',
  '.agents/skills/dev-api-smoke/SKILL.md',
  '.claude/skills/dev-api-smoke/SKILL.md',
  '.agents/skills/dev-ship/SKILL.md',
  '.claude/skills/dev-ship/SKILL.md',
]
for (const path of smokeDocs) {
  const source = read(path)
  for (const required of [
    'Bash',
    'curl',
    'Python 3',
    'Go',
    'Docker daemon',
    'postgres:16-alpine',
    'redis:7-alpine',
    'BASE_URL',
    'http://127.0.0.1:<port>',
  ]) {
    if (!source.includes(required)) {
      throw new Error(`${path} must document the API smoke prerequisite or behavior: ${required}`)
    }
  }
}

const currentOperationalDocs = smokeDocs.map((path) => read(path)).join('\n')
for (const stale of [
  'against a live API',
  '对运行中的 API',
  'temporary SQLite database',
  'If the API is not running',
]) {
  if (currentOperationalDocs.includes(stale)) {
    throw new Error(`current API smoke documentation retains stale behavior: ${stale}`)
  }
}

const smokeBehaviorMarkers: Record<string, string[]> = {
  'README.md': ['starts its own API', '`BASE_URL` only accepts', 'Exit cleanup'],
  'README.zh-CN.md': ['脚本会自行启动 API', '`BASE_URL` 只接受', '清理（cleanup）'],
  'docs/ops/api-contract-smoke-test.md': [
    'starts its own API and Worker',
    '`BASE_URL` only accepts',
    'The exit cleanup trap',
  ],
  'docs/org/workflow/DEVELOPMENT_WORKFLOW.md': [
    'starts and cleans up its own API',
    '`BASE_URL` only accepts',
  ],
  'AGENTS.md': ['starts and cleans up its own API', '`BASE_URL` only accepts'],
  '.agents/skills/dev-api-smoke/SKILL.md': [
    'starts and cleans up its own API',
    '`BASE_URL` only accepts',
  ],
  '.claude/skills/dev-api-smoke/SKILL.md': [
    'starts and cleans up its own API',
    '`BASE_URL` only accepts',
  ],
  '.agents/skills/dev-ship/SKILL.md': [
    'starts and cleans up its own API',
    '`BASE_URL` only accepts',
  ],
  '.claude/skills/dev-ship/SKILL.md': [
    'starts and cleans up its own API',
    '`BASE_URL` only accepts',
  ],
}
for (const [path, markers] of Object.entries(smokeBehaviorMarkers)) {
  const source = read(path)
  for (const marker of markers) {
    if (!source.includes(marker)) {
      throw new Error(`${path} must preserve API smoke lifecycle documentation: ${marker}`)
    }
  }
}
