# 症状 → 模块 → 文件 对照表

> 按症状查。"先看文件"一栏是**入口文件**，进去后顺着函数名找。

## core 主控（E:\WORK\9\core）

| 症状 | 模块 | 先看文件 | 提示 |
|---|---|---|---|
| core 起不来/端口占用 | cfg + app | `core/internal/app/app.go`, `core/internal/cfg/cfg.go` | 看启动日志第一行监听地址 |
| 登录失败/密码不对 | authx + auth | `core/internal/auth/auth.go`, `core/internal/authx/password.go` | 默认账号 admin/admin123 |
| token 过期/401 频繁 | authx | `core/internal/authx/jwt.go` | Issue 里 Exp 时长 |
| 路由 404 | webx + app | `core/internal/app/app.go`（注册表）, `core/internal/webx/webx.go` | 路径参数写法 `:id` |
| 统一响应格式 | webx | `core/internal/webx/respond.go` | code/message/data |
| 建表/表结构/字段 | storex | `core/internal/storex/storex.go` | Migrations []string 里加 SQL |
| 租户创建/配额不生效 | tenant + site | `core/internal/tenant/tenant.go` | MaxSites 校验在 site 的 Create |
| 站点创建后 edge 没反应 | site → outbox → edge | 1.`core/internal/site/site.go`(emit) 2.`core/internal/outbox/outbox.go` 3.`edge/internal/corex/client.go` | 按链路三段分别排查；看 core 日志 "outbox append" |
| 站点/域名保存报错 | site | `core/internal/site/site.go` | 域名唯一性、配额 |
| 节点一直离线 | node + app | `core/internal/app/app.go`(offline 协程), `core/internal/node/node.go` | 看 edge 日志是否注册成功；token 是否一致 |
| edge 注册 401 | node | `core/internal/node/node.go` GetByToken | edge 配置 node_token 与 core 节点 token 必须相同 |
| WAF 规则改了不生效 | waf + site | `core/internal/waf/waf.go` OnChanged | WAF 变更靠重新下发 site_upsert |
| 证书/HTTPS 握手失败 | cert + edge | `core/internal/cert/cert.go`, `edge/internal/driver/gopxy/gopxy.go`(TLS) | 自签 CA 信任问题属正常；看 ExpiresAt |
| 流量统计为 0 | usage + edge | `edge/internal/stats/stats.go` → `core/internal/usage/usage.go` | 心跳 reports 里 site_id 必须真实存在 |
| 操作日志缺失 | oplog | `core/internal/oplog/oplog.go` | 业务包调用点（site/tenant 的 Service） |
| 平台参数改了无效 | setting | `core/internal/setting/setting.go` | 读取处是否传了默认值 |

## edge 边缘（E:\WORK\9\edge）

| 症状 | 模块 | 先看文件 | 提示 |
|---|---|---|---|
| edge 起不来 | cfg + main | `edge/cmd/edge/main.go`, `edge/internal/cfg/cfg.go` | core_url / node_token / driver |
| 连不上 core | corex | `edge/internal/corex/client.go` Register | core 是否 8080 在听 |
| 配置一直拉不到 | corex + outbox | `edge/internal/corex/client.go` PollLoop | cursor 是否递增；core 侧 outbox 是否有事件 |
| 访问站点 404（没有匹配站点） | gopxy | `edge/internal/driver/gopxy/gopxy.go` ServeHTTP | Host 头（含端口）要剥离端口再匹配 |
| 回源失败 502 | gopxy | `edge/internal/driver/gopxy/gopxy.go` Director | origin_proto/host/port 是否对；源站是否活 |
| 缓存不命中 | gopxy | `edge/internal/driver/gopxy/cache.go` | cache_ttl=0 表示不缓存；看日志 "cache miss" |
| WAF 拦截不生效 | wafx | `edge/internal/wafx/wafx.go` | site 的 waf_enabled 必须为 true |
| HTTPS 证书没加载 | gopxy | `edge/internal/driver/gopxy/gopxy.go` GetCertificate | SiteSpec.https 与 cert_leaf_pem 是否为空 |
| nginx 模式报错 | nginx | `edge/internal/driver/nginx/nginx.go` | 需要本机有 nginx 可执行文件（配置 nginx_bin） |
| 内存里站点和 core 不一致 | corex | `edge/internal/corex/client.go` apply | site_upsert 是全量覆盖；重启 edge 会拉全量 |

## web 前端（E:\WORK\9\web）

| 症状 | 模块 | 先看文件 | 提示 |
|---|---|---|---|
| 页面 404/路由错 | router | `web/src/router/index.ts` | 角色隔离：/admin 需 admin，/portal 需 tenant |
| 接口 401 跳登录 | stores + api | `web/src/stores/auth.ts`, `web/src/api/http.ts` | token 存 localStorage key=edgetoken |
| 接口请求没发出去/跨域 | vite + http | `web/vite.config.ts`（proxy /api→:8080）, `web/src/api/http.ts` | 生产环境 core 用 -static 托管 dist 就没有跨域 |
| 登录页打不开 core | auth + http | `web/src/views/login/LoginView.vue` | scope 选错角色会 401 |
| 表格没数据但 curl 有 | views + api | 对应 `web/src/views/*` + `web/src/api/*.ts` | 响应解包：http.ts 已剥 code/data |
| 菜单少了 | layouts | `web/src/layouts/MainLayout.vue` | 菜单按角色生成 |
| 构建报错 TS | types | `web/src/types/index.ts` | 与 api-contract.md 对齐 |
