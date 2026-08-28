import { createI18n } from 'vue-i18n'
import zhCN from '@/locales/zh-CN.json'
import enUS from '@/locales/en-US.json'

export const STORAGE_KEY = 'i18n-locale'

// locale 消息源文件为 JSON，由 @intlify/unplugin-vue-i18n 在构建期预编译
// （见 vite.config.ts），运行时直接使用编译产物，不做 JIT 编译。
export type MessageSchema = typeof zhCN

function initialLocale(): 'zh-CN' | 'en-US' {
  try {
    return localStorage.getItem(STORAGE_KEY) === 'en-US' ? 'en-US' : 'zh-CN'
  } catch {
    return 'zh-CN'
  }
}

export const i18n = createI18n({
  legacy: false,
  locale: initialLocale(),
  fallbackLocale: 'zh-CN',
  messages: { 'zh-CN': zhCN, 'en-US': enUS },
})

// 让 t()/$t() 的 key 在编辑器中获得自动补全/悬浮类型提示。
// 注意：vue-i18n 9 的 t() 对「新字面量」不做编译期硬校验（运行时缺 key 会
// 告警并回退 zh-CN）——键一致性由 i18n/keys.test.ts 的结构比对测试兜底，
// 组件内 key 拼写由同文件的 key 完整性扫描测试兜底。
declare module 'vue-i18n' {
  export interface DefineLocaleMessage extends MessageSchema {}
}
