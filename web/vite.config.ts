import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'
import fs from 'node:fs'
import path from 'node:path'

function changelogPlugin() {
  const virtualModuleId = 'virtual:changelog'
  const resolvedVirtualModuleId = '\0' + virtualModuleId

  return {
    name: 'vite-plugin-changelog',
    resolveId(id: string) {
      if (id === virtualModuleId) {
        return resolvedVirtualModuleId
      }
    },
    load(id: string) {
      if (id === resolvedVirtualModuleId) {
        const candidates = [
          path.resolve(__dirname, '../CHANGELOG.md'),
          path.resolve(__dirname, './CHANGELOG.md'),
          path.resolve(process.cwd(), '../CHANGELOG.md'),
          path.resolve(process.cwd(), './CHANGELOG.md'),
        ]
        for (const p of candidates) {
          if (fs.existsSync(p)) {
            const content = fs.readFileSync(p, 'utf-8')
            return `export default ${JSON.stringify(content)};`
          }
        }
        return `export default "";`
      }
    },
  }
}

// base: './' makes asset URLs relative so the SPA works under the per-install
// secret path; the Go server injects <base href="/<secret>/"> at serve time.
export default defineConfig({
  base: './',
  plugins: [react(), tailwindcss(), changelogPlugin()],
  build: {
    outDir: 'dist',
    emptyOutDir: true,
  },
})
