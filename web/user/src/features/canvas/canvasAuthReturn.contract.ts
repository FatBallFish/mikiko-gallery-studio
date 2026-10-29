import fs from 'node:fs'

const source = fs.readFileSync(new URL('../../App.tsx', import.meta.url), 'utf8')
if (!source.includes("canvasId: currentRoute === 'creative-canvas' ? routeCanvasId : undefined")) throw new Error('session expiry must preserve canvas_id')
if (!source.includes("canvasId: route === 'creative-canvas' ? routeCanvasId : undefined")) throw new Error('unauthenticated redirects must preserve canvas_id')
if (!source.includes("canvasId: destination === 'creative-canvas' ? routeCanvasId : undefined")) throw new Error('successful login must restore the canvas detail route')
const editorRoute = source.match(/<CanvasEditorRoute\b[^\n]*?\/>/)?.[0]
if (!editorRoute) throw new Error('canvas detail route must render CanvasEditorRoute')
if (!editorRoute.includes("pageProps={{ canvasID: routeCanvasId, onBack: () => writeHash('creative-canvas') }}")) {
  throw new Error('canvas editor page props must retain the canvas ID and return to the canvas list')
}
if (!editorRoute.includes("}} onBack={() => writeHash('creative-canvas')}")) {
  throw new Error('canvas route failure fallback back action must return to the canvas list')
}
