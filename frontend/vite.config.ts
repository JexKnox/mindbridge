import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// 开发时前端跑在 :5173,/api 与 /healthz 反代到后端 :8080,避免跨域。
// 生产构建产物 frontend/dist 由 Go 服务直接托管(见 internal/server/server.go)。
export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    proxy: {
      '/api': 'http://localhost:8080',
      '/healthz': 'http://localhost:8080',
    },
  },
  build: {
    outDir: 'dist',
    emptyOutDir: true,
  },
})