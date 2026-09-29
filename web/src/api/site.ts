// Site / WAF / Certs API — used by both scopes (admin vs tenant).
// scope 'admin' -> /api/v1/admin/*, scope 'tenant' -> /api/v1/user/*
import { del, get, post, put } from './http';
import type {
  AccessLogEntry,
  Cert,
  PageData,
  Site,
  SiteConfig,
  SiteDetail,
  SiteReq,
  SiteStats,
  UsageDaily,
  UsageSummary,
  WafRule,
} from '@/types';

export type Scope = 'admin' | 'tenant';

const base = (scope: Scope) => (scope === 'admin' ? '/api/v1/admin' : '/api/v1/user');

// ---- sites ----
export function listSites(
  scope: Scope,
  q: { tenantId?: number; keyword?: string; page?: number; size?: number } = {},
): Promise<PageData<Site>> {
  const p = new URLSearchParams();
  if (scope === 'admin' && q.tenantId) p.set('tenant_id', String(q.tenantId));
  if (q.keyword) p.set('keyword', q.keyword);
  p.set('page', String(q.page || 1));
  p.set('size', String(q.size || 20));
  return get(`${base(scope)}/sites?${p.toString()}`);
}

export function getSite(scope: Scope, id: number): Promise<SiteDetail> {
  return get(`${base(scope)}/sites/${id}`);
}

export function createSite(scope: Scope, b: SiteReq & { tenant_id?: number }): Promise<Site> {
  const body = scope === 'admin' ? { ...b, tenant_id: b.tenant_id } : b;
  return post(`${base(scope)}/sites`, body);
}

export function updateSite(scope: Scope, id: number, b: Partial<SiteReq>): Promise<Site> {
  return put(`${base(scope)}/sites/${id}`, b);
}

/** Update the full config document (site settings form). */
export function updateSiteConfig(
  scope: Scope,
  id: number,
  config: SiteConfig,
  nodeIds?: number[],
): Promise<SiteDetail> {
  const body: Record<string, unknown> = { config };
  if (nodeIds !== undefined) body.node_ids = nodeIds;
  return put(`${base(scope)}/sites/${id}`, body);
}

export function setSiteStatus(scope: Scope, id: number, status: string): Promise<SiteDetail> {
  return post(`${base(scope)}/sites/${id}/status`, { status });
}

export function deleteSite(scope: Scope, id: number): Promise<unknown> {
  return del(`${base(scope)}/sites/${id}`);
}

/** Per-day usage series for one site. */
export function siteUsageSeries(
  scope: Scope,
  id: number,
  days = 7,
): Promise<{ day: string; requests: number; bytes: number; cache_hits: number; cache_misses: number }[]> {
  return get(`${base(scope)}/sites/${id}/usage?days=${days}`);
}

export function siteStats(scope: Scope, id: number): Promise<SiteStats> {
  return get(`${base(scope)}/sites/${id}/stats`);
}

export function siteAccessLogs(
  scope: Scope,
  id: number,
  q: { page?: number; size?: number } = {},
): Promise<PageData<AccessLogEntry>> {
  const p = new URLSearchParams();
  p.set('page', String(q.page || 1));
  p.set('size', String(q.size || 20));
  return get(`${base(scope)}/sites/${id}/logs?${p.toString()}`);
}

// ---- waf rules ----
export function listWafRules(scope: Scope, tenantId = 0): Promise<PageData<WafRule>> {
  const p = new URLSearchParams();
  if (scope === 'admin' && tenantId) p.set('tenant_id', String(tenantId));
  const s = p.toString();
  return get(`${base(scope)}/waf-rules${s ? `?${s}` : ''}`);
}

export function createWafRule(
  scope: Scope,
  b: { name: string; type: string; value: string; action: string; tenant_id?: number },
): Promise<WafRule> {
  const body = scope === 'admin' ? { ...b, tenant_id: b.tenant_id } : b;
  return post(`${base(scope)}/waf-rules`, body);
}

export function updateWafRule(
  scope: Scope,
  id: number,
  b: { name: string; type: string; value: string; action: string; enabled: number },
): Promise<unknown> {
  return put(`${base(scope)}/waf-rules/${id}`, b);
}

export function deleteWafRule(scope: Scope, id: number): Promise<unknown> {
  return del(`${base(scope)}/waf-rules/${id}`);
}

// ---- certs ----
export function listCerts(scope: Scope, tenantId = 0): Promise<Cert[]> {
  const p = new URLSearchParams();
  if (scope === 'admin' && tenantId) p.set('tenant_id', String(tenantId));
  const s = p.toString();
  return get(`${base(scope)}/certs${s ? `?${s}` : ''}`);
}

export function uploadCert(
  scope: Scope,
  b: { domain?: string; name?: string; cert_pem: string; key_pem: string; tenant_id?: number },
): Promise<Cert> {
  const body = scope === 'admin' ? { ...b, tenant_id: b.tenant_id } : b;
  return post(`${base(scope)}/certs/upload`, body);
}

export function reissueCert(scope: Scope, id: number): Promise<Cert> {
  return post(`${base(scope)}/certs/${id}/reissue`, {});
}

export function deleteCert(scope: Scope, id: number): Promise<unknown> {
  return del(`${base(scope)}/certs/${id}`);
}

// ---- usage (tenant scope for portal) ----
export function tenantUsageSummary(scope: Scope): Promise<UsageSummary> {
  return get(`${base(scope)}/usage/summary`);
}

export function tenantUsageDaily(scope: Scope): Promise<UsageDaily[]> {
  return get(`${base(scope)}/usage/daily`);
}
