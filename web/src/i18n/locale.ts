import { i18n, STORAGE_KEY } from './index'

export const SUPPORTED_LOCALES = ['zh-CN', 'en-US'] as const
export type SupportedLocale = (typeof SUPPORTED_LOCALES)[number]

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
