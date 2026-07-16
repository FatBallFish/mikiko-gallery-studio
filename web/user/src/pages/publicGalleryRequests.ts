export type PublicGalleryRequestState = {
  generation: number
  request: number
}

export type PublicGalleryRequestToken = PublicGalleryRequestState

export function initialPublicGalleryRequestState(): PublicGalleryRequestState {
  return { generation: 0, request: 0 }
}

export function beginPublicGalleryRequest(state: PublicGalleryRequestState, mode: 'replace' | 'append') {
  const next = {
    generation: mode === 'replace' ? state.generation + 1 : state.generation,
    request: state.request + 1,
  }
  return { state: next, token: { ...next } }
}

export function canCommitPublicGalleryRequest(active: PublicGalleryRequestState, token: PublicGalleryRequestToken) {
  return active.generation === token.generation && active.request === token.request
}

export type PublicGalleryDetailRequestState = {
  imageId: string | null
  authKey: string | null
  generation: number
  request: number
  attempted: boolean
}

export type PublicGalleryDetailRequestToken = {
  imageId: string
  authKey: string | null
  generation: number
  request: number
}

export function initialPublicGalleryDetailRequestState(): PublicGalleryDetailRequestState {
  return { imageId: null, authKey: null, generation: 0, request: 0, attempted: false }
}

export function beginPublicGalleryDetailRequest(state: PublicGalleryDetailRequestState, imageId?: string | null, authKey?: string | null) {
  const current = syncPublicGalleryDetailRequest(state, imageId, authKey)
  if (!current.imageId || current.attempted) return { state: current, token: null }
  const next = { ...current, request: current.request + 1, attempted: true }
  return {
    state: next,
    token: { imageId: current.imageId, authKey: current.authKey, generation: next.generation, request: next.request },
  }
}

export function resetPublicGalleryDetailRequest(state: PublicGalleryDetailRequestState, imageId?: string | null, authKey?: string | null) {
  const current = syncPublicGalleryDetailRequest(state, imageId, authKey)
  return { ...current, generation: current.generation + 1, attempted: false }
}

export function canCommitPublicGalleryDetailRequest(active: PublicGalleryDetailRequestState, token: PublicGalleryDetailRequestToken) {
  return active.imageId === token.imageId
    && active.authKey === token.authKey
    && active.generation === token.generation
    && active.request === token.request
}

function syncPublicGalleryDetailRequest(state: PublicGalleryDetailRequestState, imageId?: string | null, authKey?: string | null) {
  const nextImageId = imageId?.trim() || null
  const nextAuthKey = authKey?.trim() || null
  if (state.imageId === nextImageId && state.authKey === nextAuthKey) return state
  return { ...state, imageId: nextImageId, authKey: nextAuthKey, generation: state.generation + 1, attempted: false }
}
