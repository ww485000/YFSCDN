<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { useRouter } from 'vue-router';
import { message } from 'ant-design-vue';
import { useAuthStore } from '@/stores/auth';
import { listDnsSyncs, retryDnsSync } from '@/api/dns';
import { listTenants } from '@/api/admin';
import type { Scope } from '@/api/site';
import { fmtTime } from '@/utils/format';
import type { DnsSync, Tenant } from '@/types';

const auth = useAuthStore();
const router = useRouter();
const scope = computed<Scope>(() => (auth.isTenant ? 'tenant' : 'admin'));

const list = ref<DnsSync[]>([]);
const tenants = ref<Tenant[]>([]);
const total = ref(0);
const page = ref(1);
const loading = ref(false);
const retrying = ref(false);
const selectedRowKeys = ref<number[]>([]);
const tenantId = ref<number | undefined>();
const status = ref<string | undefined>();

const statusOptions = [
  { value: 'success', label: '成功' },
  { value: 'failed', label: '失败' },
];

async function load() {
  loading.value = true;
  try {
    const d = await listDnsSyncs(scope.value, { tenantId: tenantId.value, status: status.value, page: page.value, size: 20 });
    list.value = d.list;
    total.value = d.total;
    selectedRowKeys.value = selectedRowKeys.value.filter((id) => d.list.some((row) => row.id === id));
  } catch (e) {
    message.error(e instanceof Error ? e.message : 'load failed');
  } finally {
    loading.value = false;
  }
}

async function loadTenants() {
  if (scope.value !== 'admin') return;
  try {
    tenants.value = (await listTenants({ page: 1, size: 200 })).list;
  } catch {
    /* non-fatal */
  }
}

function tenantName(tid: number) {
  return tenants.value.find((t) => t.id === tid)?.name || `#${tid}`;
}

function fqdn(row: DnsSync) {
  if (!row.name) return `#${row.record_id}`;
  return row.name === row.domain ? row.domain : row.name;
}

function openTasks() {
  void router.push(scope.value === 'admin' ? '/admin/tasks' : '/portal/tasks');
}

async function retryOne(row: DnsSync) {
  try {
    const out = await retryDnsSync(scope.value, row.id);
    message.success(`已创建重试任务 #${out.task_id}`);
    void load();
  } catch (e) {
    message.error(e instanceof Error ? e.message : 'retry failed');
  }
}

async function retrySelected() {
  if (!selectedRowKeys.value.length) {
    message.warning('请选择需要重试的同步记录');
    return;
  }
  retrying.value = true;
  try {
    let ok = 0;
    for (const id of selectedRowKeys.value) {
      await retryDnsSync(scope.value, id);
      ok += 1;
    }
    message.success(`已提交 ${ok} 个重试任务`);
    selectedRowKeys.value = [];
    void load();
  } catch (e) {
    message.error(e instanceof Error ? e.message : 'retry failed');
  } finally {
    retrying.value = false;
  }
}

onMounted(() => {
  void load();
  void loadTenants();
});
</script>

<template>
  <div class="page-card">
    <div class="toolbar">
      <span class="page-title mb-0">DNS 同步</span>
      <a-select
        v-if="scope === 'admin'"
        v-model:value="tenantId"
        allow-clear
        placeholder="按租户筛选"
        style="width: 200px"
        :options="tenants.map((t) => ({ value: t.id, label: t.name }))"
        @change="load"
      />
      <a-select
        v-model:value="status"
        allow-clear
        placeholder="同步状态"
        style="width: 140px"
        :options="statusOptions"
        @change="load"
      />
      <a-button @click="load">刷新</a-button>
      <a-button :loading="retrying" :disabled="!selectedRowKeys.length" @click="retrySelected">批量重试</a-button>
      <a-button v-if="scope === 'admin'" @click="openTasks">后台任务</a-button>
    </div>

    <a-table
      :data-source="list"
      :loading="loading"
      row-key="id"
      :row-selection="{ selectedRowKeys, onChange: (keys: number[]) => (selectedRowKeys = keys) }"
      :pagination="{ current: page, total, pageSize: 20, onChange: (p: number) => (page = p) && load() }"
    >
      <a-table-column title="ID" data-index="id" width="64" />
      <a-table-column v-if="scope === 'admin'" title="租户" width="130">
        <template #default="{ record }">{{ tenantName(record.tenant_id) }}</template>
      </a-table-column>
      <a-table-column title="服务商" width="180">
        <template #default="{ record }">
          <div class="font-500">{{ record.provider_name }}</div>
          <div class="text-xs text-gray-400">{{ record.provider_type }} #{{ record.provider_id }}</div>
        </template>
      </a-table-column>
      <a-table-column title="记录" width="260">
        <template #default="{ record }">
          <div class="font-500">{{ fqdn(record) }}</div>
          <div class="text-xs text-gray-400">{{ record.record_type || '-' }} {{ record.value || '' }}</div>
        </template>
      </a-table-column>
      <a-table-column title="上游 ID" data-index="upstream_id" width="190" />
      <a-table-column title="状态" width="90">
        <template #default="{ record }">
          <a-tag :color="record.last_error ? 'red' : 'green'">{{ record.last_error ? '失败' : '成功' }}</a-tag>
        </template>
      </a-table-column>
      <a-table-column title="最后错误">
        <template #default="{ record }">
          <a-tooltip v-if="record.last_error" :title="record.last_error">
            <span class="text-red-500">{{ record.last_error }}</span>
          </a-tooltip>
          <span v-else class="text-gray-400">-</span>
        </template>
      </a-table-column>
      <a-table-column title="更新时间" width="170">
        <template #default="{ record }">{{ fmtTime(record.updated_at) }}</template>
      </a-table-column>
      <a-table-column title="操作" width="80" fixed="right">
        <template #default="{ record }">
          <a @click="retryOne(record)">重试</a>
        </template>
      </a-table-column>
    </a-table>

    <a-alert
      type="info"
      show-icon
      class="mt-3"
      message="这里展示每条 DNS 记录同步到上游服务商的结果；修正凭据或记录后，可选择失败项批量重试，也可以回到 DNS 服务商页面执行整组同步。"
    />
  </div>
</template>
