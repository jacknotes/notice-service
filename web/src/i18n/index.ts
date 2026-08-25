import { createI18n } from 'vue-i18n'
import zhCN from '@/locales/zh-CN'
import type { MessageSchema } from '@/locales/zh-CN'
import enUS from '@/locales/en-US'

export const STORAGE_KEY = 'i18n-locale'

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
// 告警并回退 zh-CN）——编译期键一致性由 locales/en-US.ts 的映射类型保证，
// 组件内 key 拼写由 src/i18n 的 key 完整性扫描测试兜底。
declare module 'vue-i18n' {
  export interface DefineLocaleMessage extends MessageSchema {}
}
