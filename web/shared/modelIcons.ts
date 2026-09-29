// Built-in model icon registry shared by the admin console (picker) and the
// user web (rendering). Keys are admin-selectable slugs; the SVG documents
// are monochrome currentColor marks. Missing icon_key + icon_svg renders the
// site favicon (defaultIconKey).
export type ModelIconDescriptor = { key: string; label: string; svg: string }

const icon = (key: string, label: string, body: string): ModelIconDescriptor => ({
  key,
  label,
  svg: `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round">${body}</svg>`,
})

export const builtinModelIcons: ModelIconDescriptor[] = [
  icon('chatgpt', 'ChatGPT', '<path d="M12 3l3.2 1.85 3.2 1.85.05 3.7L18.5 14l-3.3 1.9-3.2 1.85-3.2-1.85L5.5 14l-.05-3.6.05-3.7 3.2-1.85L12 3z"/><path d="M12 8.2l1.9 1.1v2.2L12 12.6l-1.9-1.1V9.3L12 8.2z"/>'),
  icon('grok', 'Grok', '<path d="M5 19L19 5"/><path d="M5 5v6h6"/><path d="M19 19v-6h-6"/>'),
  icon('banana', 'Banana', '<path d="M4.5 14.5c-.8-4.8 2.4-9.3 7.5-10 3.6-.5 7 1 8.5 3.5-2 .3-4.2 1.2-6 2.7-2.6 2.1-4.4 5-6.5 6.8-1.2 1-2.4 1.4-3.5 1.4-.3-1.3-.4-2.9 0-4.4z"/><path d="M4.9 18.9c1-.1 2.1-.6 3.1-1.4"/>'),
  icon('seedance', 'Seedance', '<circle cx="12" cy="12" r="8.5"/><path d="M12 3.5v17M3.5 12h17"/><circle cx="12" cy="12" r="3.2"/>'),
  icon('bailian-wan', '百炼万相', '<path d="M4 17c2.8 0 3.2-10 6-10s3.2 10 6 10c1.5 0 2.5-1.6 3.4-3.6"/><circle cx="19.6" cy="10.2" r="1.6"/>'),
  icon('minimax', 'MiniMax', '<rect x="3.5" y="3.5" width="7" height="7" rx="1.5"/><rect x="13.5" y="3.5" width="7" height="7" rx="1.5"/><rect x="3.5" y="13.5" width="7" height="7" rx="1.5"/><path d="M16.5 13.8l3.4 6.4h-6.8l3.4-6.4z"/>'),
  icon('hailuo', '海螺视频', '<path d="M4 13c0-4.4 3.6-8 8-8 3.9 0 7 2.5 7 5.6 0 2.4-1.9 4.4-4.3 4.4-1.9 0-3.2-1.2-3.2-2.8 0-1.2.9-2.2 2.1-2.2"/><path d="M4 13c.6 3.9 3.6 6.5 7.4 6.9"/>'),
  icon('gemini', 'Gemini', '<path d="M12 3l1.8 5.2L19 10l-5.2 1.8L12 17l-1.8-5.2L5 10l5.2-1.8L12 3z"/><path d="M18.5 15.5l.7 2 2 .7-2 .7-.7 2-.7-2-2-.7 2-.7.7-2z"/>'),
]

export const defaultIconKey = 'site'

export const siteIcon: ModelIconDescriptor = {
  key: defaultIconKey,
  label: '站点默认',
  svg: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6"><circle cx="12" cy="12" r="9"/><path d="M8 12a4 4 0 0 1 8 0 4 4 0 0 1-8 0z" fill="currentColor" stroke="none"/></svg>',
}

export function builtinModelIcon(key: string): ModelIconDescriptor | undefined {
  if (key === defaultIconKey) return siteIcon
  return builtinModelIcons.find((item) => item.key === key)
}

// svgToDataURI escapes an SVG document for safe <img src> embedding.
export function svgToDataURI(svg: string): string {
  return `data:image/svg+xml;utf8,${encodeURIComponent(svg.trim())}`
}

// modelIconSrc resolves the render source for an icon pair: uploaded SVG
// wins, then the built-in key, then the site fallback.
export function modelIconSrc(iconKey?: string, iconSvg?: string): string {
  if (iconSvg && iconSvg.trim()) return svgToDataURI(iconSvg)
  const builtin = iconKey ? builtinModelIcon(iconKey.trim()) : undefined
  return svgToDataURI(builtin?.svg ?? siteIcon.svg)
}
