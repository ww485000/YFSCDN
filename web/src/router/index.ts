// Router + role guards.
// /admin/*  -> operator console (role superadmin/operator)
// /portal/* -> tenant portal (role tenant)
import { createRouter, createWebHistory } from 'vue-router';
import { useAuthStore } from '@/stores/auth';

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/login', component: () => import('@/views/login/LoginView.vue'), meta: { public: true } },
    {
      path: '/admin',
      component: () => import('@/layouts/MainLayout.vue'),
      meta: { role: 'admin' },
      children: [
        { path: '', redirect: '/admin/dashboard' },
        { path: 'dashboard', component: () => import('@/views/dashboard/DashboardView.vue') },
        { path: 'tenants', component: () => import('@/views/tenant/TenantView.vue') },
        { path: 'sites', component: () => import('@/views/site/SiteView.vue') },
        { path: 'sites/:id', component: () => import('@/views/site/SiteDetailView.vue'), props: true },
        { path: 'nodes', component: () => import('@/views/node/NodeView.vue') },
        { path: 'waf', component: () => import('@/views/waf/WafView.vue') },
        { path: 'certs', component: () => import('@/views/cert/CertView.vue') },
        { path: 'tasks', component: () => import('@/views/task/TaskView.vue') },
        { path: 'dns', component: () => import('@/views/dns/DnsView.vue') },
        { path: 'goedge-parity', component: () => import('@/views/parity/ParityView.vue') },
        { path: 'usage', component: () => import('@/views/usage/UsageView.vue') },
        { path: 'admins', component: () => import('@/views/admin/AdminView.vue') },
        { path: 'settings', component: () => import('@/views/setting/SettingView.vue') },
        { path: 'oplogs', component: () => import('@/views/oplog/OplogView.vue') },
      ],
    },
    {
      path: '/portal',
      component: () => import('@/layouts/MainLayout.vue'),
      meta: { role: 'tenant' },
      children: [
        { path: '', redirect: '/portal/dashboard' },
        { path: 'dashboard', component: () => import('@/views/dashboard/DashboardView.vue') },
        { path: 'sites', component: () => import('@/views/site/SiteView.vue') },
        { path: 'sites/:id', component: () => import('@/views/site/SiteDetailView.vue'), props: true },
        { path: 'waf', component: () => import('@/views/waf/WafView.vue') },
        { path: 'certs', component: () => import('@/views/cert/CertView.vue') },
        { path: 'dns', component: () => import('@/views/dns/DnsView.vue') },
        { path: 'nodes', component: () => import('@/views/node/NodeView.vue') },
        { path: 'usage', component: () => import('@/views/usage/UsageView.vue') },
      ],
    },
    { path: '/', redirect: '/admin/dashboard' },
    { path: '/:pathMatch(.*)*', redirect: '/admin/dashboard' },
  ],
});

router.beforeEach((to) => {
  const auth = useAuthStore();
  if (to.meta.public) return true;
  if (!auth.isLogin) return { path: '/login' };
  const role = to.meta.role as string | undefined;
  if (role === 'admin' && auth.isTenant) return { path: '/portal/dashboard' };
  if (role === 'tenant' && !auth.isTenant) return { path: '/admin/dashboard' };
  return true;
});

// Refresh the user from the server once on first navigation.
router.afterEach((to) => {
  const auth = useAuthStore();
  if (auth.isLogin && !auth.user && !to.meta.public) {
    void auth.fetchMe();
  }
});

export default router;
