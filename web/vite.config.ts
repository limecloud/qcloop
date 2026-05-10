import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import { copyFileSync, existsSync, mkdirSync, readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import type { Plugin, ResolvedConfig } from 'vite'

const llmTextRoutes = new Map([
  ['/llm.txt', 'llms.txt'],
  ['/llms.txt', 'llms.txt'],
  ['/llm-full.txt', 'llms-full.txt'],
  ['/llms-full.txt', 'llms-full.txt'],
])

function llmTextFilesPlugin(): Plugin {
  let config: ResolvedConfig

  return {
    name: 'qcloop-llm-text-files',
    configResolved(resolvedConfig) {
      config = resolvedConfig
    },
    configureServer(server) {
      server.middlewares.use((req, res, next) => {
        if (req.method !== 'GET' && req.method !== 'HEAD') {
          next()
          return
        }

        const pathname = new URL(req.url || '/', 'http://localhost').pathname
        const sourceName = llmTextRoutes.get(pathname)
        if (!sourceName) {
          next()
          return
        }

        const sourcePath = resolve(config.root, '..', sourceName)
        if (!existsSync(sourcePath)) {
          res.statusCode = 404
          res.end('Not found')
          return
        }

        res.setHeader('Content-Type', 'text/plain; charset=utf-8')
        res.setHeader('Cache-Control', 'no-cache')
        if (req.method === 'HEAD') {
          res.end()
          return
        }
        res.end(readFileSync(sourcePath, 'utf8'))
      })
    },
    writeBundle() {
      for (const [route, sourceName] of llmTextRoutes) {
        const sourcePath = resolve(config.root, '..', sourceName)
        if (!existsSync(sourcePath)) continue
        const targetPath = resolve(config.root, config.build.outDir, route.slice(1))
        mkdirSync(dirname(targetPath), { recursive: true })
        copyFileSync(sourcePath, targetPath)
      }
    },
  }
}

export default defineConfig({
  plugins: [llmTextFilesPlugin(), react()],
  server: {
    port: 3000,
    proxy: {
      '/api': {
        target: 'http://localhost:8080',
        changeOrigin: true,
      },
      // WebSocket 代理:dev 模式下 frontend 3000 → backend 8080
      '/ws': {
        target: 'ws://localhost:8080',
        ws: true,
        changeOrigin: true,
      },
    },
  },
})
