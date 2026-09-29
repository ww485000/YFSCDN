// DNS record API (admin cross-tenant, tenant own scope).
import { del, get, post, put } from './http';
import type { DnsProvider, DnsRecord, DnsSync, PageData } from '@/types';
import type { Scope } from './site';

const base = (scope: Scope) => (scope === 'admin' ? '/api/v1/admin' : '/api/v1/user');

export function listDns(
  scope: Scope,
  q: { domain?: string; page?: number; size?: number } = {},
): Promise<PageData<DnsRecord>> {
  const p = new URLSearchParams();
  if (q.domain) p.set('domain', q.domain);
  p.set('page', String(q.page || 1));
  p.set('size', String(q.size || 20));
  return get(`${base(scope)}/dns/records?${p.toString()}`);
}

export function listDnsZones(scope: Scope): Promise<string[]> {
  return get(`${base(scope)}/dns/zones`);
}

export function createDns(scope: Scope, b: Partial<DnsRecord> & { tenant_id?: number }): Promise<DnsRecord> {
  return post(`${base(scope)}/dns/records`, b);
}

export function updateDns(scope: Scope, id: number, b: Partial<DnsRecord>): Promise<unknown> {
  return put(`${base(scope)}/dns/records/${id}`, b);
}

export function deleteDns(scope: Scope, id: number): Promise<unknown> {
  return del(`${base(scope)}/dns/records/${id}`);
}

export function listDnsProviders(
  scope: Scope,
  q: { tenantId?: number; page?: number; size?: number } = {},
): Promise<PageData<DnsProvider>> {
  const p = new URLSearchParams();
  if (q.tenantId) p.set('tenant_id', String(q.tenantId));
  p.set('page', String(q.page || 1));
  p.set('size', String(q.size || 20));
  return get(`${base(scope)}/dns/providers?${p.toString()}`);
}

export function createDnsProvider(scope: Scope, b: Partial<DnsProvider> & { tenant_id?: number }): Promise<DnsProvider> {
  return post(`${base(scope)}/dns/providers`, b);
}

export function updateDnsProvider(scope: Scope, id: number, b: Partial<DnsProvider>): Promise<unknown> {
  return put(`${base(scope)}/dns/providers/${id}`, b);
}

export function deleteDnsProvider(scope: Scope, id: number): Promise<unknown> {
  return del(`${base(scope)}/dns/providers/${id}`);
}

export function syncDnsProvider(scope: Scope, id: number): Promise<{ task_id: number }> {
  return post(`${base(scope)}/dns/providers/${id}/sync`, {});
}

export function listDnsSyncs(
  scope: Scope,
  q: { tenantId?: number; status?: string; page?: number; size?: number } = {},
): Promise<PageData<DnsSync>> {
  const p = new URLSearchParams();
  if (q.tenantId) p.set('tenant_id', String(q.tenantId));
  if (q.status) p.set('status', q.status);
  p.set('page', String(q.page || 1));
  p.set('size', String(q.size || 20));
  return get(`${base(scope)}/dns/syncs?${p.toString()}`);
}

export function retryDnsSync(scope: Scope, id: number): Promise<{ task_id: number }> {
  return post(`${base(scope)}/dns/syncs/${id}/retry`, {});
}
