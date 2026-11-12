import { createApp } from 'vue'
import { createPinia } from 'pinia'
import ElementPlus from 'element-plus'
import 'element-plus/dist/index.css'
import App from './App.vue'
import router from './router'
import { i18n } from './i18n'
import './styles/index.css'
import './styles/light.css'
// 自托管字体（fontsource）：替代 index.html 中的 Google Fonts CDN 引入，
// 字体重量的 woff2 文件随 vite 构建打进 dist/assets/，内网/离线环境可用。
// 字体栈在 styles/tokens.css 中定义，family 名与原来保持一致。
import '@fontsource/chakra-petch/400.css'
import '@fontsource/chakra-petch/500.css'
import '@fontsource/chakra-petch/600.css'
import '@fontsource/chakra-petch/700.css'
import '@fontsource/outfit/300.css'
import '@fontsource/outfit/400.css'
import '@fontsource/outfit/500.css'
import '@fontsource/outfit/600.css'
import '@fontsource/outfit/700.css'
import '@fontsource/jetbrains-mono/400.css'
import '@fontsource/jetbrains-mono/500.css'
import '@fontsource/jetbrains-mono/600.css'
import '@fontsource/jetbrains-mono/700.css'
// Side-effect: applies the persisted theme to <html data-theme> before mount.
import './composables/useTheme'

const app = createApp(App)
app.use(createPinia())
app.use(router)
app.use(ElementPlus)
app.use(i18n)
app.mount('#app')
