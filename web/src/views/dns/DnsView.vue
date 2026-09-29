<script setup lang="ts">
// DNS record management (GoEdge DNS module). Admin cross-tenant; tenant own.
// Provider push (upstream DNS APIs) is a documented stub in this build.
import { computed, onMounted, reactive, ref } from 'vue';
import { message } from 'ant-design-vue';
import { useAuthStore } from '@/stores/auth';
import { createDns, deleteDns, listDns, listDnsZones, updateDns } from '@/api/dns';
import { listTenants } from '@/api/admin';
import type { Scope } from '@/api/site';
import { fmtTime } from '@/utils/format';
import type { DnsRecord, Tenant } from '@/types';

const auth = useAuthStore();
const scope = computed<Scope>(() => (auth.isTenant ? 'tenant' : 'admin'));

const list = ref<DnsRecord[]>([]);
const total = ref(0);
const loading = ref(false);
const page = ref(1);
const zone = ref<string | undefined>(undefined);
const zones = ref<string[]>([]);
const tenants = ref<Tenant[]>([]);

const show = ref(false);
const editing = ref<DnsRecord | null>(null);
const form = reactive<Partial<DnsRecord> & { tenant_id?: number }>({});

const TYPES = ['A', 'AAAA', 'CNAME', 'MX', 'TXT', 'NS', 'SRV'];

async function load() {
  loading.value = true;
  try {
    const d = await listDns(scope.value, { domain: zone.value, page: page.value, size: 20 });
    list.value = d.list;
    total.value = d.total;
  } catch (e) {
    message.error(e instanceof Error ? e.message : 'load failed');
  } finally {
    loading.value = false;
  }
}

async function loadOptions() {
  try {
    zones.value = await listDnsZones(scope.value);
    if (scope.value === 'admin') {
      tenants.value = (await listTenants({ page: 1, size: 200 })).list;
    }
  } catch {
    /* non-fatal */
  }
}

function openCreate() {
  editing.value = null;
  Object.assign(form, {
    domain: zone.value || '',
    name: '',
    type: 'A',
    value: '',
    priority: 0,
    ttl: 600,
    enabled: 1,
    remark: '',
    tenant_id: scope.value === 'admin' ? undefined : undefined,
  });
  show.value = true;
}

function openEdit(r: DnsRecord) {
  editing.value = r;
  Object.assign(form, {
    tenant_id: r.tenant_id,
    domain: r.domain,
    name: r.name,
    type: r.type,
    value: r.value,
    priority: r.priority,
    ttl: r.ttl,
    enabled: r.enabled,
    remark: r.remark,
  });
  show.value = true;
}

async function submit() {
  try {
    if (editing.value) {
      await updateDns(scope.value, editing.value.id, form);
      message.success('记录已更新');
    } else {
      await createDns(scope.value, form);
      message.success('记录已创建');
    }
    show.value = false;
    void load();
    void loadOptions();
  } catch (e) {
    message.error(e instanceof Error ? e.message : 'save failed');
  }
}

async function remove(r: DnsRecord) {
  try {
    await deleteDns(scope.value, r.id);
    message.success('已删除');
    void load();
    void loadOptions();
  } catch (e) {
    message.error(e instanceof Error ? e.message : 'delete failed');
  }
}

const tenantName = (tid: number) => tenants.value.find((t) => t.id === tid)?.name || `#${tid}`;

onMounted(() => {
  void load();
  void loadOptions();
});
</script>

<template>
  <div class="page-card">
    <div class="toolbar">
      <span class="page-title mb-0">DNS 解析</span>
      <a-select
        v-model:value="zone"
        allow-clear
        placeholder="按区域筛选"
        style="width: 200px"
        :options="zones.map((z) => ({ value: z, label: z }))"
        @change="load"
      />
      <a-button @click="load">刷新</a-button>
      <a-button type="primary" @click="openCreate">添加记录</a-button>
    </div>
    <a-table :data-source="list" :loading="loading" row-key="id" :pagination="{ current: page, total, pageSize: 20, onChange: (p: number) => (page = p) && load() }">
      <a-table-column title="ID" data-index="id" width="56" />
      <a-table-column v-if="scope === 'admin'" title="租户" width="120">
        <template #default="{ record }">{{ tenantName(record.tenant_id) }}</template>
      </a-table-column>
      <a-table-column title="区域" data-index="domain" width="160" />
      <a-table-column title="主机记录" data-index="name" width="180" />
      <a-table-column title="类型" width="80">
        <template #default="{ record }"><a-tag>{{ record.type }}</a-tag></template>
      </a-table-column>
      <a-table-column title="记录值">
        <template #default="{ record }">
          {{ record.type === 'MX' ? `${record.priority} ${record.value}` : record.value }}
        </template>
      </a-table-column>
      <a-table-column title="TTL" data-index="ttl" width="70" />
      <a-table-column title="状态" width="80">
        <template #default="{ record }">
          <a-tag :color="record.enabled ? 'green' : 'default'">{{ record.enabled ? '启用' : '停用' }}</a-tag>
        </template>
      </a-table-column>
      <a-table-column title="备注" data-index="remark" width="140" />
      <a-table-column title="创建" width="160">
        <template #default="{ record }">{{ fmtTime(record.created_at) }}</template>
      </a-table-column>
      <a-table-column title="操作" width="130">
        <template #default="{ record }">
          <a @click="openEdit(record)">编辑</a>
          <a-divider type="vertical" />
          <a-popconfirm title="确认删除该记录？" @confirm="remove(record)">
            <a class="text-red-500">删除</a>
          </a-popconfirm>
        </template>
      </a-table-column>
    </a-table>
    <a-alert type="info" show-icon class="mt-3" message="记录在本平台管理；上游 DNS 服务商推送（云厂商 API）为预留集成点（任务类型 dns_resolve/dns_clean），当前构建不做外部调用。" />

    <a-modal v-model:open="show" :title="editing ? '编辑记录' : '添加记录'" width="560px" @ok="submit">
      <a-form layout="vertical" :model="form">
        <a-row :gutter="16">
          <a-col :span="12">
            <a-form-item label="租户" required v-if="scope === 'admin'">
              <a-select v-model:value="form.tenant_id" :options="tenants.map((t) => ({ value: t.id, label: t.name }))" />
            </a-form-item>
          </a-col>
          <a-col :span="12">
            <a-form-item label="区域 (apex)" required>
              <a-input v-model:value="form.domain" placeholder="example.com" />
            </a-form-item>
          </a-col>
        </a-row>
        <a-row :gutter="16">
          <a-col :span="12">
            <a-form-item label="主机记录" extra="留空 = 区域本身 (@)">
              <a-input v-model:value="form.name" placeholder="www / @ / 留空" />
            </a-form-item>
          </a-col>
          <a-col :span="12">
            <a-form-item label="类型">
              <a-select v-model:value="form.type" :options="TYPES.map((t) => ({ value: t, label: t }))" />
            </a-form-item>
          </a-col>
        </a-row>
        <a-row :gutter="16">
          <a-col :span="form.type === 'MX' ? 16 : 24">
            <a-form-item label="记录值" required>
              <a-input v-model:value="form.value" placeholder="1.2.3.4 / example.com" />
            </a-form-item>
          </a-col>
          <a-col :span="8" v-if="form.type === 'MX'">
            <a-form-item label="优先级">
              <a-input-number v-model:value="form.priority" :min="0" style="width: 100%" />
            </a-form-item>
          </a-col>
        </a-row>
        <a-row :gutter="16">
          <a-col :span="8">
            <a-form-item label="TTL (秒)">
              <a-input-number v-model:value="form.ttl" :min="60" style="width: 100%" />
            </a-form-item>
          </a-col>
          <a-col :span="8">
            <a-form-item label="状态">
              <a-select v-model:value="form.enabled" :options="[{ value: 1, label: '启用' }, { value: 0, label: '停用' }]" />
            </a-form-item>
          </a-col>
          <a-col :span="8">
            <a-form-item label="备注">
              <a-input v-model:value="form.remark" />
            </a-form-item>
          </a-col>
        </a-row>
      </a-form>
    </a-modal>
  </div>
</template>
