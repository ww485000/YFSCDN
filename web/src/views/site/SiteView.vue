<script setup lang="ts">
// Site management: admin (cross-tenant) and tenant (own sites) share this view.
import { computed, onMounted, reactive, ref } from 'vue';
import { message } from 'ant-design-vue';
import { useAuthStore } from '@/stores/auth';
import { createSite, deleteSite, listSites, updateSite, type Scope } from '@/api/site';
import { listNodesAdmin, listNodesTenant } from '@/api/node';
import { listTenants } from '@/api/admin';
import { fmtTime } from '@/utils/format';
import type { EdgeNode, Site, SiteReq, Tenant } from '@/types';

const auth = useAuthStore();
const scope = computed<Scope>(() => (auth.isTenant ? 'tenant' : 'admin'));

const list = ref<Site[]>([]);
const total = ref(0);
const loading = ref(false);
const page = ref(1);
const keyword = ref('');
const tenantFilter = ref<number | undefined>(undefined);
const tenants = ref<Tenant[]>([]);
const nodes = ref<EdgeNode[]>([]);

const show = ref(false);
const editing = ref<Site | null>(null);
const form = reactive<SiteReq & { tenant_id?: number }>({
  name: '',
  domain: '',
  origin_proto: 'http',
  origin_host: '',
  origin_port: 80,
  cache_ttl: 600,
  waf_enabled: false,
  https: false,
  status: 1,
  node_ids: [],
});

async function load() {
  loading.value = true;
  try {
    const d = await listSites(scope.value, {
      tenantId: scope.value === 'admin' ? tenantFilter.value : undefined,
      keyword: keyword.value,
      page: page.value,
      size: 20,
    });
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
    if (scope.value === 'admin') {
      [tenants.value, nodes.value] = await Promise.all([
        listTenants({ page: 1, size: 200 }).then((d) => d.list),
        listNodesAdmin({ page: 1, size: 200 }).then((d) => d.list),
      ]);
    } else {
      nodes.value = (await listNodesTenant()).list;
    }
  } catch {
    /* non-fatal */
  }
}

function openCreate() {
  editing.value = null;
  Object.assign(form, {
    name: '',
    domain: '',
    origin_proto: 'http',
    origin_host: '',
    origin_port: 80,
    cache_ttl: 600,
    waf_enabled: false,
    https: false,
    status: 1,
    node_ids: [],
    tenant_id: scope.value === 'admin' ? (tenantFilter.value || undefined) : undefined,
  });
  show.value = true;
}

function openEdit(s: Site) {
  editing.value = s;
  Object.assign(form, {
    name: s.name,
    domain: s.domain,
    origin_proto: s.origin_proto,
    origin_host: s.origin_host,
    origin_port: s.origin_port,
    cache_ttl: s.cache_ttl,
    waf_enabled: s.waf_enabled === 1,
    https: s.https === 1,
    status: s.status,
    node_ids: s.node_ids,
    tenant_id: s.tenant_id,
  });
  show.value = true;
}

async function submit() {
  try {
    const body = scope.value === 'admin' ? { ...form, tenant_id: form.tenant_id } : form;
    if (editing.value) {
      await updateSite(scope.value, editing.value.id, body);
      message.success('站点已更新，边缘节点将自动拉取新配置');
    } else {
      await createSite(scope.value, body);
      message.success('站点已创建，边缘节点将自动拉取新配置');
    }
    show.value = false;
    void load();
  } catch (e) {
    message.error(e instanceof Error ? e.message : 'save failed');
  }
}

async function remove(s: Site) {
  try {
    await deleteSite(scope.value, s.id);
    message.success('已删除');
    void load();
  } catch (e) {
    message.error(e instanceof Error ? e.message : 'delete failed');
  }
}

const originText = (s: Site) => `${s.origin_proto}://${s.origin_host}:${s.origin_port}`;

onMounted(() => {
  void load();
  void loadOptions();
});
</script>

<template>
  <div class="page-card">
    <div class="toolbar">
      <span class="page-title mb-0">站点管理</span>
      <a-button type="primary" @click="openCreate">新建站点</a-button>
      <a-input-search v-model:value="keyword" placeholder="搜索 名称/域名" style="width: 220px" @search="load" />
      <a-select
        v-if="scope === 'admin'"
        v-model:value="tenantFilter"
        allow-clear
        placeholder="按租户筛选"
        style="width: 180px"
        :options="tenants.map((t) => ({ value: t.id, label: t.name }))"
        @change="load"
      />
    </div>
    <a-table :data-source="list" :loading="loading" row-key="id" :pagination="{ current: page, total, pageSize: 20, onChange: (p: number) => (page = p) && load() }">
      <a-table-column title="ID" data-index="id" width="56" />
      <a-table-column title="名称" data-index="name" width="140" />
      <a-table-column title="域名" data-index="domain" />
      <template v-if="scope === 'admin'">
        <a-table-column title="租户" data-index="tenant_id" width="70" />
      </template>
      <a-table-column title="回源">
        <template #default="{ record }">{{ originText(record) }}</template>
      </a-table-column>
      <a-table-column title="缓存" width="80">
        <template #default="{ record }">{{ record.cache_ttl > 0 ? `${record.cache_ttl}s` : '关' }}</template>
      </a-table-column>
      <a-table-column title="WAF" width="64">
        <template #default="{ record }">
          <a-tag :color="record.waf_enabled ? 'blue' : 'default'">{{ record.waf_enabled ? '开' : '关' }}</a-tag>
        </template>
      </a-table-column>
      <a-table-column title="HTTPS" width="70">
        <template #default="{ record }">
          <a-tag :color="record.https ? 'green' : 'default'">{{ record.https ? '自签' : '关' }}</a-tag>
        </template>
      </a-table-column>
      <a-table-column title="节点" width="90">
        <template #default="{ record }">{{ record.node_ids?.length || 0 }} 个</template>
      </a-table-column>
      <a-table-column title="创建" width="160">
        <template #default="{ record }">{{ fmtTime(record.created_at) }}</template>
      </a-table-column>
      <a-table-column title="操作" width="130">
        <template #default="{ record }">
          <a @click="openEdit(record)">编辑</a>
          <a-divider type="vertical" />
          <a-popconfirm title="确认删除该站点？" @confirm="remove(record)">
            <a class="text-red-500">删除</a>
          </a-popconfirm>
        </template>
      </a-table-column>
    </a-table>

    <a-modal v-model:open="show" :title="editing ? '编辑站点' : '新建站点'" width="640px" @ok="submit">
      <a-form layout="vertical" :model="form">
        <a-row :gutter="16">
          <a-col :span="12">
            <a-form-item label="名称" required>
              <a-input v-model:value="form.name" placeholder="站点名称" />
            </a-form-item>
          </a-col>
          <a-col :span="12">
            <a-form-item label="域名" required>
              <a-input v-model:value="form.domain" placeholder="www.example.com" :disabled="!!editing" />
            </a-form-item>
          </a-col>
        </a-row>
        <a-row :gutter="16">
          <a-col :span="8">
            <a-form-item label="回源协议">
              <a-select v-model:value="form.origin_proto" :options="[{ value: 'http', label: 'http' }, { value: 'https', label: 'https' }]" />
            </a-form-item>
          </a-col>
          <a-col :span="8">
            <a-form-item label="回源地址" required>
              <a-input v-model:value="form.origin_host" placeholder="IP 或域名" />
            </a-form-item>
          </a-col>
          <a-col :span="8">
            <a-form-item label="回源端口" required>
              <a-input-number v-model:value="form.origin_port" :min="1" :max="65535" style="width: 100%" />
            </a-form-item>
          </a-col>
        </a-row>
        <a-row :gutter="16">
          <a-col :span="8">
            <a-form-item label="缓存 TTL (秒, 0=关)">
              <a-input-number v-model:value="form.cache_ttl" :min="0" style="width: 100%" />
            </a-form-item>
          </a-col>
          <a-col :span="8">
            <a-form-item label="WAF">
              <a-switch v-model:checked="form.waf_enabled" />
            </a-form-item>
          </a-col>
          <a-col :span="8">
            <a-form-item label="HTTPS(自签证书)">
              <a-switch v-model:checked="form.https" />
            </a-form-item>
          </a-col>
        </a-row>
        <a-form-item label="绑定边缘节点" extra="留空 = 广播到全部节点">
          <a-select v-model:value="form.node_ids" mode="multiple" placeholder="选择节点（可多选）" :options="nodes.map((n) => ({ value: n.id, label: `${n.name} (${n.host || '未上线'})` }))" />
        </a-form-item>
        <a-alert v-if="form.https" type="info" show-icon message="HTTPS 由平台自签 CA 签发（演示/内网模式）。浏览器会提示不受信任，可导入平台 CA。" style="margin-bottom: 12px" />
      </a-form>
    </a-modal>
  </div>
</template>
