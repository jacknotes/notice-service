import { computed, ref, type Ref } from 'vue'

/**
 * useTablePaging 客户端排序 + 分页组合式函数。
 * 适用于整表数据已在前端的列表页（任务/模板/渠道/用户）。
 * 排序键取行对象的属性（id/name/enabled/…）；数字按数值、其余按中文本地化比较。
 */
export function useTablePaging<T extends Record<string, any>>(rows: Ref<T[]>, pageSize = 20) {
  const page = ref(1)
  const size = ref(pageSize)
  const sortKey = ref('')
  const sortOrder = ref<'asc' | 'desc'>('desc')

  // el-table sort-change：prop 为列名；order 为 ascending/descending/null
  function onSortChange({ prop, order }: { prop: string; order: string | null }) {
    if (!order) {
      sortKey.value = ''
      sortOrder.value = 'desc'
    } else {
      sortKey.value = prop
      sortOrder.value = order === 'ascending' ? 'asc' : 'desc'
    }
    page.value = 1
  }

  const sorted = computed<T[]>(() => {
    if (!sortKey.value) return rows.value
    const key = sortKey.value
    const dir = sortOrder.value === 'asc' ? 1 : -1
    return [...rows.value].sort((a, b) => {
      const av = a[key] ?? ''
      const bv = b[key] ?? ''
      if (typeof av === 'number' && typeof bv === 'number') return (av - bv) * dir
      return String(av).localeCompare(String(bv), 'zh-Hans-CN') * dir
    })
  })

  const total = computed(() => sorted.value.length)

  const paged = computed<T[]>(() => {
    const start = (page.value - 1) * size.value
    return sorted.value.slice(start, start + size.value)
  })

  // 切换每页条数时回到第一页
  function onPageSizeChange() {
    page.value = 1
  }

  return { page, size, sortKey, sortOrder, onSortChange, sorted, total, paged, onPageSizeChange }
}
