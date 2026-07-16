import { shouldStartZoomDrag } from './zoomPointer'

const interactive = {
  button: 0,
  target: { closest: (selector: string) => selector.includes('button') ? {} : null },
}
if (shouldStartZoomDrag(interactive)) {
  throw new Error('zoom dragging must not capture pointer events from retry or toolbar buttons')
}

const secondary = {
  button: 2,
  target: { closest: () => null },
}
if (shouldStartZoomDrag(secondary)) {
  throw new Error('zoom dragging must ignore non-primary pointers')
}

const stage = {
  button: 0,
  target: { closest: () => null },
}
if (!shouldStartZoomDrag(stage)) {
  throw new Error('zoom dragging must start on the non-interactive image stage')
}
