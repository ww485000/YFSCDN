<script setup lang="ts">
// Background task queue (GoEdge tasks page): list + filter.
import { onMounted, ref } from 'vue';
import { listTasks } from '@/api/task';
import { fmtTime } from '@/utils/format';
import type { Task } from '@/types';

const list = ref<Task[]>([]);
const total = ref(0);
const loading = ref(false);
const page = ref(1);
const typeFilter = ref<string | undefined>(undefined);
const statusFilter = ref<string | undefined>(undefined);

async function load() {
  loading.value = true;
  try {
    const d = await listTasks({ type: typeFilter.value, status: statusFilter.value, page: page.value, size: 20 });
    list.value = d.list;
    total.value = d.total;
  } catch (e) {
    console.error(e);
  } finally {
    loading.value = false;
  }
}

const typeText: Record<string, string> = {
  cert_check: '证书检查',
  acme_renew: 'ACME 续签',
  dns_resolve: 'DNS 创建',
  dns_clean: 'DNS 清理',
  site_resync: '站点重推',
};

const statusColor = (s: string) =>
  s === 'done' ? 'green' : s === 'failed' ? 'red' : s === 'running' ? 'blue' : 'default';

onMounted(() => void load());
</script>

<template>
  <div class="page-card">
    <div class="toolbar">
      <span class="page-title mb-0">后台任务</span>
      <a-select
        v-model:value="typeFilter"
        allow-clear
        placeholder="类型"
        style="width: 150px"
        :options="Object.entries(typeText).map(([v, l]) => ({ value: v, label: l }))"
        @change="load"
      />
      <a-select
        v-model:value="statusFilter"
        allow-clear
        placeholder="状态"
        style="width: 130px"
        :options="[{ value: 'pending', label: '待处理' }, { value: 'running', label: '运行中' }, { value: 'done', label: '完成' }, { value: 'failed', label: '失败' }]"
        @change="load"
      />
      <a-button @click="load">刷新</a-button>
    </div>
    <a-table :data-source="list" :loading="loading" row-key="id" :pagination="{ current: page, total, pageSize: 20, onChange: (p: number) => (page = p) && load() }">
      <a-table-column title="ID" data-index="id" width="60" />
      <a-table-column title="类型" width="130">
        <template #default="{ record }">{{ typeText[record.type] || record.type }}</template>
      </a-table-column>
      <a-table-column title="目标" data-index="target" width="180" />
      <a-table-column title="状态" width="90">
        <template #default="{ record }">
          <a-tag :color="statusColor(record.status)">{{ record.status }}</a-tag>
        </template>
      </a-table-column>
      <a-table-column title="重试" data-index="attempts" width="60" />
      <a-table-column title="错误信息" data-index="error" />
      <a-table-column title="创建时间" width="170">
        <template #default="{ record }">{{ fmtTime(record.created_at) }}</template>
      </a-table-column>
      <a-table-column title="更新时间" width="170">
        <template #default="{ record }">{{ fmtTime(record.updated_at) }}</template>
      </a-table-column>
    </a-table>
  </div>
</template>
