import { fileURLToPath, URL } from 'node:url'
import { defineConfig, type Plugin } from 'vite'
import vue from '@vitejs/plugin-vue'

/**
 * 开发服务器把 Go 服务端的页面路径映射到对应入口，
 * 这样 npm run dev 下登录页/引导页的跳转（/、/login、/setup）也能直接工作。
 */
function devPageRoutes(): Plugin {
  const pages: Record<string, string> = {
    '/': '/static/index.html',
    '/login': '/static/login.html',
    '/setup': '/static/setup.html',
  }
  return {
    name: 'auto-anime-dev-pages',
    apply: 'serve',
    configureServer(server) {
      server.middlewares.use((req, _res, next) => {
        const url = req.url ?? ''
        const path = url.split('?')[0] ?? ''
        const target = pages[path]
        if (target) req.url = target + url.slice(path.length)
        next()
      })
    },
  }
}

// 构建产物直接输出到仓库根目录的 static/，Go 服务端与 Dockerfile 无需改动：
//   /            -> static/index.html   （管理面板）
//   /login       -> static/login.html   （登录页）
//   /setup       -> static/setup.html   （首次安装引导）
//   /static/...  -> 其余 JS/CSS/图标资源
export default defineConfig({
  plugins: [vue(), devPageRoutes()],
  base: '/static/',
  resolve: {
    alias: { '@': fileURLToPath(new URL('./src', import.meta.url)) },
  },
  server: {
    port: 5173,
    // 本地联调：先启动 Go 服务（默认 8000），再 npm run dev 访问 http://127.0.0.1:5173/
    proxy: {
      '/api': { target: 'http://127.0.0.1:8000', changeOrigin: false },
    },
  },
  build: {
    outDir: '../static',
    emptyOutDir: true,
    target: 'es2020',
    cssTarget: 'chrome90',
    rollupOptions: {
      input: {
        index: fileURLToPath(new URL('./index.html', import.meta.url)),
        login: fileURLToPath(new URL('./login.html', import.meta.url)),
        setup: fileURLToPath(new URL('./setup.html', import.meta.url)),
      },
    },
  },
})
