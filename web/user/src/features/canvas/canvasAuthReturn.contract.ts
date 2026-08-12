import fs from 'node:fs'

const source = fs.readFileSync(new URL('../../App.tsx', import.meta.url), 'utf8')
if (!source.includes("canvasId: currentRoute === 'creative-canvas' ? routeCanvasId : undefined")) throw new Error('session expiry must preserve canvas_id')
if (!source.includes("canvasId: route === 'creative-canvas' ? routeCanvasId : undefined")) throw new Error('unauthenticated redirects must preserve canvas_id')
if (!source.includes("canvasId: destination === 'creative-canvas' ? routeCanvasId : undefined")) throw new Error('successful login must restore the canvas detail route')
