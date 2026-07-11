export type ImageMediaState = {
  url: string
  status: 'loading' | 'loaded' | 'error'
  attempt: number
}

export type ImageMediaEvent =
  | { type: 'reset'; url: string }
  | { type: 'loaded'; url?: string }
  | { type: 'error'; url?: string }
  | { type: 'retry' }

export function initialImageMediaState(url: string): ImageMediaState {
  return { url, status: 'loading', attempt: 0 }
}

export function imageMediaTransition(state: ImageMediaState, event: ImageMediaEvent): ImageMediaState {
  if (event.type === 'reset') return initialImageMediaState(event.url)
  if ('url' in event && event.url && event.url !== state.url) return state
  if (event.type === 'loaded') return { ...state, status: 'loaded' }
  if (event.type === 'error') return { ...state, status: 'error' }
  return { ...state, status: 'loading', attempt: state.attempt + 1 }
}
