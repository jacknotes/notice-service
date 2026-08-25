import { createRouter, createWebHistory } from 'vue-router'
import { useAuthStore } from '@/stores/auth'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/login', component: () => import('@/views/Login.vue'), meta: { public: true } },
    {
      path: '/',
      component: () => import('@/components/AppLayout.vue'),
      redirect: '/dashboard',
      children: [
        { path: 'dashboard', component: () => import('@/views/Dashboard.vue'), meta: { title: '仪表盘' } },
        { path: 'channels', component: () => import('@/views/Channels.vue'), meta: { title: '渠道管理' } },
        { path: 'templates', component: () => import('@/views/Templates.vue'), meta: { title: '模板管理' } },
        { path: 'tasks', component: () => import('@/views/Tasks.vue'), meta: { title: '任务管理' } },
        { path: 'logs', component: () => import('@/views/Logs.vue'), meta: { title: '发送日志' } },
        { path: 'logs/:id', component: () => import('@/views/LogDetail.vue'), meta: { title: '日志详情' } },
        { path: 'audit', component: () => import('@/views/Audit.vue'), meta: { title: '操作审计', adminOnly: true } },
        { path: 'users', component: () => import('@/views/Users.vue'), meta: { title: '用户管理', adminOnly: true } },
        { path: 'settings', component: () => import('@/views/Settings.vue'), meta: { title: '个人设置' } },
      ],
    },
  ],
})

router.beforeEach((to) => {
  const auth = useAuthStore()
  if (!to.meta.public && !auth.isLoggedIn) return { path: '/login' }
  if (to.path === '/login' && auth.isLoggedIn) return { path: '/dashboard' }
  if (to.meta.adminOnly && auth.user?.role !== 'admin') return { path: '/dashboard' }
  return true
})

export default router
