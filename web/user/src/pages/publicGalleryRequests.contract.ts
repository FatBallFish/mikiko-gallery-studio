import { existsSync, readFileSync } from 'node:fs'

const modelURL = new URL('./publicGalleryRequests.ts', import.meta.url)
if (!existsSync(modelURL)) {
  throw new Error('public gallery needs an executable superseded-request generation model')
}

const requestModel = await import('./publicGalleryRequests') as unknown as Record<string, unknown>
const { beginPublicGalleryRequest, canCommitPublicGalleryRequest, initialPublicGalleryRequestState } = requestModel as {
  beginPublicGalleryRequest: typeof import('./publicGalleryRequests').beginPublicGalleryRequest
  canCommitPublicGalleryRequest: typeof import('./publicGalleryRequests').canCommitPublicGalleryRequest
  initialPublicGalleryRequestState: typeof import('./publicGalleryRequests').initialPublicGalleryRequestState
}

type DetailState = { imageId: string | null; authKey?: string | null; generation: number; request: number; attempted: boolean }
type DetailToken = { imageId: string; authKey?: string | null; generation: number; request: number }
type DetailBegin = (state: DetailState, imageId?: string | null, authKey?: string | null) => { state: DetailState; token: DetailToken | null }
type DetailReset = (state: DetailState, imageId?: string | null, authKey?: string | null) => DetailState
type DetailCanCommit = (state: DetailState, token: DetailToken) => boolean

const initialDetail = requestModel.initialPublicGalleryDetailRequestState as (() => DetailState) | undefined
const beginDetail = requestModel.beginPublicGalleryDetailRequest as DetailBegin | undefined
const resetDetail = requestModel.resetPublicGalleryDetailRequest as DetailReset | undefined
const canCommitDetail = requestModel.canCommitPublicGalleryDetailRequest as DetailCanCommit | undefined

if (!initialDetail || !beginDetail || !resetDetail || !canCommitDetail) {
  throw new Error('public gallery deep-link detail needs an executable single-attempt request gate')
}

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

let detailState = initialDetail()
const firstDetail = beginDetail(detailState, 'image-a')
detailState = firstDetail.state
if (!firstDetail.token) {
  throw new Error('a new deep-link image must be allowed one automatic detail request')
}

const duplicateDetail = beginDetail(detailState, 'image-a')
detailState = duplicateDetail.state
if (duplicateDetail.token) {
  throw new Error('the same deep-link image must not auto-request again after a failed attempt finishes')
}

const switchedDetail = beginDetail(detailState, 'image-b')
detailState = switchedDetail.state
if (!switchedDetail.token || switchedDetail.token.imageId !== 'image-b') {
  throw new Error('changing the deep-link image must allow a fresh automatic detail request')
}

const retriableDetail = resetDetail(detailState, 'image-b')
const retriedDetail = beginDetail(retriableDetail, 'image-b')
if (!retriedDetail.token || retriedDetail.token.imageId !== 'image-b') {
  throw new Error('an explicit retry must reset the attempt gate and issue a new detail request')
}

if (!switchedDetail.token || canCommitDetail(retriedDetail.state, switchedDetail.token)) {
  throw new Error('a stale detail generation must not overwrite the current retry result')
}
if (!canCommitDetail(retriedDetail.state, retriedDetail.token)) {
  throw new Error('the latest detail retry result must be allowed to commit')
}

const oldAuthDetail = beginDetail(initialDetail(), 'image-auth', 'access-token-old')
if (!oldAuthDetail.token) throw new Error('the first auth generation must start a detail request')
const newAuthDetail = beginDetail(oldAuthDetail.state, 'image-auth', 'access-token-new')
if (!newAuthDetail.token) {
  throw new Error('rotating the access token must allow one fresh detail request for the same image')
}
if (canCommitDetail(newAuthDetail.state, oldAuthDetail.token)) {
  throw new Error('the old access-token generation must not commit results or clear the new request busy state')
}
if (!canCommitDetail(newAuthDetail.state, newAuthDetail.token)) {
  throw new Error('the new access-token generation must be allowed to complete and clear its busy state')
}
if (beginDetail(newAuthDetail.state, 'image-auth', 'access-token-new').token) {
  throw new Error('a failed detail request must still auto-attempt only once within the same access-token generation')
}

const source = readFileSync(new URL('./PublicGalleryPage.tsx', import.meta.url), 'utf8')
for (const contract of [
  'beginPublicGalleryRequest',
  'canCommitPublicGalleryRequest',
  'requestStateRef',
  'if (!canCommitPublicGalleryRequest(requestStateRef.current, requestToken)) return',
  'detailRequestStateRef',
  'beginPublicGalleryDetailRequest',
  'resetPublicGalleryDetailRequest',
  'canCommitPublicGalleryDetailRequest',
]) {
  if (!source.includes(contract)) throw new Error(`public gallery request guard must wire ${contract}`)
}
