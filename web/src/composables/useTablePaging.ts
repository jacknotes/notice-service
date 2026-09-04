import { computed, ref, type Ref } from 'vue'

/**
 * useTablePaging 客户端排序 + 分页组合式函数。
 * 适用于整表数据已在前端的列表页（任务/模板/渠道/用户/分类）。
 * 排序键取行对象的属性（id/name/enabled/…），或经 getters 从派生字段
 * （渠道名/模板名/变量数/使用情况等）计算；数字按数值、布尔按 0/1、
 * 其余按中文本地化比较。
 */
export type RowGetter<T> = (row: T) => unknown

export function useTablePaging<T extends Record<string, any>>(
  rows: Ref<T[]>,
  pageSize = 20,
  getters: Record<string, RowGetter<T>> = {},
) {
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
    const get = getters[key]
    return [...rows.value].sort((a, b) => {
      const av = get ? get(a) : a[key] ?? ''
      const bv = get ? get(b) : b[key] ?? ''
      return compareValues(av, bv) * dir
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

// compareValues 按值类型比较两个排序键：布尔 0/1、数值、数字字符串按数值、
// 其余按中文本地化；null/undefined 视作空串。
function compareValues(a: unknown, b: unknown): number {
  const av = a ?? ''
  const bv = b ?? ''
  if (typeof av === 'boolean' && typeof bv === 'boolean') return (av ? 1 : 0) - (bv ? 1 : 0)
  if (typeof av === 'number' && typeof bv === 'number') return av - bv
  if (isNumericStr(av) && isNumericStr(bv)) return Number(av) - Number(bv)
  return String(av).localeCompare(String(bv), 'zh-Hans-CN')
}

// isNumericStr 判断字符串是否为纯数值（可含前导负号/小数点），供排序时按数值比较。
function isNumericStr(v: unknown): v is string {
  return typeof v === 'string' && v.trim() !== '' && !isNaN(Number(v))
}

