import type { Plugin } from 'vite'

/** The config hook sees CLI server overrides after Vite merges inline config. */
export function isolateDevCache(): Plugin {
  return {
    name: 'user-dev-cache-isolation',
    apply: 'serve',
    config(config) {
      if (config.cacheDir !== undefined) return
      const port = config.server?.port ?? 5173
      // Non-strict ports may increment, and port zero is assigned by the OS.
      // Keep their caches process-local without changing server port behavior.
      const suffix = port !== 0 && config.server?.strictPort === true ? '' : `-pid-${process.pid}`
      return { cacheDir: `node_modules/.vite/dev-${port}${suffix}` }
    },
  }
}
