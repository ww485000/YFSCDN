// Package contract is the canonical site-configuration schema shared between
// core (control plane) and edge (data plane). It mirrors the functional scope of
// GoEdge (EdgeCommon pkg/serverconfigs): every field here has a working
// counterpart in the edge engine, and the admin UI form mirrors the same fields.
//
// NOTE: this file is duplicated verbatim at edge/internal/contract/contract.go
// (same module boundary, independent builds). Keep both copies in sync.
package contract

import (
	"fmt"
	"strings"
)

// SchemaVersion of the site config document.
const SchemaVersion = 2

// ---------------------------------------------------------------------------
// Top level
// ---------------------------------------------------------------------------

// SiteConfig is the full configuration of one CDN site (GoEdge "server").
type SiteConfig struct {
	// Meta — managed by core; the edge ignores everything it does not implement.
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Status   string `json:"status"` // "normal" | "off"
	Version  int    `json:"version"`

	// Server names (domains). Exactly one should have IsDefault=true.
	ServerNames []ServerName `json:"server_names"`

	// Protocol listeners (http/https/tcp/udp).
	Listeners []Listener `json:"listeners"`

	// TLS material for the https listener. Core injects certs into Spec.Certs;
	// TLS.Cert/Key may also be set inline (inline wins when non-empty).
	TLS *TLSConfig `json:"tls,omitempty"`

	// Location is the default location (server-level feature stack, nginx "server" block).
	Location *Location `json:"location"`

	// Locations are additional path-based locations, evaluated in order;
	// the first match wins, otherwise Location (default) applies.
	Locations []NamedLocation `json:"locations,omitempty"`
}

// ServerName is one bound domain.
type ServerName struct {
	Name      string `json:"name"`
	IsDefault bool   `json:"is_default"`
	Status    string `json:"status"` // "normal" | "off"
	// RedirectTo: 301 this host to another host (GoEdge host redirect), optional.
	RedirectTo string `json:"redirect_to,omitempty"`
}

// Listener is one protocol endpoint the site listens on.
type Listener struct {
	Protocol string `json:"protocol"` // "http" | "https" | "tcp" | "udp"
	Listen   string `json:"listen"`   // ":80", "0.0.0.0:8443", ":2222"

	// SNI override for https (defaults to all server names).
	SNI []string `json:"sni,omitempty"`

	// FollowProtocol: plain-http listener auto-301s to the site's https listener.
	FollowProtocol bool `json:"follow_protocol,omitempty"`

	// Origins + Scheduling: for tcp/udp raw forwarding (GoEdge tcp/udp reverse proxy).
	Origins    []Origin    `json:"origins,omitempty"`
	Scheduling *Scheduling `json:"scheduling,omitempty"`
}

// TLSConfig: certificate + TLS hardening for the https listener.
type TLSConfig struct {
	Cert string `json:"cert,omitempty"` // full chain PEM (may be empty => core-provided)
	Key  string `json:"key,omitempty"`  // private key PEM
	// MinVersion "1.2" (default) | "1.3"
	MinVersion string `json:"min_version,omitempty"`
	HSTS       *HSTS  `json:"hsts,omitempty"`
}

// HSTS — Strict-Transport-Security header.
type HSTS struct {
	Enabled           bool `json:"enabled"`
	MaxAge            int  `json:"max_age"` // seconds
	IncludeSubDomains bool `json:"include_sub_domains"`
	Preload           bool `json:"preload"`
}

// ---------------------------------------------------------------------------
// Location (feature stack; used for default + named locations)
// ---------------------------------------------------------------------------

// Location is the nginx-style "location" block: the full per-request feature stack.
type Location struct {
	// --- upstream / content source (exactly one of the three) ---
	// ReverseProxy: forward to an origin server group.
	ReverseProxy *ReverseProxy `json:"reverse_proxy,omitempty"`
	// Root/Index: serve local static files from the node's disk.
	Root  string   `json:"root,omitempty"`
	Index []string `json:"index,omitempty"`
	// FastCGI: PHP-style backend.
	FastCGI *FastCGI `json:"fastcgi,omitempty"`

	// --- request shaping (applied in order) ---
	// Rewrites run first (redirect or URL rewrite).
	Rewrites []Rewrite `json:"rewrites,omitempty"`
	// RedirectHTTPS: 301/302 http->https for this location.
	RedirectHTTPS *RedirectHTTPS `json:"redirect_https,omitempty"`

	// --- response shaping ---
	Headers       *Headers       `json:"headers,omitempty"`
	Compression   *Compression   `json:"compression,omitempty"`
	WebP          *WebP          `json:"webp,omitempty"`
	Charset       *Charset       `json:"charset,omitempty"`
	Expires       *Expires       `json:"expires,omitempty"`
	Pages         *Pages         `json:"pages,omitempty"`
	Cache         *Cache         `json:"cache,omitempty"`
	Websocket     *Websocket     `json:"websocket,omitempty"`

	// --- access control & protection ---
	Access       *Access       `json:"access,omitempty"`
	Auth         *Auth         `json:"auth,omitempty"`
	RequestLimit *RequestLimit `json:"request_limit,omitempty"`
	TrafficLimit *TrafficLimit `json:"traffic_limit,omitempty"`
	WAF          *WAF          `json:"waf,omitempty"`

	// --- observability ---
	AccessLog *AccessLog `json:"access_log,omitempty"`
	Stat      *Stat      `json:"stat,omitempty"`
}

// NamedLocation is a path-patterned location with its own feature stack.
type NamedLocation struct {
	Name    string   `json:"name"`
	Pattern *Pattern `json:"pattern"`
	// Embedded: all Location fields live at this level.
	*Location
}

// Pattern matches a request path (nginx location semantics).
type Pattern struct {
	Type  string `json:"type"`  // "exact" | "prefix" | "regex"
	Value string `json:"value"`
}

// ---------------------------------------------------------------------------
// Origins / scheduling / health
// ---------------------------------------------------------------------------

// ReverseProxy is the upstream configuration (GoEdge http reverse proxy).
type ReverseProxy struct {
	Origins     []Origin     `json:"origins"`
	Scheduling  *Scheduling  `json:"scheduling,omitempty"`
	HealthCheck *HealthCheck `json:"health_check,omitempty"`
	// Custom request headers applied to origin requests.
	Headers *Headers `json:"headers,omitempty"`
	// TimeoutSec: origin dial+response timeout (0 = engine default 30s).
	TimeoutSec int `json:"timeout_sec,omitempty"`
}

// Origin is one upstream server.
type Origin struct {
	Name string `json:"name"`
	Addr string `json:"addr"` // "host:port"
	// Status "normal" (default) | "off"
	Status string `json:"status"`
	// SSL: use TLS to the origin.
	SSL      bool   `json:"ssl"`
	SNI      string `json:"sni,omitempty"`
	Insecure bool   `json:"insecure,omitempty"`
}

// Scheduling picks the origin (GoEdge scheduling: random/hash/round_robin/sticky).
type Scheduling struct {
	Type string `json:"type"` // "random" | "round_robin" | "hash" | "sticky" (default random)
	// Hash key source for type=hash: "ip" (default) | "cookie:<name>" | "header:<name>"
	Key string `json:"key,omitempty"`
	// TTL seconds for type=sticky (default 300).
	TTL int `json:"ttl,omitempty"`
}

// HealthCheck is the origin active probe (GoEdge health check).
type HealthCheck struct {
	Enabled  bool   `json:"enabled"`
	Interval int    `json:"interval"` // seconds between probes (default 10)
	Timeout  int    `json:"timeout"`  // probe timeout seconds (default 3)
	Scheme   string `json:"scheme"`   // http | https (default http)
	Path     string `json:"path"`     // default "/"
	Host     string `json:"host"`     // Host header override (default origin host)
	// Failures: consecutive failures before marking origin down (default 3).
	Failures int `json:"failures"`
	// Successes: consecutive successes before marking origin up (default 1).
	Successes int `json:"successes"`
}

// ---------------------------------------------------------------------------
// Rewrites / redirects / conditions
// ---------------------------------------------------------------------------

// Rewrite is one rewrite/redirect rule, executed in order (GoEdge rewrite).
type Rewrite struct {
	// Type "rewrite": replace the request URI with Value (may contain filters).
	// Type "return": answer immediately with Status + Value (301/302/307/308/200/403...).
	Type   string `json:"type"` // "rewrite" | "return"
	Value  string `json:"value"`
	Status int    `json:"status,omitempty"`
	// Conditions: all must match for the rule to apply (empty = always).
	Conditions []Cond `json:"conditions,omitempty"`
}

// Cond is one request-condition check.
type Cond struct {
	// Target: "path" | "query" | "host" | "method" | "user_agent" | "referer" | "ip" |
	//        "header:<Name>" | "cookie:<Name>" | "uri" (path+query)
	Target   string `json:"target"`
	Operator string `json:"operator"` // equals|not_equals|contains|not_contains|starts_with|ends_with|regex
	Value    string `json:"value"`
}

// RedirectHTTPS forces https.
type RedirectHTTPS struct {
	Enabled bool `json:"enabled"`
	// Status 301 (default) | 302
	Status int `json:"status,omitempty"`
}

// ---------------------------------------------------------------------------
// Headers / CORS
// ---------------------------------------------------------------------------

// Headers controls request (upstream) and response (downstream) headers.
type Headers struct {
	Upstream   []HeaderOp `json:"upstream,omitempty"`
	Downstream []HeaderOp `json:"downstream,omitempty"`
	CORS       *CORS      `json:"cors,omitempty"`
}

// HeaderOp is one header manipulation.
type HeaderOp struct {
	Op    string `json:"op"` // "add" | "set" | "delete" | "replace"
	Name  string `json:"name"`
	Value string `json:"value"`
}

// CORS adds Access-Control-* headers.
type CORS struct {
	Enabled      bool     `json:"enabled"`
	AllowOrigins []string `json:"allow_origins"` // ["*"] or explicit list
	AllowMethods []string `json:"allow_methods"`
	AllowHeaders []string `json:"allow_headers"`
	MaxAge       int      `json:"max_age"`
}

// ---------------------------------------------------------------------------
// Performance: compression / webp / charset / expires
// ---------------------------------------------------------------------------

// Compression: gzip/deflate/brotli on the fly.
type Compression struct {
	Enabled   bool     `json:"enabled"`
	Types     []string `json:"types"`    // subset of ["gzip","deflate","brotli"]
	MinSize   int      `json:"min_size"` // bytes; skip below (default 1024)
	MimeTypes []string `json:"mime_types"` // compress only these (empty = all compressible)
	Level     int      `json:"level"`    // gzip level (default 6, -1 = fast default)
}

// WebP converts JPEG/PNG responses to WebP (GoEdge webp auto conversion).
type WebP struct {
	Enabled    bool     `json:"enabled"`
	Quality    int      `json:"quality"`    // 1..100 (default 80)
	MimeTypes  []string `json:"mime_types"` // ["image/jpeg","image/png"]
	// Only convert when client Accept includes image/webp (always true in practice).
}

// Charset converts response encoding (GoEdge charset, e.g. GBK -> UTF-8).
type Charset struct {
	Enabled  bool   `json:"enabled"`
	Encoding string `json:"encoding"` // target charset, e.g. "utf-8"
	// ConvertFrom: source charset to convert FROM (empty = auto from Content-Type)
	ConvertFrom string `json:"convert_from,omitempty"`
}

// Expires sets the Cache-Control/Expires freshness (GoEdge expires).
type Expires struct {
	Enabled bool   `json:"enabled"`
	// Time: "30d", "2h10m", "1w" ...
	Time string `json:"time"`
}

// ---------------------------------------------------------------------------
// Caching
// ---------------------------------------------------------------------------

// Cache is the edge-side response cache (GoEdge cache).
type Cache struct {
	Enabled bool `json:"enabled"`
	// Storage "file" (default) | "memory"
	Storage string `json:"storage"`
	// MaxSize: "10GB", "512MB" (file default 10GB, memory default 256MB)
	MaxSize string `json:"max_size"`
	// TTL seconds: default lifetime when upstream gives no freshness (default 300)
	TTL int `json:"ttl"`
	// IgnoreCacheControl: ignore upstream Cache-Control/Expires, use TTL only.
	IgnoreCacheControl bool `json:"ignore_cache_control"`
	// OnlyGet: cache GET only (default true)
	OnlyGet bool `json:"only_get"`
	Key    *CacheKey `json:"key,omitempty"`
	Stale  *Stale    `json:"stale,omitempty"`
	// NoCacheHeaders: response headers that disallow caching when present
	// (default: Set-Cookie)
	NoCacheHeaders []string `json:"no_cache_headers,omitempty"`
}

// CacheKey customizes the cache key composition.
type CacheKey struct {
	// IncludeQuery: specific query params to include (empty = all params)
	IncludeQuery []string `json:"include_query"`
	// IgnoreQuery: query params to drop from the key
	IgnoreQuery []string `json:"ignore_query"`
	// IncludeHeaders: extra request headers folded into the key
	IncludeHeaders []string `json:"include_headers"`
}

// Stale serves expired entries when the origin fails (stale-while-error).
type Stale struct {
	Enabled bool `json:"enabled"`
	TTL     int  `json:"ttl"` // how long after expiry stale serving is allowed (default 86400)
}

// ---------------------------------------------------------------------------
// Access control
// ---------------------------------------------------------------------------

// Access = IP + Referer + UserAgent allow/deny (GoEdge access control).
type Access struct {
	IPs        *IPList    `json:"ips,omitempty"`
	Referers   *Patterns  `json:"referers,omitempty"`
	UserAgents *Patterns  `json:"user_agents,omitempty"`
}

// IPList is an IP allow/deny list (CIDR or single IPs, one per entry).
type IPList struct {
	Allow []string `json:"allow"`
	Deny  []string `json:"deny"`
}

// Patterns is a regex/glob allow/deny list against a request field.
type Patterns struct {
	// Empty Allow = allow everything (subject to Deny).
	Allow []string `json:"allow"`
	Deny  []string `json:"deny"`
}

// ---------------------------------------------------------------------------
// Authentication
// ---------------------------------------------------------------------------

// Auth is HTTP authentication (GoEdge http auth).
type Auth struct {
	Enabled    bool            `json:"enabled"`
	Realm      string          `json:"realm"`
	Basic      *BasicAuth      `json:"basic,omitempty"`
	SubRequest *SubRequestAuth `json:"sub_request,omitempty"`
}

// BasicAuth: username/password list. Password may be "plain" text or "sha1:<hex>".
type BasicAuth struct {
	Users []BasicUser `json:"users"`
}

type BasicUser struct {
	User     string `json:"user"`
	Password string `json:"password"`
}

// SubRequestAuth: ask an external endpoint (2xx = allow).
type SubRequestAuth struct {
	URL     string `json:"url"`
	Timeout int    `json:"timeout"` // seconds
}

// ---------------------------------------------------------------------------
// Limits
// ---------------------------------------------------------------------------

// RequestLimit = CC protection: max requests per interval per client IP (GoEdge request limit).
type RequestLimit struct {
	Enabled  bool   `json:"enabled"`
	Interval int    `json:"interval"` // window seconds (default 60)
	Limit    int    `json:"limit"`    // max requests per window (default 100)
	Action   string `json:"action"`   // "block" (429) | "log"
}

// TrafficLimit caps site traffic/connections (GoEdge traffic limit).
type TrafficLimit struct {
	Enabled  bool   `json:"enabled"`
	// MaxBytes: total bytes this site may serve (e.g. "100GB"); 0/empty = unlimited.
	MaxBytes string `json:"max_bytes"`
	// MaxConn: max concurrent connections; 0 = unlimited.
	MaxConn int `json:"max_conn"`
}

// ---------------------------------------------------------------------------
// WAF
// ---------------------------------------------------------------------------

// WAF is the site-level WAF (GoEdge WAF): IP lists + rule engine.
type WAF struct {
	Enabled bool    `json:"enabled"`
	IPs     *IPList `json:"ips,omitempty"`
	Rules   []WAFRule `json:"rules,omitempty"`
}

// WAFRule is one WAF rule (GoEdge WAF rule).
type WAFRule struct {
	ID     int64  `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"` // "normal" | "off"
	// Target: "url" | "query" | "body" | "ip" | "user_agent" | "referer" |
	//         "header:<Name>" | "cookie:<Name>" | "method" | "host"
	Target   string `json:"target"`
	Operator string `json:"operator"` // equals|not_equals|contains|not_contains|starts_with|ends_with|regex
	Value    string `json:"value"`
	// Action: "block" (403) | "captcha" | "js" (JS challenge) | "log"
	Action string `json:"action"`
	// Level: severity 1..5 (informational, used for sorting/UI)
	Level int `json:"level"`
}

// ---------------------------------------------------------------------------
// Pages / minify
// ---------------------------------------------------------------------------

// Pages = custom status pages + optional minify (GoEdge custom pages).
type Pages struct {
	Codes  []ErrorPage `json:"codes"`
	Minify *Minify     `json:"minify,omitempty"`
}

// ErrorPage serves a custom body for a status code.
type ErrorPage struct {
	Status      int    `json:"status"`
	Enabled     bool   `json:"enabled"`
	Title       string `json:"title"`
	Content     string `json:"content"` // HTML body
	ContentType string `json:"content_type"` // default "text/html; charset=utf-8"
}

// Minify strips comments/whitespace (GoEdge page optimization).
type Minify struct {
	HTML bool `json:"html"`
	CSS  bool `json:"css"`
	JS   bool `json:"js"`
}

// ---------------------------------------------------------------------------
// Websocket / FastCGI
// ---------------------------------------------------------------------------

// Websocket enables WebSocket upgrade passthrough (GoEdge websocket).
type Websocket struct {
	Enabled bool `json:"enabled"`
	// Paths: optional prefix list; empty = all paths.
	Paths []string `json:"paths,omitempty"`
}

// FastCGI backend (GoEdge fastcgi).
type FastCGI struct {
	Addr  string `json:"addr"` // host:port
	Index string `json:"index"` // default "index.php"
	// Params: extra FastCGI params (REQUEST_METHOD etc. are auto-set)
	Params map[string]string `json:"params,omitempty"`
}

// ---------------------------------------------------------------------------
// Observability
// ---------------------------------------------------------------------------

// AccessLog configures edge access logging (GoEdge access log).
type AccessLog struct {
	Enabled bool   `json:"enabled"`
	Format  string `json:"format"` // "combined" (default) | "json"
	// LocalFile: also write to a local file on the node (empty = only report to core)
	LocalFile string `json:"local_file,omitempty"`
}

// Stat configures metrics collection.
type Stat struct {
	Enabled bool `json:"enabled"`
}

// ---------------------------------------------------------------------------
// Spec envelope (core -> edge wire format)
// ---------------------------------------------------------------------------

// Spec is the full payload of a site_upsert event.
type Spec struct {
	// Site: the config document (certs usually injected via Certs below).
	Site SiteConfig `json:"site"`
	// Certs: core-issued / uploaded certs, one per domain (wildcard domain "*").
	Certs []DomainCert `json:"certs,omitempty"`
	// GlobalRules: tenant-global WAF rules merged on top of site rules.
	GlobalRules []WAFRule `json:"global_rules,omitempty"`
	// GlobalWAFEnabled: whether tenant global rules are active at all.
	GlobalWAFEnabled bool `json:"global_waf_enabled,omitempty"`
}

// DomainCert is one certificate attached to the spec.
type DomainCert struct {
	Domain    string `json:"domain"`
	Cert      string `json:"cert"` // full chain PEM
	Key       string `json:"key"`  // private key PEM
	ExpiresAt string `json:"expires_at"`
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// PrimaryDomain returns the default server name (or the first one).
func (s *SiteConfig) PrimaryDomain() string {
	for _, sn := range s.ServerNames {
		if sn.IsDefault {
			return sn.Name
		}
	}
	if len(s.ServerNames) > 0 {
		return s.ServerNames[0].Name
	}
	return ""
}

// Domains lists all active server names.
func (s *SiteConfig) Domains() []string {
	out := []string{}
	for _, sn := range s.ServerNames {
		if sn.Status != "off" {
			out = append(out, sn.Name)
		}
	}
	return out
}

// EnsureDefaults fills missing fields with safe defaults (idempotent).
func (s *SiteConfig) EnsureDefaults() {
	if s.Version == 0 {
		s.Version = SchemaVersion
	}
	if s.Status == "" {
		s.Status = "normal"
	}
	if len(s.ServerNames) > 0 {
		hasDefault := false
		for i := range s.ServerNames {
			if s.ServerNames[i].Status == "" {
				s.ServerNames[i].Status = "normal"
			}
			if s.ServerNames[i].IsDefault {
				hasDefault = true
			}
		}
		if !hasDefault {
			s.ServerNames[0].IsDefault = true
		}
	}
	for i := range s.Listeners {
		ln := &s.Listeners[i]
		if ln.Protocol == "" {
			ln.Protocol = "http"
		}
		if ln.Listen == "" {
			switch ln.Protocol {
			case "https":
				ln.Listen = ":443"
			case "tcp":
				ln.Listen = ":9000"
			case "udp":
				ln.Listen = ":9000"
			default:
				ln.Listen = ":80"
			}
		}
		if (ln.Protocol == "tcp" || ln.Protocol == "udp") && ln.Scheduling != nil && ln.Scheduling.Type == "" {
			ln.Scheduling.Type = "random"
		}
	}
	if s.Location == nil {
		s.Location = &Location{}
	}
	loc := s.Location
	if loc.ReverseProxy != nil {
		for i := range loc.ReverseProxy.Origins {
			if loc.ReverseProxy.Origins[i].Status == "" {
				loc.ReverseProxy.Origins[i].Status = "normal"
			}
		}
		if loc.ReverseProxy.Scheduling != nil && loc.ReverseProxy.Scheduling.Type == "" {
			loc.ReverseProxy.Scheduling.Type = "random"
		}
		if hc := loc.ReverseProxy.HealthCheck; hc != nil && hc.Enabled {
			if hc.Interval <= 0 {
				hc.Interval = 10
			}
			if hc.Timeout <= 0 {
				hc.Timeout = 3
			}
			if hc.Scheme == "" {
				hc.Scheme = "http"
			}
			if hc.Path == "" {
				hc.Path = "/"
			}
			if hc.Failures <= 0 {
				hc.Failures = 3
			}
			if hc.Successes <= 0 {
				hc.Successes = 1
			}
		}
	}
	if loc.Cache != nil {
		c := loc.Cache
		if c.Storage == "" {
			c.Storage = "file"
		}
		if c.TTL <= 0 {
			c.TTL = 300
		}
		if c.OnlyGet == false {
			// default true; only meaningful when explicitly set false
		}
		if c.Stale != nil && c.Stale.Enabled && c.Stale.TTL <= 0 {
			c.Stale.TTL = 86400
		}
	}
	if loc.Compression != nil && len(loc.Compression.Types) == 0 && loc.Compression.Enabled {
		loc.Compression.Types = []string{"gzip"}
	}
	if loc.WebP != nil {
		if loc.WebP.Quality <= 0 || loc.WebP.Quality > 100 {
			loc.WebP.Quality = 80
		}
		if len(loc.WebP.MimeTypes) == 0 {
			loc.WebP.MimeTypes = []string{"image/jpeg", "image/png"}
		}
	}
	if loc.RequestLimit != nil {
		if loc.RequestLimit.Interval <= 0 {
			loc.RequestLimit.Interval = 60
		}
		if loc.RequestLimit.Limit <= 0 {
			loc.RequestLimit.Limit = 100
		}
		if loc.RequestLimit.Action == "" {
			loc.RequestLimit.Action = "block"
		}
	}
	if hsts := s.TLS_HSTS(); hsts != nil && hsts.Enabled && hsts.MaxAge <= 0 {
		hsts.MaxAge = 31536000
	}
	for i := range s.Locations {
		if s.Locations[i].Location == nil {
			s.Locations[i].Location = &Location{}
		}
		s.Locations[i].EnsureLocationDefaults()
	}
}

// TLS_HSTS is a small helper so EnsureDefaults can touch site-level HSTS.
// (Site config keeps HSTS under TLS; locations never do.)
func (s *SiteConfig) TLS_HSTS() *HSTS {
	if s.TLS == nil {
		return nil
	}
	return s.TLS.HSTS
}

// EnsureLocationDefaults fills per-location defaults (idempotent).
func (l *NamedLocation) EnsureLocationDefaults() {
	loc := l.Location
	if loc == nil {
		return
	}
	if loc.Cache != nil {
		if loc.Cache.Storage == "" {
			loc.Cache.Storage = "file"
		}
		if loc.Cache.TTL <= 0 {
			loc.Cache.TTL = 300
		}
	}
	if loc.RequestLimit != nil && loc.RequestLimit.Interval <= 0 {
		loc.RequestLimit.Interval = 60
	}
}

// Validate checks the document for structural errors.
func (s *SiteConfig) Validate() error {
	if s.Name == "" {
		return fmt.Errorf("site name required")
	}
	if len(s.ServerNames) == 0 {
		return fmt.Errorf("at least one server name required")
	}
	for _, sn := range s.ServerNames {
		if sn.Name == "" {
			return fmt.Errorf("empty server name")
		}
		if sn.Status != "off" && sn.Status != "normal" {
			return fmt.Errorf("invalid server name status %q", sn.Status)
		}
	}
	if len(s.Listeners) == 0 {
		return fmt.Errorf("at least one listener required")
	}
	listenSet := map[string]bool{}
	for _, ln := range s.Listeners {
		switch ln.Protocol {
		case "http", "https", "tcp", "udp":
		default:
			return fmt.Errorf("unknown listener protocol %q", ln.Protocol)
		}
		if ln.Listen == "" {
			return fmt.Errorf("listener %s: empty listen addr", ln.Protocol)
		}
		key := strings.ToLower(ln.Protocol) + "|" + strings.ToLower(ln.Listen)
		if listenSet[key] {
			return fmt.Errorf("duplicate listener %s %s", ln.Protocol, ln.Listen)
		}
		listenSet[key] = true
		if ln.Protocol == "tcp" || ln.Protocol == "udp" {
			if len(ln.Origins) == 0 {
				return fmt.Errorf("listener %s: origins required", ln.Protocol)
			}
		}
	}
	if s.Location == nil {
		return fmt.Errorf("default location required")
	}
	if err := validateLocation(s.Location); err != nil {
		return err
	}
	for i, nl := range s.Locations {
		if nl.Pattern == nil || nl.Pattern.Value == "" {
			return fmt.Errorf("location[%d]: pattern required", i)
		}
		switch nl.Pattern.Type {
		case "exact", "prefix", "regex":
		default:
			return fmt.Errorf("location[%d]: unknown pattern type %q", i, nl.Pattern.Type)
		}
		if nl.Location == nil {
			return fmt.Errorf("location[%d]: config required", i)
		}
		if err := validateLocation(nl.Location); err != nil {
			return fmt.Errorf("location[%d]: %w", i, err)
		}
	}
	if s.TLS != nil && (s.TLS.Cert != "" || s.TLS.Key != "") && (s.TLS.Cert == "" || s.TLS.Key == "") {
		return fmt.Errorf("tls: cert and key must be provided together")
	}
	return nil
}

func validateLocation(l *Location) error {
	sources := 0
	if l.ReverseProxy != nil {
		sources++
		if len(l.ReverseProxy.Origins) == 0 {
			return fmt.Errorf("reverse_proxy: origins required")
		}
		for _, o := range l.ReverseProxy.Origins {
			if o.Addr == "" {
				return fmt.Errorf("reverse_proxy: origin addr required")
			}
		}
		if l.ReverseProxy.Scheduling != nil {
			switch l.ReverseProxy.Scheduling.Type {
			case "", "random", "round_robin", "hash", "sticky":
			default:
				return fmt.Errorf("reverse_proxy: unknown scheduling type %q", l.ReverseProxy.Scheduling.Type)
			}
		}
	}
	if l.Root != "" {
		sources++
	}
	if l.FastCGI != nil {
		sources++
		if l.FastCGI.Addr == "" {
			return fmt.Errorf("fastcgi: addr required")
		}
	}
	// sources may be 0 only when rewrites/pages answer everything; keep it lenient.
	if l.Cache != nil && l.Cache.Enabled && l.Cache.TTL < 0 {
		return fmt.Errorf("cache: ttl must be >= 0")
	}
	if l.Compression != nil && l.Compression.Enabled {
		for _, t := range l.Compression.Types {
			switch t {
			case "gzip", "deflate", "brotli":
			default:
				return fmt.Errorf("compression: unknown type %q", t)
			}
		}
	}
	if l.RequestLimit != nil && l.RequestLimit.Enabled && l.RequestLimit.Limit <= 0 {
		return fmt.Errorf("request_limit: limit must be > 0")
	}
	for i, rw := range l.Rewrites {
		switch rw.Type {
		case "rewrite", "return":
		default:
			return fmt.Errorf("rewrite[%d]: type must be rewrite|return", i)
		}
		if rw.Type == "return" && rw.Status == 0 {
			return fmt.Errorf("rewrite[%d]: return requires status", i)
		}
	}
	return nil
}

// ParseSize parses "10GB", "512MB", "1KB", "100" into bytes.
func ParseSize(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}
	units := []struct {
		suffix string
		mult   int64
	}{
		{"TB", 1024 * 1024 * 1024 * 1024},
		{"GB", 1024 * 1024 * 1024},
		{"MB", 1024 * 1024},
		{"KB", 1024},
		{"B", 1},
	}
	for _, u := range units {
		if strings.HasSuffix(strings.ToUpper(s), u.suffix) {
			var n float64
			if _, err := fmt.Sscanf(strings.TrimSuffix(strings.ToUpper(s), u.suffix), "%g", &n); err != nil {
				return 0, fmt.Errorf("bad size %q", s)
			}
			return int64(n * float64(u.mult)), nil
		}
	}
	var n int64
	if _, err := fmt.Sscanf(s, "%d", &n); err != nil {
		return 0, fmt.Errorf("bad size %q (use e.g. 1024, 512KB, 10GB)", s)
	}
	return n, nil
}

// ParseDurationStr parses "30d", "1w", "2h10m", "90s" into seconds.
func ParseDurationStr(s string) (int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}
	mults := map[string]int{"d": 86400, "w": 604800, "h": 3600, "m": 60, "s": 1}
	// Try plain Go duration first ("2h10m").
	type parsed struct{ total int }
	var res int
	i := 0
	for i < len(s) {
		j := i
		for j < len(s) && s[j] >= '0' && s[j] <= '9' {
			j++
		}
		if j == i {
			return 0, fmt.Errorf("bad duration %q", s)
		}
		var n int
		fmt.Sscanf(s[i:j], "%d", &n)
		i = j
		if i >= len(s) {
			// bare number = seconds
			res += n
			break
		}
		unit := s[i]
		i++
		m, ok := mults[string(unit)]
		if !ok {
			return 0, fmt.Errorf("bad duration unit %q in %q", string(unit), s)
		}
		res += n * m
	}
	return res, nil
}
