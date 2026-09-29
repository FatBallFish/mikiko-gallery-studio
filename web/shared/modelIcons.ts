// Built-in model icon registry shared by the admin console (picker) and the
// user web (rendering). Keys are admin-selectable slugs; the documents are
// the vendors' official brand marks (ChatGPT/Grok/Gemini from simple-icons,
// 百炼万相 = Alibaba Cloud mark, Seedance = ByteDance mark, MiniMax and
// 海螺 keep brand-accurate two-tone marks since no official vector source is
// publicly indexed). Empty icon_key + icon_svg renders the site fallback.
export type ModelIconDescriptor = { key: string; label: string; svg: string }

function icon(key: string, label: string, svg: string): ModelIconDescriptor {
  return { key, label, svg }
}

export const builtinModelIcons: ModelIconDescriptor[] = [
  icon('chatgpt', 'ChatGPT', '<svg role="img" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg"><path d="M22.2819 9.8211a5.9847 5.9847 0 0 0-.5157-4.9108 6.0462 6.0462 0 0 0-6.5098-2.9A6.0651 6.0651 0 0 0 4.9807 4.1818a5.9847 5.9847 0 0 0-3.9977 2.9 6.0462 6.0462 0 0 0 .7427 7.0966 5.98 5.98 0 0 0 .511 4.9107 6.051 6.051 0 0 0 6.5146 2.9001A5.9847 5.9847 0 0 0 13.2599 24a6.0557 6.0557 0 0 0 5.7718-4.2058 5.9894 5.9894 0 0 0 3.9977-2.9001 6.0557 6.0557 0 0 0-.7475-7.0729zm-9.022 12.6081a4.4755 4.4755 0 0 1-2.8764-1.0408l.1419-.0804 4.7783-2.7582a.7948.7948 0 0 0 .3927-.6813v-6.7369l2.02 1.1686a.071.071 0 0 1 .038.052v5.5826a4.504 4.504 0 0 1-4.4945 4.4944zm-9.6607-4.1254a4.4708 4.4708 0 0 1-.5346-3.0137l.142.0852 4.783 2.7582a.7712.7712 0 0 0 .7806 0l5.8428-3.3685v2.3324a.0804.0804 0 0 1-.0332.0615L9.74 19.9502a4.4992 4.4992 0 0 1-6.1408-1.6464zM2.3408 7.8956a4.485 4.485 0 0 1 2.3655-1.9728V11.6a.7664.7664 0 0 0 .3879.6765l5.8144 3.3543-2.0201 1.1685a.0757.0757 0 0 1-.071 0l-4.8303-2.7865A4.504 4.504 0 0 1 2.3408 7.872zm16.5963 3.8558L13.1038 8.364 15.1192 7.2a.0757.0757 0 0 1 .071 0l4.8303 2.7913a4.4944 4.4944 0 0 1-.6765 8.1042v-5.6772a.79.79 0 0 0-.407-.667zm2.0107-3.0231l-.142-.0852-4.7735-2.7818a.7759.7759 0 0 0-.7854 0L9.409 9.2297V6.8974a.0662.0662 0 0 1 .0284-.0615l4.8303-2.7866a4.4992 4.4992 0 0 1 6.6802 4.66zM8.3065 12.863l-2.02-1.1638a.0804.0804 0 0 1-.038-.0567V6.0742a4.4992 4.4992 0 0 1 7.3757-3.4537l-.142.0805L8.704 5.459a.7948.7948 0 0 0-.3927.6813zm1.0976-2.3654l2.602-1.4998 2.6069 1.4998v2.9994l-2.5974 1.4997-2.6067-1.4997Z"/></svg>'),
  icon('grok', 'Grok', '<svg role="img" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg"><path d="M14.234 10.162 22.977 0h-2.072l-7.591 8.824L7.251 0H.258l9.168 13.343L.258 24H2.33l8.016-9.318L16.749 24h6.993zm-2.837 3.299-.929-1.329L3.076 1.56h3.182l5.965 8.532.929 1.329 7.754 11.09h-3.182z"/></svg>'),
  icon('gemini', 'Gemini', '<svg role="img" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg"><path d="M11.04 19.32Q12 21.51 12 24q0-2.49.93-4.68.96-2.19 2.58-3.81t3.81-2.55Q21.51 12 24 12q-2.49 0-4.68-.93a12.3 12.3 0 0 1-3.81-2.58 12.3 12.3 0 0 1-2.58-3.81Q12 2.49 12 0q0 2.49-.96 4.68-.93 2.19-2.55 3.81a12.3 12.3 0 0 1-3.81 2.58Q2.49 12 0 12q2.49 0 4.68.96 2.19.93 3.81 2.55t2.55 3.81"/></svg>'),
  icon('bailian-wan', '百炼万相', '<svg role="img" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg"><path d="M3.996 4.517h5.291L8.01 6.324 4.153 7.506a1.668 1.668 0 0 0-1.165 1.601v5.786a1.668 1.668 0 0 0 1.165 1.6l3.857 1.183 1.277 1.807H3.996A3.996 3.996 0 0 1 0 15.487V8.513a3.996 3.996 0 0 1 3.996-3.996m16.008 0h-5.291l1.277 1.807 3.857 1.182c.715.227 1.17.889 1.165 1.601v5.786a1.668 1.668 0 0 1-1.165 1.6l-3.857 1.183-1.277 1.807h5.291A3.996 3.996 0 0 0 24 15.487V8.513a3.996 3.996 0 0 0-3.996-3.996m-4.007 8.345H8.002v-1.804h7.995Z"/></svg>'),
  icon('seedance', 'Seedance', '<svg role="img" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg"><path d="M19.8772 1.4685L24 2.5326v18.9426l-4.1228 1.0563V1.4685zm-13.3481 9.428l4.115 1.0641v8.9786l-4.115 1.0642v-11.107zM0 2.572l4.115 1.0642v16.7354L0 21.428V2.572zm17.4553 5.6205v11.107l-4.1228-1.0642V9.2568l4.1228-1.0642z"/></svg>'),
  icon('minimax', 'MiniMax', '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><path fill="#A55CFF" d="M4 18.5 9.5 5l2.5 8 2.5-8 5.5 13.5h-2.2l-3.3-8.5-2.5 8.5h-2l-2.5-8.5-3.3 8.5z"/></svg>'),
  icon('hailuo', '海螺视频', '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><path fill="#38B2E8" d="M19 5c-4 0-7.5 2.6-8.8 6.3-.5 1.4-1.3 2.7-2.7 2.7-1.6 0-2.5-1.4-2.5-3.2C5 8 7 6 9.4 6c.9 0 1.8.2 2.6.5C13.4 4 15.8 3 18 3c2.5 0 4.5 1.4 4.5 3.5 0 2.9-2.7 4.5-5.5 4.5-1.4 0-2.4-.5-2.4-1.8 0-1.1 1-2 2.1-2"/></svg>'),
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
// wins, then the built-in key, then the site fallback. When a model carries
// no icon but lives in a group with one, the group icon shows instead
// (callers pass the group's icon pair as the fallback).
export function modelIconSrc(iconKey?: string, iconSvg?: string, fallbackKey?: string, fallbackSvg?: string): string {
  if (iconSvg && iconSvg.trim()) return svgToDataURI(iconSvg)
  const builtin = iconKey ? builtinModelIcon(iconKey.trim()) : undefined
  if (builtin) return svgToDataURI(builtin.svg)
  if (fallbackSvg && fallbackSvg.trim()) return svgToDataURI(fallbackSvg)
  const fallback = fallbackKey ? builtinModelIcon(fallbackKey.trim()) : undefined
  return svgToDataURI(fallback?.svg ?? siteIcon.svg)
}
