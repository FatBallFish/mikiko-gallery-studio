import { createStore } from 'zustand/vanilla'
import {
  addCanvasNode,
  acknowledgeCanvasSave,
  attachCanvasResults,
  connectCanvasNodes,
  copyCanvasSelection,
  createCanvasState,
  markCanvasSaved,
  moveCanvasNodes,
  pasteCanvasSelection,
  redoCanvasCommand,
  removeCanvasEdges,
  removeCanvasNodes,
  resizeCanvasNode,
  undoCanvasCommand,
  updateCanvasNode,
} from '../core/canvasState'
import { autoLayoutCanvasNodes } from '../core/canvasLayout'
import type { CanvasClipboard, CanvasCommandState, CanvasDocument, CanvasEdge, CanvasNode, CanvasResult, CanvasViewport } from '../core/types'

export type CanvasInteractionMode = 'select' | 'pan' | 'connect'
export type CanvasStoreState = {
  command: CanvasCommandState
  selectedIDs: string[]
  selectedEdgeIDs: string[]
  mode: CanvasInteractionMode
  clipboard: CanvasClipboard | null
  pasteCount: number
  setMode: (mode: CanvasInteractionMode) => void
  select: (nodeIDs: string[]) => void
  selectEdges: (edgeIDs: string[]) => void
  setViewport: (viewport: CanvasViewport) => void
  addNode: (node: CanvasNode) => void
  moveSelected: (delta: { x: number; y: number }) => void
  resizeNode: (nodeID: string, size: { width: number; height: number }) => void
  updateNode: (nodeID: string, update: (node: CanvasNode) => CanvasNode) => void
  connect: (edge: CanvasEdge) => void
  copySelected: () => void
  pasteClipboard: () => void
  deleteSelected: () => void
  attachResults: (runID: string, sourceNodeID: string, results: CanvasResult[]) => void
  autoLayoutSelected: () => void
  undo: () => void
  redo: () => void
  replaceRemote: (document: CanvasDocument, revision: number) => void
  markSaved: (document: CanvasDocument, revision: number) => void
  acknowledgeSave: (submitted: CanvasDocument, revision: number) => void
}

export function createCanvasStore(document: CanvasDocument, revision: number, options: { recoveredDraft?: boolean } = {}) {
  return createStore<CanvasStoreState>((set) => ({
    command: { ...createCanvasState(document, revision), dirty: Boolean(options.recoveredDraft) },
    selectedIDs: [],
    selectedEdgeIDs: [],
    mode: 'select',
    clipboard: null,
    pasteCount: 0,
    setMode: (mode) => set({ mode }),
    select: (selectedIDs) => set({ selectedIDs: Array.from(new Set(selectedIDs)), selectedEdgeIDs: [] }),
    selectEdges: (selectedEdgeIDs) => set({ selectedIDs: [], selectedEdgeIDs: Array.from(new Set(selectedEdgeIDs)) }),
    setViewport: (viewport) => set((state) => ({ command: { ...state.command, present: { ...state.command.present, viewport }, dirty: true } })),
    addNode: (node) => set((state) => ({ command: addCanvasNode(state.command, node), selectedIDs: [node.id], selectedEdgeIDs: [] })),
    moveSelected: (delta) => set((state) => ({ command: moveCanvasNodes(state.command, state.selectedIDs, delta) })),
    resizeNode: (nodeID, size) => set((state) => ({ command: resizeCanvasNode(state.command, nodeID, size) })),
    updateNode: (nodeID, update) => set((state) => ({ command: updateCanvasNode(state.command, nodeID, update) })),
    connect: (edge) => set((state) => ({ command: connectCanvasNodes(state.command, edge) })),
    copySelected: () => set((state) => ({ clipboard: copyCanvasSelection(state.command.present, state.selectedIDs), pasteCount: 0 })),
    pasteClipboard: () => set((state) => {
      if (!state.clipboard?.nodes.length) return state
      const pasteID = crypto.randomUUID().slice(0, 8)
      const command = pasteCanvasSelection(state.command, state.clipboard, { x: 32 * (state.pasteCount + 1), y: 32 * (state.pasteCount + 1) }, (sourceID) => `${sourceID}-copy-${pasteID}`)
      return { command, selectedIDs: command.present.nodes.slice(-state.clipboard.nodes.length).map((node) => node.id), selectedEdgeIDs: [], pasteCount: state.pasteCount + 1 }
    }),
    deleteSelected: () => set((state) => ({
      command: state.selectedEdgeIDs.length ? removeCanvasEdges(state.command, state.selectedEdgeIDs) : removeCanvasNodes(state.command, state.selectedIDs),
      selectedIDs: [],
      selectedEdgeIDs: [],
    })),
    attachResults: (runID, sourceNodeID, results) => set((state) => ({ command: attachCanvasResults(state.command, runID, sourceNodeID, results) })),
    autoLayoutSelected: () => set((state) => ({
      command: state.selectedIDs.length < 2 ? state.command : {
        ...state.command,
        present: autoLayoutCanvasNodes(state.command.present, state.selectedIDs),
        past: [...state.command.past, state.command.present].slice(-100),
        future: [],
        dirty: true,
      },
    })),
    undo: () => set((state) => ({ command: undoCanvasCommand(state.command) })),
    redo: () => set((state) => ({ command: redoCanvasCommand(state.command) })),
    replaceRemote: (nextDocument, nextRevision) => set({ command: createCanvasState(nextDocument, nextRevision), selectedIDs: [], selectedEdgeIDs: [], pasteCount: 0 }),
    markSaved: (nextDocument, nextRevision) => set((state) => ({ command: markCanvasSaved(state.command, nextDocument, nextRevision) })),
    acknowledgeSave: (submitted, nextRevision) => set((state) => ({ command: acknowledgeCanvasSave(state.command, submitted, nextRevision) })),
  }))
}
