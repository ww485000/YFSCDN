<script setup lang="ts">
// Site full-settings page — mirrors GoEdge's section tree (one tab per section).
// Loads the full SiteConfig document from core, edits any section, and pushes
// the whole document back (core validates + re-emits to the edge nodes).
import { computed, onMounted, ref } from 'vue';
import { message } from 'ant-design-vue';
import { useRoute, useRouter } from 'vue-router';
import { useAuthStore } from '@/stores/auth';
import { getSite, setSiteStatus, updateSiteConfig, type Scope } from '@/api/site';
import { listNodesAdmin, listNodesTenant } from '@/api/node';
import { fmtTime } from '@/utils/format';
import type { EdgeNode, SiteDetail, SiteNamedLocation, SiteOrigin, SiteRewrite, SiteServerName, SiteWAFRule } from '@/types';

const auth = useAuthStore();
const route = useRoute();
const router = useRouter();
const scope: Scope = auth.isTenant ? 'tenant' : 'admin';
const id = Number(route.params.id);

const detail = ref<SiteDetail | null>(null);
const cfg = ref<any>(null);
const nodeIds = ref<number[]>([]);
const nodes = ref<EdgeNode[]>([]);
const activeTab = ref('basic');
const saving = ref(false);
const showJson = ref(false);
const jsonText = ref('');
const showLocJson = ref(false);
const locJsonText = ref('');
const locJsonIndex = ref(0);

// `any` on purpose: the config document is a flexible JSON tree; the form
// binds sections directly (validated server-side on save).
const L = computed<any>(() => (cfg.value ? cfg.value.location : null));

// derived text bindings for list fields stored as string[]
const indexText = computed({
  get: () => (L.value?.index || []).join(', '),
  set: (v: string) => {
    L.value.index = (v || '').split(',').map((s) => s.trim()).filter(Boolean);
  },
});
const wsPathsText = computed({
  get: () => (L.value?.websocket?.paths || []).join(', '),
  set: (v: string) => {
    L.value.websocket.paths = (v || '').split(',').map((s) => s.trim()).filter(Boolean);
  },
});

// ---------- helpers ----------
function lines(v: string[] | undefined): string {
  return (v || []).join('\n');
}
function setLines(target: any, key: string, v: string | undefined): void {
  target[key] = (v || '').split('\n').map((s) => s.trim()).filter(Boolean);
}
function csv(v: string[] | undefined): string {
  return (v || []).join(', ');
}
function setCsv(target: any, key: string, v: string | undefined): void {
  target[key] = (v || '').split(',').map((s) => s.trim()).filter(Boolean);
}

function addRow<T>(arr: T[], proto: T): void {
  arr.push(JSON.parse(JSON.stringify(proto)));
}
function rmRow(arr: any[], i: number | string): void {
  arr.splice(Number(i), 1);
}

function addServerName(): void {
  const arr: SiteServerName[] = cfg.value.server_names;
  addRow(arr, { name: '', is_default: arr.length === 0, status: 'normal', redirect_to: '' });
}
function makeDefault(i: number): void {
  cfg.value.server_names.forEach((s: SiteServerName, j: number) => (s.is_default = j === i));
}

function addListener(proto = 'http'): void {
  const arr: any[] = cfg.value.listeners;
  const port = proto === 'https' ? 443 : proto === 'http' ? 80 : 9000;
  addRow(arr, {
    protocol: proto,
    listen: `:${port}`,
    follow_protocol: proto === 'http',
    origins: proto === 'tcp' || proto === 'udp' ? [{ name: 'origin-1', addr: '', status: 'normal' }] : undefined,
  });
}

function tcpOriginsText(r: any): string {
  return (r.origins || []).map((o: SiteOrigin) => o.addr).join(', ');
}
function setTcpOrigins(r: any, v: string): void {
  r.origins = (v || '')
    .split(',')
    .map((s) => s.trim())
    .filter(Boolean)
    .map((a, i) => ({ name: `origin-${i + 1}`, addr: a, status: 'normal' }));
}

function addOrigin(): void {
  const rp = L.value!.reverse_proxy!;
  addRow(rp.origins, { name: `origin-${rp.origins.length + 1}`, addr: '', status: 'normal', ssl: false } as SiteOrigin);
}

function addRewrite(): void {
  const arr: SiteRewrite[] = L.value!.rewrites!;
  addRow(arr, { type: 'rewrite', value: '', conditions: [] });
}
function addCond(r: SiteRewrite): void {
  r.conditions = r.conditions || [];
  r.conditions.push({ target: 'path', operator: 'contains', value: '' });
}

function addHeaderOp(kind: 'upstream' | 'downstream'): void {
  const h = (L.value!.headers ||= { upstream: [], downstream: [] });
  h[kind] = h[kind] || [];
  h[kind].push({ op: 'add', name: '', value: '' });
}

function addErrorPage(): void {
  L.value!.pages!.codes!.push({ status: 404, enabled: true, title: '404', content: '<h1>404 Not Found</h1>', content_type: 'text/html; charset=utf-8' });
}

function addWafRule(): void {
  const arr: SiteWAFRule[] = L.value!.waf!.rules!;
  addRow(arr, { name: 'rule', status: 'normal', target: 'url', operator: 'contains', value: '', action: 'block', level: 3 });
}

function addBasicUser(): void {
  L.value!.auth!.basic = L.value!.auth!.basic || { users: [] };
  L.value!.auth!.basic.users.push({ user: '', password: '' });
}

function addNamedLoc(): void {
  const arr: SiteNamedLocation[] = cfg.value.locations || (cfg.value.locations = []);
  addRow(arr, { name: 'loc', pattern: { type: 'prefix', value: '/api/' } });
}

// ---------- load / save ----------
async function load() {
  if (!id) return;
  const d = await getSite(scope, id);
  detail.value = d;
  cfg.value = JSON.parse(JSON.stringify(d.config || {}));
  cfg.value.server_names = cfg.value.server_names || [];
  cfg.value.listeners = cfg.value.listeners || [];
  cfg.value.locations = cfg.value.locations || [];
  cfg.value.location = cfg.value.location || {};
  const loc = cfg.value.location;
  loc.reverse_proxy = loc.reverse_proxy || { origins: [] };
  loc.reverse_proxy.origins = loc.reverse_proxy.origins || [];
  loc.reverse_proxy.scheduling = loc.reverse_proxy.scheduling || { type: 'random' };
  loc.reverse_proxy.health_check = loc.reverse_proxy.health_check || {
    enabled: false, interval: 10, timeout: 3, scheme: 'http', path: '/', failures: 3, successes: 1,
  };
  loc.rewrites = loc.rewrites || [];
  loc.headers = loc.headers || { upstream: [], downstream: [] };
  loc.headers.upstream = loc.headers.upstream || [];
  loc.headers.downstream = loc.headers.downstream || [];
  loc.headers.cors = loc.headers.cors || { enabled: false, allow_origins: [], allow_methods: [], allow_headers: [], max_age: 0 };
  loc.pages = loc.pages || { codes: [] };
  loc.pages.codes = loc.pages.codes || [];
  loc.pages.minify = loc.pages.minify || { html: false, css: false, js: false };
  loc.cache = loc.cache || { enabled: false, storage: 'file', max_size: '10GB', ttl: 300, only_get: true };
  loc.cache.key = loc.cache.key || { include_query: [], ignore_query: [], include_headers: [] };
  loc.cache.stale = loc.cache.stale || { enabled: false, ttl: 86400 };
  loc.access = loc.access || {};
  loc.access.ips = loc.access.ips || { allow: [], deny: [] };
  loc.access.referers = loc.access.referers || { allow: [], deny: [] };
  loc.access.user_agents = loc.access.user_agents || { allow: [], deny: [] };
  loc.waf = loc.waf || { enabled: false, ips: { allow: [], deny: [] }, rules: [] };
  loc.waf.ips = loc.waf.ips || { allow: [], deny: [] };
  loc.waf.rules = loc.waf.rules || [];
  loc.auth = loc.auth || { enabled: false };
  loc.auth.basic = loc.auth.basic || { users: [] };
  loc.auth.basic.users = loc.auth.basic.users || [];
  loc.auth.sub_request = loc.auth.sub_request || { url: '', timeout: 5 };
  loc.request_limit = loc.request_limit || { enabled: false, interval: 60, limit: 100, action: 'block' };
  loc.traffic_limit = loc.traffic_limit || { enabled: false, max_bytes: '', max_conn: 0 };
  loc.compression = loc.compression || { enabled: false, types: ['gzip'], min_size: 1024, level: 6, mime_types: [] };
  loc.webp = loc.webp || { enabled: false, quality: 80, mime_types: ['image/jpeg', 'image/png'] };
  loc.charset = loc.charset || { enabled: false, encoding: 'utf-8' };
  loc.expires = loc.expires || { enabled: false, time: '30d' };
  loc.websocket = loc.websocket || { enabled: false };
  loc.fastcgi = loc.fastcgi || { addr: '', index: 'index.php' };
  loc.access_log = loc.access_log || { enabled: true, format: 'combined' };
  loc.stat = loc.stat || { enabled: true };
  for (const li of cfg.value.listeners) {
    if (li.protocol === 'tcp' || li.protocol === 'udp') {
      li.origins = li.origins || [{ name: 'origin-1', addr: '', status: 'normal' }];
      li.scheduling = li.scheduling || { type: 'random' };
    }
  }
  nodeIds.value = d.node_ids || [];
}

async function loadNodes() {
  try {
    nodes.value = scope === 'admin'
      ? (await listNodesAdmin({ page: 1, size: 200 })).list
      : (await listNodesTenant()).list;
  } catch {
    /* non-fatal */
  }
}

async function save() {
  saving.value = true;
  try {
    const clean = JSON.parse(JSON.stringify(cfg.value));
    const d = await updateSiteConfig(scope, id, clean, nodeIds.value);
    detail.value = d;
    message.success('已保存，配置已推送到边缘节点');
  } catch (e) {
    message.error(e instanceof Error ? e.message : 'save failed');
  } finally {
    saving.value = false;
  }
}

async function toggleStatus() {
  const s = cfg.value.status === 'normal' ? 'off' : 'normal';
  try {
    const d = await setSiteStatus(scope, id, s);
    cfg.value.status = d.status;
    detail.value = d;
    message.success(s === 'normal' ? '站点已启用' : '站点已停用');
  } catch (e) {
    message.error(e instanceof Error ? e.message : 'status failed');
  }
}

function openJson() {
  jsonText.value = JSON.stringify(cfg.value, null, 2);
  showJson.value = true;
}

function editLocJson(i: number) {
  locJsonIndex.value = i;
  locJsonText.value = JSON.stringify(cfg.value.locations[i], null, 2);
  showLocJson.value = true;
}
function applyLocJson() {
  try {
    const o = JSON.parse(locJsonText.value);
    cfg.value.locations[locJsonIndex.value] = o;
    showLocJson.value = false;
    message.success('Location JSON 已应用，点击"保存配置"生效');
  } catch (e) {
    message.error('JSON 解析失败: ' + (e instanceof Error ? e.message : ''));
  }
}
function applyJson() {
  try {
    const o = JSON.parse(jsonText.value);
    const clean = JSON.parse(JSON.stringify(o));
    clean.server_names = clean.server_names || [];
    clean.listeners = clean.listeners || [];
    clean.location = clean.location || {};
    cfg.value = clean;
    showJson.value = false;
    message.success('JSON 已应用，点击"保存"生效');
  } catch (e) {
    message.error('JSON 解析失败: ' + (e instanceof Error ? e.message : ''));
  }
}

function back() {
  void router.push(auth.isTenant ? '/portal/sites' : '/admin/sites');
}

onMounted(() => {
  void load().catch((e) => message.error(e instanceof Error ? e.message : 'load failed'));
  void loadNodes();
});
</script>

<template>
  <div class="page-card">
    <div class="toolbar">
      <a-button type="link" @click="back">← 返回</a-button>
      <span class="page-title mb-0">站点设置 · {{ detail?.name || `#${id}` }}</span>
      <a-tag :color="cfg?.status === 'normal' ? 'green' : 'red'">{{ cfg?.status === 'normal' ? '运行中' : '已停用' }}</a-tag>
      <a-tag v-if="detail" color="blue">{{ detail.domain }}</a-tag>
      <a-button :disabled="!cfg" @click="toggleStatus">{{ cfg?.status === 'normal' ? '停用' : '启用' }}</a-button>
      <a-button @click="openJson">高级 (JSON)</a-button>
      <a-button type="primary" :loading="saving" :disabled="!cfg" @click="save">保存配置</a-button>
    </div>

    <a-spin :spinning="!cfg">
      <a-tabs v-if="cfg" v-model:activeKey="activeTab">
        <!-- ================= 基础 ================= -->
        <a-tab-pane key="basic" tab="基础">
          <a-form layout="vertical" class="mb-4">
            <a-row :gutter="16">
              <a-col :span="8">
                <a-form-item label="站点名称">
                  <a-input v-model:value="cfg.name" placeholder="站点名称" />
                </a-form-item>
              </a-col>
              <a-col :span="8">
                <a-form-item label="状态">
                  <a-select v-model:value="cfg.status" :options="[{ value: 'normal', label: '正常' }, { value: 'off', label: '停用' }]" />
                </a-form-item>
              </a-col>
            </a-row>
          </a-form>

          <div class="section-title">多域名 (server_names)</div>
          <a-table :data-source="cfg.server_names" row-key="name" size="small" :pagination="false">
            <a-table-column title="域名" data-index="name" />
            <a-table-column title="默认" width="80">
              <template #default="{ record, index }">
                <a-radio :checked="record.is_default" @change="makeDefault(index)">默认</a-radio>
              </template>
            </a-table-column>
            <a-table-column title="状态" width="110">
              <template #default="{ record }">
                <a-select v-model:value="record.status" size="small" style="width: 90px" :options="[{ value: 'normal', label: '正常' }, { value: 'off', label: '停用' }]" />
              </template>
            </a-table-column>
            <a-table-column title="跳转 (301 到其他域名)" width="200" data-index="redirect_to" />
            <a-table-column title="" width="60">
              <template #default="{ index }">
                <a class="text-red-500" @click="rmRow(cfg.server_names, index)">删除</a>
              </template>
            </a-table-column>
          </a-table>
          <a-button class="mt-2" size="small" @click="addServerName">+ 添加域名</a-button>
        </a-tab-pane>

        <!-- ================= 监听 ================= -->
        <a-tab-pane key="listeners" tab="监听">
          <a-alert type="info" show-icon class="mb-3" message="HTTP/HTTPS/TCP/UDP 监听。TCP/UDP 为透传回源（可配源站组+调度）。" />
          <a-table :data-source="cfg.listeners" row-key="listen" size="small" :pagination="false">
            <a-table-column title="协议" width="100">
              <template #default="{ record }">
                <a-select v-model:value="record.protocol" size="small" style="width: 90px" :options="['http','https','tcp','udp'].map((p) => ({ value: p, label: p }))" />
              </template>
            </a-table-column>
            <a-table-column title="监听地址" width="180">
              <template #default="{ record }">
                <a-input v-model:value="record.listen" size="small" placeholder=":80 / 0.0.0.0:8443" />
              </template>
            </a-table-column>
            <a-table-column title="HTTP 自动跳转 HTTPS" width="150">
              <template #default="{ record }">
                <a-checkbox v-model:checked="record.follow_protocol" :disabled="record.protocol !== 'http'" />
              </template>
            </a-table-column>
            <a-table-column title="SNI (逗号分隔, 空=全部域名)">
              <template #default="{ record }">
                <a-input :value="csv(record.sni)" @update:value="(v: string) => setCsv(record, 'sni', v)" size="small" placeholder="*.example.com, api.example.com" />
              </template>
            </a-table-column>
            <a-table-column title="TCP/UDP 源站" width="280">
              <template #default="{ record }">
                <template v-if="record.protocol === 'tcp' || record.protocol === 'udp'">
                  <a-input :value="tcpOriginsText(record)" @update:value="(v: string) => setTcpOrigins(record, v)" size="small" placeholder="host:port, host:port" style="width: 170px" />
                  <a-select :value="record.scheduling?.type || 'random'" @update:value="(v: string) => { record.scheduling = { type: v }; }" size="small" style="width: 120px" class="ml-2" :options="[{ value: 'random', label: 'random' }, { value: 'round_robin', label: 'round_robin' }, { value: 'hash', label: 'hash' }, { value: 'sticky', label: 'sticky' }]" />
                </template>
              </template>
            </a-table-column>
            <a-table-column title="" width="60">
              <template #default="{ index }">
                <a class="text-red-500" @click="rmRow(cfg.listeners, index)">删除</a>
              </template>
            </a-table-column>
          </a-table>
          <a-space class="mt-2">
            <a-button size="small" @click="addListener('http')">+ HTTP</a-button>
            <a-button size="small" @click="addListener('https')">+ HTTPS</a-button>
            <a-button size="small" @click="addListener('tcp')">+ TCP</a-button>
            <a-button size="small" @click="addListener('udp')">+ UDP</a-button>
          </a-space>
        </a-tab-pane>

        <!-- ================= 回源 ================= -->
        <a-tab-pane key="origin" tab="回源代理">
          <a-form layout="vertical" class="mb-3">
            <a-form-item label="回源超时 (秒, 0=默认30)">
              <a-input-number v-model:value="L.reverse_proxy.timeout_sec" :min="0" style="width: 160px" />
            </a-form-item>
          </a-form>
          <div class="section-title">源站组 (origins)</div>
          <a-table :data-source="L.reverse_proxy.origins" row-key="addr" size="small" :pagination="false">
            <a-table-column title="名称" width="140">
              <template #default="{ record }"><a-input v-model:value="record.name" size="small" /></template>
            </a-table-column>
            <a-table-column title="地址 (host:port)" width="200">
              <template #default="{ record }"><a-input v-model:value="record.addr" size="small" placeholder="10.0.0.1:8080" /></template>
            </a-table-column>
            <a-table-column title="状态" width="100">
              <template #default="{ record }">
                <a-select v-model:value="record.status" size="small" style="width: 85px" :options="[{ value: 'normal', label: '正常' }, { value: 'off', label: '停用' }]" />
              </template>
            </a-table-column>
            <a-table-column title="SSL" width="70">
              <template #default="{ record }"><a-checkbox v-model:checked="record.ssl" /></template>
            </a-table-column>
            <a-table-column title="SNI" width="150">
              <template #default="{ record }"><a-input v-model:value="record.sni" size="small" placeholder="origin.example.com" /></template>
            </a-table-column>
            <a-table-column title="不校验证书" width="90">
              <template #default="{ record }"><a-checkbox v-model:checked="record.insecure" /></template>
            </a-table-column>
            <a-table-column title="" width="60">
              <template #default="{ index }"><a class="text-red-500" @click="rmRow(L.reverse_proxy.origins, index)">删除</a></template>
            </a-table-column>
          </a-table>
          <a-button class="mt-2" size="small" @click="addOrigin">+ 添加源站</a-button>

          <a-row :gutter="16" class="mt-4">
            <a-col :span="12">
              <div class="section-title">调度 (scheduling)</div>
              <a-form layout="vertical">
                <a-row :gutter="12">
                  <a-col :span="8">
                    <a-form-item label="类型">
                      <a-select v-model:value="L.reverse_proxy.scheduling!.type" :options="[{ value: 'random', label: 'random' }, { value: 'round_robin', label: 'round_robin' }, { value: 'hash', label: 'hash' }, { value: 'sticky', label: 'sticky' }]" />
                    </a-form-item>
                  </a-col>
                  <a-col :span="8" v-if="L.reverse_proxy.scheduling!.type === 'hash'">
                    <a-form-item label="Hash 键">
                      <a-select v-model:value="L.reverse_proxy.scheduling!.key" :options="[{ value: 'ip', label: 'ip' }, { value: 'cookie:session', label: 'cookie:session' }, { value: 'header:X-User', label: 'header:X-User' }]" />
                    </a-form-item>
                  </a-col>
                  <a-col :span="8" v-if="L.reverse_proxy.scheduling!.type === 'sticky'">
                    <a-form-item label="TTL(秒)">
                      <a-input-number v-model:value="L.reverse_proxy.scheduling!.ttl" :min="1" style="width: 100%" />
                    </a-form-item>
                  </a-col>
                </a-row>
              </a-form>
            </a-col>
            <a-col :span="12">
              <div class="section-title">健康检查 (health_check)</div>
              <a-form layout="vertical">
                <a-form-item label="启用">
                  <a-checkbox v-model:checked="L.reverse_proxy.health_check!.enabled" />
                </a-form-item>
                <a-row :gutter="12">
                  <a-col :span="6">
                    <a-form-item label="间隔(秒)"><a-input-number v-model:value="L.reverse_proxy.health_check!.interval" :min="1" style="width: 100%" /></a-form-item>
                  </a-col>
                  <a-col :span="6">
                    <a-form-item label="超时(秒)"><a-input-number v-model:value="L.reverse_proxy.health_check!.timeout" :min="1" style="width: 100%" /></a-form-item>
                  </a-col>
                  <a-col :span="6">
                    <a-form-item label="方案"><a-select v-model:value="L.reverse_proxy.health_check!.scheme" :options="[{ value: 'http', label: 'http' }, { value: 'https', label: 'https' }]" /></a-form-item>
                  </a-col>
                  <a-col :span="6">
                    <a-form-item label="路径"><a-input v-model:value="L.reverse_proxy.health_check!.path" size="small" /></a-form-item>
                  </a-col>
                </a-row>
                <a-row :gutter="12">
                  <a-col :span="6">
                    <a-form-item label="失败阈值"><a-input-number v-model:value="L.reverse_proxy.health_check!.failures" :min="1" style="width: 100%" /></a-form-item>
                  </a-col>
                  <a-col :span="6">
                    <a-form-item label="恢复阈值"><a-input-number v-model:value="L.reverse_proxy.health_check!.successes" :min="1" style="width: 100%" /></a-form-item>
                  </a-col>
                  <a-col :span="12">
                    <a-form-item label="Host 覆盖"><a-input v-model:value="L.reverse_proxy.health_check!.host" size="small" placeholder="空=源站地址" /></a-form-item>
                  </a-col>
                </a-row>
              </a-form>
            </a-col>
          </a-row>
        </a-tab-pane>

        <!-- ================= 重写 ================= -->
        <a-tab-pane key="rewrite" tab="重写/跳转">
          <a-alert type="info" show-icon class="mb-3" message="按顺序执行。rewrite=URL 重写；return=直接返回状态码(+可选跳转地址)。可附加过滤器条件（全部满足才生效）。" />
          <div v-for="(r, i) in L.rewrites" :key="i" class="rule-box">
            <a-row :gutter="12" align="middle">
              <a-col :span="5">
                <a-select v-model:value="r.type" :options="[{ value: 'rewrite', label: '重写 rewrite' }, { value: 'return', label: '返回 return' }]" />
              </a-col>
              <a-col :span="12">
                <a-input v-model:value="r.value" :placeholder="r.type === 'rewrite' ? '新 URI, 如 /new$1' : '跳转地址 (301/302/307/308 时必填)'" />
              </a-col>
              <a-col :span="5" v-if="r.type === 'return'">
                <a-input-number v-model:value="r.status" :min="200" :max="599" style="width: 100%" placeholder="状态码" />
              </a-col>
              <a-col :span="2">
                <a class="text-red-500" @click="rmRow(L.rewrites, i)">删除</a>
              </a-col>
            </a-row>
            <div class="ml-4 mt-2">
              <div v-for="(c, ci) in r.conditions" :key="ci" class="flex items-center gap-2 mb-2">
                <a-select v-model:value="c.target" size="small" style="width: 170px" :options="['path','query','host','method','user_agent','referer','ip','uri'].map((t) => ({ value: t, label: t }))" />
                <a-select v-model:value="c.operator" size="small" style="width: 150px" :options="['equals','not_equals','contains','not_contains','starts_with','ends_with','regex'].map((o) => ({ value: o, label: o }))" />
                <a-input v-model:value="c.value" size="small" style="width: 240px" placeholder="匹配值" />
                <a class="text-red-500" @click="rmRow(r.conditions!, ci)">×</a>
              </div>
              <a-button size="small" @click="addCond(r)">+ 条件</a-button>
            </div>
          </div>
          <a-button class="mt-2" @click="addRewrite">+ 添加规则</a-button>
        </a-tab-pane>

        <!-- ================= 自定义页面 ================= -->
        <a-tab-pane key="pages" tab="自定义页面">
          <a-alert type="info" show-icon class="mb-3" message="自定义错误页 + 页面压缩 (minify)。(GoEdge 页面优化)" />
          <a-table :data-source="L.pages.codes" row-key="status" size="small" :pagination="false">
            <a-table-column title="状态码" width="100">
              <template #default="{ record }"><a-input-number v-model:value="record.status" :min="100" :max="599" size="small" style="width: 90px" /></template>
            </a-table-column>
            <a-table-column title="启用" width="70">
              <template #default="{ record }"><a-checkbox v-model:checked="record.enabled" /></template>
            </a-table-column>
            <a-table-column title="标题" width="140" data-index="title" />
            <a-table-column title="内容 (HTML)">
              <template #default="{ record }">
                <a-input v-model:value="record.content" size="small" placeholder="<h1>…</h1>" />
              </template>
            </a-table-column>
            <a-table-column title="" width="60">
              <template #default="{ index }"><a class="text-red-500" @click="rmRow(L.pages.codes, index)">删除</a></template>
            </a-table-column>
          </a-table>
          <a-button class="mt-2" size="small" @click="addErrorPage">+ 添加错误页</a-button>
          <div class="section-title mt-4">页面压缩 (minify)</div>
          <a-space>
            <a-checkbox v-model:checked="L.pages.minify!.html">HTML</a-checkbox>
            <a-checkbox v-model:checked="L.pages.minify!.css">CSS</a-checkbox>
            <a-checkbox v-model:checked="L.pages.minify!.js">JS</a-checkbox>
          </a-space>
        </a-tab-pane>

        <!-- ================= 响应头 ================= -->
        <a-tab-pane key="headers" tab="响应头/CORS">
          <div class="section-title">请求头 (回源前修改 upstream)</div>
          <div v-for="(h, i) in L.headers.upstream" :key="'u' + i" class="flex items-center gap-2 mb-2">
            <a-select v-model:value="h.op" size="small" style="width: 110px" :options="['add','set','delete','replace'].map((o) => ({ value: o, label: o }))" />
            <a-input v-model:value="h.name" size="small" style="width: 200px" placeholder="X-Header" />
            <a-input v-model:value="h.value" size="small" style="width: 240px" placeholder="值" />
            <a class="text-red-500" @click="rmRow(L.headers.upstream, i)">×</a>
          </div>
          <a-button size="small" class="mb-4" @click="addHeaderOp('upstream')">+ 请求头规则</a-button>

          <div class="section-title">响应头 (回边修改 downstream)</div>
          <div v-for="(h, i) in L.headers.downstream" :key="'d' + i" class="flex items-center gap-2 mb-2">
            <a-select v-model:value="h.op" size="small" style="width: 110px" :options="['add','set','delete','replace'].map((o) => ({ value: o, label: o }))" />
            <a-input v-model:value="h.name" size="small" style="width: 200px" placeholder="X-Header" />
            <a-input v-model:value="h.value" size="small" style="width: 240px" placeholder="值" />
            <a class="text-red-500" @click="rmRow(L.headers.downstream, i)">×</a>
          </div>
          <a-button size="small" @click="addHeaderOp('downstream')">+ 响应头规则</a-button>

          <div class="section-title mt-4">CORS</div>
          <a-checkbox v-model:checked="L.headers.cors.enabled" class="mb-2">启用 CORS</a-checkbox>
          <a-row :gutter="16">
            <a-col :span="8">
              <a-form-item label="允许来源 (逗号分隔, *=全部)"><a-input :value="csv(L.headers.cors.allow_origins)" @update:value="(v: string) => setCsv(L.headers.cors, 'allow_origins', v)" size="small" /></a-form-item>
            </a-col>
            <a-col :span="8">
              <a-form-item label="允许方法"><a-input :value="csv(L.headers.cors.allow_methods)" @update:value="(v: string) => setCsv(L.headers.cors, 'allow_methods', v)" size="small" /></a-form-item>
            </a-col>
            <a-col :span="8">
              <a-form-item label="允许头"><a-input :value="csv(L.headers.cors.allow_headers)" @update:value="(v: string) => setCsv(L.headers.cors, 'allow_headers', v)" size="small" /></a-form-item>
              <a-form-item label="Max-Age(秒)"><a-input-number v-model:value="L.headers.cors.max_age" :min="0" style="width: 100%" /></a-form-item>
            </a-col>
          </a-row>
        </a-tab-pane>

        <!-- ================= 压缩 ================= -->
        <a-tab-pane key="compression" tab="压缩">
          <a-form layout="vertical">
            <a-form-item label="启用"><a-checkbox v-model:checked="L.compression.enabled" /></a-form-item>
            <a-row :gutter="16">
              <a-col :span="8">
                <a-form-item label="算法">
                  <a-select v-model:value="L.compression.types" mode="multiple" :options="[{ value: 'gzip', label: 'gzip' }, { value: 'deflate', label: 'deflate' }, { value: 'brotli', label: 'brotli' }]" />
                </a-form-item>
              </a-col>
              <a-col :span="8">
                <a-form-item label="最小体积 (字节, 低于不压缩)"><a-input-number v-model:value="L.compression.min_size" :min="0" style="width: 100%" /></a-form-item>
              </a-col>
              <a-col :span="8">
                <a-form-item label="gzip 级别 (1-9, 6=默认)"><a-input-number v-model:value="L.compression.level" :min="-1" :max="9" style="width: 100%" /></a-form-item>
              </a-col>
            </a-row>
            <a-form-item label="仅压缩这些 MIME (空=全部可压缩, 每行一个)">
              <a-textarea :value="lines(L.compression.mime_types)" :rows="4" @update:value="(v: string) => setLines(L.compression, 'mime_types', v)" />
            </a-form-item>
          </a-form>
        </a-tab-pane>

        <!-- ================= WebP ================= -->
        <a-tab-pane key="webp" tab="WebP">
          <a-form layout="vertical">
            <a-form-item label="启用图片自动转 WebP"><a-checkbox v-model:checked="L.webp.enabled" /></a-form-item>
            <a-row :gutter="16">
              <a-col :span="8">
                <a-form-item label="质量 (1-100)"><a-input-number v-model:value="L.webp.quality" :min="1" :max="100" style="width: 100%" /></a-form-item>
              </a-col>
              <a-col :span="16">
                <a-form-item label="转换的 MIME (每行一个)">
                  <a-textarea :value="lines(L.webp.mime_types)" :rows="3" @update:value="(v: string) => setLines(L.webp, 'mime_types', v)" />
                </a-form-item>
              </a-col>
            </a-row>
          </a-form>
        </a-tab-pane>

        <!-- ================= 字符集 ================= -->
        <a-tab-pane key="charset" tab="字符集">
          <a-form layout="vertical">
            <a-form-item label="启用"><a-checkbox v-model:checked="L.charset.enabled" /></a-form-item>
            <a-row :gutter="16">
              <a-col :span="8">
                <a-form-item label="目标编码"><a-input v-model:value="L.charset.encoding" size="small" placeholder="utf-8" /></a-form-item>
              </a-col>
              <a-col :span="8">
                <a-form-item label="源编码 (空=从 Content-Type 自动识别)"><a-input v-model:value="L.charset.convert_from" size="small" placeholder="gbk" /></a-form-item>
              </a-col>
            </a-row>
          </a-form>
        </a-tab-pane>

        <!-- ================= 过期时间 ================= -->
        <a-tab-pane key="expires" tab="过期时间">
          <a-form layout="vertical">
            <a-form-item label="启用 (设置 Cache-Control/Expires)"><a-checkbox v-model:checked="L.expires.enabled" /></a-form-item>
            <a-form-item label="时间 (如 30d / 2h / 1w)"><a-input v-model:value="L.expires.time" size="small" style="width: 200px" /></a-form-item>
          </a-form>
        </a-tab-pane>

        <!-- ================= 缓存 ================= -->
        <a-tab-pane key="cache" tab="缓存">
          <a-form layout="vertical">
            <a-form-item label="启用边缘缓存"><a-checkbox v-model:checked="L.cache.enabled" /></a-form-item>
            <a-row :gutter="16">
              <a-col :span="6">
                <a-form-item label="存储"><a-select v-model:value="L.cache.storage" :options="[{ value: 'file', label: 'file' }, { value: 'memory', label: 'memory' }]" /></a-form-item>
              </a-col>
              <a-col :span="6">
                <a-form-item label="最大体积"><a-input v-model:value="L.cache.max_size" size="small" placeholder="10GB / 256MB" /></a-form-item>
              </a-col>
              <a-col :span="6">
                <a-form-item label="默认 TTL (秒)"><a-input-number v-model:value="L.cache.ttl" :min="0" style="width: 100%" /></a-form-item>
              </a-col>
            </a-row>
            <a-space class="mb-3">
              <a-checkbox v-model:checked="L.cache.ignore_cache_control">忽略源站 Cache-Control</a-checkbox>
              <a-checkbox v-model:checked="L.cache.only_get">仅缓存 GET</a-checkbox>
            </a-space>
            <a-row :gutter="16">
              <a-col :span="8">
                <a-form-item label="缓存键包含的 Query (每行一个, 空=全部)">
                  <a-textarea :value="lines(L.cache.key?.include_query)" :rows="3" @update:value="(v: string) => { L.cache.key = L.cache.key || {}; setLines(L.cache.key, 'include_query', v); }" />
                </a-form-item>
              </a-col>
              <a-col :span="8">
                <a-form-item label="缓存键忽略的 Query (每行一个)">
                  <a-textarea :value="lines(L.cache.key?.ignore_query)" :rows="3" @update:value="(v: string) => { L.cache.key = L.cache.key || {}; setLines(L.cache.key, 'ignore_query', v); }" />
                </a-form-item>
              </a-col>
              <a-col :span="8">
                <a-form-item label="缓存键包含的头 (每行一个)">
                  <a-textarea :value="lines(L.cache.key?.include_headers)" :rows="3" @update:value="(v: string) => { L.cache.key = L.cache.key || {}; setLines(L.cache.key, 'include_headers', v); }" />
                </a-form-item>
              </a-col>
            </a-row>
            <a-row :gutter="16">
              <a-col :span="8">
                <a-form-item label="源站故障时返回过期内容 (stale-while-error)">
                  <a-checkbox v-model:checked="L.cache.stale!.enabled" />
                  <a-input-number v-model:value="L.cache.stale!.ttl" :min="0" style="width: 140px; margin-left: 8px" placeholder="允许时长(秒)" />
                </a-form-item>
              </a-col>
              <a-col :span="16">
                <a-form-item label="存在这些响应头时不缓存 (每行一个, 默认 Set-Cookie)">
                  <a-textarea :value="lines(L.cache.no_cache_headers)" :rows="3" @update:value="(v: string) => setLines(L.cache, 'no_cache_headers', v)" />
                </a-form-item>
              </a-col>
            </a-row>
          </a-form>
        </a-tab-pane>

        <!-- ================= 访问控制 ================= -->
        <a-tab-pane key="access" tab="访问控制">
          <a-row :gutter="16">
            <a-col :span="8">
              <div class="section-title">IP 允许 (每行一个, 支持 CIDR)</div>
              <a-textarea :value="lines(L.access.ips.allow)" :rows="6" placeholder="192.168.0.0/16&#10;10.1.2.3" @update:value="(v: string) => setLines(L.access.ips, 'allow', v)" />
            </a-col>
            <a-col :span="8">
              <div class="section-title">IP 拒绝</div>
              <a-textarea :value="lines(L.access.ips.deny)" :rows="6" @update:value="(v: string) => setLines(L.access.ips, 'deny', v)" />
            </a-col>
            <a-col :span="8">
              <div class="section-title">Referer 允许 (正则/glob, 空=全部)</div>
              <a-textarea :value="lines(L.access.referers.allow)" :rows="6" @update:value="(v: string) => setLines(L.access.referers, 'allow', v)" />
              <div class="section-title mt-3">Referer 拒绝</div>
              <a-textarea :value="lines(L.access.referers.deny)" :rows="3" @update:value="(v: string) => setLines(L.access.referers, 'deny', v)" />
            </a-col>
          </a-row>
          <a-row :gutter="16" class="mt-4">
            <a-col :span="8">
              <div class="section-title">UserAgent 允许</div>
              <a-textarea :value="lines(L.access.user_agents.allow)" :rows="4" @update:value="(v: string) => setLines(L.access.user_agents, 'allow', v)" />
            </a-col>
            <a-col :span="8">
              <div class="section-title">UserAgent 拒绝 (防盗刷)</div>
              <a-textarea :value="lines(L.access.user_agents.deny)" :rows="4" @update:value="(v: string) => setLines(L.access.user_agents, 'deny', v)" />
            </a-col>
          </a-row>
        </a-tab-pane>

        <!-- ================= 鉴权 ================= -->
        <a-tab-pane key="auth" tab="鉴权">
          <a-form layout="vertical" class="mb-3">
            <a-form-item label="启用"><a-checkbox v-model:checked="L.auth.enabled" /></a-form-item>
            <a-form-item label="Realm"><a-input v-model:value="L.auth.realm" size="small" style="width: 240px" placeholder="Restricted" /></a-form-item>
          </a-form>
          <div class="section-title">Basic 账号 (密码可写 plain 或 sha1:&lt;hex&gt;)</div>
          <a-table :data-source="L.auth.basic!.users" row-key="user" size="small" :pagination="false">
            <a-table-column title="用户名" data-index="user" />
            <a-table-column title="密码">
              <template #default="{ record }"><a-input-password v-model:value="record.password" size="small" /></template>
            </a-table-column>
            <a-table-column title="" width="60">
              <template #default="{ index }"><a class="text-red-500" @click="rmRow(L.auth.basic.users, index)">删除</a></template>
            </a-table-column>
          </a-table>
          <a-button class="mt-2" size="small" @click="addBasicUser">+ 添加账号</a-button>
          <div class="section-title mt-4">子请求鉴权 (外部端点, 2xx=放行)</div>
          <a-row :gutter="12">
            <a-col :span="16">
              <a-input v-model:value="L.auth.sub_request!.url" size="small" placeholder="http://auth.internal/check" />
            </a-col>
            <a-col :span="4">
              <a-input-number v-model:value="L.auth.sub_request!.timeout" :min="1" style="width: 100%" placeholder="超时(秒)" />
            </a-col>
          </a-row>
        </a-tab-pane>

        <!-- ================= 限流 ================= -->
        <a-tab-pane key="limits" tab="限流">
          <div class="section-title">CC 防护 (request_limit: 每客户端 IP 请求频率)</div>
          <a-form layout="vertical">
            <a-form-item label="启用"><a-checkbox v-model:checked="L.request_limit.enabled" /></a-form-item>
            <a-row :gutter="16">
              <a-col :span="6">
                <a-form-item label="时间窗口 (秒)"><a-input-number v-model:value="L.request_limit.interval" :min="1" style="width: 100%" /></a-form-item>
              </a-col>
              <a-col :span="6">
                <a-form-item label="最大请求数"><a-input-number v-model:value="L.request_limit.limit" :min="1" style="width: 100%" /></a-form-item>
              </a-col>
              <a-col :span="6">
                <a-form-item label="动作">
                  <a-select v-model:value="L.request_limit.action" :options="[{ value: 'block', label: 'block (429)' }, { value: 'log', label: 'log (仅记录)' }]" />
                </a-form-item>
              </a-col>
            </a-row>
          </a-form>
          <div class="section-title mt-4">流量限制 (traffic_limit)</div>
          <a-form layout="vertical">
            <a-form-item label="启用"><a-checkbox v-model:checked="L.traffic_limit.enabled" /></a-form-item>
            <a-row :gutter="16">
              <a-col :span="8">
                <a-form-item label="总流量上限 (如 100GB, 空=不限)"><a-input v-model:value="L.traffic_limit.max_bytes" size="small" /></a-form-item>
              </a-col>
              <a-col :span="8">
                <a-form-item label="最大并发连接 (0=不限)"><a-input-number v-model:value="L.traffic_limit.max_conn" :min="0" style="width: 100%" /></a-form-item>
              </a-col>
            </a-row>
          </a-form>
        </a-tab-pane>

        <!-- ================= WAF ================= -->
        <a-tab-pane key="waf" tab="站点 WAF">
          <a-form layout="vertical" class="mb-3">
            <a-form-item label="启用站点 WAF"><a-checkbox v-model:checked="L.waf.enabled" /></a-form-item>
          </a-form>
          <a-row :gutter="16" class="mb-3">
            <a-col :span="12">
              <div class="section-title">IP 允许</div>
              <a-textarea :value="lines(L.waf.ips.allow)" :rows="3" @update:value="(v: string) => setLines(L.waf.ips, 'allow', v)" />
            </a-col>
            <a-col :span="12">
              <div class="section-title">IP 拒绝</div>
              <a-textarea :value="lines(L.waf.ips.deny)" :rows="3" @update:value="(v: string) => setLines(L.waf.ips, 'deny', v)" />
            </a-col>
          </a-row>
          <div class="section-title">WAF 规则</div>
          <a-table :data-source="L.waf.rules" row-key="name" size="small" :pagination="false">
            <a-table-column title="名称" width="120">
              <template #default="{ record }"><a-input v-model:value="record.name" size="small" /></template>
            </a-table-column>
            <a-table-column title="检查目标" width="150">
              <template #default="{ record }">
                <a-select v-model:value="record.target" size="small" :options="['url','query','body','ip','user_agent','referer','method','host'].map((t) => ({ value: t, label: t }))" />
              </template>
            </a-table-column>
            <a-table-column title="操作符" width="150">
              <template #default="{ record }">
                <a-select v-model:value="record.operator" size="small" :options="['equals','not_equals','contains','not_contains','starts_with','ends_with','regex'].map((o) => ({ value: o, label: o }))" />
              </template>
            </a-table-column>
            <a-table-column title="值">
              <template #default="{ record }"><a-input v-model:value="record.value" size="small" /></template>
            </a-table-column>
            <a-table-column title="动作" width="120">
              <template #default="{ record }">
                <a-select v-model:value="record.action" size="small" :options="[{ value: 'block', label: 'block (403)' }, { value: 'captcha', label: 'captcha' }, { value: 'js', label: 'js 挑战' }, { value: 'log', label: 'log' }]" />
              </template>
            </a-table-column>
            <a-table-column title="级别" width="80">
              <template #default="{ record }"><a-input-number v-model:value="record.level" :min="1" :max="5" size="small" style="width: 70px" /></template>
            </a-table-column>
            <a-table-column title="状态" width="90">
              <template #default="{ record }"><a-select v-model:value="record.status" size="small" :options="[{ value: 'normal', label: '启用' }, { value: 'off', label: '停用' }]" /></template>
            </a-table-column>
            <a-table-column title="" width="60">
              <template #default="{ index }"><a class="text-red-500" @click="rmRow(L.waf.rules, index)">删除</a></template>
            </a-table-column>
          </a-table>
          <a-button class="mt-2" size="small" @click="addWafRule">+ 添加规则</a-button>
        </a-tab-pane>

        <!-- ================= 内容 ================= -->
        <a-tab-pane key="content" tab="静态/FastCGI/WS">
          <a-alert type="info" show-icon class="mb-3" message="回源代理 / 静态目录 / FastCGI 三选一。以下配置与[回源代理]页互斥生效（有源站组时优先回源）。" />
          <div class="section-title">静态文件 (root/index)</div>
          <a-row :gutter="16">
            <a-col :span="12">
              <a-form-item label="根目录 (节点本机路径)"><a-input v-model:value="L.root" size="small" placeholder="E:/cdn/static" /></a-form-item>
            </a-col>
            <a-col :span="12">
              <a-form-item label="默认首页 (逗号分隔)"><a-input v-model:value="indexText" size="small" placeholder="index.html, index.htm" /></a-form-item>
            </a-col>
          </a-row>
          <div class="section-title mt-4">FastCGI (PHP 等后端)</div>
          <a-row :gutter="12">
            <a-col :span="10">
              <a-input v-model:value="L.fastcgi.addr" size="small" placeholder="127.0.0.1:9000 (空=未启用)" />
            </a-col>
            <a-col :span="8">
              <a-input v-model:value="L.fastcgi.index" size="small" placeholder="index.php" />
            </a-col>
          </a-row>
          <div class="section-title mt-4">WebSocket</div>
          <a-checkbox v-model:checked="L.websocket.enabled" class="mb-2">启用 WebSocket 透传</a-checkbox>
          <a-input v-model:value="wsPathsText" size="small" style="width: 320px" placeholder="路径前缀 (逗号分隔, 空=全部路径)" />
        </a-tab-pane>

        <!-- ================= 日志统计 ================= -->
        <a-tab-pane key="log" tab="日志/统计">
          <a-form layout="vertical">
            <a-form-item label="访问日志 (上报核心)"><a-checkbox v-model:checked="L.access_log.enabled" /></a-form-item>
            <a-row :gutter="16">
              <a-col :span="8">
                <a-form-item label="格式">
                  <a-select v-model:value="L.access_log.format" :options="[{ value: 'combined', label: 'combined' }, { value: 'json', label: 'json' }]" />
                </a-form-item>
              </a-col>
              <a-col :span="16">
                <a-form-item label="节点本地文件 (可选, 同时写本地)"><a-input v-model:value="L.access_log.local_file" size="small" placeholder="空=仅上报" /></a-form-item>
              </a-col>
            </a-row>
            <a-form-item label="统计采集 (用量统计)"><a-checkbox v-model:checked="L.stat.enabled" /></a-form-item>
          </a-form>
        </a-tab-pane>

        <!-- ================= 路径 ================= -->
        <a-tab-pane key="locations" tab="路径 Location">
          <a-alert type="info" show-icon class="mb-3" message="按顺序匹配，首个命中生效，否则使用默认 Location。每个 Location 可含完整功能栈（高级页 JSON 编辑）。" />
          <a-table :data-source="cfg.locations" row-key="name" size="small" :pagination="false">
            <a-table-column title="名称" width="140">
              <template #default="{ record }"><a-input v-model:value="record.name" size="small" /></template>
            </a-table-column>
            <a-table-column title="匹配类型" width="130">
              <template #default="{ record }">
                <a-select v-model:value="record.pattern.type" size="small" :options="[{ value: 'exact', label: 'exact' }, { value: 'prefix', label: 'prefix' }, { value: 'regex', label: 'regex' }]" />
              </template>
            </a-table-column>
            <a-table-column title="匹配值">
              <template #default="{ record }"><a-input v-model:value="record.pattern.value" size="small" placeholder="/api/ 或 ^/v[0-9]+/" /></template>
            </a-table-column>
            <a-table-column title="" width="120">
              <template #default="{ index }">
                <a @click="editLocJson(index)">JSON</a>
                <a-divider type="vertical" />
                <a class="text-red-500" @click="rmRow(cfg.locations, index)">删除</a>
              </template>
            </a-table-column>
          </a-table>
          <a-button class="mt-2" size="small" @click="addNamedLoc">+ 添加 Location</a-button>
        </a-tab-pane>

        <!-- ================= 节点 ================= -->
        <a-tab-pane key="nodes" tab="节点绑定">
          <a-form layout="vertical">
            <a-form-item label="绑定边缘节点" extra="留空 = 广播到全部节点">
              <a-select v-model:value="nodeIds" mode="multiple" placeholder="选择节点（可多选）" :options="nodes.map((n) => ({ value: n.id, label: `${n.name} (${n.host || '未上线'})` }))" />
            </a-form-item>
          </a-form>
          <a-descriptions v-if="detail" :column="2" size="small">
            <a-descriptions-item label="创建时间">{{ fmtTime(detail.created_at) }}</a-descriptions-item>
            <a-descriptions-item label="最近更新">{{ fmtTime(detail.updated_at) }}</a-descriptions-item>
          </a-descriptions>
        </a-tab-pane>
      </a-tabs>
    </a-spin>

    <a-modal v-model:open="showJson" title="高级配置 (完整 JSON)" width="760px" :footer="null">
      <a-alert type="info" show-icon class="mb-2" message="直接编辑完整站点配置 JSON。应用后需点击[保存配置]生效。" />
      <a-textarea v-model:value="jsonText" :rows="24" class="font-mono text-xs" />
      <div class="mt-3 text-right">
        <a-space>
          <a-button @click="showJson = false">取消</a-button>
          <a-button type="primary" @click="applyJson">应用</a-button>
        </a-space>
      </div>
    </a-modal>

    <a-modal v-model:open="showLocJson" title="Location 完整 JSON" width="680px" :footer="null">
      <a-textarea v-model:value="locJsonText" :rows="20" class="font-mono text-xs" />
      <div class="mt-3 text-right">
        <a-space>
          <a-button @click="showLocJson = false">取消</a-button>
          <a-button type="primary" @click="applyLocJson">应用</a-button>
        </a-space>
      </div>
    </a-modal>
  </div>
</template>

<style scoped>
.section-title {
  color: var(--yf-text);
}
.rule-box {
  border-color: var(--yf-card-border);
  border-radius: var(--yf-radius);
  background: color-mix(in srgb, var(--yf-card-bg) 88%, var(--yf-layout-bg));
}
</style>
