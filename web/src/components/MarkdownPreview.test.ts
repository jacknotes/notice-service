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

  it('空内容渲染为空（不报错）', () => {
    const w = mount(MarkdownPreview, { props: { content: '' } })
    expect(w.exists()).toBe(true)
  })
})
