<script setup lang="ts">
// Certificate management (GoEdge cert module): list / upload / reissue / delete.
// Admin sees all tenants; tenant sees own certs. CA certs can be re-issued.
import { computed, onMounted, reactive, ref } from 'vue';
import { message } from 'ant-design-vue';
import { useAuthStore } from '@/stores/auth';
import { deleteCert, listCerts, reissueCert, uploadCert, type Scope } from '@/api/site';
import { fmtTime } from '@/utils/format';
import type { Cert } from '@/types';

const auth = useAuthStore();
const scope = computed<Scope>(() => (auth.isTenant ? 'tenant' : 'admin'));

const list = ref<Cert[]>([]);
const loading = ref(false);
const show = ref(false);
const form = reactive({
  domain: '',
  name: '',
  cert_pem: '',
  key_pem: '',
});

async function load() {
  loading.value = true;
  try {
    list.value = await listCerts(scope.value, 0);
  } catch (e) {
    message.error(e instanceof Error ? e.message : 'load failed');
  } finally {
    loading.value = false;
  }
}

function openCreate() {
  Object.assign(form, { domain: '', name: '', cert_pem: '', key_pem: '' });
  show.value = true;
}

async function submit() {
  try {
    await uploadCert(scope.value, {
      domain: form.domain,
      name: form.name,
      cert_pem: form.cert_pem,
      key_pem: form.key_pem,
    });
    message.success('证书已上传');
    show.value = false;
    void load();
  } catch (e) {
    message.error(e instanceof Error ? e.message : 'upload failed');
  }
}

async function reissue(c: Cert) {
  try {
    await reissueCert(scope.value, c.id);
    message.success('已重新签发');
    void load();
  } catch (e) {
    message.error(e instanceof Error ? e.message : 'reissue failed');
  }
}

async function remove(c: Cert) {
  try {
    await deleteCert(scope.value, c.id);
    message.success('已删除');
    void load();
  } catch (e) {
    message.error(e instanceof Error ? e.message : 'delete failed');
  }
}

const typeColor = (t?: string) =>
  t === 'custom' ? 'blue' : t === 'acme' ? 'purple' : 'green';

const typeText = (t?: string) =>
  t === 'custom' ? '自定义' : t === 'acme' ? 'ACME' : '平台 CA';

onMounted(() => void load());
</script>

<template>
  <div class="page-card">
    <div class="toolbar">
      <span class="page-title mb-0">证书管理</span>
      <a-button type="primary" @click="openCreate">上传证书</a-button>
    </div>
    <a-table :data-source="list" :loading="loading" row-key="id" :pagination="{ pageSize: 20 }">
      <a-table-column title="ID" data-index="id" width="56" />
      <a-table-column title="域名" data-index="domain" width="200" />
      <a-table-column title="类型" width="100">
        <template #default="{ record }">
          <a-tag :color="typeColor(record.type)">{{ typeText(record.type) }}</a-tag>
        </template>
      </a-table-column>
      <a-table-column title="颁发者" data-index="issuer" width="160" />
      <a-table-column title="过期时间" width="180">
        <template #default="{ record }">{{ fmtTime(record.expires_at) }}</template>
      </a-table-column>
      <a-table-column title="创建时间" width="180">
        <template #default="{ record }">{{ fmtTime(record.created_at) }}</template>
      </a-table-column>
      <a-table-column title="操作" width="150">
        <template #default="{ record }">
          <a v-if="record.type === 'ca'" @click="reissue(record)">续签</a>
          <a-divider type="vertical" />
          <a-popconfirm title="确认删除该证书？" @confirm="remove(record)">
            <a class="text-red-500">删除</a>
          </a-popconfirm>
        </template>
      </a-table-column>
    </a-table>
    <a-alert
      type="info"
      show-icon
      class="mt-3"
      message="HTTPS 站点默认使用平台自签 CA 证书（演示/内网模式，浏览器需导入平台 CA）。上传自定义证书或后续接入 ACME (Let's Encrypt) 可获得公网可信证书。"
    />

    <a-modal v-model:open="show" title="上传证书" width="640px" @ok="submit">
      <a-form layout="vertical" :model="form">
        <a-row :gutter="16">
          <a-col :span="12">
            <a-form-item label="域名" extra="留空则取证书第一个 SAN">
              <a-input v-model:value="form.domain" placeholder="www.example.com" />
            </a-form-item>
          </a-col>
          <a-col :span="12">
            <a-form-item label="名称">
              <a-input v-model:value="form.name" placeholder="可选" />
            </a-form-item>
          </a-col>
        </a-row>
        <a-form-item label="证书 (PEM, 含完整链)" required>
          <a-textarea v-model:value="form.cert_pem" :rows="5" placeholder="-----BEGIN CERTIFICATE-----" />
        </a-form-item>
        <a-form-item label="私钥 (PEM)" required>
          <a-textarea v-model:value="form.key_pem" :rows="5" placeholder="-----BEGIN PRIVATE KEY-----" />
        </a-form-item>
      </a-form>
    </a-modal>
  </div>
</template>
