// @ts-ignore contract scripts run in Node; browser tsconfigs do not include node types.
import { readFileSync } from 'node:fs'

const root = new URL('../../', import.meta.url)
const read = (path: string) => readFileSync(new URL(path, root), 'utf8')

const packageScript = read('scripts/devops/package.sh')
for (const required of [
  'package_native',
  './cmd/gateway',
  './cmd/servicehost',
  'pic-gallery-native-${GOOS_TARGET}-${GOARCH_TARGET}.tar.gz',
  'sha256sum',
  'shasum -a 256',
  'package_native',
]) {
  if (!packageScript.includes(required)) throw new Error(`native package workflow is missing: ${required}`)
}
for (const directory of ['bin', 'web', 'api']) {
  if (!packageScript.includes(` ${directory}`)) throw new Error(`native archive does not include ${directory}`)
}

const deployctlMain = read('cmd/deployctl/main.go')
if (!deployctlMain.includes('NativeExecutor') || !deployctlMain.includes('NativeActionInstall')) {
  throw new Error('deployctl main does not execute native installs')
}

const windowsManager = read('scripts/service/manage.ps1')
for (const retired of ['Register-ScheduledTask', 'New-ScheduledTaskAction', 'Start-ScheduledTask', 'schtasks']) {
  if (windowsManager.includes(retired)) throw new Error(`Windows service wrapper still uses Scheduled Tasks: ${retired}`)
}
if (!windowsManager.includes('scripts/install.ps1') && !windowsManager.includes('DEPLOYCTL_BIN')) {
  throw new Error('Windows service wrapper must delegate to deployctl')
}
for (const unsupported of ['"start"', '"stop"']) {
  if (windowsManager.includes(unsupported)) throw new Error(`Windows wrapper exposes unsupported deployctl action: ${unsupported}`)
}

const unixManager = read('scripts/service/manage.sh')
if (!unixManager.includes('gateway)') || !unixManager.includes('./cmd/gateway')) {
  throw new Error('local Unix service manager does not preserve Gateway development support')
}

const native = read('internal/deployctl/native.go')
for (const retired of ['PIC_GALLERY_ENV_FILE', 'EnvironmentFile=', 'Register-ScheduledTask']) {
  if (native.includes(retired)) throw new Error(`native production service plan contains retired behavior: ${retired}`)
}
