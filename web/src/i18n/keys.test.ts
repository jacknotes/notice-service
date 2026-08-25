import { describe, expect, it } from 'vitest'
import { readFileSync, readdirSync, statSync } from 'node:fs'
import { join } from 'node:path'
import zhCN from '@/locales/zh-CN'

// 组件里静态用到的 t('...')/$t('...') key 必须在 zh-CN 文案表中存在。
// vue-i18n 9 的 t() 对「新字面量」不做编译期硬校验，这个扫描测试就是
// 拼写错误（如 t('login.usernaem')）在测试期的兜底（Tasks 10–19 依赖此网）。

function collectFiles(dir: string): string[] {
  const out: string[] = []
  for (const name of readdirSync(dir)) {
    const p = join(dir, name)
    const st = statSync(p)
    if (st.isDirectory()) out.push(...collectFiles(p))
    else if (/\.(vue|ts)$/.test(name) && !name.endsWith('.test.ts')) out.push(p)
  }
  return out
}

// 只匹配静态 t('...') / $t('...') 字面量：t 前必须是非单词字符，
// 避免误伤 get('/api')、import('@/views/..')、mount('#app') 等以 t 结尾的函数名。
const KEY_RE = /(?:^|[^A-Za-z0-9_])(?:t|\$t)\(\s*'([^']+)'/g

function usedKeys(files: string[]): Set<string> {
  const keys = new Set<string>()
  for (const f of files) {
    const src = readFileSync(f, 'utf8')
    KEY_RE.lastIndex = 0
    let m: RegExpExecArray | null
    while ((m = KEY_RE.exec(src))) keys.add(m[1])
  }
  return keys
}

// 展平嵌套对象为 'a.b' key 集合
function flatten(obj: Record<string, unknown>, prefix = ''): Set<string> {
  const out = new Set<string>()
  for (const [k, v] of Object.entries(obj)) {
    const key = prefix ? `${prefix}.${k}` : k
    if (v && typeof v === 'object') for (const x of flatten(v as Record<string, unknown>, key)) out.add(x)
    else out.add(key)
  }
  return out
}

describe('i18n key 完整性', () => {
  it('组件里用到的每个静态 t() key 都存在于 zh-CN 文案表', () => {
    const known = flatten(zhCN as unknown as Record<string, unknown>)
    const used = usedKeys(collectFiles('src'))
    const missing = [...used].filter((k) => !known.has(k))
    expect(missing).toEqual([])
  })
})
