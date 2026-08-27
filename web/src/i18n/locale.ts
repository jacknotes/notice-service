import { watch } from 'vue'
import router from '@/router'
import { i18n, STORAGE_KEY } from './index'

export const SUPPORTED_LOCALES = ['zh-CN', 'en-US'] as const
export type SupportedLocale = (typeof SUPPORTED_LOCALES)[number]

// 切换语言后立即刷新浏览器标签页标题：afterEach 只在导航时设置 title，
// 停留当前页切换语言（登录页/任意页）时需要这里补一次。
watch(i18n.global.locale, () => {
  const key = router.currentRoute.value.meta?.titleKey as string | undefined
  if (key) document.title = i18n.global.t(key)
})

export function setLocale(locale: SupportedLocale) {
  i18n.global.locale.value = locale
  try {
    localStorage.setItem(STORAGE_KEY, locale)
  } catch {
    /* private mode — 本次会话生效即可 */
  }
}

export function currentLocale(): SupportedLocale {
  const v = i18n.global.locale.value
  return v === 'en-US' ? 'en-US' : 'zh-CN'
}
