import { readFileSync } from 'node:fs'

const read = (file: string) => readFileSync(new URL(file, import.meta.url), 'utf8')
const videoPanel = read('./features/creation/VideoCreationPanel.tsx')
const mediaCard = read('./features/media/MediaAssetCard.tsx')
const mediaPreview = read('./features/media/MediaPreviewDialog.tsx')

// Media elements cannot attach an Authorization header, so every relative
// media-asset URL must carry the access token in the query string.
const required = [
  [videoPanel, 'VideoCreationPanel', 'userApi.mediaAssetUrl(access.url, app.session?.token)'],
  [mediaCard, 'MediaAssetCard', 'userApi.mediaAssetUrl(posterURL, app.session?.token)'],
  [mediaCard, 'MediaAssetCard', 'userApi.mediaAssetUrl(hoverURL, app.session?.token)'],
  [mediaCard, 'MediaAssetCard', 'userApi.mediaAssetUrl(audioURL, app.session?.token)'],
  [mediaPreview, 'MediaPreviewDialog', "const mediaURL = (url?: string) => url ? userApi.mediaAssetUrl(url, app.session?.token) : ''"],
  [mediaPreview, 'MediaPreviewDialog', 'userApi.mediaAssetUrl(access.url, app.session?.token)'],
]

for (const [source, file, snippet] of required) {
  if (!String(source).includes(snippet)) {
    throw new Error(`${file} must authenticate media URLs with the session token: ${snippet}`)
  }
}