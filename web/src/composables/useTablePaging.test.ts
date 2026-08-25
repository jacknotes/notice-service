import { describe, expect, it } from 'vitest'
import { ref } from 'vue'
import { useTablePaging } from './useTablePaging'

const rows = ref([
  { id: 3, name: '乙', enabled: true },
  { id: 1, name: '甲', enabled: false },
  { id: 2, name: '丙', enabled: true },
  { id: 4, name: '丁', enabled: false },
])

describe('useTablePaging', () => {
  it('未排序时返回原序，total 等于行数', () => {
    const { sorted, total, paged } = useTablePaging(rows)
    expect(total.value).toBe(4)
    expect(sorted.value.map((r) => r.id)).toEqual([3, 1, 2, 4])
    expect(paged.value.length).toBe(4)
  })

  it('数字列 id 降序排序', () => {
    const { onSortChange, sorted } = useTablePaging(rows)
    onSortChange({ prop: 'id', order: 'descending' })
    expect(sorted.value.map((r) => r.id)).toEqual([4, 3, 2, 1])
  })

  it('数字列 id 升序排序', () => {
    const { onSortChange, sorted } = useTablePaging(rows)
    onSortChange({ prop: 'id', order: 'ascending' })
    expect(sorted.value.map((r) => r.id)).toEqual([1, 2, 3, 4])
  })

  it('字符串列 name 按中文比较升序', () => {
    const { onSortChange, sorted } = useTablePaging(rows)
    onSortChange({ prop: 'name', order: 'ascending' })
    expect(sorted.value.map((r) => r.name)).toEqual(['乙', '丙', '丁', '甲'].sort((a, b) => a.localeCompare(b, 'zh-Hans-CN')))
  })

  it('onSortChange 传 null 清除排序', () => {
    const { onSortChange, sorted } = useTablePaging(rows)
    onSortChange({ prop: 'id', order: 'descending' })
    onSortChange({ prop: 'id', order: null })
    expect(sorted.value.map((r) => r.id)).toEqual([3, 1, 2, 4])
  })

  it('排序后回到第 1 页', () => {
    const { page, onSortChange, sorted } = useTablePaging(rows)
    page.value = 2
    onSortChange({ prop: 'id', order: 'descending' })
    expect(sorted.value.map((r) => r.id)).toEqual([4, 3, 2, 1])
    expect(page.value).toBe(1)
  })

  it('翻页切片正确（每页 2 条）', () => {
    const { page, paged } = useTablePaging(rows, 2)
    expect(paged.value.length).toBe(2)
    expect(paged.value[0].id).toBe(3)
    page.value = 2
    expect(paged.value.map((r) => r.id)).toEqual([2, 4])
  })

  it('切换每页条数回到第 1 页', () => {
    const { page, onPageSizeChange } = useTablePaging(rows, 2)
    page.value = 3
    onPageSizeChange()
    expect(page.value).toBe(1)
  })
})
