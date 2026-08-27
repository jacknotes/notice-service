import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import MarkdownPreview from './MarkdownPreview.vue'

describe('MarkdownPreview', () => {
  it('渲染标题/加粗/列表', () => {
    const w = mount(MarkdownPreview, { props: { content: '# 标题\n**加粗**\n- 项一\n- 项二' } })
    expect(w.find('h1').text()).toBe('标题')
    expect(w.find('strong').text()).toBe('加粗')
    expect(w.findAll('li')).toHaveLength(2)
  })

  it('渲染代码块', () => {
    const w = mount(MarkdownPreview, { props: { content: '```\ncode()\n```' } })
    expect(w.find('pre code').text()).toBe('code()')
  })

  it('模板变量 {{name}} 被高亮为 .var 且不当作 HTML 注入', () => {
    const w = mount(MarkdownPreview, { props: { content: 'hi {{name}}' } })
    expect(w.find('.var').text()).toBe('{{name}}')
  })

  it('注入的 <script> 被转义，不产生真实元素', () => {
    const w = mount(MarkdownPreview, { props: { content: '<script>alert(1)</script>' } })
    expect(w.find('script').exists()).toBe(false)
  })

  it('javascript: / data: 协议链接被 DOMPurify 剥离 href（XSS 回归）', () => {
    const w = mount(MarkdownPreview, {
      props: { content: '[点我](javascript:alert(document.cookie))\n[a](data:text/html;base64,PHNjcmlwdD4=)' },
    })
    const hrefs = w.findAll('a').map((a) => a.attributes('href') || '')
    expect(hrefs.length).toBeGreaterThanOrEqual(2)
    for (const h of hrefs) {
      expect(h).not.toMatch(/^\s*(javascript|data):/i)
    }
  })

  it('正常 https 链接在消毒后保留可点击', () => {
    const w = mount(MarkdownPreview, { props: { content: '[官网](https://example.com)' } })
    const a = w.find('a')
    expect(a.exists()).toBe(true)
    expect(a.attributes('href')).toBe('https://example.com')
  })

  it('空内容渲染为空（不报错）', () => {
    const w = mount(MarkdownPreview, { props: { content: '' } })
    expect(w.exists()).toBe(true)
  })
})
