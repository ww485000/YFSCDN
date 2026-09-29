<script setup lang="ts">
// Root component: router-view only (layout lives in MainLayout per route group).
import zhCN from 'ant-design-vue/es/locale/zh_CN';
import { useThemeStore } from '@/stores/theme';

const theme = useThemeStore();
</script>

<template>
  <a-config-provider :theme="theme.antd" :locale="zhCN">
    <a-app>
      <router-view />
    </a-app>
  </a-config-provider>
</template>
