<script setup lang="ts">
// Main layout: dark sider + header + content. Menu is role-driven (admin vs tenant).
import { computed, ref, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { message } from 'ant-design-vue';
import {
  ApartmentOutlined,
  BgColorsOutlined,
  BulbOutlined,
  CloudServerOutlined,
  CloseOutlined,
  CompressOutlined,
  DashboardOutlined,
  FileTextOutlined,
  GlobalOutlined,
  LogoutOutlined,
  MenuFoldOutlined,
  MenuUnfoldOutlined,
  PoweroffOutlined,
  ProfileOutlined,
  SafetyCertificateOutlined,
  ScheduleOutlined,
  SettingOutlined,
  TeamOutlined,
  UserOutlined,
} from '@ant-design/icons-vue';
import { useAuthStore } from '@/stores/auth';
import { useThemeStore } from '@/stores/theme';

const auth = useAuthStore();
const theme = useThemeStore();
const route = useRoute();
const router = useRouter();
const collapsed = ref(false);
const colors = ['#646cff', '#1677ff', '#0f9f6e', '#d97706', '#dc2626'];

interface MenuItem {
  key: string;
  label: string;
  icon: unknown;
}

interface PageTab {
  path: string;
  title: string;
}

const adminMenus: MenuItem[] = [
  { key: '/admin/dashboard', label: '总览', icon: DashboardOutlined },
  { key: '/admin/tenants', label: '租户', icon: TeamOutlined },
  { key: '/admin/sites', label: '站点', icon: CloudServerOutlined },
  { key: '/admin/nodes', label: '边缘节点', icon: ApartmentOutlined },
  { key: '/admin/waf', label: 'WAF 规则', icon: SafetyCertificateOutlined },
  { key: '/admin/certs', label: '证书', icon: SafetyCertificateOutlined },
  { key: '/admin/dns', label: 'DNS 解析', icon: GlobalOutlined },
  { key: '/admin/dns-providers', label: 'DNS 服务商', icon: CloudServerOutlined },
  { key: '/admin/dns-syncs', label: 'DNS 同步', icon: ScheduleOutlined },
  { key: '/admin/goedge-parity', label: '复刻矩阵', icon: ProfileOutlined },
  { key: '/admin/usage', label: '流量计量', icon: FileTextOutlined },
  { key: '/admin/tasks', label: '后台任务', icon: ScheduleOutlined },
  { key: '/admin/admins', label: '运营账号', icon: UserOutlined },
  { key: '/admin/settings', label: '平台参数', icon: SettingOutlined },
  { key: '/admin/oplogs', label: '操作日志', icon: FileTextOutlined },
];

const tenantMenus: MenuItem[] = [
  { key: '/portal/dashboard', label: '总览', icon: DashboardOutlined },
  { key: '/portal/sites', label: '我的站点', icon: CloudServerOutlined },
  { key: '/portal/waf', label: 'WAF 规则', icon: SafetyCertificateOutlined },
  { key: '/portal/certs', label: '证书', icon: SafetyCertificateOutlined },
  { key: '/portal/dns', label: 'DNS 解析', icon: GlobalOutlined },
  { key: '/portal/dns-providers', label: 'DNS 服务商', icon: CloudServerOutlined },
  { key: '/portal/dns-syncs', label: 'DNS 同步', icon: ScheduleOutlined },
  { key: '/portal/nodes', label: '边缘节点', icon: ApartmentOutlined },
  { key: '/portal/usage', label: '流量用量', icon: FileTextOutlined },
];

const isTenant = computed(() => auth.isTenant);
const menus = computed<MenuItem[]>(() => (isTenant.value ? tenantMenus : adminMenus));
const selected = computed<string[]>(() => {
  const activeMenu = route.meta.activeMenu as string | undefined;
  const found = menus.value.find((m) => (activeMenu || route.path) === m.key || route.path.startsWith(`${m.key}/`));
  return [found?.key || route.path];
});
const title = computed(() => (isTenant.value ? '租户门户' : '运营控制台'));
const pageTitle = computed(() => (route.meta.title as string | undefined) || '未命名页面');
const breadcrumbItems = computed(() => [
  title.value,
  menus.value.find((m) => m.key === selected.value[0])?.label || pageTitle.value,
  ...(pageTitle.value === menus.value.find((m) => m.key === selected.value[0])?.label ? [] : [pageTitle.value]),
]);
const tabs = ref<PageTab[]>([]);

function onMenuClick(info: { key: string }) {
  void router.push(info.key);
}

function logout() {
  auth.logout();
  message.success('已退出登录');
  void router.push('/login');
}

function closeTab(tab: PageTab) {
  if (tabs.value.length <= 1) return;
  const idx = tabs.value.findIndex((t) => t.path === tab.path);
  tabs.value = tabs.value.filter((t) => t.path !== tab.path);
  if (route.path === tab.path) {
    const next = tabs.value[Math.max(0, idx - 1)];
    void router.push(next.path);
  }
}

watch(
  () => route.fullPath,
  () => {
    if (route.meta.public) return;
    const path = route.fullPath;
    if (!tabs.value.some((t) => t.path === path)) {
      tabs.value.push({ path, title: pageTitle.value });
    }
    if (tabs.value.length > 12) {
      tabs.value = tabs.value.slice(-12);
    }
    document.title = `${pageTitle.value} - YFSCDN`;
  },
  { immediate: true },
);
</script>

<template>
  <a-layout class="app-shell">
    <a-layout-sider v-model:collapsed="collapsed" class="app-sider" collapsible theme="dark" width="220">
      <div class="logo">{{ collapsed ? 'YF' : 'YFSCDN' }}</div>
      <a-menu theme="dark" mode="inline" :selected-keys="selected" @click="onMenuClick">
        <a-menu-item v-for="m in menus" :key="m.key">
          <template #icon><component :is="m.icon" /></template>
          {{ m.label }}
        </a-menu-item>
      </a-menu>
    </a-layout-sider>
    <a-layout>
      <a-layout-header class="app-header flex items-center justify-between gap-4">
        <div class="flex items-center gap-3">
          <a-button type="text" size="small" @click="collapsed = !collapsed">
            <template #icon>
              <MenuUnfoldOutlined v-if="collapsed" />
              <MenuFoldOutlined v-else />
            </template>
          </a-button>
          <div>
            <div class="text-base font-600">{{ pageTitle }}</div>
            <a-breadcrumb class="app-breadcrumb">
              <a-breadcrumb-item v-for="item in breadcrumbItems" :key="item">{{ item }}</a-breadcrumb-item>
            </a-breadcrumb>
          </div>
        </div>
        <div class="flex items-center gap-2">
          <a-tooltip title="主题模式">
            <a-button type="text" size="small" @click="theme.toggleMode">
              <template #icon>
                <BulbOutlined v-if="theme.isDark" />
                <PoweroffOutlined v-else />
              </template>
            </a-button>
          </a-tooltip>
          <a-tooltip title="紧凑密度">
            <a-button type="text" size="small" @click="theme.toggleDensity">
              <template #icon><CompressOutlined /></template>
            </a-button>
          </a-tooltip>
          <a-dropdown trigger="click">
            <a-button type="text" size="small">
              <template #icon><BgColorsOutlined /></template>
            </a-button>
            <template #overlay>
              <a-menu>
                <a-menu-item v-for="color in colors" :key="color" @click="theme.setPrimaryColor(color)">
                  <div class="flex items-center gap-2">
                    <span class="theme-swatch" :style="{ background: color }" />
                    <span>{{ color }}</span>
                  </div>
                </a-menu-item>
              </a-menu>
            </template>
          </a-dropdown>
          <a-divider type="vertical" />
          <span class="flex items-center gap-1 text-sm">
            <UserOutlined />
            <span>{{ auth.displayName || '用户' }}</span>
          </span>
          <a-button size="small" @click="logout">
            <template #icon><LogoutOutlined /></template>
            退出登录
          </a-button>
        </div>
      </a-layout-header>
      <a-layout-content class="app-content">
        <div class="page-tabs" v-if="tabs.length">
          <button
            v-for="tab in tabs"
            :key="tab.path"
            class="page-tab"
            :class="{ active: tab.path === route.fullPath }"
            type="button"
            @click="router.push(tab.path)"
          >
            <span>{{ tab.title }}</span>
            <CloseOutlined v-if="tabs.length > 1" class="page-tab-close" @click.stop="closeTab(tab)" />
          </button>
        </div>
        <router-view v-slot="{ Component }">
          <transition name="fade-slide" mode="out-in">
            <component :is="Component" :key="route.fullPath" />
          </transition>
        </router-view>
      </a-layout-content>
      <a-layout-footer class="text-center text-gray-400">EdgeCDN · 模块化 CDN 系统</a-layout-footer>
    </a-layout>
  </a-layout>
</template>
