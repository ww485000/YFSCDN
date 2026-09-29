<script setup lang="ts">
// Usage (traffic metering): admin can filter by tenant; tenant sees own.
import { computed, onMounted, ref } from 'vue';
import { message } from 'ant-design-vue';
import { useAuthStore } from '@/stores/auth';
import { usageDaily, usageSummary } from '@/api/admin';
import { tenantUsageDaily, tenantUsageSummary } from '@/api/site';
import { listTenants } from '@/api/admin';
import { bytes } from '@/utils/format';
import LineChart from '@/components/LineChart.vue';
import type { Tenant, UsageDaily, UsageSummary } from '@/types';

const auth = useAuthStore();
const isAdmin = computed(() => !auth.isTenant);

const tenants = ref<Tenant[]>([]);
const tenantFilter = ref<number | undefined>(undefined);
const summary = ref<UsageSummary | null>(null);
const daily = ref<UsageDaily[]>([]);
const loading = ref(false);

// aggregate per-day series from the daily rows
const chart = computed(() => {
  const m = new Map<string, { requests: number; bytes: number }>();
  for (const d of daily.value) {
    const cur = m.get(d.day) || { requests: 0, bytes: 0 };
    cur.requests += d.requests;
    cur.bytes += d.bytes;
    m.set(d.day, cur);
  }
  const labels = Array.from(m.keys()).sort();
  if (labels.length === 0) return null;
  return {
    labels,
    series: [
      { name: '请求数', color: '#1677ff', values: labels.map((l) => m.get(l)!.requests) },
      { name: '流量 (字节)', color: '#52c41a', values: labels.map((l) => m.get(l)!.bytes) },
    ],
  };
});

async function load() {
  loading.value = true;
  try {
    if (isAdmin.value) {
      const tid = tenantFilter.value || 0;
      [summary.value, daily.value] = await Promise.all([usageSummary(tid), usageDaily(tid)]);
    } else {
      [summary.value, daily.value] = await Promise.all([tenantUsageSummary('tenant'), tenantUsageDaily('tenant')]);
    }
  } catch (e) {
    message.error(e instanceof Error ? e.message : 'load failed');
  } finally {
    loading.value = false;
  }
}

onMounted(async () => {
  if (isAdmin.value) {
    try {
      tenants.value = (await listTenants({ page: 1, size: 200 })).list;
    } catch {
      /* non-fatal */
    }
  }
  void load();
});
</script>

<template>
  <div class="page-card">
    <div class="toolbar">
      <span class="page-title mb-0">流量计量</span>
      <a-select
        v-if="isAdmin"
        v-model:value="tenantFilter"
        allow-clear
        placeholder="全部租户"
        style="width: 200px"
        :options="tenants.map((t) => ({ value: t.id, label: t.name }))"
        @change="load"
      />
      <a-button :loading="loading" @click="load">刷新</a-button>
    </div>

    <div class="mb-4 grid grid-cols-5 gap-4">
      <div class="stat-card">
        <div class="stat-value">{{ summary ? summary.requests : '-' }}</div>
        <div class="stat-label">请求总数</div>
      </div>
      <div class="stat-card">
        <div class="stat-value">{{ summary ? bytes(summary.bytes) : '-' }}</div>
        <div class="stat-label">流量总量</div>
      </div>
      <div class="stat-card">
        <div class="stat-value">{{ summary ? summary.cache_hits : '-' }}</div>
        <div class="stat-label">缓存命中</div>
      </div>
      <div class="stat-card">
        <div class="stat-value">{{ summary ? summary.cache_misses : '-' }}</div>
        <div class="stat-label">缓存未命中</div>
      </div>
      <div class="stat-card">
        <div class="stat-value">{{ summary ? summary.days : '-' }}</div>
        <div class="stat-label">统计天数</div>
      </div>
    </div>

    <div class="page-card mb-4" v-if="chart">
      <div class="page-title mb-2">趋势（按天汇总）</div>
      <LineChart :labels="chart.labels" :series="chart.series" :height="240" />
    </div>

    <a-table :data-source="daily" :loading="loading" row-key="id" :pagination="{ pageSize: 20 }" size="small">
      <a-table-column title="日期" data-index="day" width="110" />
      <template v-if="isAdmin">
        <a-table-column title="租户" data-index="tenant_id" width="70" />
      </template>
      <a-table-column title="节点" data-index="node_id" width="70" />
      <a-table-column title="站点" data-index="site_id" width="70" />
      <a-table-column title="请求" data-index="requests" />
      <a-table-column title="流量">
        <template #default="{ record }">{{ bytes(record.bytes) }}</template>
      </a-table-column>
      <a-table-column title="命中" data-index="cache_hits" />
      <a-table-column title="未命中" data-index="cache_misses" />
    </a-table>
  </div>
</template>
