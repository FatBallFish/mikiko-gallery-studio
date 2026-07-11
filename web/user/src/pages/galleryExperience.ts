export function galleryImageAspect(input: { width?: number; height?: number; aspectRatio?: string }) {
  if ((input.width ?? 0) > 0 && (input.height ?? 0) > 0) return `${input.width} / ${input.height}`
  const match = input.aspectRatio?.trim().match(/^(\d+(?:\.\d+)?)\s*:\s*(\d+(?:\.\d+)?)$/)
  if (match && Number(match[1]) > 0 && Number(match[2]) > 0) return `${match[1]} / ${match[2]}`
  return '4 / 3'
}

export function toggleGalleryImageSelection(current: ReadonlySet<string>, imageID: string, checked?: boolean) {
  const next = new Set(current)
  const shouldSelect = checked ?? !next.has(imageID)
  if (shouldSelect) next.add(imageID)
  else next.delete(imageID)
  return next
}

export function selectVisibleGalleryImages(_current: ReadonlySet<string>, visibleIDs: string[], selected: boolean) {
  return selected ? new Set(visibleIDs) : new Set<string>()
}
