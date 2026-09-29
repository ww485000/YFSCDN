import { computed, ref, watch } from 'vue';
import { defineStore } from 'pinia';
import { theme as antdTheme } from 'ant-design-vue';

type ThemeMode = 'light' | 'dark';
type ThemeDensity = 'default' | 'compact';

const storageKey = 'yfscdn-theme';

interface ThemeState {
  mode: ThemeMode;
  primaryColor: string;
  density: ThemeDensity;
  radius: number;
}

const defaultState: ThemeState = {
  mode: 'light',
  primaryColor: '#646cff',
  density: 'default',
  radius: 6,
};

function loadState(): ThemeState {
  try {
    const raw = localStorage.getItem(storageKey);
    if (!raw) return { ...defaultState };
    return { ...defaultState, ...JSON.parse(raw) };
  } catch {
    return { ...defaultState };
  }
}

export const useThemeStore = defineStore('theme', () => {
  const state = ref<ThemeState>(loadState());

  const isDark = computed(() => state.value.mode === 'dark');
  const isCompact = computed(() => state.value.density === 'compact');
  const antd = computed(() => ({
    algorithm: [
      isDark.value ? antdTheme.darkAlgorithm : antdTheme.defaultAlgorithm,
      ...(isCompact.value ? [antdTheme.compactAlgorithm] : []),
    ],
    token: {
      colorPrimary: state.value.primaryColor,
      borderRadius: state.value.radius,
      fontFamily:
        "-apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'PingFang SC', 'Microsoft YaHei', sans-serif",
    },
    components: {
      Layout: {
        headerBg: 'var(--yf-header-bg)',
        siderBg: 'var(--yf-sider-bg)',
        bodyBg: 'var(--yf-layout-bg)',
      },
      Menu: {
        darkItemBg: 'transparent',
        darkSubMenuItemBg: 'transparent',
        darkItemSelectedBg: 'rgba(255, 255, 255, 0.16)',
        itemBorderRadius: state.value.radius,
      },
      Table: {
        headerBg: 'var(--yf-table-header-bg)',
      },
    },
  }));

  function applyDomVars() {
    const root = document.documentElement;
    root.dataset.theme = state.value.mode;
    root.style.setProperty('--yf-primary', state.value.primaryColor);
    root.style.setProperty('--yf-radius', `${state.value.radius}px`);
    root.style.setProperty('--yf-content-padding', isCompact.value ? '12px' : '16px');
  }

  function toggleMode() {
    state.value.mode = state.value.mode === 'dark' ? 'light' : 'dark';
  }

  function toggleDensity() {
    state.value.density = state.value.density === 'compact' ? 'default' : 'compact';
  }

  function setPrimaryColor(color: string) {
    state.value.primaryColor = color;
  }

  watch(
    state,
    (next) => {
      localStorage.setItem(storageKey, JSON.stringify(next));
      applyDomVars();
    },
    { deep: true },
  );

  applyDomVars();

  return {
    state,
    isDark,
    antd,
    toggleMode,
    toggleDensity,
    setPrimaryColor,
  };
});
