<script setup lang="ts">
// Soybean-style login page: shared theme tokens, role switch, product capability panel.
import { reactive, ref } from 'vue';
import { useRouter } from 'vue-router';
import { message } from 'ant-design-vue';
import {
  ApiOutlined,
  BulbOutlined,
  CloudServerOutlined,
  GlobalOutlined,
  PoweroffOutlined,
  SafetyCertificateOutlined,
} from '@ant-design/icons-vue';
import { useAuthStore } from '@/stores/auth';
import { useThemeStore } from '@/stores/theme';

const auth = useAuthStore();
const theme = useThemeStore();
const router = useRouter();

const scope = ref<'admin' | 'user'>('admin');
const form = reactive({ username: '', password: '' });
const loading = ref(false);
const capabilities = [
  { label: '多租户运营', icon: CloudServerOutlined },
  { label: '边缘节点调度', icon: ApiOutlined },
  { label: 'DNS / HTTPS', icon: GlobalOutlined },
  { label: 'WAF 安全策略', icon: SafetyCertificateOutlined },
];

async function submit() {
  if (!form.username || !form.password) {
    message.warning('请输入用户名和密码');
    return;
  }
  loading.value = true;
  try {
    await auth.login(form.username, form.password, scope.value);
    message.success('登录成功');
    void router.push(scope.value === 'admin' ? '/admin/dashboard' : '/portal/dashboard');
  } catch (e) {
    message.error(e instanceof Error ? e.message : '登录失败');
  } finally {
    loading.value = false;
  }
}
</script>

<template>
  <div class="login-shell">
    <div class="login-header">
      <div class="login-brand">
        <div class="login-logo">YF</div>
        <div>
          <div class="login-brand-title">YFSCDN</div>
          <div class="login-brand-sub">GoEdge 功能复刻 · Soybean 风格控制台</div>
        </div>
      </div>
      <a-tooltip title="主题模式">
        <a-button type="text" @click="theme.toggleMode">
          <template #icon>
            <BulbOutlined v-if="theme.isDark" />
            <PoweroffOutlined v-else />
          </template>
        </a-button>
      </a-tooltip>
    </div>

    <main class="login-main">
      <section class="login-brief">
        <div class="login-kicker">Modular CDN Platform</div>
        <h1>模块化、可运营、多租户 CDN 主控</h1>
        <p>
          以 GoEdge 功能为目标，保留 core、edge、web 独立模块边界，并用统一 Soybean 风格模板承载后续功能。
        </p>
        <div class="login-capabilities">
          <div v-for="item in capabilities" :key="item.label" class="login-capability">
            <component :is="item.icon" />
            <span>{{ item.label }}</span>
          </div>
        </div>
      </section>

      <section class="login-panel">
        <div class="login-panel-head">
          <div>
            <div class="login-panel-title">登录控制台</div>
            <div class="login-panel-sub">选择运营端或租户端身份进入</div>
          </div>
        </div>

        <a-segmented
          v-model:value="scope"
          block
          class="mb-5"
          :options="[
            { label: '运营后台', value: 'admin' },
            { label: '租户门户', value: 'user' },
          ]"
        />

        <a-form layout="vertical" :model="form" @finish="submit">
          <a-form-item label="用户名" :rules="[{ required: true, message: '请输入用户名' }]">
            <a-input v-model:value="form.username" size="large" placeholder="admin / 租户用户名" allow-clear />
          </a-form-item>
          <a-form-item label="密码" :rules="[{ required: true, message: '请输入密码' }]">
            <a-input-password v-model:value="form.password" size="large" placeholder="默认 admin / admin123" />
          </a-form-item>
          <a-button type="primary" html-type="submit" size="large" :loading="loading" block>
            登录
          </a-button>
        </a-form>

        <div class="login-hint">
          首次启动：运营账号 admin / admin123。租户账号由运营端创建。
        </div>
      </section>
    </main>
  </div>
</template>

<style scoped>
.login-shell {
  min-height: 100vh;
  background:
    linear-gradient(90deg, color-mix(in srgb, var(--yf-primary) 11%, transparent) 1px, transparent 1px),
    linear-gradient(180deg, color-mix(in srgb, var(--yf-primary) 9%, transparent) 1px, transparent 1px),
    var(--yf-layout-bg);
  background-size: 42px 42px;
  color: var(--yf-text);
}

.login-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  height: 72px;
  padding: 0 40px;
}

.login-brand {
  display: flex;
  align-items: center;
  gap: 12px;
}

.login-logo {
  display: grid;
  width: 38px;
  height: 38px;
  place-items: center;
  border-radius: 10px;
  background: var(--yf-primary);
  color: #fff;
  font-weight: 700;
}

.login-brand-title {
  font-size: 18px;
  font-weight: 700;
}

.login-brand-sub,
.login-panel-sub,
.login-hint,
.login-kicker {
  color: var(--yf-text-2);
}

.login-main {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 420px;
  gap: 48px;
  align-items: center;
  width: min(1120px, calc(100% - 48px));
  min-height: calc(100vh - 72px);
  margin: 0 auto;
  padding: 32px 0 72px;
}

.login-brief h1 {
  max-width: 660px;
  margin: 14px 0 18px;
  font-size: 44px;
  line-height: 1.12;
}

.login-brief p {
  max-width: 620px;
  margin: 0;
  color: var(--yf-text-2);
  font-size: 16px;
  line-height: 1.8;
}

.login-kicker {
  font-size: 13px;
  font-weight: 700;
  letter-spacing: 0.08em;
  text-transform: uppercase;
}

.login-capabilities {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 190px));
  gap: 12px;
  margin-top: 32px;
}

.login-capability {
  display: flex;
  align-items: center;
  gap: 10px;
  height: 44px;
  padding: 0 14px;
  border: 1px solid var(--yf-card-border);
  border-radius: var(--yf-radius);
  background: color-mix(in srgb, var(--yf-card-bg) 78%, transparent);
}

.login-capability .anticon {
  color: var(--yf-primary);
}

.login-panel {
  padding: 28px;
  border: 1px solid var(--yf-card-border);
  border-radius: calc(var(--yf-radius) + 4px);
  background: color-mix(in srgb, var(--yf-card-bg) 92%, transparent);
  box-shadow: 0 20px 60px rgb(15 23 42 / 12%);
  backdrop-filter: blur(16px);
}

.login-panel-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 22px;
}

.login-panel-title {
  font-size: 22px;
  font-weight: 700;
}

.login-hint {
  margin-top: 16px;
  font-size: 12px;
  line-height: 1.7;
}

@media (max-width: 900px) {
  .login-header {
    padding: 0 20px;
  }

  .login-main {
    grid-template-columns: 1fr;
    gap: 28px;
    width: min(520px, calc(100% - 32px));
  }

  .login-brief h1 {
    font-size: 32px;
  }

  .login-capabilities {
    grid-template-columns: 1fr;
  }
}
</style>
