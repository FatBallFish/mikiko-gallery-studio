import { existsSync, readFileSync } from 'node:fs'

const modelURL = new URL('./publicGalleryRequests.ts', import.meta.url)
if (!existsSync(modelURL)) {
  throw new Error('public gallery needs an executable superseded-request generation model')
}

const { beginPublicGalleryRequest, canCommitPublicGalleryRequest, initialPublicGalleryRequestState } = await import('./publicGalleryRequests')

let active = initialPublicGalleryRequestState()
const oldReplace = beginPublicGalleryRequest(active, 'replace')
active = oldReplace.state
const newReplace = beginPublicGalleryRequest(active, 'replace')
active = newReplace.state

if (canCommitPublicGalleryRequest(active, oldReplace.token)) {
  throw new Error('an old replace response must not commit after a newer replace request begins')
}
if (!canCommitPublicGalleryRequest(active, newReplace.token)) {
  throw new Error('the newest replace response must be allowed to commit')
}

const staleAppend = beginPublicGalleryRequest(active, 'append')
active = staleAppend.state
const filterReplace = beginPublicGalleryRequest(active, 'replace')
active = filterReplace.state
if (canCommitPublicGalleryRequest(active, staleAppend.token)) {
  throw new Error('an append response must not commit after query or filter replacement begins')
}
if (!canCommitPublicGalleryRequest(active, filterReplace.token)) {
  throw new Error('the current filter replacement must be allowed to commit')
}

const source = readFileSync(new URL('./PublicGalleryPage.tsx', import.meta.url), 'utf8')
for (const contract of [
  'beginPublicGalleryRequest',
  'canCommitPublicGalleryRequest',
  'requestStateRef',
  'if (!canCommitPublicGalleryRequest(requestStateRef.current, requestToken)) return',
]) {
  if (!source.includes(contract)) throw new Error(`public gallery request guard must wire ${contract}`)
}
