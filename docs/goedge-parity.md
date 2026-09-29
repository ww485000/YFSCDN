# GoEdge 100% 复刻功能矩阵

本文档是 YFSCDN 对 GoEdge 功能的验收清单。目标不是复制 GoEdge 的旧模板，而是在 `core + edge + web` 模块架构下复刻功能能力，前端统一迁移到 SoybeanAdmin 风格。

参考来源：
- GoEdge README 功能列表：多用户、日志审计、集群、HTTP/HTTPS/TCP/UDP、WAF、缓存、DNS 自动解析、多域名、免费证书、IP 黑白名单、访问日志、统计、压缩、Proxy Protocol、本地静态、跳转、路由、重写、访问控制、字符编码、自定义页面、自定义 Header、WebSocket、WebP、FastCGI、请求限制、流量限制。
- GoEdge 源码模块：管理模块权限、API 节点、DNS 服务商/任务、集群区域、用户 AccessKey、财务、套餐、工单、日志清理、安装恢复、UI 配置。
- SoybeanAdmin 方向：Vue3、Vite、TypeScript、Pinia、UnoCSS、权限路由、主题化后台布局。

## 当前结论

YFSCDN 当前已具备可运行的核心 CDN 闭环，但还不是 GoEdge 的 100% 生产功能复刻。已新增 `parity` 主控矩阵：

- 后端接口：`GET /api/v1/admin/goedge-parity`
- 前端入口：`/admin/goedge-parity`
- 代码位置：`core/internal/parity`

## 模块优先级

| 优先级 | 模块 | 目标 |
|---|---|---|
| P0 | ACME 免费证书 | Let's Encrypt/ZeroSSL 账号、订单、HTTP-01、DNS-01、自动续签 |
| P0 | DNS Provider | 已有 Provider 表、CRUD/API、租户隔离、任务推送、线路/权重/代理/同步模式、同步映射表、同步审计视图、批量重试和 Cloudflare/DNSPod/阿里云真实 API 同步；线路自动发现待补 |
| P0 | 集群/区域/API 节点 | 集群、区域、节点安装/升级、API 节点状态 |
| P1 | WAF 策略集 | 全局策略、规则组、CC 高级策略、动作审计 |
| P1 | 计费/套餐 | 套餐、订单、流水、额度扣减、欠费停用联动 |
| P1 | 管理员模块权限 | 模块级权限、按钮级权限、菜单过滤 |
| P2 | AccessKey/OpenAPI | AK/SK 签名、租户开放接口、调用审计 |
| P2 | 工单 | 租户工单、回复、状态流转 |
| P2 | Proxy Protocol | listener 入站解析、回源出站传递、v1/v2 |

## 实施约定

1. 每个 GoEdge 功能必须对应一个独立 YFSCDN 子模块，避免把逻辑塞进 `app.go` 或单个大文件。
2. 新模块至少包含：`Store`、`handler.go`、DTO 类型、web API 文件、页面、文档清单项。
3. 数据库变更只追加到 `core/internal/storex/storex.go`，老迁移不修改。
4. 前端不复制 GoEdge 旧模板；页面按 SoybeanAdmin 的后台信息架构重做。
5. 每完成一项，更新 `core/internal/parity/parity.go` 中状态与说明。
