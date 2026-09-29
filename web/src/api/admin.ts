// Operator (admin-scope) API — /api/v1/admin/*.
import { del, get, post, put } from './http';
import type { AdminUser, DashboardStats, DashboardTopSite, DayPoint, OpLog, PageData, Tenant, UsageDaily, UsageSummary } from '@/types';

// dashboard
export const getDashboard = (): Promise<DashboardStats> => get('/api/v1/admin/dashboard');
export const dashboardSeries = (days = 14): Promise<DayPoint[]> => get(`/api/v1/admin/dashboard/series?days=${days}`);
export const dashboardTopSites = (days = 7): Promise<DashboardTopSite[]> => get(`/api/v1/admin/dashboard/top-sites?days=${days}`);

// admins
export const listAdmins = (): Promise<AdminUser[]> => get('/api/v1/admin/admins');
export const createAdmin = (b: { username: string; password: string; role: string }): Promise<AdminUser> =>
  post('/api/v1/admin/admins', b);
export const updateAdmin = (id: number, b: { role?: string; status?: number; password?: string }): Promise<unknown> =>
  put(`/api/v1/admin/admins/${id}`, b);
export const deleteAdmin = (id: number): Promise<unknown> => del(`/api/v1/admin/admins/${id}`);

// tenants
export const listTenants = (q: { keyword?: string; page?: number; size?: number } = {}): Promise<PageData<Tenant>> => {
  const p = new URLSearchParams();
  if (q.keyword) p.set('keyword', q.keyword);
  if (q.page) p.set('page', String(q.page));
  if (q.size) p.set('size', String(q.size));
  const s = p.toString();
  return get(`/api/v1/admin/tenants${s ? `?${s}` : ''}`);
};
export const createTenant = (b: {
  name: string;
  username: string;
  password: string;
  email?: string;
}): Promise<Tenant> => post('/api/v1/admin/tenants', b);
export const updateTenant = (
  id: number,
  b: { name: string; email: string; max_sites: number; traffic_quota_mb: number; status: number; password?: string },
): Promise<unknown> => put(`/api/v1/admin/tenants/${id}`, b);
export const deleteTenant = (id: number): Promise<unknown> => del(`/api/v1/admin/tenants/${id}`);

// oplogs
export const listOplogs = (q: { page?: number; size?: number; keyword?: string } = {}): Promise<PageData<OpLog>> => {
  const p = new URLSearchParams();
  p.set('page', String(q.page || 1));
  p.set('size', String(q.size || 20));
  if (q.keyword) p.set('keyword', q.keyword);
  return get(`/api/v1/admin/oplogs?${p.toString()}`);
};

// settings
export const getSettings = (): Promise<Record<string, string>> => get('/api/v1/admin/settings');
export const saveSettings = (m: Record<string, string>): Promise<unknown> => put('/api/v1/admin/settings', m);

// usage (admin scope: may filter by tenant)
export const usageSummary = (tenantId = 0, from = '', to = ''): Promise<UsageSummary> => {
  const p = new URLSearchParams();
  if (tenantId) p.set('tenant_id', String(tenantId));
  if (from) p.set('from', from);
  if (to) p.set('to', to);
  const s = p.toString();
  return get(`/api/v1/admin/usage/summary${s ? `?${s}` : ''}`);
};
export const usageDaily = (tenantId = 0, from = '', to = ''): Promise<UsageDaily[]> => {
  const p = new URLSearchParams();
  if (tenantId) p.set('tenant_id', String(tenantId));
  if (from) p.set('from', from);
  if (to) p.set('to', to);
  const s = p.toString();
  return get(`/api/v1/admin/usage/daily${s ? `?${s}` : ''}`);
};
