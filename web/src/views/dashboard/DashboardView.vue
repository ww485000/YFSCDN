<script setup lang="ts">
// Dashboard: admin sees platform stats + trend chart + top sites + oplogs;
// tenant sees own usage.
import { onMounted, ref } from 'vue';
import { useAuthStore } from '@/stores/auth';
import { dashboardSeries, dashboardTopSites, getDashboard, listOplogs } from '@/api/admin';
import { tenantUsageDaily, tenantUsageSummary } from '@/api/site';
import { bytes, fmtTime, num } from '@/utils/format';
import LineChart from '@/components/LineChart.vue';
import type { DashboardStats, DashboardTopSite, DayPoint, OpLog, UsageDaily, UsageSummary } from '@/types';

const auth = useAuthStore();
const stats = ref<DashboardStats | null>(null);
const oplogs = ref<OpLog[]>([]);
const series = ref<DayPoint[]>([]);
const topSites = ref<DashboardTopSite[]>([]);
const summary = ref<UsageSummary | null>(null);
const daily = ref<UsageDaily[]>([]);
const isTenant = auth.isTenant;

const chart = () => {
  const s = series.value;
  if (!s.length) return null;
  return {
    labels: s.map((p) => p.day.slice(5)),
    series: [
      { name: '请求数', color: '#1677ff', values: s.map((p) => p.requests) },
      { name: '流量 (字节)', color: '#52c41a', values: s.map((p) => p.bytes) },
    ],
  };
};

onMounted(async () => {
  if (isTenant) {
    try {
      [summary.value, daily.value] = await Promise.all([
        tenantUsageSummary('tenant'),
        tenantUsageDaily('tenant'),
      ]);
    } catch {
      /* http layer reports */
    }
  } else {
    try {
      const [dashboard, opPage] = await Promise.all([
        getDashboard(),
        listOplogs({ page: 1, size: 10 }),
      ]);
      stats.value = dashboard;
      oplogs.value = opPage.list;
    } catch {
      /* http layer reports */
    }
    try {
      [series.value, topSites.value] = await Promise.all([
        dashboardSeries(14),
        dashboardTopSites(7),
      ]);
    } catch {
      /* http layer reports */
    }
  }
});
</script>

<template>
  <div>
    <div v-if="isTenant">
      <div class="mb-4 grid grid-cols-4 gap-4">
        <div class="stat-card">
          <div class="stat-value">{{ summary ? num(summary.requests) : '-' }}</div>
          <div class="stat-label">累计请求数</div>
        </div>
        <div class="stat-card">
          <div class="stat-value">{{ summary ? bytes(summary.bytes) : '-' }}</div>
          <div class="stat-label">累计流量</div>
        </div>
        <div class="stat-card">
          <div class="stat-value">{{ summary ? num(summary.cache_hits) : '-' }}</div>
          <div class="stat-label">缓存命中</div>
        </div>
        <div class="stat-card">
          <div class="stat-value">{{ summary ? num(summary.cache_misses) : '-' }}</div>
          <div class="stat-label">缓存未命中</div>
        </div>
      </div>
      <div class="page-card">
        <div class="page-title">每日流量</div>
        <a-table :data-source="daily" :pagination="false" size="small" row-key="id">
          <a-table-column title="日期" data-index="day" />
          <a-table-column title="节点" data-index="node_id" />
          <a-table-column title="站点" data-index="site_id" />
          <a-table-column title="请求" data-index="requests" />
          <a-table-column title="流量">
            <template #default="{ text }">{{ bytes(text?.bytes ?? 0) }}</template>
          </a-table-column>
          <a-table-column title="命中/未命中">
            <template #default="{ text }">{{ text.cache_hits }} / {{ text.cache_misses }}</template>
          </a-table-column>
        </a-table>
      </div>
    </div>
    <div v-else>
      <div class="mb-4 grid grid-cols-5 gap-4">
        <div class="stat-card">
          <div class="stat-value">{{ stats?.tenants ?? '-' }}</div>
          <div class="stat-label">租户</div>
        </div>
        <div class="stat-card">
          <div class="stat-value">{{ stats?.sites ?? '-' }}</div>
          <div class="stat-label">站点</div>
        </div>
        <div class="stat-card">
          <div class="stat-value">{{ stats ? `${stats.nodes_online}/${stats.nodes}` : '-' }}</div>
          <div class="stat-label">在线节点/总节点</div>
        </div>
        <div class="stat-card">
          <div class="stat-value">{{ stats ? num(stats.today_requests) : '-' }}</div>
          <div class="stat-label">今日请求</div>
        </div>
        <div class="stat-card">
          <div class="stat-value">{{ stats ? bytes(stats.today_bytes) : '-' }}</div>
          <div class="stat-label">今日流量</div>
        </div>
      </div>
      <div class="page-card mb-4" v-if="chart()">
        <div class="page-title mb-2">近 14 天趋势</div>
        <LineChart :labels="chart()!.labels" :series="chart()!.series" :height="240" />
      </div>
      <div class="grid grid-cols-2 gap-4 mb-4">
        <div class="page-card">
          <div class="page-title mb-2">近 7 天 TOP 站点</div>
          <a-table :data-source="topSites" :pagination="false" size="small" row-key="site_id">
            <a-table-column title="站点" data-index="name" />
            <a-table-column title="域名" data-index="domain" />
            <a-table-column title="请求">
              <template #default="{ text }">{{ num(text.requests) }}</template>
            </a-table-column>
            <a-table-column title="流量">
              <template #default="{ text }">{{ bytes(text.bytes) }}</template>
            </a-table-column>
          </a-table>
        </div>
        <div class="page-card">
          <div class="page-title mb-2">最近操作</div>
          <a-table :data-source="oplogs" :pagination="false" size="small" row-key="id">
            <a-table-column title="时间" data-index="created_at" width="150">
              <template #default="{ text }">{{ fmtTime(text) }}</template>
            </a-table-column>
            <a-table-column title="动作" data-index="action" width="120" />
            <a-table-column title="对象" data-index="target" />
          </a-table>
        </div>
      </div>
    </div>
  </div>
</template>
