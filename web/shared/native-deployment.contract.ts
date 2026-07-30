// @ts-ignore contract scripts run in Node; browser tsconfigs do not include node types.
import { existsSync, readFileSync } from 'node:fs'

const root = new URL('../../', import.meta.url)
const read = (path: string) => readFileSync(new URL(path, root), 'utf8')

const packageScript = read('scripts/devops/package.sh')
for (const required of [
  'package_native',
  './cmd/gateway',
  './cmd/servicehost',
  'mikiko-gallery-studio-native-${GOOS_TARGET}-${GOARCH_TARGET}.tar.gz',
  'sha256sum',
  'shasum -a 256',
  'package_native',
]) {
  if (!packageScript.includes(required)) throw new Error(`native package workflow is missing: ${required}`)
}
for (const directory of ['bin', 'web', 'api']) {
  if (!packageScript.includes(` ${directory}`)) throw new Error(`native archive does not include ${directory}`)
}

const mgsctlMain = read('cmd/mgsctl/main.go')
if (!mgsctlMain.includes('NativeExecutor') || !mgsctlMain.includes('NativeActionInstall')) {
  throw new Error('mgsctl main does not execute native installs')
}

for (const retiredPath of [
  'scripts/local/pgctl.sh',
  'scripts/local/pgctl_contract_test.sh',
  'scripts/service/manage.sh',
  'scripts/service/manage.ps1',
  'scripts/service/install.sh',
  'scripts/service/install.ps1',
  'scripts/service/uninstall.sh',
  'scripts/service/uninstall.ps1',
  'scripts/service/service_config_contract_test.sh',
]) {
  if (existsSync(new URL(retiredPath, root))) throw new Error(`legacy production deployment entrypoint still exists: ${retiredPath}`)
}

const native = read('internal/mgsctl/native.go')
for (const required of [
  'BuildNativeProcessSpecs',
  'Executable: "systemctl"',
  'Executable: "sc.exe"',
  'NativeActionInstall',
  'NativeActionRestart',
  'NativeActionStatus',
  'NativeActionUninstall',
  'mikiko-gallery-studio-service-host.exe',
]) {
  if (!native.includes(required)) throw new Error(`mgsctl native service ownership is missing: ${required}`)
}
for (const retired of ['PIC_GALLERY_ENV_FILE', 'EnvironmentFile=', 'Register-ScheduledTask']) {
  if (native.includes(retired)) throw new Error(`native production service plan contains retired behavior: ${retired}`)
}
