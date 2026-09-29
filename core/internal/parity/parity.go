// Package parity exposes the GoEdge feature parity matrix.
// It is intentionally static and code-owned so local AI agents can use one
// canonical checklist when adding modules.
package parity

type Status string

const (
	StatusDone    Status = "done"
	StatusPartial Status = "partial"
	StatusMissing Status = "missing"
)

type Item struct {
	Code       string `json:"code"`
	Name       string `json:"name"`
	Category   string `json:"category"`
	Status     Status `json:"status"`
	Module     string `json:"module"`
	AdminPath  string `json:"admin_path"`
	UserPath   string `json:"user_path,omitempty"`
	Notes      string `json:"notes"`
	NextAction string `json:"next_action,omitempty"`
}

type Summary struct {
	Total   int `json:"total"`
	Done    int `json:"done"`
	Partial int `json:"partial"`
	Missing int `json:"missing"`
}

type Matrix struct {
	Summary Summary `json:"summary"`
	Items   []Item  `json:"items"`
}

func MatrixData() Matrix {
	items := []Item{
		{Code: "multi-user", Name: "多用户/多租户", Category: "运营", Status: StatusDone, Module: "tenant,user,auth", AdminPath: "/admin/tenants", UserPath: "/portal/dashboard", Notes: "运营账号与租户账号隔离，业务表按 tenant_id 隔离。"},
		{Code: "audit-log", Name: "日志审计", Category: "运营", Status: StatusDone, Module: "oplog", AdminPath: "/admin/oplogs", Notes: "核心 CRUD 已写入操作日志。"},
		{Code: "cluster-node", Name: "集群/边缘节点", Category: "节点", Status: StatusPartial, Module: "node,outbox,edge", AdminPath: "/admin/nodes", UserPath: "/portal/nodes", Notes: "已支持节点 token、心跳、长轮询、站点下发；缺 GoEdge 的集群区域、API 节点升级与安装向导。", NextAction: "新增 cluster/region/api-node 子模块。"},
		{Code: "http-https", Name: "HTTP/HTTPS 反代", Category: "站点", Status: StatusDone, Module: "site,cert,edge/engine", AdminPath: "/admin/sites", UserPath: "/portal/sites", Notes: "支持 HTTP/HTTPS listener、SNI 证书、回源代理。"},
		{Code: "tcp-udp", Name: "TCP/UDP 转发", Category: "站点", Status: StatusPartial, Module: "contract,edge/engine", AdminPath: "/admin/sites", UserPath: "/portal/sites", Notes: "契约与 edge 引擎有 TCP/UDP 基础；UI 和端到端验证需要补齐。", NextAction: "在 SiteDetailView 增加 TCP/UDP 专属配置视图并补 smoke。"},
		{Code: "waf", Name: "WAF", Category: "安全", Status: StatusPartial, Module: "waf,site,edge/engine", AdminPath: "/admin/waf", UserPath: "/portal/waf", Notes: "已有 IP/UA/path/rate 与站点内规则；缺 GoEdge 完整防火墙策略集、规则组、CC 高级策略。", NextAction: "新增 firewall policy/rule-set 表与页面。"},
		{Code: "cache", Name: "缓存", Category: "性能", Status: StatusDone, Module: "site,edge/engine/cache", AdminPath: "/admin/sites", UserPath: "/portal/sites", Notes: "支持边缘磁盘缓存、缓存键、stale、no-cache headers。"},
		{Code: "dns-auto", Name: "DNS 自动解析", Category: "DNS", Status: StatusPartial, Module: "dns,dnsprovider,task", AdminPath: "/admin/dns-syncs", UserPath: "/portal/dns-syncs", Notes: "已有 DNS 记录 CRUD、服务商配置、租户隔离、同步映射表、任务处理器、同步审计/批量重试，以及 Cloudflare/DNSPod/阿里云真实 API 同步。", NextAction: "补同步日志明细和更多线路/权重字段。"},
		{Code: "multi-domain", Name: "多域名绑定", Category: "站点", Status: StatusDone, Module: "site,contract", AdminPath: "/admin/sites", UserPath: "/portal/sites", Notes: "server_names 支持多个域名与默认域名。"},
		{Code: "free-cert", Name: "免费证书申请", Category: "证书", Status: StatusPartial, Module: "cert,task", AdminPath: "/admin/certs", UserPath: "/portal/certs", Notes: "当前为平台 CA 和上传证书；ACME/Let's Encrypt 待接入。", NextAction: "新增 acme account/order/challenge 模块与 DNS-01/HTTP-01。"},
		{Code: "ip-access", Name: "IP 黑白名单/访问控制", Category: "安全", Status: StatusDone, Module: "site,edge/engine/access", AdminPath: "/admin/sites", UserPath: "/portal/sites", Notes: "支持 IP、Referer、User-Agent allow/deny。"},
		{Code: "access-log", Name: "访问日志", Category: "日志", Status: StatusDone, Module: "logs,site,edge/engine", AdminPath: "/admin/sites", UserPath: "/portal/sites", Notes: "edge 上报访问日志，core 可分页查询。"},
		{Code: "stats", Name: "统计", Category: "统计", Status: StatusDone, Module: "usage,dashboard", AdminPath: "/admin/dashboard", UserPath: "/portal/dashboard", Notes: "请求、流量、缓存命中统计已按天聚合。"},
		{Code: "compression", Name: "内容压缩", Category: "性能", Status: StatusDone, Module: "site,edge/engine/compress", AdminPath: "/admin/sites", UserPath: "/portal/sites", Notes: "支持 gzip/deflate/brotli 配置字段和 gzip/deflate 处理。"},
		{Code: "proxy-protocol", Name: "Proxy Protocol", Category: "网络", Status: StatusMissing, Module: "edge/engine", AdminPath: "/admin/sites", Notes: "尚未实现 PROXY protocol v1/v2 解析与传递。", NextAction: "在 listener/origin 增加 proxy_protocol 配置并实现解析。"},
		{Code: "local-static", Name: "本地静态文件", Category: "站点", Status: StatusDone, Module: "site,edge/engine", AdminPath: "/admin/sites", UserPath: "/portal/sites", Notes: "location.root/index 支持本地静态目录。"},
		{Code: "redirect", Name: "URL 跳转", Category: "站点", Status: StatusDone, Module: "site,edge/engine/rewrite", AdminPath: "/admin/sites", UserPath: "/portal/sites", Notes: "return 规则支持跳转状态码和地址。"},
		{Code: "route", Name: "路由规则/Location", Category: "站点", Status: StatusDone, Module: "site,contract", AdminPath: "/admin/sites", UserPath: "/portal/sites", Notes: "支持 exact/prefix/regex locations。"},
		{Code: "rewrite", Name: "重写规则", Category: "站点", Status: StatusDone, Module: "site,edge/engine/rewrite", AdminPath: "/admin/sites", UserPath: "/portal/sites", Notes: "支持条件过滤与 URI rewrite。"},
		{Code: "charset", Name: "字符编码", Category: "性能", Status: StatusDone, Module: "site,edge/engine/charsetx", AdminPath: "/admin/sites", UserPath: "/portal/sites", Notes: "支持 charset 配置与响应转换。"},
		{Code: "pages", Name: "自定义页面", Category: "站点", Status: StatusDone, Module: "site,edge/engine/pages", AdminPath: "/admin/sites", UserPath: "/portal/sites", Notes: "支持错误页面和页面压缩配置。"},
		{Code: "headers", Name: "自定义 HTTP Header", Category: "站点", Status: StatusDone, Module: "site,edge/engine/headers", AdminPath: "/admin/sites", UserPath: "/portal/sites", Notes: "支持 upstream/downstream header 操作与 CORS。"},
		{Code: "websocket", Name: "WebSocket", Category: "网络", Status: StatusDone, Module: "site,edge/engine", AdminPath: "/admin/sites", UserPath: "/portal/sites", Notes: "支持 WebSocket 升级透传。"},
		{Code: "webp", Name: "WebP 自动转换", Category: "性能", Status: StatusDone, Module: "site,edge/engine/webp", AdminPath: "/admin/sites", UserPath: "/portal/sites", Notes: "支持 JPEG/PNG 转 WebP。"},
		{Code: "fastcgi", Name: "FastCGI", Category: "站点", Status: StatusDone, Module: "site,edge/engine/fastcgi", AdminPath: "/admin/sites", UserPath: "/portal/sites", Notes: "支持 FastCGI 后端配置。"},
		{Code: "request-limit", Name: "请求限制", Category: "安全", Status: StatusDone, Module: "site,edge/engine/ratelimit", AdminPath: "/admin/sites", UserPath: "/portal/sites", Notes: "支持按客户端 IP 的请求频率限制。"},
		{Code: "traffic-limit", Name: "流量限制", Category: "运营", Status: StatusPartial, Module: "site,usage,edge/engine", AdminPath: "/admin/sites", UserPath: "/portal/sites", Notes: "站点配置支持限制；租户额度扣费/停用联动待补。", NextAction: "新增 billing/quota worker。"},
		{Code: "finance-plan", Name: "套餐/财务/计费", Category: "运营", Status: StatusMissing, Module: "billing,plan", AdminPath: "/admin", Notes: "GoEdge 有 finance/plan 模块；当前只有余额字段与单价设置。", NextAction: "新增 plan、order、transaction、invoice 表与页面。"},
		{Code: "ticket", Name: "工单", Category: "运营", Status: StatusMissing, Module: "ticket", AdminPath: "/admin", UserPath: "/portal", Notes: "GoEdge 菜单包含 ticket；当前未实现。", NextAction: "新增 ticket CRUD、回复、状态流转。"},
		{Code: "access-key", Name: "用户 AccessKey/API", Category: "开放接口", Status: StatusMissing, Module: "accesskey,openapi", AdminPath: "/admin", UserPath: "/portal", Notes: "GoEdge 支持用户 AccessKey；当前仅 JWT 控制台接口。", NextAction: "新增 AK/SK 签名与 OpenAPI scope。"},
		{Code: "admin-module", Name: "管理员模块权限", Category: "权限", Status: StatusPartial, Module: "auth,user", AdminPath: "/admin/admins", Notes: "当前只有 superadmin/operator 粗粒度角色；缺模块级授权。", NextAction: "新增 admin_modules/admin_permissions 并接入路由守卫。"},
	}
	s := Summary{Total: len(items)}
	for _, item := range items {
		switch item.Status {
		case StatusDone:
			s.Done++
		case StatusPartial:
			s.Partial++
		default:
			s.Missing++
		}
	}
	return Matrix{Summary: s, Items: items}
}
