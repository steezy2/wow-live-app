import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// The Go site (backend/cmd/site) serves the built page and the API on port 8080; `npm run dev` proxies the API to it.
export default defineConfig({
  plugins: [react()],
  server: { proxy: { '/api': 'http://localhost:8080' } },
})
