<script setup lang="ts">
// Main layout: dark sider + header + content. Menu is role-driven (admin vs tenant).
import { computed, ref } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { message } from 'ant-design-vue';
import {
  ApartmentOutlined,
  CloudServerOutlined,
  DashboardOutlined,
  FileTextOutlined,
  GlobalOutlined,
  LogoutOutlined,
  ProfileOutlined,
  SafetyCertificateOutlined,
  ScheduleOutlined,
  SettingOutlined,
  TeamOutlined,
  UserOutlined,
} from '@ant-design/icons-vue';
import { useAuthStore } from '@/stores/auth';

const auth = useAuthStore();
const route = useRoute();
const router = useRouter();
const collapsed = ref(false);

interface MenuItem {
  key: string;
  label: string;
  icon: unknown;
}

const adminMenus: MenuItem[] = [
  { key: '/admin/dashboard', label: '总览', icon: DashboardOutlined },
  { key: '/admin/tenants', label: '租户', icon: TeamOutlined },
  { key: '/admin/sites', label: '站点', icon: CloudServerOutlined },
  { key: '/admin/nodes', label: '边缘节点', icon: ApartmentOutlined },
  { key: '/admin/waf', label: 'WAF 规则', icon: SafetyCertificateOutlined },
  { key: '/admin/certs', label: '证书', icon: SafetyCertificateOutlined },
  { key: '/admin/dns', label: 'DNS 解析', icon: GlobalOutlined },
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
  { key: '/portal/nodes', label: '边缘节点', icon: ApartmentOutlined },
  { key: '/portal/usage', label: '流量用量', icon: FileTextOutlined },
];

const isTenant = computed(() => auth.isTenant);
const menus = computed<MenuItem[]>(() => (isTenant.value ? tenantMenus : adminMenus));
const selected = computed<string[]>(() => [route.path]);
const title = computed(() => (isTenant.value ? '租户门户' : '运营控制台'));

function onMenuClick(info: { key: string }) {
  void router.push(info.key);
}

function logout() {
  auth.logout();
  message.success('已退出登录');
  void router.push('/login');
}
</script>

<template>
  <a-layout class="h-full">
    <a-layout-sider v-model:collapsed="collapsed" collapsible theme="dark" width="210">
      <div class="logo">EdgeCDN</div>
      <a-menu theme="dark" mode="inline" :selected-keys="selected" @click="onMenuClick">
        <a-menu-item v-for="m in menus" :key="m.key">
          <template #icon><component :is="m.icon" /></template>
          {{ m.label }}
        </a-menu-item>
      </a-menu>
    </a-layout-sider>
    <a-layout>
      <a-layout-header class="flex items-center justify-end gap-4 bg-white px-6" style="padding: 0 24px">
        <span class="text-sm text-gray-500">{{ title }}</span>
        <span class="flex items-center gap-1 text-sm text-gray-700">
          <UserOutlined />
          <span>{{ auth.displayName || '用户' }}</span>
        </span>
        <a-button size="small" @click="logout">
          <template #icon><LogoutOutlined /></template>
          退出登录
        </a-button>
      </a-layout-header>
      <a-layout-content class="p-4" style="min-height: 281px">
        <router-view />
      </a-layout-content>
      <a-layout-footer class="text-center text-gray-400">EdgeCDN · 模块化 CDN 系统</a-layout-footer>
    </a-layout>
  </a-layout>
</template>
