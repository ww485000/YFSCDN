<script setup lang="ts">
// Edge node management: admin CRUD (token shown once on create); tenant read-only.
import { computed, onMounted, reactive, ref } from 'vue';
import { message } from 'ant-design-vue';
import { useAuthStore } from '@/stores/auth';
import { createNode, deleteNode, listNodesAdmin, listNodesTenant, updateNode } from '@/api/node';
import { listTenants } from '@/api/admin';
import { fmtTime } from '@/utils/format';
import type { EdgeNode, Tenant } from '@/types';

const auth = useAuthStore();
const isAdmin = computed(() => !auth.isTenant);

const list = ref<EdgeNode[]>([]);
const total = ref(0);
const loading = ref(false);
const page = ref(1);
const keyword = ref('');
const tenants = ref<Tenant[]>([]);

const show = ref(false);
const editing = ref<EdgeNode | null>(null);
const newToken = ref('');
const form = reactive({ name: '', tenant_id: 0 });

async function load() {
  loading.value = true;
  try {
    const d = isAdmin.value
      ? await listNodesAdmin({ keyword: keyword.value, page: page.value, size: 20 })
      : await listNodesTenant();
    list.value = d.list;
    total.value = d.total;
  } catch (e) {
    message.error(e instanceof Error ? e.message : 'load failed');
  } finally {
    loading.value = false;
  }
}

async function loadOptions() {
  if (!isAdmin.value) return;
  try {
    tenants.value = (await listTenants({ page: 1, size: 200 })).list;
  } catch {
    /* non-fatal */
  }
}

function openCreate() {
  editing.value = null;
  newToken.value = '';
  Object.assign(form, { name: '', tenant_id: 0 });
  show.value = true;
}

function openEdit(n: EdgeNode) {
  editing.value = n;
  Object.assign(form, { name: n.name, tenant_id: n.tenant_id });
  show.value = true;
}

async function submit() {
  try {
    if (editing.value) {
      await updateNode(editing.value.id, form);
      message.success('节点已更新');
    } else {
      const n = await createNode(form);
      newToken.value = n.token;
      message.success('节点已创建，请复制 token 配置到 edge 节点');
    }
    void load();
  } catch (e) {
    message.error(e instanceof Error ? e.message : 'save failed');
  }
}

async function remove(n: EdgeNode) {
  try {
    await deleteNode(n.id);
    message.success('已删除');
    void load();
  } catch (e) {
    message.error(e instanceof Error ? e.message : 'delete failed');
  }
}

function tenantName(id: number): string {
  return id === 0 ? '平台共享' : `#${id}`;
}

onMounted(() => {
  void load();
  void loadOptions();
});
</script>

<template>
  <div class="page-card">
    <div class="toolbar">
      <span class="page-title mb-0">边缘节点</span>
      <a-button v-if="isAdmin" type="primary" @click="openCreate">新建节点</a-button>
      <a-input-search v-if="isAdmin" v-model:value="keyword" placeholder="搜索 名称/主机/IP" style="width: 220px" @search="load" />
    </div>

    <a-alert v-if="newToken" type="success" show-icon style="margin-bottom: 12px">
      <template #message>
        新节点 token（edge 配置 node_token，仅此时显示）：
        <a-typography-text copyable style="font-family: monospace">{{ newToken }}</a-typography-text>
      </template>
    </a-alert>

    <a-table :data-source="list" :loading="loading" row-key="id" :pagination="isAdmin ? { current: page, total, pageSize: 20, onChange: (p: number) => (page = p) && load() } : false">
      <a-table-column title="ID" data-index="id" width="56" />
      <a-table-column title="名称" data-index="name" width="140" />
      <a-table-column title="状态" width="80">
        <template #default="{ record }">
          <a-badge :status="record.status === 1 ? 'success' : 'default'" :text="record.status === 1 ? '在线' : '离线'" />
        </template>
      </a-table-column>
      <a-table-column title="归属" width="100">
        <template #default="{ record }">{{ tenantName(record.tenant_id) }}</template>
      </a-table-column>
      <a-table-column title="驱动" data-index="driver" width="80" />
      <a-table-column title="地址">
        <template #default="{ record }">{{ record.host || '-' }} ({{ record.ip || '-' }})</template>
      </a-table-column>
      <a-table-column title="最后心跳" width="170">
        <template #default="{ record }">{{ fmtTime(record.last_seen) }}</template>
      </a-table-column>
      <a-table-column title="创建" width="160">
        <template #default="{ record }">{{ fmtTime(record.created_at) }}</template>
      </a-table-column>
      <a-table-column v-if="isAdmin" title="操作" width="130">
        <template #default="{ record }">
          <a @click="openEdit(record)">编辑</a>
          <a-divider type="vertical" />
          <a-popconfirm title="确认删除该节点？" @confirm="remove(record)">
            <a class="text-red-500">删除</a>
          </a-popconfirm>
        </template>
      </a-table-column>
    </a-table>

    <a-modal v-model:open="show" :title="editing ? '编辑节点' : '新建节点'" @ok="submit">
      <a-form layout="vertical" :model="form">
        <a-form-item label="名称" required>
          <a-input v-model:value="form.name" placeholder="node-a" />
        </a-form-item>
        <a-form-item label="归属" extra="平台共享 = 0；也可指定某个租户独占">
          <a-select v-model:value="form.tenant_id" :options="[{ value: 0, label: '平台共享节点' }, ...tenants.map((t) => ({ value: t.id, label: `租户: ${t.name}` }))]">
          </a-select>
        </a-form-item>
        <a-alert v-if="!editing" type="info" show-icon message="创建后把 token 写入 edge 节点配置 (edge/config.json 的 node_token)，edge 启动即自动注册。" />
      </a-form>
    </a-modal>
  </div>
</template>
