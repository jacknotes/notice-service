import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import VueI18nPlugin from '@intlify/unplugin-vue-i18n/vite'
import { fileURLToPath, URL } from 'node:url'
import { dirname, join } from 'node:path'

const root = dirname(fileURLToPath(import.meta.url))

export default defineConfig({
  plugins: [
    vue(),
    // 预编译 locale 消息：构建期把 JSON 消息编译为消息函数，运行时不再
    // 走 vue-i18n 的 JIT 编译（new Function），从而兼容后端严格 CSP
    // （script-src 不含 'unsafe-eval'），见 internal/router/router.go。
    VueI18nPlugin({
      include: [join(root, 'src/locales/**')],
      // 文案里含 <api_key>、<unix 秒> 等尖括号字面量（非 HTML），
      // 关闭 strictMessage 的 HTML 误判检查；文案均为自有内容且按纯文本渲染。
      strictMessage: false,
      // AOT 预编译为消息函数（而非 AST + 运行时 JIT），
      // 运行时完全不需要消息编译器，严格 CSP 下可用。
      jitCompilation: false,
    }),
  ],
  resolve: { alias: { '@': fileURLToPath(new URL('./src', import.meta.url)) } },
  server: { port: 5173, proxy: { '/api': 'http://127.0.0.1:8080', '/swagger': 'http://127.0.0.1:8080' } },
  build: { outDir: 'dist' }
})
