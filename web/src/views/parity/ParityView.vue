<script setup lang="ts">
// GoEdge feature parity page: operator-facing implementation checklist.
import { computed, onMounted, ref } from 'vue';
import { message } from 'ant-design-vue';
import { goEdgeParity } from '@/api/admin';
import type { GoEdgeParityItem, GoEdgeParityMatrix, GoEdgeParityStatus } from '@/types';

const matrix = ref<GoEdgeParityMatrix | null>(null);
const loading = ref(false);
const statusFilter = ref<GoEdgeParityStatus | undefined>(undefined);
const categoryFilter = ref<string | undefined>(undefined);

async function load() {
  loading.value = true;
  try {
    matrix.value = await goEdgeParity();
  } catch (e) {
    message.error(e instanceof Error ? e.message : 'load failed');
  } finally {
    loading.value = false;
  }
}

const categories = computed(() => {
  const set = new Set((matrix.value?.items || []).map((i) => i.category));
  return Array.from(set).map((value) => ({ value, label: value }));
});

const rows = computed(() => {
  return (matrix.value?.items || []).filter((item) => {
    if (statusFilter.value && item.status !== statusFilter.value) return false;
    if (categoryFilter.value && item.category !== categoryFilter.value) return false;
    return true;
  });
});

const percent = computed(() => {
  const s = matrix.value?.summary;
  if (!s?.total) return 0;
  return Math.round((s.done / s.total) * 100);
});

function statusColor(s: GoEdgeParityStatus) {
  return s === 'done' ? 'green' : s === 'partial' ? 'gold' : 'red';
}

function statusText(s: GoEdgeParityStatus) {
  return s === 'done' ? '已完成' : s === 'partial' ? '部分完成' : '未实现';
}

function rowClass(record: GoEdgeParityItem) {
  return `parity-row-${record.status}`;
}

onMounted(() => void load());
</script>

<template>
  <div class="page-card">
    <div class="toolbar">
      <span class="page-title mb-0">GoEdge 复刻矩阵</span>
      <a-select
        v-model:value="statusFilter"
        allow-clear
        placeholder="状态"
        style="width: 140px"
        :options="[
          { value: 'done', label: '已完成' },
          { value: 'partial', label: '部分完成' },
          { value: 'missing', label: '未实现' },
        ]"
      />
      <a-select
        v-model:value="categoryFilter"
        allow-clear
        placeholder="模块分类"
        style="width: 150px"
        :options="categories"
      />
      <a-button @click="load">刷新</a-button>
    </div>

    <a-row :gutter="16" class="mb-4">
      <a-col :span="6">
        <div class="stat-card">
          <div class="stat-value">{{ matrix?.summary.total || 0 }}</div>
          <div class="stat-label">GoEdge 功能项</div>
        </div>
      </a-col>
      <a-col :span="6">
        <div class="stat-card">
          <div class="stat-value text-green-600">{{ matrix?.summary.done || 0 }}</div>
          <div class="stat-label">已完成</div>
        </div>
      </a-col>
      <a-col :span="6">
        <div class="stat-card">
          <div class="stat-value text-amber-600">{{ matrix?.summary.partial || 0 }}</div>
          <div class="stat-label">部分完成</div>
        </div>
      </a-col>
      <a-col :span="6">
        <div class="stat-card">
          <div class="stat-value text-red-600">{{ matrix?.summary.missing || 0 }}</div>
          <div class="stat-label">未实现</div>
        </div>
      </a-col>
    </a-row>

    <a-progress class="mb-4" :percent="percent" status="active" />

    <a-table
      :data-source="rows"
      :loading="loading"
      row-key="code"
      :pagination="{ pageSize: 20 }"
      :row-class-name="rowClass"
    >
      <a-table-column title="功能" data-index="name" width="170" />
      <a-table-column title="分类" data-index="category" width="90" />
      <a-table-column title="状态" width="100">
        <template #default="{ record }">
          <a-tag :color="statusColor(record.status)">{{ statusText(record.status) }}</a-tag>
        </template>
      </a-table-column>
      <a-table-column title="YFSCDN 模块" data-index="module" width="180" />
      <a-table-column title="入口" width="170">
        <template #default="{ record }">
          <router-link v-if="record.admin_path.startsWith('/admin/')" :to="record.admin_path">运营端</router-link>
          <span v-else>{{ record.admin_path }}</span>
          <template v-if="record.user_path">
            <a-divider type="vertical" />
            <router-link :to="record.user_path">租户端</router-link>
          </template>
        </template>
      </a-table-column>
      <a-table-column title="说明" data-index="notes" />
      <a-table-column title="下一步" data-index="next_action" />
    </a-table>
  </div>
</template>

<style scoped>
:deep(.parity-row-missing) td {
  background: #fff7f7;
}

:deep(.parity-row-partial) td {
  background: #fffbe6;
}
</style>
