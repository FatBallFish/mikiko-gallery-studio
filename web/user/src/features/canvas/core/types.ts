export type CanvasNodeType = 'prompt' | 'image' | 'video' | 'audio' | 'image_generation' | 'video_generation' | 'note'
export type CanvasInputRole = 'prompt' | 'reference' | 'first_frame' | 'last_frame' | 'result'
export type CanvasPoint = { x: number; y: number }
export type CanvasSize = { width: number; height: number }
export type CanvasViewport = { x: number; y: number; zoom: number }

export type CanvasNode = {
  id: string
  type: CanvasNodeType
  asset_id?: string
  position: CanvasPoint
  size: CanvasSize
  payload?: Record<string, unknown>
}

export type CanvasEdge = {
  id: string
  source: string
  target: string
  source_handle?: string
  target_handle?: string
  input_role: CanvasInputRole
  ordinal?: number
}

export type CanvasDocument = {
  schema_version: 1
  viewport: CanvasViewport
  nodes: CanvasNode[]
  edges: CanvasEdge[]
}

export type CanvasCommandState = {
  present: CanvasDocument
  past: CanvasDocument[]
  future: CanvasDocument[]
  revision: number
  dirty: boolean
}

export type CanvasResult = { asset_id: string; media_type: 'image' | 'video' }

export type CanvasClipboard = {
  nodes: CanvasNode[]
  edges: CanvasEdge[]
}
