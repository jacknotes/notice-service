import { createI18n } from 'vue-i18n'
import zhCN from '@/locales/zh-CN'
import type { MessageSchema } from '@/locales/zh-CN'
import enUS from '@/locales/en-US'

const STORAGE_KEY = 'i18n-locale'

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

// 让 t('common.save') 这类 key 在模板/脚本里获得类型检查。
declare module 'vue-i18n' {
  export interface DefineLocaleMessage extends MessageSchema {}
}
