<script setup lang="ts">
// Operation log viewer (admin only).
import { onMounted, ref } from 'vue';
import { message } from 'ant-design-vue';
import { listOplogs } from '@/api/admin';
import { fmtTime } from '@/utils/format';
import type { OpLog } from '@/types';

const list = ref<OpLog[]>([]);
const total = ref(0);
const page = ref(1);
const keyword = ref('');
const loading = ref(false);

async function load() {
  loading.value = true;
  try {
    const d = await listOplogs({ page: page.value, size: 20, keyword: keyword.value });
    list.value = d.list;
    total.value = d.total;
  } catch (e) {
    message.error(e instanceof Error ? e.message : 'load failed');
  } finally {
    loading.value = false;
  }
}

onMounted(load);
</script>

<template>
  <div class="page-card">
    <div class="toolbar">
      <span class="page-title mb-0">操作日志</span>
      <a-input-search v-model:value="keyword" placeholder="搜索 动作/对象/详情" style="width: 260px" @search="load" />
    </div>
    <a-table :data-source="list" :loading="loading" row-key="id" :pagination="{ current: page, total, pageSize: 20, onChange: (p: number) => (page = p) && load() }">
      <a-table-column title="ID" data-index="id" width="70" />
      <a-table-column title="时间" width="180">
        <template #default="{ record }">{{ fmtTime(record.created_at) }}</template>
      </a-table-column>
      <a-table-column title="操作者" width="130">
        <template #default="{ record }">{{ record.actor_type }}#{{ record.actor_id }}</template>
      </a-table-column>
      <a-table-column title="动作" data-index="action" width="140" />
      <a-table-column title="对象" data-index="target" />
      <a-table-column title="详情" data-index="detail" ellipsis />
    </a-table>
  </div>
</template>
