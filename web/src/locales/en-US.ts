import type { MessageSchema } from './zh-CN'

// Record<keyof MessageSchema, ...> 的严格版：按 zh-CN 的键结构做映射，
// 任何 zh 有而 en 缺的键（或 en 多出的键）都会在类型层面报错；值可自由翻译。
type EnMessages = { [K in keyof MessageSchema]: { [P in keyof MessageSchema[K]]: string } }

const enUS: EnMessages = {
  common: {
    save: 'Save',
    cancel: 'Cancel',
    create: 'Create',
    delete: 'Delete',
    edit: 'Edit',
    confirm: 'Confirm',
    close: 'Close',
    refresh: 'Refresh',
    copy: 'Copy',
    search: 'Search',
    name: 'Name',
    type: 'Type',
    status: 'Status',
    action: 'Actions',
    time: 'Time',
    enabled: 'Enabled',
    disabled: 'Disabled',
    all: 'All',
    none: 'No data',
    loading: 'Loading…',
    saveFailed: 'Save failed',
    deleteFailed: 'Delete failed',
    loadFailed: 'Load failed',
    batchDelete: 'Batch delete',
    batchDeleteFailed: 'Batch delete failed',
    copied: 'Copied',
    copyFailed: 'Copy failed, please copy manually',
    startDate: 'Start date',
    endDate: 'End date',
    to: 'to',
    today: 'Today',
    lastWeek: 'Last week',
    lastMonth: 'Last month',
    success: 'Succeeded',
    failed: 'Failed',
  },
  nav: {
    dashboard: 'Dashboard',
    channels: 'Channels',
    templates: 'Templates',
    tasks: 'Tasks',
    logs: 'Send Logs',
    audit: 'Audit',
    users: 'Users',
    settings: 'Settings',
  },
}

export default enUS
