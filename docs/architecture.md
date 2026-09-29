# EdgeCDN 架构总览

> 一个模块化、多租户、可运营的轻量 CDN 系统。设计灵感来自 GoEdge，
> 但每个模块都是**独立 Go module / 独立前端工程**，模块之间只通过
> `docs/api-contract.md` 定义的 HTTP/JSON 契约通信，禁止跨模块 import。

## 设计原则（为本地 27B 小模型 AI 优化）

1. **一个目录 = 一个模块**：模块有自己的 `go.mod` / `package.json`，可单独编译、单独部署、单独删掉不影响其他模块。
2. **模块内四件套**：每个业务特性（feature）固定 2 个文件 —— `<feature>.go`（模型 + 存储 + 业务逻辑）和 `handler.go`（HTTP 路由处理）。改任何业务只进这两个文件。
3. **契约唯一**：跨模块通信一律走 `docs/api-contract.md`。DTO 在 core 和 edge 各自复制一份（有意为之，保持模块独立可编译）。
4. **依赖极简**：core 只依赖 `modernc.org/sqlite`（纯 Go 无 CGO）和 `golang.org/x/crypto`（bcrypt）。JWT、路由、CA 证书全部用标准库自实现，文件少且可读。
5. **单文件 ≤ 300 行**：超过就拆。文件名即功能名。
6. **装配点唯一**：core 的 `internal/app/app.go` 是"总装车间"——所有模块在这里被创建并挂路由。新增模块只需在 app.go 加 3~5 行。

## 模块地图

```
E:\WORK\9\
├── core/          ① 主控模块（Go）控制面：多租户账号、站点、节点调度、WAF、证书、计量计费
├── edge/          ② 边缘节点模块（Go）数据面：拉取配置、反代回源、缓存、WAF 执行
├── web/           ③ 前端模块（soybean 风格：Vue3 + Vite + TS + Pinia + UnoCSS + antd）
│                  一个应用两个角色：运营后台 /admin/* 与租户门户 /portal/*
├── docs/          架构 + AI 导航 + API 契约
├── start.ps1      一键构建并启动 core(+edge)（Windows；日志在 .tools\*.log）
└── stop.ps1       停止 core/edge（-All 连演示源站一起停）
```

## 运行时拓扑

```
                ┌──────────────┐  /admin/*  ┌──────────────┐
                │  web 前端     │◄──────────►│              │
   (Vite dev /  │  (两个角色)   │  /portal/* │              │
    Nginx 静态)  └──────────────┘            │   core 主控   │
                                             │   :8080      │
                    ┌────────────────────────┤              │
                    │ 长轮询 /api/v1/edge/poll│  SQLite      │
                    ▼                        │  (WAL)       │
             ┌──────────────┐   ┌──────────────┐            │
             │ edge 节点 A   │   │ edge 节点 B   │            │
             │ gopxy/nginx  │   │ gopxy/nginx  │            │
             └──────┬───────┘   └──────┬───────┘            │
                    │ 回源              │                    │
                    ▼                  ▼                    │
              ┌──────────────────────────────────────────────┘
              │  源站 (任意 HTTP 服务)
```

- **core**：唯一写 SQLite 的进程。收到 admin/user 的变更 → 写库 → 往 `outbox` 追加事件。
- **edge**：无状态（除本地缓存盘）。30s 长轮询拉增量事件（`site_upsert` / `site_delete`），
  通过 Driver 应用到数据面（gopxy 纯 Go 反代 或 nginx）；每 30s 心跳上报流量。
- **web**：纯静态。Vite 开发代理 `/api` → core；生产由 core 的 `-static` 目录直接托管。

## 数据流（一次"添加站点"）

1. 租户在 web 门户 POST `/api/v1/user/sites`（JWT 里带 tenant_id）。
2. `core/internal/site` 校验配额 → 写库 → 若开 HTTPS 由 `cert` 签发自签叶子证书 → 组装 `SiteSpec`。
3. `outbox` 为站点绑定的每个节点追加事件（version 单调递增）。
4. edge 长轮询拿到事件 → Driver 应用（gopxy 内存路由表 + TLS 证书池；nginx 生成 conf 并 reload）。
5. 客户端请求 `https://www.example.com/` → edge 命中站点 → WAF 引擎判定 → 磁盘缓存/回源 → 统计计数。
6. 下次心跳上报 → core 按 (租户, 天) 聚合进 `usage_daily`（计量计费数据源）。

## 多租户与运营

- **角色**：`superadmin`（平台超管）/ `operator`（运营）/ `tenant`（租户用户）。
- **隔离**：所有业务表带 `tenant_id`；user 作用域 API 只用 JWT 里的 tenant_id 过滤，
  运营 API 可跨租户（带 tenant_id 参数）。
- **运营能力**：租户 CRUD（配额 max_sites、流量额度）、虚拟余额、每日流量计量、
  操作日志（oplog）、平台参数（settings KV，含单价 `cdn.rate_per_mb`）。

## 模块新增规则（详见 docs/ai/NEW-MODULE.md）

新功能 = 新模块目录（core 内 feature 或全新子模块），必须：
① 独立可编译 ② 契约写入 api-contract.md ③ 在 docs/ai/SYMPTOMS.md 登记 ④ 在 app.go 装配。
