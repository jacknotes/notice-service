import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

// useTheme 是模块级单例，模块加载即 applyTheme。每个用例用 vi.resetModules()
// + 动态 import 隔离状态，保证 localStorage / document 状态互不污染。
async function loadTheme() {
  vi.resetModules()
  return await import('./useTheme')
}

describe('useTheme', () => {
  beforeEach(() => {
    localStorage.clear()
    document.documentElement.removeAttribute('data-theme')
  })
  afterEach(() => {
    localStorage.clear()
    document.documentElement.removeAttribute('data-theme')
    vi.restoreAllMocks()
  })

  it('默认初始主题为 dark，并写入 <html data-theme>', async () => {
    const mod = await loadTheme()
    expect(mod.theme.value).toBe('dark')
    expect(document.documentElement.getAttribute('data-theme')).toBe('dark')
  })

  it('localStorage.theme=light 时初始主题为 light', async () => {
    localStorage.setItem('theme', 'light')
    const mod = await loadTheme()
    expect(mod.theme.value).toBe('light')
    expect(document.documentElement.getAttribute('data-theme')).toBe('light')
  })

  it('applyTheme 更新响应式状态、data-theme 属性与 localStorage', async () => {
    const mod = await loadTheme()
    mod.applyTheme('light')
    expect(mod.theme.value).toBe('light')
    expect(document.documentElement.getAttribute('data-theme')).toBe('light')
    expect(localStorage.getItem('theme')).toBe('light')
  })

  it('toggleTheme 在 dark/light 间翻转', async () => {
    const mod = await loadTheme()
    mod.toggleTheme()
    expect(mod.theme.value).toBe('light')
    expect(localStorage.getItem('theme')).toBe('light')
    mod.toggleTheme()
    expect(mod.theme.value).toBe('dark')
  })

  it('localStorage 抛异常时降级不崩溃（默认 dark）', async () => {
    const getItem = vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
      throw new Error('denied')
    })
    const mod = await loadTheme()
    expect(mod.theme.value).toBe('dark')
    getItem.mockRestore()
  })
})
