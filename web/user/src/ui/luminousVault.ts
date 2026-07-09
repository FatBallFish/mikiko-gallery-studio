export const luminousRadii = {
  sm: 8,
  md: 12,
  lg: 16,
  xl: 24,
} as const

export const luminousMotion = {
  instantMs: 80,
  fastMs: 140,
  routeMs: 220,
  slowMs: 480,
  easeOut: 'cubic-bezier(0.16, 1, 0.3, 1)',
  easeSpring: 'cubic-bezier(0.32, 0.72, 0, 1)',
} as const

export const luminousShell = {
  sidebarPx: 108,
  topbarPx: 76,
} as const

const chineseSansFallback = '"Noto Sans SC", "PingFang SC", "Microsoft YaHei", sans-serif'

export const luminousType = {
  display: `"Satoshi", ${chineseSansFallback}`,
  ui: `"Satoshi", ${chineseSansFallback}`,
  mono: '"JetBrains Mono", "SFMono-Regular", Consolas, monospace',
} as const
