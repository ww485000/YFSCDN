<script setup lang="ts">
// Tenant management (admin only): CRUD + quotas + balance.
import { onMounted, reactive, ref } from 'vue';
import { message } from 'ant-design-vue';
import { createTenant, deleteTenant, listTenants, updateTenant } from '@/api/admin';
import { bytes, fmtTime } from '@/utils/format';
import type { Tenant } from '@/types';

const list = ref<Tenant[]>([]);
const total = ref(0);
const loading = ref(false);
const page = ref(1);
const keyword = ref('');
const show = ref(false);
const editing = ref<Tenant | null>(null);
const form = reactive({
  name: '',
  username: '',
  password: '',
  email: '',
  max_sites: 10,
  traffic_quota_mb: 10240,
  status: 1,
});

async function load() {
  loading.value = true;
  try {
    const d = await listTenants({ keyword: keyword.value, page: page.value, size: 20 });
    list.value = d.list;
    total.value = d.total;
  } catch (e) {
    message.error(e instanceof Error ? e.message : 'load failed');
  } finally {
    loading.value = false;
  }
}

function openCreate() {
  editing.value = null;
  Object.assign(form, { name: '', username: '', password: '', email: '', max_sites: 10, traffic_quota_mb: 10240, status: 1 });
  show.value = true;
}

function openEdit(t: Tenant) {
  editing.value = t;
  Object.assign(form, {
    name: t.name,
    username: t.username,
    password: '',
    email: t.email,
    max_sites: t.max_sites,
    traffic_quota_mb: t.traffic_quota_mb,
    status: t.status,
  });
  show.value = true;
}

async function submit() {
  try {
    if (editing.value) {
      await updateTenant(editing.value.id, {
        name: form.name,
        email: form.email,
        max_sites: form.max_sites,
        traffic_quota_mb: form.traffic_quota_mb,
        status: form.status,
        ...(form.password ? { password: form.password } : {}),
      });
      message.success('租户已更新');
    } else {
      await createTenant({ name: form.name, username: form.username, password: form.password, email: form.email });
      message.success('租户已创建（登录密码即此处设置）');
    }
    show.value = false;
    void load();
  } catch (e) {
    message.error(e instanceof Error ? e.message : 'save failed');
  }
}

async function remove(t: Tenant) {
  try {
    await deleteTenant(t.id);
    message.success('已删除');
    void load();
  } catch (e) {
    message.error(e instanceof Error ? e.message : 'delete failed');
  }
}

onMounted(load);
</script>

<template>
  <div class="page-card">
    <div class="toolbar">
      <span class="page-title mb-0">租户管理</span>
      <a-button type="primary" @click="openCreate">新建租户</a-button>
      <a-input-search v-model:value="keyword" placeholder="搜索 名称/用户名/邮箱" style="width: 240px" @search="load" />
    </div>
    <a-table :data-source="list" :loading="loading" row-key="id" :pagination="{ current: page, total, pageSize: 20, onChange: (p: number) => (page = p) && load() }">
      <a-table-column title="ID" data-index="id" width="60" />
      <a-table-column title="名称" data-index="name" />
      <a-table-column title="登录名" data-index="username" />
      <a-table-column title="邮箱" data-index="email" />
      <a-table-column title="站点配额" data-index="max_sites" width="90" />
      <a-table-column title="流量额度" width="110">
        <template #default="{ text }">{{ text.traffic_quota_mb === 0 ? '不限' : bytes(text.traffic_quota_mb * 1024 * 1024) }}</template>
      </a-table-column>
      <a-table-column title="状态" width="80">
        <template #default="{ text }">
          <a-tag :color="text.status === 1 ? 'green' : 'red'">{{ text.status === 1 ? '启用' : '禁用' }}</a-tag>
        </template>
      </a-table-column>
      <a-table-column title="创建时间" width="170">
        <template #default="{ text }">{{ fmtTime(text.created_at) }}</template>
      </a-table-column>
      <a-table-column title="操作" width="140">
        <template #default="{ record }">
          <a @click="openEdit(record)">编辑</a>
          <a-divider type="vertical" />
          <a-popconfirm title="确认删除该租户？" @confirm="remove(record)">
            <a class="text-red-500">删除</a>
          </a-popconfirm>
        </template>
      </a-table-column>
    </a-table>

    <a-modal v-model:open="show" :title="editing ? '编辑租户' : '新建租户'" @ok="submit" :okButtonProps="{ loading: false }">
      <a-form layout="vertical" :model="form">
        <a-form-item label="名称" required>
          <a-input v-model:value="form.name" placeholder="客户名称" />
        </a-form-item>
        <a-form-item v-if="!editing" label="登录用户名" required>
          <a-input v-model:value="form.username" placeholder="租户门户登录名" />
        </a-form-item>
        <a-form-item :label="editing ? '重置密码（留空不修改）' : '登录密码'" :required="!editing">
          <a-input-password v-model:value="form.password" :placeholder="editing ? '留空保持原密码' : '至少 6 位'" />
        </a-form-item>
        <a-form-item label="邮箱">
          <a-input v-model:value="form.email" />
        </a-form-item>
        <a-form-item label="站点配额">
          <a-input-number v-model:value="form.max_sites" :min="1" />
        </a-form-item>
        <a-form-item label="流量额度 (MB, 0=不限)">
          <a-input-number v-model:value="form.traffic_quota_mb" :min="0" />
        </a-form-item>
        <a-form-item label="状态">
          <a-select v-model:value="form.status" :options="[{ value: 1, label: '启用' }, { value: 0, label: '禁用' }]" />
        </a-form-item>
      </a-form>
    </a-modal>
  </div>
</template>
