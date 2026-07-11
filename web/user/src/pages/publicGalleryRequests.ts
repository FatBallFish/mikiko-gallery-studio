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
