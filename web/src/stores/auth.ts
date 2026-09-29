// Auth store: login state, role detection.
import { defineStore } from 'pinia';
import { login as apiLogin, me as apiMe } from '@/api/auth';
import { clearToken, getToken, setToken } from '@/api/http';

export interface AuthUser {
  id: number;
  username?: string;
  name?: string;
  role: string; // superadmin | operator | tenant
}

export const useAuthStore = defineStore('auth', {
  state: () => ({
    token: getToken(),
    user: null as AuthUser | null,
  }),
  getters: {
    isLogin: (s): boolean => !!s.token,
    isTenant: (s): boolean => s.user?.role === 'tenant',
    isAdmin: (s): boolean => s.user?.role === 'superadmin' || s.user?.role === 'operator',
    displayName: (s): string => s.user?.username || s.user?.name || '',
  },
  actions: {
    async login(username: string, password: string, scope: 'admin' | 'user') {
      const d = await apiLogin({ username, password, scope });
      this.token = d.token;
      setToken(d.token);
      this.user = d.user as AuthUser;
    },
    async fetchMe() {
      try {
        const me = await apiMe();
        this.user = { id: me.user_id, username: me.name, name: me.name, role: me.role };
      } catch {
        this.logout();
      }
    },
    logout() {
      this.token = '';
      this.user = null;
      clearToken();
    },
  },
});
