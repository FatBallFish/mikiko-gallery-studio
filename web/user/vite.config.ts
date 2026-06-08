import { defineConfig, loadEnv } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), '')
  const apiTarget = env.VITE_API_PROXY_TARGET || env.VITE_API_BASE_URL || 'http://127.0.0.1:8080'
  const base = env.VITE_BASE_PATH || '/'

  return {
    base,
    plugins: [tailwindcss(), react()],
    server: {
      fs: { allow: ['..'] },
      proxy: {
        '/api': {
          target: apiTarget,
          changeOrigin: true,
        },
        '/docs': {
          target: apiTarget,
          changeOrigin: true,
        },
      },
    },
  }
})
