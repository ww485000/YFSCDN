<script setup lang="ts">
// Operator account management (admin only).
import { onMounted, reactive, ref } from 'vue';
import { message } from 'ant-design-vue';
import { createAdmin, deleteAdmin, listAdmins, updateAdmin } from '@/api/admin';
import { fmtTime } from '@/utils/format';
import type { AdminUser } from '@/types';

const list = ref<AdminUser[]>([]);
const loading = ref(false);
const show = ref(false);
const editing = ref<AdminUser | null>(null);
const form = reactive({ username: '', password: '', role: 'operator', status: 1 });

async function load() {
  loading.value = true;
  try {
    list.value = await listAdmins();
  } catch (e) {
    message.error(e instanceof Error ? e.message : 'load failed');
  } finally {
    loading.value = false;
  }
}

function openCreate() {
  editing.value = null;
  Object.assign(form, { username: '', password: '', role: 'operator', status: 1 });
  show.value = true;
}

function openEdit(a: AdminUser) {
  editing.value = a;
  Object.assign(form, { username: a.username, password: '', role: a.role, status: a.status });
  show.value = true;
}

async function submit() {
  try {
    if (editing.value) {
      await updateAdmin(editing.value.id, {
        role: form.role,
        status: form.status,
        ...(form.password ? { password: form.password } : {}),
      });
      message.success('已更新');
    } else {
      await createAdmin(form);
      message.success('已创建');
    }
    show.value = false;
    void load();
  } catch (e) {
    message.error(e instanceof Error ? e.message : 'save failed');
  }
}

async function remove(a: AdminUser) {
  try {
    await deleteAdmin(a.id);
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
      <span class="page-title mb-0">运营账号</span>
      <a-button type="primary" @click="openCreate">新建账号</a-button>
    </div>
    <a-table :data-source="list" :loading="loading" row-key="id" :pagination="false">
      <a-table-column title="ID" data-index="id" width="60" />
      <a-table-column title="用户名" data-index="username" />
      <a-table-column title="角色" width="120">
        <template #default="{ record }">
          <a-tag :color="record.role === 'superadmin' ? 'purple' : 'blue'">{{ record.role }}</a-tag>
        </template>
      </a-table-column>
      <a-table-column title="状态" width="80">
        <template #default="{ record }">
          <a-tag :color="record.status === 1 ? 'green' : 'red'">{{ record.status === 1 ? '启用' : '禁用' }}</a-tag>
        </template>
      </a-table-column>
      <a-table-column title="创建时间" data-index="created_at">
        <template #default="{ record }">{{ fmtTime(record.created_at) }}</template>
      </a-table-column>
      <a-table-column title="操作" width="140">
        <template #default="{ record }">
          <a @click="openEdit(record)">编辑</a>
          <a-divider type="vertical" />
          <a-popconfirm title="确认删除该账号？" @confirm="remove(record)">
            <a class="text-red-500">删除</a>
          </a-popconfirm>
        </template>
      </a-table-column>
    </a-table>

    <a-modal v-model:open="show" :title="editing ? '编辑账号' : '新建账号'" @ok="submit">
      <a-form layout="vertical" :model="form">
        <a-form-item label="用户名" required>
          <a-input v-model:value="form.username" :disabled="!!editing" />
        </a-form-item>
        <a-form-item :label="editing ? '重置密码（留空不修改）' : '密码'" :required="!editing">
          <a-input-password v-model:value="form.password" />
        </a-form-item>
        <a-form-item label="角色">
          <a-select v-model:value="form.role" :options="[{ value: 'superadmin', label: '超级管理员' }, { value: 'operator', label: '运营' }]" />
        </a-form-item>
        <a-form-item label="状态">
          <a-select v-model:value="form.status" :options="[{ value: 1, label: '启用' }, { value: 0, label: '禁用' }]" />
        </a-form-item>
      </a-form>
    </a-modal>
  </div>
</template>
