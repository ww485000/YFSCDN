# AI 导航入口（本地小模型专用）

> 你是要改这个项目的 AI。按下面 4 步走，不要跳步。

## 第 1 步：先读 3 个文件

1. `docs/architecture.md` —— 模块地图（哪个模块管什么）
2. `docs/api-contract.md` —— 所有 HTTP API 和 DTO（改接口先读这里）
3. `docs/ai/SYMPTOMS.md` —— 症状 → 模块 → 文件 对照表（找 bug 直接查表）

## 第 2 步：按"模块 → 文件"定位

模块内结构固定，不用搜：

**core（Go，主控）**
```
core/internal/
├── cfg/         配置加载（改端口、数据库路径看这里）
├── webx/        迷你路由 + JSON 响应 + 中间件（改路由规则/统一响应格式看这里）
├── authx/       JWT 签发/解析 + 密码哈希（改登录机制看这里）
├── storex/      SQLite 连接 + 全部建表 SQL（改数据表结构看这里）
├── app/         ★ 总装配点：创建所有模块、挂所有路由（新增模块必改这里）
├── auth/        登录接口
├── tenant/      租户（多用户）
├── user/        运营端账号
├── site/        站点 ★ 核心业务
├── node/        边缘节点
├── waf/         WAF 规则
├── cert/        证书（自签 CA）
├── usage/       流量计量
├── setting/     平台参数 KV
├── oplog/       操作日志
└── outbox/      事件队列（core → edge 的桥梁）
```
每个业务包固定两个文件：`<name>.go`（模型+存储+业务）+ `handler.go`（HTTP）。

**edge（Go，边缘节点）**
```
edge/internal/
├── cfg/          配置
├── corex/        与 core 通信：注册/长轮询/心跳 + 事件应用
├── driver/       数据面驱动接口
│   ├── driver.go     接口 + SiteSpec（契约 DTO 的 edge 侧副本）
│   ├── gopxy/        纯 Go 反代驱动（默认，无需安装 nginx）
│   └── nginx/        nginx 驱动（edge 自己拉起 nginx 实例）
├── wafx/         WAF 规则执行引擎
└── stats/        流量计数器（心跳数据源）
```

**web（前端，soybean 风格）**
```
web/src/
├── api/       每个后端 API 一个文件，与路由一一对应（http.ts 是统一请求器）
├── views/     每个页面一个目录一个文件
├── stores/    pinia（auth.ts 管登录态）
├── router/    路由 + 守卫（角色隔离在这里）
├── layouts/   主布局（侧边菜单）
└── types/     DTO 类型（与 api-contract.md 对应）
```

## 第 3 步：改代码时遵守的规矩

- 模块之间**禁止** import（core 不 import edge，反之亦然；web 只通过 HTTP 调 core）。
- 改 API → 必须同步 `docs/api-contract.md` + web/src/types。
- 改数据表 → 只改 `core/internal/storex/storex.go` 里的建表 SQL（IF NOT EXISTS 幂等）。
- 新加业务特性 → 复制任意现有 feature 包做模板（如 site/），然后读 `NEW-MODULE.md`。
- Go 模块独立编译：`cd core && go build ./...`；`cd edge && go build ./...`。
- 前端：`cd web && pnpm dev`（开发）/ `pnpm build`（产物 dist 给 core 托管）。

## 第 4 步：验证

```bash
cd core && go vet ./... && go build ./...
cd edge && go vet ./... && go build ./...
# 端到端（Windows）：仓库根目录 .\start.ps1 -Edge 一键拉起 core+edge（托管 web/dist，:8080）
# 本机演示源站：node .tools\origin.js（:9000，测试用，非交付物）
# 停掉：.\stop.ps1 -All
```
