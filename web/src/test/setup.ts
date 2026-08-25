// 全局测试环境兜底：jsdom 缺口的 window 能力在此补齐，避免组件挂载即崩。
// localStorage 由 jsdom 原生提供，无需 mock。
if (!window.matchMedia) {
  Object.defineProperty(window, 'matchMedia', {
    writable: true,
    value: (query: string) => ({
      matches: false,
      media: query,
      onchange: null,
      addListener: () => {},
      removeListener: () => {},
      addEventListener: () => {},
      removeEventListener: () => {},
      dispatchEvent: () => false,
    }),
  })
}
