<script setup lang="ts">
// DNS provider management. The cloud adapters are wired by task type, so adding
// a real provider later stays inside the dnsprovider module.
import { computed, onMounted, reactive, ref } from 'vue';
import { message } from 'ant-design-vue';
import { useAuthStore } from '@/stores/auth';
import { createDnsProvider, deleteDnsProvider, listDnsProviders, syncDnsProvider, updateDnsProvider } from '@/api/dns';
import { listTenants } from '@/api/admin';
import type { Scope } from '@/api/site';
import { fmtTime } from '@/utils/format';
import type { DnsProvider, Tenant } from '@/types';

const auth = useAuthStore();
const scope = computed<Scope>(() => (auth.isTenant ? 'tenant' : 'admin'));

const list = ref<DnsProvider[]>([]);
const total = ref(0);
const loading = ref(false);
const saving = ref(false);
const syncing = ref<Record<number, boolean>>({});
const page = ref(1);
const tenantId = ref<number | undefined>();
const tenants = ref<Tenant[]>([]);

const show = ref(false);
const editing = ref<DnsProvider | null>(null);
const form = reactive<Partial<DnsProvider> & { tenant_id?: number }>({});

const providerTypes = [
  { value: 'manual', label: 'Manual' },
  { value: 'mock', label: 'Mock' },
  { value: 'dnspod', label: 'DNSPod' },
  { value: 'cloudflare', label: 'Cloudflare' },
  { value: 'aliyun', label: 'Aliyun DNS' },
];

async function load() {
  loading.value = true;
  try {
    const d = await listDnsProviders(scope.value, { tenantId: tenantId.value, page: page.value, size: 20 });
    list.value = d.list;
    total.value = d.total;
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

function openCreate() {
  editing.value = null;
  Object.assign(form, {
    tenant_id: tenantId.value,
    name: '',
    type: 'manual',
    access_key: '',
    secret_key: '',
    api_endpoint: '',
    status: 1,
    remark: '',
  });
  show.value = true;
}

function openEdit(row: DnsProvider) {
  editing.value = row;
  Object.assign(form, {
    tenant_id: row.tenant_id,
    name: row.name,
    type: row.type,
    access_key: row.access_key,
    secret_key: '',
    api_endpoint: row.api_endpoint,
    status: row.status,
    remark: row.remark,
  });
  show.value = true;
}

async function submit() {
  saving.value = true;
  try {
    if (editing.value) {
      await updateDnsProvider(scope.value, editing.value.id, form);
      message.success('服务商已更新');
    } else {
      await createDnsProvider(scope.value, form);
      message.success('服务商已创建');
    }
    show.value = false;
    void load();
  } catch (e) {
    message.error(e instanceof Error ? e.message : 'save failed');
  } finally {
    saving.value = false;
  }
}

async function remove(row: DnsProvider) {
  try {
    await deleteDnsProvider(scope.value, row.id);
    message.success('已删除');
    void load();
  } catch (e) {
    message.error(e instanceof Error ? e.message : 'delete failed');
  }
}

async function sync(row: DnsProvider) {
  syncing.value[row.id] = true;
  try {
    const out = await syncDnsProvider(scope.value, row.id);
    message.success(`同步任务已创建 #${out.task_id}`);
  } catch (e) {
    message.error(e instanceof Error ? e.message : 'sync failed');
  } finally {
    syncing.value[row.id] = false;
  }
}

function tenantName(tid: number) {
  return tenants.value.find((t) => t.id === tid)?.name || `#${tid}`;
}

function typeLabel(type: string) {
  return providerTypes.find((p) => p.value === type)?.label || type;
}

onMounted(() => {
  void load();
  void loadTenants();
});
</script>

<template>
  <div class="page-card">
    <div class="toolbar">
      <span class="page-title mb-0">DNS 服务商</span>
      <a-select
        v-if="scope === 'admin'"
        v-model:value="tenantId"
        allow-clear
        placeholder="按租户筛选"
        style="width: 200px"
        :options="tenants.map((t) => ({ value: t.id, label: t.name }))"
        @change="load"
      />
      <a-button @click="load">刷新</a-button>
      <a-button type="primary" @click="openCreate">添加服务商</a-button>
    </div>

    <a-table
      :data-source="list"
      :loading="loading"
      row-key="id"
      :pagination="{ current: page, total, pageSize: 20, onChange: (p: number) => (page = p) && load() }"
    >
      <a-table-column title="ID" data-index="id" width="64" />
      <a-table-column v-if="scope === 'admin'" title="租户" width="140">
        <template #default="{ record }">{{ tenantName(record.tenant_id) }}</template>
      </a-table-column>
      <a-table-column title="名称" data-index="name" width="180" />
      <a-table-column title="类型" width="130">
        <template #default="{ record }">
          <a-tag color="blue">{{ typeLabel(record.type) }}</a-tag>
        </template>
      </a-table-column>
      <a-table-column title="Access Key" data-index="access_key" width="180" />
      <a-table-column title="API Endpoint" data-index="api_endpoint" />
      <a-table-column title="状态" width="88">
        <template #default="{ record }">
          <a-tag :color="record.status ? 'green' : 'default'">{{ record.status ? '启用' : '停用' }}</a-tag>
        </template>
      </a-table-column>
      <a-table-column title="备注" data-index="remark" width="160" />
      <a-table-column title="更新" width="160">
        <template #default="{ record }">{{ fmtTime(record.updated_at || record.created_at) }}</template>
      </a-table-column>
      <a-table-column title="操作" width="190" fixed="right">
        <template #default="{ record }">
          <a :class="{ 'text-gray-400': syncing[record.id] }" @click="sync(record)">同步</a>
          <a-divider type="vertical" />
          <a @click="openEdit(record)">编辑</a>
          <a-divider type="vertical" />
          <a-popconfirm title="确认删除该服务商？" @confirm="remove(record)">
            <a class="text-red-500">删除</a>
          </a-popconfirm>
        </template>
      </a-table-column>
    </a-table>

    <a-alert
      type="info"
      show-icon
      class="mt-3"
      message="已具备服务商配置、租户隔离、同步任务入队，以及 Cloudflare / DNSPod / Aliyun 真实 API 同步。Cloudflare Token 填 Secret Key；DNSPod/阿里云 AccessKeyId 填 Access Key、AccessKeySecret 填 Secret Key。"
    />

    <a-modal v-model:open="show" :title="editing ? '编辑 DNS 服务商' : '添加 DNS 服务商'" width="640px" :confirm-loading="saving" @ok="submit">
      <a-form layout="vertical" :model="form">
        <a-row :gutter="16">
          <a-col :span="12">
            <a-form-item v-if="scope === 'admin'" label="租户" required>
              <a-select v-model:value="form.tenant_id" :options="tenants.map((t) => ({ value: t.id, label: t.name }))" />
            </a-form-item>
          </a-col>
          <a-col :span="12">
            <a-form-item label="名称" required>
              <a-input v-model:value="form.name" placeholder="主账号 / 客户 A DNS" />
            </a-form-item>
          </a-col>
        </a-row>
        <a-row :gutter="16">
          <a-col :span="12">
            <a-form-item label="类型" required>
              <a-select v-model:value="form.type" :options="providerTypes" />
            </a-form-item>
          </a-col>
          <a-col :span="12">
            <a-form-item label="状态">
              <a-select v-model:value="form.status" :options="[{ value: 1, label: '启用' }, { value: 0, label: '停用' }]" />
            </a-form-item>
          </a-col>
        </a-row>
        <a-row :gutter="16">
          <a-col :span="12">
            <a-form-item label="Access Key">
              <a-input v-model:value="form.access_key" autocomplete="off" />
            </a-form-item>
          </a-col>
          <a-col :span="12">
            <a-form-item label="Secret Key" :extra="editing ? '留空则保持原密钥' : ''">
              <a-input-password v-model:value="form.secret_key" autocomplete="new-password" />
            </a-form-item>
          </a-col>
        </a-row>
        <a-form-item label="API Endpoint">
          <a-input v-model:value="form.api_endpoint" placeholder="可选；私有化或代理 API 地址" />
        </a-form-item>
        <a-form-item label="备注">
          <a-textarea v-model:value="form.remark" :rows="3" />
        </a-form-item>
      </a-form>
    </a-modal>
  </div>
</template>
