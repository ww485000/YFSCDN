// DTO types — keep in sync with docs/api-contract.md.

export interface ApiResponse<T> {
  code: number;
  message: string;
  data?: T;
}

export interface PageData<T> {
  list: T[];
  total: number;
}

export interface LoginResult {
  token: string;
  user: { id: number; username?: string; name?: string; role: string };
}

export interface Me {
  user_id: number;
  name: string;
  role: string; // admin | tenant
  tenant_id: number;
}

export interface AdminUser {
  id: number;
  username: string;
  role: string; // superadmin | operator
  status: number;
  created_at: string;
}

export interface Tenant {
  id: number;
  name: string;
  username: string;
  email: string;
  status: number;
  max_sites: number;
  traffic_quota_mb: number;
  balance: number;
  created_at: string;
}

export interface Site {
  id: number;
  tenant_id: number;
  name: string;
  domain: string;
  origin_proto: string;
  origin_host: string;
  origin_port: number;
  cache_ttl: number;
  waf_enabled: number;
  https: number;
  status: number;
  node_ids: number[];
  created_at: string;
}

export interface SiteReq {
  name: string;
  domain: string;
  origin_proto: string;
  origin_host: string;
  origin_port: number;
  cache_ttl: number;
  waf_enabled: boolean;
  https: boolean;
  status: number;
  node_ids: number[];
}

export interface EdgeNode {
  id: number;
  name: string;
  token: string;
  tenant_id: number;
  status: number;
  driver: string;
  host: string;
  ip: string;
  last_seen: string;
  created_at: string;
}

export type WafRuleType = 'ip_blacklist' | 'ip_whitelist' | 'ua_blacklist' | 'path_blacklist' | 'rate_limit';

export interface WafRule {
  id: number;
  tenant_id: number;
  name: string;
  type: string;
  value: string;
  action: string; // block | log
  enabled: number;
  created_at: string;
}

export interface Cert {
  id: number;
  tenant_id: number;
  domain: string;
  name?: string;
  type?: string; // ca | custom | acme
  issuer?: string;
  leaf_pem: string;
  key_pem: string;
  expires_at: string;
  created_at: string;
}

export interface UsageDaily {
  id: number;
  tenant_id: number;
  node_id: number;
  site_id: number;
  day: string;
  requests: number;
  bytes: number;
  cache_hits: number;
  cache_misses: number;
}

export interface UsageSummary {
  requests: number;
  bytes: number;
  cache_hits: number;
  cache_misses: number;
  days: number;
}

export interface DayPoint {
  day: string;
  requests: number;
  bytes: number;
  cache_hits: number;
  cache_misses: number;
}

export interface SiteTotal {
  site_id: number;
  requests: number;
  bytes: number;
  cache_hits: number;
  cache_misses: number;
}

export interface DashboardTopSite extends SiteTotal {
  name: string;
  domain: string;
}

export interface OpLog {
  id: number;
  actor_type: string;
  actor_id: number;
  action: string;
  target: string;
  detail: string;
  created_at: string;
}

export interface DashboardStats {
  tenants: number;
  sites: number;
  nodes: number;
  nodes_online: number;
  today_requests: number;
  today_bytes: number;
}

// ---- site config document (mirrors edge/internal/contract/contract.go) ----

export interface SiteServerName {
  name: string;
  is_default: boolean;
  status: string;
  redirect_to?: string;
}

export interface SiteOrigin {
  name: string;
  addr: string;
  status?: string;
  ssl?: boolean;
  sni?: string;
  insecure?: boolean;
}

export interface SiteScheduling {
  type: string; // random | round_robin | hash | sticky
  key?: string;
  ttl?: number;
}

export interface SiteHealthCheck {
  enabled: boolean;
  interval: number;
  timeout: number;
  scheme: string;
  path: string;
  host?: string;
  failures: number;
  successes: number;
}

export interface SiteReverseProxy {
  origins: SiteOrigin[];
  scheduling?: SiteScheduling | null;
  health_check?: SiteHealthCheck | null;
  timeout_sec?: number;
}

export interface SiteCond {
  target: string;
  operator: string;
  value: string;
}

export interface SiteRewrite {
  type: 'rewrite' | 'return';
  value: string;
  status?: number;
  conditions?: SiteCond[];
}

export interface SiteHeaderOp {
  op: string;
  name: string;
  value: string;
}

export interface SiteCORS {
  enabled: boolean;
  allow_origins: string[];
  allow_methods: string[];
  allow_headers: string[];
  max_age: number;
}

export interface SiteHeaders {
  upstream?: SiteHeaderOp[];
  downstream?: SiteHeaderOp[];
  cors?: SiteCORS | null;
}

export interface SiteCompression {
  enabled: boolean;
  types: string[];
  min_size: number;
  mime_types: string[];
  level: number;
}

export interface SiteWebP {
  enabled: boolean;
  quality: number;
  mime_types: string[];
}

export interface SiteCharset {
  enabled: boolean;
  encoding: string;
  convert_from?: string;
}

export interface SiteExpires {
  enabled: boolean;
  time: string;
}

export interface SiteCacheKey {
  include_query: string[];
  ignore_query: string[];
  include_headers: string[];
}

export interface SiteStale {
  enabled: boolean;
  ttl: number;
}

export interface SiteCache {
  enabled: boolean;
  storage: string;
  max_size: string;
  ttl: number;
  ignore_cache_control: boolean;
  only_get: boolean;
  key?: SiteCacheKey | null;
  stale?: SiteStale | null;
  no_cache_headers?: string[];
}

export interface SiteIPList {
  allow: string[];
  deny: string[];
}

export interface SitePatterns {
  allow: string[];
  deny: string[];
}

export interface SiteAccess {
  ips?: SiteIPList | null;
  referers?: SitePatterns | null;
  user_agents?: SitePatterns | null;
}

export interface SiteBasicUser {
  user: string;
  password: string;
}

export interface SiteAuth {
  enabled: boolean;
  realm?: string;
  basic?: { users: SiteBasicUser[] } | null;
  sub_request?: { url: string; timeout: number } | null;
}

export interface SiteRequestLimit {
  enabled: boolean;
  interval: number;
  limit: number;
  action: string;
}

export interface SiteTrafficLimit {
  enabled: boolean;
  max_bytes: string;
  max_conn: number;
}

export interface SiteWAFRule {
  id?: number;
  name: string;
  status: string;
  target: string;
  operator: string;
  value: string;
  action: string;
  level: number;
}

export interface SiteWAF {
  enabled: boolean;
  ips?: SiteIPList | null;
  rules?: SiteWAFRule[];
}

export interface SiteErrorPage {
  status: number;
  enabled: boolean;
  title?: string;
  content: string;
  content_type?: string;
}

export interface SitePages {
  codes: SiteErrorPage[];
  minify?: { html: boolean; css: boolean; js: boolean } | null;
}

export interface SiteWebsocket {
  enabled: boolean;
  paths?: string[];
}

export interface SiteFastCGI {
  addr: string;
  index: string;
  params?: Record<string, string>;
}

export interface SiteAccessLog {
  enabled: boolean;
  format: string;
  local_file?: string;
}

export interface SiteStat {
  enabled: boolean;
}

export interface SiteListener {
  protocol: string;
  listen: string;
  sni?: string[];
  follow_protocol?: boolean;
  origins?: SiteOrigin[];
  scheduling?: SiteScheduling | null;
}

export interface SiteHSTS {
  enabled: boolean;
  max_age: number;
  include_sub_domains: boolean;
  preload: boolean;
}

export interface SiteTLS {
  cert?: string;
  key?: string;
  min_version?: string;
  hsts?: SiteHSTS | null;
}

export interface SiteLocation {
  reverse_proxy?: SiteReverseProxy | null;
  root?: string;
  index?: string[];
  fastcgi?: SiteFastCGI | null;
  rewrites?: SiteRewrite[];
  redirect_https?: { enabled: boolean; status?: number } | null;
  headers?: SiteHeaders | null;
  compression?: SiteCompression | null;
  webp?: SiteWebP | null;
  charset?: SiteCharset | null;
  expires?: SiteExpires | null;
  pages?: SitePages | null;
  cache?: SiteCache | null;
  websocket?: SiteWebsocket | null;
  access?: SiteAccess | null;
  auth?: SiteAuth | null;
  request_limit?: SiteRequestLimit | null;
  traffic_limit?: SiteTrafficLimit | null;
  waf?: SiteWAF | null;
  access_log?: SiteAccessLog | null;
  stat?: SiteStat | null;
}

// Named location: Go's embedded *Location is flattened into the same JSON
// object (plus name + pattern), hence extends SiteLocation.
export interface SiteNamedLocation extends SiteLocation {
  name: string;
  pattern: { type: string; value: string };
}

export interface SiteConfig {
  id?: number;
  name?: string;
  status?: string;
  version?: number;
  server_names: SiteServerName[];
  listeners: SiteListener[];
  tls?: SiteTLS | null;
  location: SiteLocation;
  locations?: SiteNamedLocation[];
}

export interface SiteDetail extends Site {
  config: SiteConfig;
  node_ids: number[];
  status_text?: string;
  updated_at?: string;
}

// ---- tasks ----

export interface Task {
  id: number;
  type: string;
  target: string;
  payload: string;
  status: string; // pending | running | done | failed
  error: string;
  attempts: number;
  created_at: string;
  updated_at: string;
}

// ---- dns records ----

export interface DnsRecord {
  id: number;
  tenant_id: number;
  domain: string;
  name: string;
  type: string; // A | AAAA | CNAME | MX | TXT | NS | SRV
  value: string;
  priority: number;
  ttl: number;
  enabled: number;
  remark: string;
  created_at: string;
  updated_at: string;
}

export interface DnsProvider {
  id: number;
  tenant_id: number;
  name: string;
  type: string; // manual | mock | dnspod | cloudflare | aliyun
  access_key: string;
  secret_key?: string;
  api_endpoint: string;
  status: number;
  remark: string;
  created_at: string;
  updated_at: string;
}

export interface DnsSync {
  id: number;
  tenant_id: number;
  provider_id: number;
  provider_name: string;
  provider_type: string;
  record_id: number;
  domain: string;
  name: string;
  record_type: string;
  value: string;
  upstream_id: string;
  last_hash: string;
  last_error: string;
  created_at: string;
  updated_at: string;
}

// ---- site stats / logs (detail page) ----

export interface SiteStats {
  requests: number;
  bytes: number;
  cache_hits: number;
  cache_misses: number;
  cache_hit_rate: number;
  days: number;
}

export interface AccessLogEntry {
  id: number;
  node_id: number;
  site_id: number;
  ts: string;
  ip: string;
  method: string;
  host: string;
  path: string;
  status: number;
  bytes: number;
  cache: string;
  ua: string;
  referer: string;
  latency_ms: number;
}

// ---- GoEdge parity matrix ----

export type GoEdgeParityStatus = 'done' | 'partial' | 'missing';

export interface GoEdgeParityItem {
  code: string;
  name: string;
  category: string;
  status: GoEdgeParityStatus;
  module: string;
  admin_path: string;
  user_path?: string;
  notes: string;
  next_action?: string;
}

export interface GoEdgeParitySummary {
  total: number;
  done: number;
  partial: number;
  missing: number;
}

export interface GoEdgeParityMatrix {
  summary: GoEdgeParitySummary;
  items: GoEdgeParityItem[];
}
