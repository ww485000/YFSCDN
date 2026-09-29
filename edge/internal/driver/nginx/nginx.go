// Package nginx: data-plane driver that runs a DEDICATED nginx instance.
// Edge owns the whole config root under dataDir/nginx — no user nginx.conf surgery.
//
// Layout generated at dataDir/nginx:
//
//	nginx.conf               main conf (we own it)
//	edgeconf/http.conf       limit_req_zone / map definitions (from WAF rules)
//	edgeconf/sites/<id>.conf per-site server blocks
//	edgeconf/certs/<id>.crt  TLS leaf cert (https sites)
//	edgeconf/certs/<id>.key  TLS leaf key
//
// WAF support: ip_blacklist/ip_whitelist (deny/allow), path_blacklist (location),
// ua_blacklist (map + if), rate_limit (limit_req). See docs/api-contract.md.
package nginx

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"edgecdn/edge/internal/contract"
)

// Version is the edge protocol/engine version reported to core.
const Version = "2.0.0"

// Nginx runs a private nginx instance.
type Nginx struct {
	bin        string
	dataDir    string
	listen     string
	tlsListen  string
	originPort string // fixed port nginx listens on for CDN traffic (default = ListenAddr port)

	mu    sync.Mutex
	sites map[int64]legacySpec
}

// legacySpec is the basic-mode view of a site for nginx conf generation.
// (The nginx driver implements the core feature subset: proxy + cache + WAF basics.
// Full GoEdge feature set is provided by the gopxy driver.)
type legacySpec struct {
	ID          int64
	Domain      string
	OriginProto string
	OriginHost  string
	OriginPort  int
	CacheTTL    int
	WAFEnabled  bool
	HTTPS       bool
	CertLeafPEM string
	CertKeyPEM  string
	Rules       []legacyRule
}

type legacyRule struct {
	Type   string // ip_blacklist|ip_whitelist|ua_blacklist|path_blacklist|rate_limit
	Value  string
	Action string
}

// deriveLegacy maps a full contract spec onto the basic nginx view.
func deriveLegacy(spec contract.Spec) legacySpec {
	cfg := spec.Site
	ls := legacySpec{
		ID:     cfg.ID,
		Domain: cfg.PrimaryDomain(),
		HTTPS:  false,
	}
	loc := cfg.Location
	if loc != nil {
		if loc.ReverseProxy != nil && len(loc.ReverseProxy.Origins) > 0 {
			o := loc.ReverseProxy.Origins[0]
			ls.OriginProto = "http"
			if o.SSL {
				ls.OriginProto = "https"
			}
			host, port, ok := splitAddr(o.Addr)
			if !ok {
				port = 80
			}
			ls.OriginHost, ls.OriginPort = host, port
		}
		if loc.Cache != nil && loc.Cache.Enabled {
			ls.CacheTTL = loc.Cache.TTL
		}
		if loc.WAF != nil && loc.WAF.Enabled {
			ls.WAFEnabled = true
			for _, cidr := range loc.WAF.IPs.Deny {
				ls.Rules = append(ls.Rules, legacyRule{Type: "ip_blacklist", Value: cidr, Action: "block"})
			}
			for _, cidr := range loc.WAF.IPs.Allow {
				ls.Rules = append(ls.Rules, legacyRule{Type: "ip_whitelist", Value: cidr, Action: "block"})
			}
			for i := range loc.WAF.Rules {
				r := &loc.WAF.Rules[i]
				if r.Status == "off" {
					continue
				}
				switch r.Target {
				case "user-agent":
					ls.Rules = append(ls.Rules, legacyRule{Type: "ua_blacklist", Value: r.Value, Action: r.Action})
				case "url", "path":
					if r.Action == "block" {
						ls.Rules = append(ls.Rules, legacyRule{Type: "path_blacklist", Value: r.Value, Action: "block"})
					}
				}
			}
			if loc.RequestLimit != nil && loc.RequestLimit.Enabled {
				ls.Rules = append(ls.Rules, legacyRule{Type: "rate_limit", Value: fmt.Sprintf("%d/%d", loc.RequestLimit.Limit, loc.RequestLimit.Interval)})
			}
		}
	}
	for _, ln := range cfg.Listeners {
		if ln.Protocol == "https" {
			ls.HTTPS = true
		}
	}
	for i := range spec.Certs {
		c := &spec.Certs[i]
		if c.Domain == ls.Domain || ls.Domain == "" {
			ls.CertLeafPEM = c.Cert
			ls.CertKeyPEM = c.Key
			break
		}
	}
	if ls.CertLeafPEM == "" {
		ls.CertLeafPEM, ls.CertKeyPEM = cfg.TLS.Cert, cfg.TLS.Key
	}
	return ls
}

func splitAddr(addr string) (string, int, bool) {
	i := strings.LastIndexByte(addr, ':')
	if i < 0 {
		return "", 0, false
	}
	var p int
	if _, err := fmt.Sscanf(addr[i+1:], "%d", &p); err != nil || p < 1 || p > 65535 {
		return "", 0, false
	}
	return addr[:i], p, true
}

// New validates the nginx binary exists (soft check; Start will hard-fail).
func New(bin, dataDir, listenAddr, tlsListen string) (*Nginx, error) {
	if bin == "" {
		bin = "nginx"
	}
	root := filepath.Join(dataDir, "nginx")
	for _, d := range []string{root, filepath.Join(root, "edgeconf"), filepath.Join(root, "edgeconf/sites"), filepath.Join(root, "edgeconf/certs"), filepath.Join(root, "cache")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return nil, err
		}
	}
	if _, err := exec.LookPath(bin); err != nil {
		log.Printf("[nginx] WARNING: nginx binary %q not found; start with -config nginx_bin or use driver=gopxy", bin)
	}
	return &Nginx{
		bin:       bin,
		dataDir:   root,
		listen:    listenAddr,
		tlsListen: tlsListen,
		sites:     map[int64]legacySpec{},
	}, nil
}

func (n *Nginx) Name() string    { return "nginx" }
func (n *Nginx) Version() string { return Version }

// DrainLogs is not supported by the nginx driver (nginx writes its own access log).
func (n *Nginx) DrainLogs() []map[string]any { return nil }

// Start renders config and launches nginx if not already running.
func (n *Nginx) Start() error {
	if err := n.renderAll(); err != nil {
		return err
	}
	if !n.isRunning() {
		if err := n.testConf(); err != nil {
			return fmt.Errorf("nginx -t failed: %w", err)
		}
		cmd := exec.Command(n.bin, "-p", n.dataDir, "-c", "nginx.conf")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Start(); err != nil {
			return fmt.Errorf("start nginx: %w", err)
		}
		log.Printf("[nginx] started dedicated nginx (prefix %s)", n.dataDir)
	}
	return nil
}

// ApplySite stores the spec and reloads nginx.
func (n *Nginx) ApplySite(spec contract.Spec) error {
	ls := deriveLegacy(spec)
	n.mu.Lock()
	n.sites[spec.Site.ID] = ls
	n.mu.Unlock()
	if err := n.renderAll(); err != nil {
		return err
	}
	return n.reload()
}

// RemoveSite drops the site and reloads.
func (n *Nginx) RemoveSite(id int64) error {
	n.mu.Lock()
	delete(n.sites, id)
	n.mu.Unlock()
	if err := n.renderSiteFile(id, nil); err != nil {
		return err
	}
	return n.reload()
}

// Stop quits the dedicated nginx.
func (n *Nginx) Stop() error {
	if !n.isRunning() {
		return nil
	}
	return exec.Command(n.bin, "-p", n.dataDir, "-c", "nginx.conf", "-s", "quit").Run()
}

// isRunning is best-effort: checks the pid file we asked nginx to write.
func (n *Nginx) isRunning() bool {
	b, err := os.ReadFile(filepath.Join(n.dataDir, "nginx.pid"))
	if err != nil {
		return false
	}
	var pid int
	if _, err := fmt.Sscanf(strings.TrimSpace(string(b)), "%d", &pid); err != nil || pid <= 0 {
		return false
	}
	_, err = os.FindProcess(pid)
	return err == nil
}

func (n *Nginx) testConf() error {
	out, err := exec.Command(n.bin, "-t", "-p", n.dataDir, "-c", "nginx.conf").CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s", strings.TrimSpace(string(out)))
	}
	return nil
}

func (n *Nginx) reload() error {
	if !n.isRunning() {
		return n.Start()
	}
	out, err := exec.Command(n.bin, "-p", n.dataDir, "-c", "nginx.conf", "-s", "reload").CombinedOutput()
	if err != nil {
		return fmt.Errorf("nginx reload: %s %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

// renderAll writes main conf, http-level conf, and every site conf.
func (n *Nginx) renderAll() error {
	n.mu.Lock()
	sites := make([]legacySpec, 0, len(n.sites))
	for _, s := range n.sites {
		sites = append(sites, s)
	}
	n.mu.Unlock()

	mainConf := fmt.Sprintf(`worker_processes 1;
pid nginx.pid;
error_log logs/error.log warn;
events { worker_connections 1024; }
http {
    access_log logs/access.log;
    default_type application/octet-stream;
    proxy_cache_path cache levels=1:2 keys_zone=edgecdn:128m max_size=512m inactive=1d;
    include edgeconf/http.conf;
    include edgeconf/sites/*.conf;
}
`)
	if err := os.WriteFile(filepath.Join(n.dataDir, "nginx.conf"), []byte(mainConf), 0o644); err != nil {
		return err
	}
	if err := n.renderHTTPConf(sites); err != nil {
		return err
	}
	// render every current site; also delete stale site files
	dir := filepath.Join(n.dataDir, "edgeconf", "sites")
	entries, _ := os.ReadDir(dir)
	current := map[int64]bool{}
	for _, s := range sites {
		current[s.ID] = true
		if err := n.renderSiteFile(s.ID, &s); err != nil {
			return err
		}
	}
	for _, e := range entries {
		if !e.IsDir() {
			var id int64
			if _, err := fmt.Sscanf(strings.TrimSuffix(e.Name(), ".conf"), "%d", &id); err == nil && !current[id] {
				_ = os.Remove(filepath.Join(dir, e.Name()))
			}
		}
	}
	return nil
}

// renderHTTPConf writes http-level directives derived from WAF rules.
func (n *Nginx) renderHTTPConf(sites []legacySpec) error {
	var b strings.Builder
	b.WriteString("# auto-generated by edgecdn edge (nginx driver) — do not edit\n")
	for _, s := range sites {
		for _, r := range s.Rules {
			switch r.Type {
			case "rate_limit":
				rate := "10r/s"
				if parts := strings.SplitN(r.Value, "/", 2); len(parts) == 2 {
					if n, err := strconv.Atoi(parts[0]); err == nil && n > 0 {
						rate = fmt.Sprintf("%dr/s", n/60)
						if n < 60 {
							rate = "1r/s"
						}
					}
				}
				fmt.Fprintf(&b, "limit_req_zone $binary_remote_addr zone=rl_%d:10m rate=%s;\n", s.ID, rate)
			case "ua_blacklist":
				fmt.Fprintf(&b, "map $http_user_agent bad_ua_%d { default 0; \"~*%s\" 1; }\n", s.ID, sanitizeRe(r.Value))
			}
		}
	}
	return os.WriteFile(filepath.Join(n.dataDir, "edgeconf", "http.conf"), []byte(b.String()), 0o644)
}

// renderSiteFile writes (or deletes, when spec==nil) one server block.
func (n *Nginx) renderSiteFile(id int64, spec *legacySpec) error {
	path := filepath.Join(n.dataDir, "edgeconf", "sites", strconv.FormatInt(id, 10)+".conf")
	if spec == nil {
		_ = os.Remove(path)
		return nil
	}
	var b strings.Builder
	listen80 := "listen " + n.listen + ";"
	b.WriteString(listen80 + "\n")
	var tls string
	if spec.HTTPS && spec.CertLeafPEM != "" {
		crt := filepath.Join(n.dataDir, "edgeconf", "certs", strconv.FormatInt(id, 10)+".crt")
		key := filepath.Join(n.dataDir, "edgeconf", "certs", strconv.FormatInt(id, 10)+".key")
		_ = os.WriteFile(crt, []byte(spec.CertLeafPEM), 0o600)
		_ = os.WriteFile(key, []byte(spec.CertKeyPEM), 0o600)
		tls = fmt.Sprintf("listen %s ssl;\nssl_certificate %s;\nssl_certificate_key %s;\n",
			n.tlsListen, strings.ReplaceAll(crt, "\\", "/"), strings.ReplaceAll(key, "\\", "/"))
		b.WriteString(tls)
	}
	fmt.Fprintf(&b, "server_name %s;\n", spec.Domain)

	// WAF: ip rules
	for _, r := range spec.Rules {
		switch r.Type {
		case "ip_blacklist":
			for _, v := range strings.Split(r.Value, ",") {
				fmt.Fprintf(&b, "deny %s;\n", strings.TrimSpace(v))
			}
		case "ip_whitelist":
			for _, v := range strings.Split(r.Value, ",") {
				fmt.Fprintf(&b, "allow %s;\n", strings.TrimSpace(v))
			}
			b.WriteString("deny all;\n")
		case "ua_blacklist":
			fmt.Fprintf(&b, "if ($bad_ua_%d) { return 403; }\n", id)
		case "path_blacklist":
			for _, v := range strings.Split(r.Value, ",") {
				fmt.Fprintf(&b, "location ^~ %s { return 403; }\n", strings.TrimSpace(v))
			}
		}
	}
	b.WriteString("location / {\n")
	if spec.CacheTTL > 0 {
		fmt.Fprintf(&b, "proxy_cache edgecdn;\nproxy_cache_valid 200 %ds;\nproxy_cache_use_stale error timeout;\n", spec.CacheTTL)
	}
	if spec.WAFEnabled {
		for _, r := range spec.Rules {
			if r.Type == "rate_limit" {
				fmt.Fprintf(&b, "limit_req zone=rl_%d burst=20 nodelay;\n", id)
			}
		}
	}
	fmt.Fprintf(&b, "proxy_pass %s://%s:%d;\n", spec.OriginProto, spec.OriginHost, spec.OriginPort)
	b.WriteString("proxy_set_header Host $host;\n" +
		"proxy_set_header X-Real-IP $remote_addr;\n" +
		"proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;\n" +
		"proxy_set_header X-Forwarded-Proto $scheme;\n")
	b.WriteString("}\n")
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

// sanitizeRe escapes regex metacharacters for nginx map "~*" patterns.
func sanitizeRe(s string) string {
	r := strings.NewReplacer(`\`, `\\\\`, `.`, `\\.`, `*`, `\*`, `?`, `\?`, `+`, `\+`,
		`(`, `\(`, `)`, `\)`, `[`, `\[`, `]`, `\]`, `{`, `\{`, `}`, `\}`, `^`, `\^`, `$`, `\$`, `|`, `\|`)
	return r.Replace(s)
}
