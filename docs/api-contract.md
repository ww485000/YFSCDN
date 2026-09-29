# API 契约（模块间通信的唯一事实来源）

> 改任何 API 必须同步改本文件。core 与 edge 的 DTO 是**有意的重复**
> （core/internal/site 的 SiteSpec 与 edge/internal/driver 的 SiteSpec），
> 二者字段必须与本文件一致。

## 统一响应包

```json
{ "code": 0, "message": "ok", "data": ... }
```
`code != 0` 即错误；HTTP 状态码：200 成功，400 参数错，401 未认证，403 无权限，
404 不存在，500 内部错误。

## 认证

- 登录：`POST /api/v1/auth/login` `{"username","password","scope":"admin"|"user"}`
  → `data: {"token": "<jwt>", "user": {...}}`
- 之后所有请求：`Authorization: Bearer <jwt>`
- JWT Claims：`{sub, name, role: admin|tenant, tenant_id, exp}`（HS256）
- 节点认证：`X-Node-Token: <token>`（edge 的 token 由运营在节点创建时生成）

## 运营端 API（role=admin，前缀 /api/v1/admin）

| 方法 路径 | 说明 |
|---|---|
| GET /dashboard | 统计：租户数、站点数、在线节点数、今日流量字节 |
| GET/POST /admins，PUT/DELETE /admins/:id | 管理端账号 CRUD（role: superadmin/operator） |
| GET/POST /tenants，PUT/DELETE /tenants/:id | 租户 CRUD；POST 时 password 必填 |
| GET/POST /sites，PUT/DELETE /sites/:id | 站点 CRUD（跨租户，GET 支持 ?tenant_id=&keyword=） |
| GET/POST /nodes，PUT/DELETE /nodes/:id | 节点 CRUD；POST 返回新节点含 token |
| GET/POST /waf-rules，PUT/DELETE /waf-rules/:id | WAF 规则 CRUD（?tenant_id= 过滤） |
| GET /certs | 证书列表（?tenant_id=） |
| GET /usage/summary?tenant_id=&from=&to= | 流量汇总（总请求/字节/命中） |
| GET /usage/daily?tenant_id=&from=&to= | 每日明细 |
| GET /settings，PUT /settings | 平台参数 KV（整体读 / 整体写 map） |
| GET /oplogs?page=&size=&keyword= | 操作日志 |

## 租户端 API（role=tenant，前缀 /api/v1/user，全部自动限定 JWT 内的 tenant_id）

| 方法 路径 | 说明 |
|---|---|
| GET/POST /sites，PUT/DELETE /sites/:id | 自己的站点 |
| GET/POST /waf-rules，PUT/DELETE /waf-rules/:id | 自己的 WAF 规则 |
| GET /nodes | 自己被分配的节点（只读） |
| GET /certs | 自己的证书 |
| GET /usage/summary，/usage/daily | 自己的流量 |
| POST /password `{"old_password","new_password"}` | 改自己密码 |

## 节点端 API（X-Node-Token，前缀 /api/v1/edge）

### POST /register
```json
req : {"token":"<创建节点时生成的 token>","host":"1.2.3.4","ip":"1.2.3.4","driver":"gopxy"}
resp: {"node_id":1,"name":"node-a"}
```

### GET /poll?token=&cursor=<int>   （长轮询，服务端最多挂 30s）
```json
resp: {"cursor": 1024,
       "events":[{"type":"site_upsert","payload":{SiteSpec}},
                 {"type":"site_delete","payload":{"id":12}}]}
```
- `cursor` = 事件 version。edge 记住返回的 cursor 下次带上。
- 事件 type 目前只有两种：`site_upsert`（全量站点规格，幂等）与 `site_delete`。
  WAF/证书变更通过重新下发 site_upsert 表达，edge 永远以最新全量为准。

### POST /heartbeat   （token 走 X-Node-Token 请求头，body 里没有 token）
```json
req : {"host":"1.2.3.4","ip":"1.2.3.4","driver":"gopxy",
       "reports":[{"site_id":1,"requests":120,"bytes":345678,"cache_hits":100,"cache_misses":20}]}
resp: {"code":0,"message":"ok","data":{"ok":true,"node_id":1}}
```
core 把 reports 按站点归属租户聚合进 usage_daily（当天）。

> token 位置速记：register=body、poll=`?token=` 查询参数（也接受 X-Node-Token 头）、heartbeat=X-Node-Token 头。

## SiteSpec（site_upsert 的 payload，core 与 edge 各持一份）

```json
{
  "id": 12,
  "domain": "www.example.com",
  "origin_proto": "http",
  "origin_host": "192.168.1.10",
  "origin_port": 8000,
  "cache_ttl": 600,
  "waf_enabled": true,
  "https": true,
  "cert_leaf_pem": "-----BEGIN CERTIFICATE-----...",
  "cert_key_pem": "-----BEGIN PRIVATE KEY-----...",
  "rules": [
    {"type": "ip_blacklist", "value": "10.0.0.0/8", "action": "block"},
    {"type": "rate_limit",   "value": "100/60s",     "action": "block"}
  ]
}
```

## WAF 规则类型（edge 侧引擎语义，两边必须一致）

| type | value | 语义 |
|---|---|---|
| ip_blacklist | IP 或 CIDR | 命中 → action |
| ip_whitelist | IP/CIDR 逗号分隔 | 全部未命中 → block |
| ua_blacklist | 子串（不区分大小写） | User-Agent 包含 → action |
| path_blacklist | 路径前缀，逗号分隔 | 命中 → action |
| rate_limit | `N/60s` | 每 IP 60 秒超 N 次 → action |

`action`: `block`（403）或 `log`（放行但记录）。

## 设置项（settings KV）

| key | 默认 | 说明 |
|---|---|---|
| platform.name | EdgeCDN | 平台显示名 |
| tenant.default_max_sites | 10 | 新租户默认站点配额 |
| tenant.default_traffic_mb | 10240 | 新租户默认流量额度(MB)，0=不限 |
| cdn.rate_per_mb | 0 | 每 MB 单价（虚拟币），0=免费 |
| edge.poll_timeout_sec | 30 | 长轮询超时 |
