<script setup lang="ts">
// DNS record management (GoEdge DNS module). Admin cross-tenant; tenant own.
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
const LINE_OPTIONS = ['default', 'telecom', 'unicom', 'mobile', 'oversea', 'search'];

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
    line: 'default',
    weight: 0,
    proxied: 0,
    sync_mode: 'auto',
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
    line: r.line || 'default',
    weight: r.weight || 0,
    proxied: r.proxied || 0,
    sync_mode: r.sync_mode || 'auto',
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
      <a-table-column title="线路" width="90">
        <template #default="{ record }">{{ record.line || 'default' }}</template>
      </a-table-column>
      <a-table-column title="权重" data-index="weight" width="70" />
      <a-table-column title="状态" width="80">
        <template #default="{ record }">
          <a-tag :color="record.enabled ? 'green' : 'default'">{{ record.enabled ? '启用' : '停用' }}</a-tag>
        </template>
      </a-table-column>
      <a-table-column title="同步" width="110">
        <template #default="{ record }">
          <a-tag :color="record.sync_mode === 'manual' ? 'orange' : 'blue'">{{ record.sync_mode === 'manual' ? '手动' : '自动' }}</a-tag>
          <a-tag v-if="record.proxied" color="purple">代理</a-tag>
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
    <a-alert type="info" show-icon class="mt-3" message="DNS 记录会通过后台任务同步到启用的上游服务商；手动同步模式仅保存本地记录，不自动推送。" />

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
          <a-col :span="6">
            <a-form-item label="TTL (秒)">
              <a-input-number v-model:value="form.ttl" :min="60" style="width: 100%" />
            </a-form-item>
          </a-col>
          <a-col :span="6">
            <a-form-item label="状态">
              <a-select v-model:value="form.enabled" :options="[{ value: 1, label: '启用' }, { value: 0, label: '停用' }]" />
            </a-form-item>
          </a-col>
          <a-col :span="6">
            <a-form-item label="线路">
              <a-select
                v-model:value="form.line"
                :options="LINE_OPTIONS.map((line) => ({ value: line, label: line }))"
              />
            </a-form-item>
          </a-col>
          <a-col :span="6">
            <a-form-item label="权重">
              <a-input-number v-model:value="form.weight" :min="0" style="width: 100%" />
            </a-form-item>
          </a-col>
        </a-row>
        <a-row :gutter="16">
          <a-col :span="8">
            <a-form-item label="同步模式">
              <a-select v-model:value="form.sync_mode" :options="[{ value: 'auto', label: '自动同步' }, { value: 'manual', label: '仅本地/手动' }]" />
            </a-form-item>
          </a-col>
          <a-col :span="8">
            <a-form-item label="Cloudflare 代理">
              <a-select v-model:value="form.proxied" :options="[{ value: 0, label: '关闭' }, { value: 1, label: '开启' }]" />
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
