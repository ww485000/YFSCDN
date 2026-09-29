<script setup lang="ts">
// Login page: choose scope (运营后台 / 租户门户) then log in.
import { reactive, ref } from 'vue';
import { useRouter } from 'vue-router';
import { message } from 'ant-design-vue';
import { useAuthStore } from '@/stores/auth';

const auth = useAuthStore();
const router = useRouter();

const scope = ref<'admin' | 'user'>('admin');
const form = reactive({ username: '', password: '' });
const loading = ref(false);

async function submit() {
  if (!form.username || !form.password) {
    message.warning('请输入用户名和密码');
    return;
  }
  loading.value = true;
  try {
    await auth.login(form.username, form.password, scope.value);
    message.success('登录成功');
    void router.push(scope.value === 'admin' ? '/admin/dashboard' : '/portal/dashboard');
  } catch (e) {
    message.error(e instanceof Error ? e.message : '登录失败');
  } finally {
    loading.value = false;
  }
}
</script>

<template>
  <div class="flex min-h-full items-center justify-center" style="background: linear-gradient(135deg, #1f2937 0%, #374151 100%)">
    <div class="w-[400px] rounded-lg bg-white p-8 shadow-xl">
      <div class="mb-6 text-center text-2xl font-bold text-gray-800">EdgeCDN</div>
      <div class="mb-4 text-center text-sm text-gray-400">模块化 · 多租户 · 可运营 CDN</div>
      <a-tabs v-model:activeKey="scope" centered>
        <a-tab-pane key="admin" tab="运营后台" />
        <a-tab-pane key="user" tab="租户门户" />
      </a-tabs>
      <a-form :model="form" @finish="submit">
        <a-form-item label="用户名" :rules="[{ required: true, message: '请输入用户名' }]">
          <a-input v-model:value="form.username" placeholder="admin / 租户用户名" allow-clear />
        </a-form-item>
        <a-form-item label="密码" :rules="[{ required: true, message: '请输入密码' }]">
          <a-input-password v-model:value="form.password" placeholder="默认 admin / admin123" />
        </a-form-item>
        <a-button type="primary" html-type="submit" :loading="loading" block>
          登 录
        </a-button>
      </a-form>
      <div class="mt-4 text-xs text-gray-400">
        首次启动：运营账号 admin / admin123（登录后请修改）；租户账号由运营创建。
      </div>
    </div>
  </div>
</template>
