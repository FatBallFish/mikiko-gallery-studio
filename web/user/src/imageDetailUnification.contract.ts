import { readFileSync } from 'node:fs'

const read = (path: string) => readFileSync(new URL(path, import.meta.url), 'utf8')
const components = read('./components.tsx')
const productionPages = [
  './pages/HomePage.tsx',
  './pages/WorkspacePage.tsx',
  './pages/GalleryPage.tsx',
  './pages/PublicGalleryPage.tsx',
]

if (components.includes('export function ImageLightbox')) {
  throw new Error('the legacy detail-style ImageLightbox should be removed after callers migrate')
}
for (const page of productionPages) {
  const source = read(page)
  if (source.includes('ImageLightbox')) {
    throw new Error(`${page} should use the shared ImageDetailModal instead of ImageLightbox`)
  }
  if (!source.includes('ImageDetailModal')) {
    throw new Error(`${page} should render the shared ImageDetailModal`)
  }
}

const modalStart = components.indexOf('export function ImageDetailModal')
const modalEnd = components.indexOf('\nexport function PublicImageDetail', modalStart)
const modalSource = components.slice(modalStart, modalEnd)
for (const required of ['setZoomImage', '<ImageZoomViewer', 'onPreviewImage={setZoomImage}']) {
  if (!modalSource.includes(required)) {
    throw new Error(`ImageDetailModal should open zoom directly from its image: ${required}`)
  }
}

if (!components.includes('className="u-prompt"') || !components.includes('tabIndex={0}') || !components.includes('u-detail')) {
  throw new Error('long prompts should use the bounded, scrollable, keyboard-focusable u-prompt region inside u-detail')
}

if (!components.includes("source: '原图引用'")) {
  throw new Error('reference images should open the shared zoom viewer directly')
}
