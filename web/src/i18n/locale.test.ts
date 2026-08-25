import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { STORAGE_KEY } from './index'

// locale.ts 依赖 i18n 单例（模块加载时读 localStorage 初始化 locale）。
// 每个用例用 vi.resetModules() + 动态 import 隔离模块级状态，
// 保证 localStorage 状态互不污染（同 useTheme.test.ts 的模式）。
async function loadLocale() {
  vi.resetModules()
  return await import('./locale')
}

describe('i18n locale', () => {
  beforeEach(() => {
    localStorage.clear()
  })
  afterEach(() => {
    localStorage.clear()
    vi.restoreAllMocks()
  })

  it('localStorage 为空时 currentLocale() 默认 zh-CN', async () => {
    const mod = await loadLocale()
    expect(mod.currentLocale()).toBe('zh-CN')
  })

  it('localStorage 已持久化 en-US 时初始 locale 为 en-US', async () => {
    localStorage.setItem(STORAGE_KEY, 'en-US')
    const mod = await loadLocale()
    expect(mod.currentLocale()).toBe('en-US')
  })

  it('setLocale("en-US") 后 currentLocale() 与 localStorage 同步', async () => {
    const mod = await loadLocale()
    mod.setLocale('en-US')
    expect(mod.currentLocale()).toBe('en-US')
    expect(localStorage.getItem(STORAGE_KEY)).toBe('en-US')
  })

  it('setLocale("zh-CN") 可从 en-US 往返切回并持久化', async () => {
    const mod = await loadLocale()
    mod.setLocale('en-US')
    mod.setLocale('zh-CN')
    expect(mod.currentLocale()).toBe('zh-CN')
    expect(localStorage.getItem(STORAGE_KEY)).toBe('zh-CN')
  })

  it('localStorage.setItem 抛异常（隐私模式）时 setLocale 不崩溃且内存值生效', async () => {
    const mod = await loadLocale()
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
      throw new Error('denied')
    })
    expect(() => mod.setLocale('en-US')).not.toThrow()
    expect(mod.currentLocale()).toBe('en-US')
  })
})
