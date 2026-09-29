# 新增模块清单（AI 照抄执行）

> "加新功能 = 加新模块"。分三种粒度，按小到大。

## 粒度 1：core 内新业务特性（最常见）

例：加"回源 IP 池"功能。

1. 复制模板：把 `core/internal/waf/` 整个目录复制为 `core/internal/<name>/`，
   把两个文件改名 `<name>.go` + `handler.go`，包名改成 `<name>`。
2. `<name>.go`：写模型 struct、Store（CRUD，用 `*sql.DB`）、Service（业务规则）。
3. 建表 SQL 追加到 `core/internal/storex/storex.go` 的 `Migrations` 末尾
   （`CREATE TABLE IF NOT EXISTS ...`，幂等）。
4. `handler.go`：注册路由到传入的 `*webx.Router`。
5. 装配：`core/internal/app/app.go` 里 ① 创建 Store/Service ② 调 RegisterRoutes
   ③ 若需跨模块调用，把依赖 Store 传进构造器。
6. 前端：`web/src/api/<name>.ts` + `web/src/views/<name>/`，在
   `web/src/router/index.ts` 加路由、`MainLayout.vue` 加菜单项。
7. 文档：`docs/api-contract.md` 加接口行；`docs/ai/SYMPTOMS.md` 加一行症状。
8. 验证：`cd core && go vet ./... && go build ./...`

## 粒度 2：core → edge 的新事件类型

例：让 edge 支持"回源 IP 池"。

1. `docs/api-contract.md` 先加字段/事件定义（契约先行！）。
2. core：事件 payload 组装在对应 feature 的 Service 里（走 outbox.Append）。
   优先复用 `site_upsert`（全量规格）——**能塞进 SiteSpec 就不要新增事件类型**。
3. edge：`edge/internal/driver/driver.go` 的 SiteSpec 同步加字段；
   驱动内消费该字段（gopxy 必改，nginx 若无关可不动，注释说明）。
4. 验证：两个模块分别 go build。

## 粒度 3：全新子模块（独立部署的进程）

例：加"日志采集模块 loger"。

```
loger/
├── go.mod            module edgecdn/loger
├── cmd/loger/main.go
└── internal/...      （cfg / api / 业务，同 core 的风格）
```

1. `go mod init edgecdn/loger`，独立依赖。
2. 只允许通过 `docs/api-contract.md` 的 HTTP API 与 core 通信
   （新增 /api/v1/xxx 段，契约先行）。
3. 在 `docs/architecture.md` 模块地图加一行；`docs/ai/NAVIGATION.md` 加一节结构图；
   `docs/ai/SYMPTOMS.md` 加症状行。
4. 验证：`cd loger && go vet ./... && go build ./...`。
5. 删除即下线：停掉进程，其他模块零改动（这就是模块化的意义）。

## 红线（违反会让 27B 小模型迷路）

- ❌ 跨模块 Go import（core↔edge↔loger 之间）
- ❌ 单文件 > 300 行（拆）
- ❌ 新增事件类型却不更新 api-contract.md
- ❌ 在 web 里手写 axios/跨域（统一走 src/api/http.ts）
