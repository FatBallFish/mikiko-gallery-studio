import assert from 'node:assert/strict'
import { existsSync, readFileSync } from 'node:fs'
import test from 'node:test'

const scriptPath = 'scripts/workflow/multimedia-acceptance.sh'

test('heavy multimedia acceptance is explicit, isolated, and self-cleaning', () => {
  assert.equal(existsSync(scriptPath), true, `${scriptPath} must exist`)
  const source = readFileSync(scriptPath, 'utf8')
  for (const required of [
    'MULTIMEDIA_ACCEPTANCE=1', 'TestMultimediaLoadAcceptance', 'TestMultimediaLocalOneGiBAcceptance',
    'TestMultimediaS3OneGiBAcceptance', 'minio/minio:', 'minio/mc:', 'docker rm -f', 'trap cleanup EXIT',
  ]) assert.ok(source.includes(required), `multimedia acceptance must include ${required}`)
  assert.ok(!source.includes('rm -rf'), 'multimedia acceptance must not use broad recursive deletion')
})
