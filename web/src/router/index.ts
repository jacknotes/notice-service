import { createRouter, createWebHistory } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { i18n } from '@/i18n'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/login', component: () => import('@/views/Login.vue'), meta: { public: true, titleKey: 'nav.dashboard' } },
    {
      path: '/',
      component: () => import('@/components/AppLayout.vue'),
      redirect: '/dashboard',
      children: [
        { path: 'dashboard', component: () => import('@/views/Dashboard.vue'), meta: { titleKey: 'nav.dashboard' } },
        { path: 'channels', component: () => import('@/views/Channels.vue'), meta: { titleKey: 'nav.channels' } },
        { path: 'templates', component: () => import('@/views/Templates.vue'), meta: { titleKey: 'nav.templates' } },
        { path: 'tasks', component: () => import('@/views/Tasks.vue'), meta: { titleKey: 'nav.tasks' } },
        { path: 'logs', component: () => import('@/views/Logs.vue'), meta: { titleKey: 'nav.logs' } },
        { path: 'logs/:id', component: () => import('@/views/LogDetail.vue'), meta: { titleKey: 'logs.detailTitle' } },
        { path: 'audit', component: () => import('@/views/Audit.vue'), meta: { titleKey: 'nav.audit', adminOnly: true } },
        { path: 'users', component: () => import('@/views/Users.vue'), meta: { titleKey: 'nav.users', adminOnly: true } },
        { path: 'settings', component: () => import('@/views/Settings.vue'), meta: { titleKey: 'nav.settings' } },
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

router.afterEach((to) => {
  const key = to.meta.titleKey as string | undefined
  if (key) document.title = i18n.global.t(key)
})

export default router
