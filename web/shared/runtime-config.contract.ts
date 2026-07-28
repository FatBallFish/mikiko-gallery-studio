import { resolveRuntimeAPIBase } from './runtime-config.ts'

const directUser = resolveRuntimeAPIBase(
  { apiPort: '8080', directFrontendPort: '5173' },
  'http://10.0.0.8:5173/workspace',
)
if (directUser !== 'http://10.0.0.8:8080') throw new Error(`direct user API base: ${directUser}`)

const directAdmin = resolveRuntimeAPIBase(
  { apiPort: '18080', directFrontendPort: '15174' },
  'http://[::1]:15174/',
)
if (directAdmin !== 'http://[::1]:18080') throw new Error(`direct admin API base: ${directAdmin}`)

const gateway = resolveRuntimeAPIBase(
  { apiPort: '8080', directFrontendPort: '5173' },
  'http://10.0.0.8/',
)
if (gateway !== '') throw new Error(`gateway API base must remain same-origin: ${gateway}`)

const explicit = resolveRuntimeAPIBase(
  { apiBaseUrl: 'https://api.example.test/base/', apiPort: '8080', directFrontendPort: '5173' },
  'http://10.0.0.8:5173/',
)
if (explicit !== 'https://api.example.test/base') throw new Error(`explicit API base did not win: ${explicit}`)

for (const unsafe of ['//evil.example/api', 'javascript:alert(1)', 'ftp://api.example.test', 'https://user:pass@api.example.test']) {
  try {
    resolveRuntimeAPIBase({ apiBaseUrl: unsafe }, 'https://user.example.test/')
    throw new Error(`unsafe runtime API URL was accepted: ${unsafe}`)
  } catch (error) {
    if (!(error instanceof TypeError)) throw error
  }
}
