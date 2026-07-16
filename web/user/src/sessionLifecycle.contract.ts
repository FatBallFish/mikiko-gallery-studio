import { readFileSync } from 'node:fs'

const source = readFileSync(new URL('./App.tsx', import.meta.url), 'utf8')

for (const required of [
  'const sessionVersionRef = useRef(0)',
  'getSessionVersion: () => sessionVersionRef.current',
  'const refreshSessionVersion = sessionVersionRef.current',
  'if (sessionVersionRef.current !== refreshSessionVersion) return null',
  'if (sessionVersionRef.current === refreshSessionVersion) expireSession()',
]) {
  if (!source.includes(required)) throw new Error(`user session lifecycle is missing stale-refresh protection: ${required}`)
}

function sourceBetween(start: string, end: string) {
  const startIndex = source.indexOf(start)
  const endIndex = source.indexOf(end, startIndex + start.length)
  if (startIndex < 0 || endIndex < 0) throw new Error(`could not inspect session lifecycle block: ${start} -> ${end}`)
  return source.slice(startIndex, endIndex)
}

const expireBlock = sourceBetween('const expireSession = useCallback', '\n\n  useLayoutEffect')
const installBlock = sourceBetween('const installSession = useCallback', '\n\n  const refreshAccount')
const refreshAccountBlock = sourceBetween('const refreshAccount = useCallback', '\n\n  useEffect(() => {')
const loginBlock = sourceBetween('const login = useCallback', '\n\n  const logout')
const logoutBlock = sourceBetween('const logout = useCallback', '\n\n  const appValue')

for (const [name, block] of [['expire', expireBlock], ['login', loginBlock], ['logout', logoutBlock]] as const) {
  if (!block.includes('sessionVersionRef.current += 1')) {
    throw new Error(`${name} must advance the user session generation`)
  }
}

if (installBlock.includes('sessionVersionRef.current += 1')) {
  throw new Error('ordinary profile/session synchronization must not advance the identity generation')
}
if (refreshAccountBlock.includes('sessionVersionRef.current += 1')) {
  throw new Error('ordinary account refresh must not advance the identity generation')
}
if (!refreshAccountBlock.includes('if (sessionVersionRef.current === refreshAccountVersion) expireSession()')) {
  throw new Error('a stale account refresh 401 must not expire a newer user session')
}
if (refreshAccountBlock.includes('installSession({ token: currentSession.token')) {
  throw new Error('account refresh must not restore the access token captured before a silent refresh')
}

const refreshAccountSuccessGuard = refreshAccountBlock.indexOf(
  'if (sessionVersionRef.current !== refreshAccountVersion) return',
)
const latestAccountTokenRead = refreshAccountBlock.indexOf(
  'const latestToken = sessionRef.current?.token',
  refreshAccountSuccessGuard,
)
const latestAccountTokenGuard = refreshAccountBlock.indexOf('if (!latestToken) return', latestAccountTokenRead)
const latestAccountTokenInstall = refreshAccountBlock.indexOf(
  'installSession({ token: latestToken, profile: nextProfile })',
  latestAccountTokenGuard,
)
const refreshAccountCatchStart = refreshAccountBlock.indexOf('} catch (caught) {')
if (
  refreshAccountSuccessGuard < 0
  || latestAccountTokenRead < 0
  || latestAccountTokenGuard < 0
  || latestAccountTokenInstall < 0
  || refreshAccountCatchStart < 0
  || !(
    refreshAccountSuccessGuard < latestAccountTokenRead
    && latestAccountTokenRead < latestAccountTokenGuard
    && latestAccountTokenGuard < latestAccountTokenInstall
    && latestAccountTokenInstall < refreshAccountCatchStart
  )
) {
  throw new Error('account refresh must install the latest access token after validating the session generation')
}

const refreshAccountCatch = refreshAccountBlock.indexOf('} catch (caught) {')
const staleAccountFailureGuard = refreshAccountBlock.indexOf(
  'if (sessionVersionRef.current !== refreshAccountVersion) return',
  refreshAccountCatch,
)
const accountFailureStatusCheck = refreshAccountBlock.indexOf("if (caught && typeof caught === 'object'", refreshAccountCatch)
if (
  refreshAccountCatch < 0
  || staleAccountFailureGuard < 0
  || accountFailureStatusCheck < 0
  || !(refreshAccountCatch < staleAccountFailureGuard && staleAccountFailureGuard < accountFailureStatusCheck)
) {
  throw new Error('stale account refresh failures must be ignored before handling the response status')
}

const requestStart = logoutBlock.indexOf('const logoutRequest = userApi.logout()')
const invalidate = logoutBlock.indexOf('sessionRef.current = null')
const awaitRequest = logoutBlock.indexOf('await logoutRequest')
if (requestStart < 0 || invalidate < 0 || awaitRequest < 0 || !(requestStart < invalidate && invalidate < awaitRequest)) {
  throw new Error('logout must start the remote request, synchronously invalidate local session state, then await completion')
}

console.log('OK: user session lifecycle contract passed')
