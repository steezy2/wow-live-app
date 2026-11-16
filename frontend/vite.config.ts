import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// The Go site (backend/cmd/site) serves the built page and the API on port 8080; `npm run dev` proxies the API to it.
export default defineConfig({
  plugins: [react()],
  // The API refuses an Origin that isn't its own host, and the proxy changes Host but not Origin.
  server: {
    proxy: {
      '/api': { target: 'http://localhost:8080', changeOrigin: true, headers: { Origin: 'http://localhost:8080' } },
    },
  },
})
