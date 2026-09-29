# EdgeCDN

模块化、多租户、可运营的 CDN 系统。参考 GoEdge（边缘转发能力）与 soybean-admin（前端风格），
按「主控 core + 边缘 edge + 控制台 web」三个独立模块构建，每个模块可单独编译、单独部署、单独演进。

```
                ┌────────────────────────────────────────────┐
                │  web/  运营控制台 + 租户门户 (Vue3 SPA)     │
                │  /admin/*  /portal/*                       │
                └──────────────────┬─────────────────────────┘
                                   │ REST /api/v1 (JWT)
                ┌──────────────────▼─────────────────────────┐
                │  core/  主控 (Go, 单二进制)                 │
                │  认证 · 租户 · 站点 · 节点 · WAF · 用量      │
                │  设置 · 操作日志 · 自签 CA · outbox 事件总线 │
                │  SQLite(单文件) · 同时托管 web/dist         │
                └───┬──────────────────────────────┬─────────┘
        REST+长轮询  │                              │  证书/规则/配置
        (节点 token) │                              │  (site_upsert 事件)
                ┌───▼───────────┐          ┌────────▼────────┐
                │ edge/ 节点 A   │          │ edge/ 节点 B     │
                │ driver: gopxy │          │ driver: nginx   │
                │ 纯Go反代+磁盘缓存│          │ 生成nginx.conf   │
                │ 或 nginx 生成  │          │ 并 reload        │
                └───────┬───────┘          └────────┬────────┘
                        └────────────┬──────────────┘
                                     ▼ 回源
                                 源站 (任意)
```

## 模块与目录

| 模块 | 目录 | 说明 |
|------|------|------|
| core | `core/` | 主控：全部管理面 + 事件总线 + 自签 CA。`cmd/core` 入口，`internal/*` 按功能分包 |
| edge | `edge/` | 边缘节点：corex 客户端（注册/长轮询/全量同步/心跳）+ 可插拔驱动 |
| web | `web/` | 前端：Vite + Vue3 + TS + Pinia + UnoCSS + antd，soybean 风格布局 |
| docs | `docs/` | 架构、API 契约、AI 导航/排障/新模块指南 |
| .tools | `.tools/` | 本地工具链（Go SDK、编译产物、冒烟脚本）— 不入库 |

core 内部包（每个包即一个「子模块」，互不反向依赖）：
`webx`(路由) `storex`(SQLite+迁移) `authx`(JWT+bcrypt) `auth`(管理员) `tenant` `site` `node`
`waf` `cert`(自签CA) `outbox`(事件) `usage`(计量) `setting` `oplog` `user`(租户自助) `app`(装配)

## 快速开始（Windows）

前置：本仓库自带 Go 于 `.tools/go/`（也可用系统 Go ≥1.22）；前端用 pnpm（node ≥ 20）。

```powershell
cd E:\WORK\9
.\start.ps1            # 编译 core+edge+web，启动 core（托管前端）
# 浏览器打开 http://127.0.0.1:8080
# 登录：运营后台 admin / admin123（登录后请立即改密：运营账号页）
```

首次上线流程（全部在控制台里点）：
1. **租户** 页 → 新建租户（设置门户登录名/密码、站点配额、流量额度）
2. **边缘节点** 页 → 新建节点 → 复制 token
3. 在节点机器上写 `edge/config.json`（见下）→ 启动 `edge.exe`
4. **站点** 页 → 新建站点：域名、回源地址/端口、缓存 TTL、WAF 开关、HTTPS 开关、绑定节点
   - 边缘节点 3 秒内自动收到配置并生效（长轮询推送，无需重启）
5. **WAF 规则** 页 → 配规则后自动重新下发到相关站点
6. 浏览器把域名指向边缘节点 IP（hosts 或 DNS）即可访问

edge/config.json 示例：

```json
{
  "core_url": "http://<core-ip>:8080",
  "node_token": "<在控制台创建的节点 token>",
  "name": "node-a",
  "driver": "gopxy",
  "listen_addr": "0.0.0.0:80",
  "tls_addr": "0.0.0.0:443",
  "data_dir": "D:\\edgecdn\\data",
  "heartbeat_sec": 30,
  "host": "<本机对外IP，用于回源/展示>"
}
```

启动 edge：`edge.exe -config D:\edgecdn\config.json -log D:\edgecdn\edge.log`（Windows 服务/计划任务或 systemd 均可；
`-log` 可选，日志同时写文件与 stderr）。core 同样支持 `-log`。

Linux 部署：`cd core && go build -o core ./cmd/core`；`cd edge && go build -o edge ./cmd/edge`，
core 加 `-static web/dist` 即托管前端。

## 已验证（E2E 冒烟，全部通过）

`.tools/smoke.js` / `.tools/https-smoke.js`（本地冒烟，非仓库交付物）覆盖：

- 健康检查、admin/tenant 双通道登录、作用域守卫（租户 token 调 admin API → 403）
- 租户 CRUD、节点 CRUD（token 仅创建时显示）、站点 CRUD + 配额
- 边缘：注册、长轮询收 `site_upsert`、反代 200、磁盘缓存 MISS→HIT、未知域名 404
- WAF：UA 黑名单拦截 403 / 白名单放行；改规则后自动重下发
- 用量：心跳上报 → 按 租户/节点/站点/天 聚合，admin 与租户视角均可查
- HTTPS：平台自签 CA 签发站点证书，edge 按 SNI 提供（演示模式，浏览器需导入平台 CA）
- 重启恢复：edge 重启后全量同步拉回全部绑定站点（含广播站点）

## 给本地小模型 AI 的约定（重要）

本项目刻意保持「低复杂度 + 强约定」，方便算力有限的本地模型参与维护：

- **所有 API 响应**：`{code, message, data}`，`code=0` 成功。前端统一走 `web/src/api/http.ts`。
- **core 路由**：全部在 `core/internal/app/app.go` 一处装配，改路由先看这里。
- **数据库 DDL**：只在 `core/internal/storex/storex.go` 的 `Migrations` 里**追加**（老库依赖既有语句）。
- **单连接 SQLite**：`rows.Next()` 循环体内**禁止**再发任何 DB 查询（会自死锁）——
  先收集 id、`rows.Close()`、再做逐条查询（见 `node/handler.go` 的 `full`）。
- **事件下发**：站点变更只通过 outbox 事件推给绑定节点（空绑定=广播），不要在别处开推送通道。
- **契约**：`docs/api-contract.md` 是前后端 + core/edge 的单一事实来源，改接口先改契约。
- 导航：`docs/ai/NAVIGATION.md`（去哪找什么）· 排障：`docs/ai/SYMPTOMS.md` · 加功能：`docs/ai/NEW-MODULE.md`

## 生产化清单（当前为演示/内网形态）

- [ ] 修改 `core/config.json` 的 `jwt_secret` 与 admin 初始密码
- [ ] HTTPS 目前用内置自签 CA（演示/内网）。对外业务需接入真实 CA
      （改 `core/internal/cert`，让 `IssueLeaf` 出真证书即可，edge 侧协议不变）
- [ ] core 前置 Nginx/Caddy 做 TLS 终结（core 自身只出 HTTP）
- [ ] 计费：`settings` 已有 `cdn.rate_per_mb` 单价键，余额字段已就位，可叠加扣费逻辑
- [ ] 备份：仅 `core/data/edgecdn.db` 一个文件，定时拷贝即可
- [ ] edge 多机：每台一份 `edge/config.json`；站点「不绑定节点」= 广播全部节点

## 常用命令

```powershell
.\start.ps1 -Edge    # 连本地 edge 一起起
.\stop.ps1 -All      # 停 core/edge/测试用 origin
cd web; pnpm dev     # 前端开发模式（:5173，/api 代理到 :8080）
node .tools\smoke.js # 全链路冒烟（需 core+origin(:9000) 在跑）
```
