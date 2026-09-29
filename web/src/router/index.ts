// Router + role guards.
// /admin/*  -> operator console (role superadmin/operator)
// /portal/* -> tenant portal (role tenant)
import { createRouter, createWebHistory } from 'vue-router';
import { useAuthStore } from '@/stores/auth';

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/login', component: () => import('@/views/login/LoginView.vue'), meta: { public: true, title: '登录' } },
    {
      path: '/admin',
      component: () => import('@/layouts/MainLayout.vue'),
      meta: { role: 'admin' },
      children: [
        { path: '', redirect: '/admin/dashboard' },
        { path: 'dashboard', component: () => import('@/views/dashboard/DashboardView.vue'), meta: { title: '总览' } },
        { path: 'tenants', component: () => import('@/views/tenant/TenantView.vue'), meta: { title: '租户' } },
        { path: 'sites', component: () => import('@/views/site/SiteView.vue'), meta: { title: '站点' } },
        { path: 'sites/:id', component: () => import('@/views/site/SiteDetailView.vue'), props: true, meta: { title: '站点设置', activeMenu: '/admin/sites' } },
        { path: 'nodes', component: () => import('@/views/node/NodeView.vue'), meta: { title: '边缘节点' } },
        { path: 'waf', component: () => import('@/views/waf/WafView.vue'), meta: { title: 'WAF 规则' } },
        { path: 'certs', component: () => import('@/views/cert/CertView.vue'), meta: { title: '证书' } },
        { path: 'tasks', component: () => import('@/views/task/TaskView.vue'), meta: { title: '后台任务' } },
        { path: 'dns', component: () => import('@/views/dns/DnsView.vue'), meta: { title: 'DNS 解析' } },
        { path: 'dns-providers', component: () => import('@/views/dns/DnsProviderView.vue'), meta: { title: 'DNS 服务商' } },
        { path: 'goedge-parity', component: () => import('@/views/parity/ParityView.vue'), meta: { title: '复刻矩阵' } },
        { path: 'usage', component: () => import('@/views/usage/UsageView.vue'), meta: { title: '流量计量' } },
        { path: 'admins', component: () => import('@/views/admin/AdminView.vue'), meta: { title: '运营账号' } },
        { path: 'settings', component: () => import('@/views/setting/SettingView.vue'), meta: { title: '平台参数' } },
        { path: 'oplogs', component: () => import('@/views/oplog/OplogView.vue'), meta: { title: '操作日志' } },
      ],
    },
    {
      path: '/portal',
      component: () => import('@/layouts/MainLayout.vue'),
      meta: { role: 'tenant' },
      children: [
        { path: '', redirect: '/portal/dashboard' },
        { path: 'dashboard', component: () => import('@/views/dashboard/DashboardView.vue'), meta: { title: '总览' } },
        { path: 'sites', component: () => import('@/views/site/SiteView.vue'), meta: { title: '我的站点' } },
        { path: 'sites/:id', component: () => import('@/views/site/SiteDetailView.vue'), props: true, meta: { title: '站点设置', activeMenu: '/portal/sites' } },
        { path: 'waf', component: () => import('@/views/waf/WafView.vue'), meta: { title: 'WAF 规则' } },
        { path: 'certs', component: () => import('@/views/cert/CertView.vue'), meta: { title: '证书' } },
        { path: 'dns', component: () => import('@/views/dns/DnsView.vue'), meta: { title: 'DNS 解析' } },
        { path: 'dns-providers', component: () => import('@/views/dns/DnsProviderView.vue'), meta: { title: 'DNS 服务商' } },
        { path: 'nodes', component: () => import('@/views/node/NodeView.vue'), meta: { title: '边缘节点' } },
        { path: 'usage', component: () => import('@/views/usage/UsageView.vue'), meta: { title: '流量用量' } },
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
