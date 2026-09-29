<script setup lang="ts">
// Platform settings KV editor (admin only).
import { onMounted, reactive, ref } from 'vue';
import { message } from 'ant-design-vue';
import { getSettings, saveSettings } from '@/api/admin';

const loading = ref(false);
const saving = ref(false);
const rows = reactive<Record<string, string>>({});

const keys = [
  ['platform.name', '平台显示名'],
  ['tenant.default_max_sites', '新租户默认站点配额'],
  ['tenant.default_traffic_mb', '新租户默认流量额度(MB, 0=不限)'],
  ['cdn.rate_per_mb', '每 MB 单价(虚拟币, 0=免费)'],
  ['edge.poll_timeout_sec', 'edge 长轮询超时(秒)'],
];

async function load() {
  loading.value = true;
  try {
    const m = await getSettings();
    for (const k of Object.keys(m)) rows[k] = m[k];
  } catch (e) {
    message.error(e instanceof Error ? e.message : 'load failed');
  } finally {
    loading.value = false;
  }
}

async function save() {
  saving.value = true;
  try {
    await saveSettings({ ...rows });
    message.success('已保存');
  } catch (e) {
    message.error(e instanceof Error ? e.message : 'save failed');
  } finally {
    saving.value = false;
  }
}

onMounted(load);
</script>

<template>
  <div class="page-card" style="max-width: 720px">
    <div class="toolbar">
      <span class="page-title mb-0">平台参数</span>
      <a-button type="primary" :loading="saving" @click="save">保存</a-button>
    </div>
    <a-table :data-source="keys.map(([k, label]) => ({ key: k, label, value: rows[k] ?? '' }))" :loading="loading" :pagination="false" row-key="key">
      <a-table-column title="键" data-index="key" width="240" />
      <a-table-column title="说明" data-index="label" />
      <a-table-column title="值">
        <template #default="{ record }">
          <a-input v-model:value="rows[record.key]" style="width: 160px" />
        </template>
      </a-table-column>
    </a-table>
  </div>
</template>
