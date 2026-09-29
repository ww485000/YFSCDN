<script setup lang="ts">
// WAF rule management: admin (cross-tenant) and tenant (own rules) share this view.
import { computed, onMounted, reactive, ref } from 'vue';
import { message } from 'ant-design-vue';
import { useAuthStore } from '@/stores/auth';
import { createWafRule, deleteWafRule, listWafRules, updateWafRule, type Scope } from '@/api/site';
import { listTenants } from '@/api/admin';
import { fmtTime } from '@/utils/format';
import type { Tenant, WafRule } from '@/types';

const auth = useAuthStore();
const scope = computed<Scope>(() => (auth.isTenant ? 'tenant' : 'admin'));

const list = ref<WafRule[]>([]);
const total = ref(0);
const loading = ref(false);
const tenantFilter = ref<number | undefined>(undefined);
const tenants = ref<Tenant[]>([]);

const show = ref(false);
const editing = ref<WafRule | null>(null);
const form = reactive({ name: '', type: 'ip_blacklist', value: '', action: 'block', enabled: 1 });

const typeOptions = [
  { value: 'ip_blacklist', label: 'IP 黑名单 (IP 或 CIDR, 逗号分隔)' },
  { value: 'ip_whitelist', label: 'IP 白名单 (仅放行)' },
  { value: 'ua_blacklist', label: 'UA 黑名单 (子串)' },
  { value: 'path_blacklist', label: '路径黑名单 (前缀, 逗号分隔)' },
  { value: 'rate_limit', label: '限流 (N/60s, 如 100/60s)' },
];

async function load() {
  loading.value = true;
  try {
    const d = await listWafRules(scope.value, scope.value === 'admin' ? (tenantFilter.value || 0) : 0);
    list.value = d.list;
    total.value = d.total;
  } catch (e) {
    message.error(e instanceof Error ? e.message : 'load failed');
  } finally {
    loading.value = false;
  }
}

async function loadOptions() {
  if (scope.value !== 'admin') return;
  try {
    tenants.value = (await listTenants({ page: 1, size: 200 })).list;
  } catch {
    /* non-fatal */
  }
}

function openCreate() {
  editing.value = null;
  Object.assign(form, { name: '', type: 'ip_blacklist', value: '', action: 'block', enabled: 1 });
  show.value = true;
}

function openEdit(r: WafRule) {
  editing.value = r;
  Object.assign(form, { name: r.name, type: r.type, value: r.value, action: r.action, enabled: r.enabled });
  show.value = true;
}

async function submit() {
  try {
    if (editing.value) {
      await updateWafRule(scope.value, editing.value.id, form);
      message.success('规则已更新，相关站点配置将重新下发');
    } else {
      await createWafRule(scope.value, {
        ...form,
        tenant_id: scope.value === 'admin' ? tenantFilter.value : undefined,
      } as Parameters<typeof createWafRule>[1]);
      message.success('规则已创建，相关站点配置将重新下发');
    }
    show.value = false;
    void load();
  } catch (e) {
    message.error(e instanceof Error ? e.message : 'save failed');
  }
}

async function remove(r: WafRule) {
  try {
    await deleteWafRule(scope.value, r.id);
    message.success('已删除');
    void load();
  } catch (e) {
    message.error(e instanceof Error ? e.message : 'delete failed');
  }
}

onMounted(() => {
  void load();
  void loadOptions();
});
</script>

<template>
  <div class="page-card">
    <div class="toolbar">
      <span class="page-title mb-0">WAF 规则</span>
      <a-button type="primary" @click="openCreate">新建规则</a-button>
      <a-select
        v-if="scope === 'admin'"
        v-model:value="tenantFilter"
        allow-clear
        placeholder="按租户筛选"
        style="width: 180px"
        :options="tenants.map((t) => ({ value: t.id, label: t.name }))"
        @change="load"
      />
      <a-alert type="warning" show-icon message="规则对开启了 WAF 的站点生效；保存后自动重新下发到边缘节点。" style="max-width: 560px" />
    </div>
    <a-table :data-source="list" :loading="loading" row-key="id" :pagination="{ total, pageSize: 50 }">
      <a-table-column title="ID" data-index="id" width="56" />
      <a-table-column title="名称" data-index="name" width="160" />
      <template v-if="scope === 'admin'">
        <a-table-column title="租户" data-index="tenant_id" width="70" />
      </template>
      <a-table-column title="类型" data-index="type" width="140" />
      <a-table-column title="值" data-index="value" />
      <a-table-column title="动作" width="80">
        <template #default="{ record }">
          <a-tag :color="record.action === 'block' ? 'red' : 'orange'">{{ record.action }}</a-tag>
        </template>
      </a-table-column>
      <a-table-column title="启用" width="70">
        <template #default="{ record }">
          <a-tag :color="record.enabled ? 'green' : 'default'">{{ record.enabled ? '是' : '否' }}</a-tag>
        </template>
      </a-table-column>
      <a-table-column title="创建" width="160">
        <template #default="{ record }">{{ fmtTime(record.created_at) }}</template>
      </a-table-column>
      <a-table-column title="操作" width="130">
        <template #default="{ record }">
          <a @click="openEdit(record)">编辑</a>
          <a-divider type="vertical" />
          <a-popconfirm title="确认删除该规则？" @confirm="remove(record)">
            <a class="text-red-500">删除</a>
          </a-popconfirm>
        </template>
      </a-table-column>
    </a-table>

    <a-modal v-model:open="show" :title="editing ? '编辑规则' : '新建规则'" @ok="submit">
      <a-form layout="vertical" :model="form">
        <a-form-item label="名称" required>
          <a-input v-model:value="form.name" placeholder="如：封禁内网爬虫" />
        </a-form-item>
        <a-form-item label="类型" required>
          <a-select v-model:value="form.type" :options="typeOptions" />
        </a-form-item>
        <a-form-item label="值" required>
          <a-input v-model:value="form.value" placeholder="按类型填写：10.0.0.0/8 | curl | /admin,/api | 100/60s" />
        </a-form-item>
        <a-row :gutter="16">
          <a-col :span="12">
            <a-form-item label="动作">
              <a-select v-model:value="form.action" :options="[{ value: 'block', label: '拦截 (403)' }, { value: 'log', label: '仅记录' }]" />
            </a-form-item>
          </a-col>
          <a-col :span="12">
            <a-form-item label="启用">
              <a-select v-model:value="form.enabled" :options="[{ value: 1, label: '启用' }, { value: 0, label: '停用' }]" />
            </a-form-item>
          </a-col>
        </a-row>
        <a-alert v-if="scope === 'admin'" type="info" show-icon :message="tenantFilter ? '规则归属当前筛选的租户' : '请先用右上角选择租户，再新建规则'" />
      </a-form>
    </a-modal>
  </div>
</template>
