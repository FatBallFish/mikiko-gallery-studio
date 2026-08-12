import dagre from '@dagrejs/dagre'
import type { CanvasDocument, CanvasNode, CanvasViewport } from './types'

export type CanvasBounds = { x: number; y: number; width: number; height: number }
export type ViewportSize = { width: number; height: number }

export function visibleCanvasNodeIDs(nodes: CanvasNode[], viewport: CanvasViewport, viewportSize: ViewportSize, overscan = 160, pinnedIDs: string[] = []) {
  const zoom = Math.max(viewport.zoom, 0.001)
  const bounds = {
    left: (-viewport.x / zoom) - overscan,
    top: (-viewport.y / zoom) - overscan,
    right: ((viewportSize.width - viewport.x) / zoom) + overscan,
    bottom: ((viewportSize.height - viewport.y) / zoom) + overscan,
  }
  const visible = new Set(pinnedIDs)
  nodes.forEach((node) => {
    if (node.position.x + node.size.width >= bounds.left && node.position.x <= bounds.right
      && node.position.y + node.size.height >= bounds.top && node.position.y <= bounds.bottom) visible.add(node.id)
  })
  return visible
}

export function autoLayoutCanvasNodes(document: CanvasDocument, nodeIDs: string[]) {
  const selected = new Set(nodeIDs)
  const targets = document.nodes.filter((node) => selected.has(node.id))
  if (targets.length < 2) return document
  const graph = new dagre.graphlib.Graph()
  graph.setGraph({ rankdir: 'LR', ranksep: 96, nodesep: 48, marginx: 0, marginy: 0 })
  graph.setDefaultEdgeLabel(() => ({}))
  targets.forEach((node) => graph.setNode(node.id, { width: node.size.width, height: node.size.height }))
  document.edges.forEach((edge) => {
    if (selected.has(edge.source) && selected.has(edge.target)) graph.setEdge(edge.source, edge.target)
  })
  dagre.layout(graph)
  const laidOut = targets.map((node) => {
    const position = graph.node(node.id) as { x: number; y: number }
    return { id: node.id, x: position.x - node.size.width / 2, y: position.y - node.size.height / 2 }
  })
  const originalBounds = computeCanvasBounds(targets)
  const rawMinX = Math.min(...laidOut.map((item) => item.x))
  const rawMinY = Math.min(...laidOut.map((item) => item.y))
  const positions = new Map(laidOut.map((item) => [item.id, { x: item.x - rawMinX + originalBounds.x, y: item.y - rawMinY + originalBounds.y }]))
  return {
    ...document,
    nodes: document.nodes.map((node) => selected.has(node.id) ? { ...node, position: positions.get(node.id) ?? node.position } : node),
  }
}

export function computeCanvasBounds(nodes: CanvasNode[]): CanvasBounds {
  if (!nodes.length) return { x: -500, y: -500, width: 1000, height: 1000 }
  const x = Math.min(...nodes.map((node) => node.position.x))
  const y = Math.min(...nodes.map((node) => node.position.y))
  const right = Math.max(...nodes.map((node) => node.position.x + node.size.width))
  const bottom = Math.max(...nodes.map((node) => node.position.y + node.size.height))
  return { x, y, width: Math.max(1, right - x), height: Math.max(1, bottom - y) }
}

export function fitCanvasViewport(bounds: CanvasBounds, viewport: ViewportSize, padding = 48): CanvasViewport {
  const usableWidth = Math.max(1, viewport.width - padding * 2)
  const usableHeight = Math.max(1, viewport.height - padding * 2)
  const zoom = clamp(Math.min(usableWidth / bounds.width, usableHeight / bounds.height), 0.05, 2)
  return {
    x: viewport.width / 2 - (bounds.x + bounds.width / 2) * zoom,
    y: viewport.height / 2 - (bounds.y + bounds.height / 2) * zoom,
    zoom,
  }
}

export function minimapGeometry(nodes: CanvasNode[], viewport: CanvasViewport, viewportSize: ViewportSize, minimapSize: ViewportSize) {
  const bounds = computeCanvasBounds(nodes)
  const padding = 8
  const scale = Math.min((minimapSize.width - padding * 2) / bounds.width, (minimapSize.height - padding * 2) / bounds.height)
  const offsetX = (minimapSize.width - bounds.width * scale) / 2
  const offsetY = (minimapSize.height - bounds.height * scale) / 2
  const project = (x: number, y: number) => ({ x: (x - bounds.x) * scale + offsetX, y: (y - bounds.y) * scale + offsetY })
  const topLeft = project(-viewport.x / viewport.zoom, -viewport.y / viewport.zoom)
  return {
    bounds,
    scale,
    offset: { x: offsetX, y: offsetY },
    nodes: nodes.map((node) => {
      const point = project(node.position.x, node.position.y)
      return { id: node.id, type: node.type, x: point.x, y: point.y, width: Math.max(2, node.size.width * scale), height: Math.max(2, node.size.height * scale) }
    }),
    viewport: {
      x: topLeft.x,
      y: topLeft.y,
      width: Math.max(4, viewportSize.width / viewport.zoom * scale),
      height: Math.max(4, viewportSize.height / viewport.zoom * scale),
    },
  }
}

function clamp(value: number, minimum: number, maximum: number) {
  return Math.max(minimum, Math.min(maximum, value))
}
