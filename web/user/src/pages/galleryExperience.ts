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

export function selectVisibleGalleryImages(current: ReadonlySet<string>, visibleIDs: string[], selected: boolean) {
	const next = pruneGallerySelection(current, visibleIDs)
  visibleIDs.forEach((id) => {
    if (selected) next.add(id)
    else next.delete(id)
  })
  return next
}

export function pruneGallerySelection(current: ReadonlySet<string>, visibleIDs: string[]) {
	const visible = new Set(visibleIDs)
	const next = new Set(Array.from(current).filter((id) => visible.has(id)))
	if (next.size === current.size && Array.from(next).every((id) => current.has(id))) {
		return current instanceof Set ? current : next
	}
	return next
}

export function selectedVisibleGalleryItems<T extends { id: string }>(rows: T[], selectedIds: ReadonlySet<string>) {
  return rows.filter((row) => selectedIds.has(row.id))
}

export function areAllVisibleGalleryItemsSelected(rows: Array<{ id: string }>, selectedIds: ReadonlySet<string>) {
  return rows.length > 0 && rows.every((row) => selectedIds.has(row.id))
}

export type GallerySelectionPoint = { x: number; y: number }
export type GallerySelectionRect = { left: number; top: number; right: number; bottom: number }

export function gallerySelectionDragDistance(start: GallerySelectionPoint, current: GallerySelectionPoint) {
  return Math.hypot(current.x - start.x, current.y - start.y)
}

export function gallerySelectionRectangle(start: GallerySelectionPoint, current: GallerySelectionPoint): GallerySelectionRect {
  return {
    left: Math.min(start.x, current.x),
    top: Math.min(start.y, current.y),
    right: Math.max(start.x, current.x),
    bottom: Math.max(start.y, current.y),
  }
}

export function galleryMarqueeSelection(
  current: ReadonlySet<string>,
  items: ReadonlyArray<{ id: string; rect: GallerySelectionRect }>,
  marquee: GallerySelectionRect,
  additive: boolean,
) {
  const next = additive ? new Set(current) : new Set<string>()
  items.forEach(({ id, rect }) => {
    if (rect.right >= marquee.left && rect.left <= marquee.right && rect.bottom >= marquee.top && rect.top <= marquee.bottom) next.add(id)
  })
  return next
}

export function gallerySelectionClickAction(selectedCount: number): 'toggle' | 'open' {
  return selectedCount > 0 ? 'toggle' : 'open'
}
