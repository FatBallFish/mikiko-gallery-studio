import { defineConfig, loadEnv } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), '')
  const apiTarget = env.VITE_API_PROXY_TARGET || env.VITE_API_BASE_URL || 'http://127.0.0.1:8080'
  const base = env.VITE_BASE_PATH || '/'

  return {
    base,
    build: {
      manifest: true,
    },
    plugins: [tailwindcss(), react()],
    server: {
      fs: { allow: ['..'] },
      proxy: {
        '/api': {
          target: apiTarget,
          // Keep the browser Host header so the API derives the public origin
          // (e.g. the OIDC redirect_uri) from the page the user is on.
          changeOrigin: false,
        },
        // Exact public protocol routes: never send browser consent or other
        // SPA paths to the API. Keep Host while PublicAPIURL remains canonical.
        '^/mcp(?:\\?.*)?$': { target: apiTarget, changeOrigin: false },
        '^/\\.well-known/oauth-(?:authorization-server|protected-resource)(?:/mcp)?(?:\\?.*)?$': { target: apiTarget, changeOrigin: false },
        '^/oauth/(?:authorize|token|register|revoke)(?:\\?.*)?$': { target: apiTarget, changeOrigin: false },
        '/docs': {
          target: apiTarget,
          changeOrigin: false,
        },
      },
    },
  }
})
