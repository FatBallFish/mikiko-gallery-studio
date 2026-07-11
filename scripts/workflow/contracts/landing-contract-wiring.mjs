import { readFileSync } from 'node:fs'

const verifySource = readFileSync('scripts/workflow/verify-contracts.sh', 'utf8')
for (const contract of [
  'web/user/src/pages/landingContent.contract.ts',
  'web/user/src/pages/landingPage.contract.ts',
  'web/user/src/ui/useLandingMotion.contract.ts',
]) {
  if (!verifySource.includes(contract)) throw new Error(`landing contract is not explicitly wired: ${contract}`)
}

console.log('OK: landing contracts are explicitly wired')
