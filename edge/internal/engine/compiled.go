package engine

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"edgecdn/edge/internal/contract"
	"edgecdn/edge/internal/stats"
)

// site is one applied site (runtime object).
type site struct {
	id         int64
	spec       contract.Spec
	cfg        *contract.SiteConfig
	def        *compiledLoc
	locs       []*compiledLoc
	httpsPort  int
	httpFollow map[string]bool // listener addr -> follow_protocol (site-level)
	traffic    *trafficLimiter
	waf        *wafEngine
	webp       *webpConverter
	accessLog  bool // per-site access-log switch (default on)
	stats      *stats.Stats
	logWriter  *AccessLogWriter
}

func (s *site) domains() []string {
	var out []string
	for _, sn := range s.cfg.ServerNames {
		if sn.Status != "off" {
			out = append(out, strings.ToLower(sn.Name))
		}
	}
	return out
}

// compiledLoc is one location with all features pre-compiled.
type compiledLoc struct {
	name      string
	pattern   *locPattern
	loc       *contract.Location
	rewrites  []*compiledRewrite
	headers   *headerPolicy
	cache     *cacheStore
	compress  *compressor
	charset   *charsetConverter
	expires   time.Duration
	pages     map[int]*contract.ErrorPage
	access    *accessCtl
	authZ     *authZ
	cc        *ccLimiter
	origins   *originGroup
	fastcgi   *fastcgiBackend
	root      string
	index     []string
	minify    *contract.Minify
}

type locPattern struct {
	typ  string // exact|prefix|regex
	re   *regexp.Regexp
	val  string
}

func (p *locPattern) match(path string) bool {
	if p == nil {
		return true
	}
	switch p.typ {
	case "exact":
		return path == p.val
	case "prefix":
		return strings.HasPrefix(path, p.val)
	case "regex":
		return p.re != nil && p.re.MatchString(path)
	}
	return false
}

// compileSite builds the runtime site from a spec.
func compileSite(spec contract.Spec, dataDir string, stStats *stats.Stats, logs *AccessLogWriter) (*site, error) {
	cfg := spec.Site
	loc := cfg.Location
	if loc == nil {
		loc = &contract.Location{}
	}

	st := &site{
		id:         cfg.ID,
		spec:       spec,
		cfg:        &cfg,
		httpFollow: map[string]bool{},
		stats:      stStats,
		logWriter:  logs,
	}

	// WebP converter (site-level, from default location).
	if loc.WebP != nil && loc.WebP.Enabled {
		st.webp = newWebpConverter(loc.WebP)
	}

	// Access log switch (default on; explicit disabled turns it off).
	st.accessLog = true
	if loc.AccessLog != nil && !loc.AccessLog.Enabled {
		st.accessLog = false
	}

	// Site-wide WAF (default location's WAF + tenant global rules).
	if loc.WAF != nil && loc.WAF.Enabled {
		rules := append([]contract.WAFRule{}, loc.WAF.Rules...)
		if spec.GlobalWAFEnabled {
			rules = append(rules, spec.GlobalRules...)
		}
		st.waf = newWafEngine(loc.WAF.IPs, rules)
	}

	// Site traffic limit (default location).
	if loc.TrafficLimit != nil && loc.TrafficLimit.Enabled {
		maxBytes, _ := contract.ParseSize(loc.TrafficLimit.MaxBytes)
		st.traffic = newTrafficLimiter(maxBytes, loc.TrafficLimit.MaxConn)
	}

	// Default location.
	def, err := compileLocation("default", nil, loc, dataDir, st)
	if err != nil {
		return nil, err
	}
	st.def = def

	// Named locations.
	for i := range cfg.Locations {
		nl := &cfg.Locations[i]
		inner := nl.Location
		if inner == nil {
			inner = &contract.Location{}
		}
		var pat *locPattern
		if nl.Pattern != nil {
			pat = &locPattern{typ: nl.Pattern.Type, val: nl.Pattern.Value}
			if nl.Pattern.Type == "regex" {
				re, err := regexp.Compile(nl.Pattern.Value)
				if err != nil {
					return nil, fmt.Errorf("location %q: bad regex: %w", nl.Name, err)
				}
				pat.re = re
			}
		}
		// WAF for named locations inherits site WAF if it has none of its own.
		if inner.WAF == nil || !inner.WAF.Enabled {
			if loc.WAF != nil && loc.WAF.Enabled {
				inner = cloneLoc(inner)
				rules := append([]contract.WAFRule{}, loc.WAF.Rules...)
				if spec.GlobalWAFEnabled {
					rules = append(rules, spec.GlobalRules...)
				}
				inner.WAF = &contract.WAF{Enabled: true, IPs: loc.WAF.IPs, Rules: rules}
			}
		}
		cl, err := compileLocation(nl.Name, pat, inner, dataDir, st)
		if err != nil {
			return nil, fmt.Errorf("location %q: %w", nl.Name, err)
		}
		st.locs = append(st.locs, cl)
	}

	// https port (for redirects) + follow protocol flags.
	for _, ln := range cfg.Listeners {
		switch ln.Protocol {
		case "https":
			if port := listenPort(ln.Listen, 443); port > 0 {
				st.httpsPort = port
			}
		case "http":
			if ln.FollowProtocol {
				st.httpFollow[ln.Listen] = true
			}
		}
	}
	return st, nil
}

func cloneLoc(l *contract.Location) *contract.Location {
	out := *l
	return &out
}

// compileLocation builds one compiled location.
func compileLocation(name string, pat *locPattern, l *contract.Location, dataDir string, st *site) (*compiledLoc, error) {
	cl := &compiledLoc{name: name, pattern: pat, loc: l}

	// Rewrites.
	for i := range l.Rewrites {
		rw, err := compileRewrite(&l.Rewrites[i])
		if err != nil {
			return nil, err
		}
		cl.rewrites = append(cl.rewrites, rw)
	}

	// Headers.
	if l.Headers != nil {
		cl.headers = compileHeaders(l.Headers)
	}

	// Cache.
	if l.Cache != nil && l.Cache.Enabled {
		cs, err := newCacheStore(dataDir, l.Cache)
		if err != nil {
			return nil, err
		}
		cl.cache = cs
	}

	// Compression.
	if l.Compression != nil && l.Compression.Enabled {
		cl.compress = newCompressor(l.Compression)
	}

	// Charset.
	if l.Charset != nil && l.Charset.Enabled {
		cc, err := newCharsetConverter(l.Charset)
		if err != nil {
			return nil, err
		}
		cl.charset = cc
	}

	// Expires.
	if l.Expires != nil && l.Expires.Enabled {
		sec, err := contract.ParseDurationStr(l.Expires.Time)
		if err != nil {
			return nil, err
		}
		cl.expires = time.Duration(sec) * time.Second
	}

	// Pages.
	if l.Pages != nil {
		cl.pages = map[int]*contract.ErrorPage{}
		for i := range l.Pages.Codes {
			ec := &l.Pages.Codes[i]
			if ec.Enabled && ec.Status > 0 {
				cl.pages[ec.Status] = ec
			}
		}
		cl.minify = l.Pages.Minify
	}

	// Access control.
	if l.Access != nil {
		cl.access = compileAccess(l.Access)
	}

	// Auth.
	if l.Auth != nil && l.Auth.Enabled {
		cl.authZ = compileAuth(l.Auth)
	}

	// CC limit.
	if l.RequestLimit != nil && l.RequestLimit.Enabled {
		cl.cc = newCClimiter(l.RequestLimit)
	}

	// Reverse proxy.
	if l.ReverseProxy != nil && len(l.ReverseProxy.Origins) > 0 {
		og, err := newOriginGroup(l.ReverseProxy)
		if err != nil {
			return nil, err
		}
		cl.origins = og
		if l.ReverseProxy.HealthCheck != nil && l.ReverseProxy.HealthCheck.Enabled {
			og.startHealthCheck()
		}
	}

	// FastCGI.
	if l.FastCGI != nil && l.FastCGI.Addr != "" {
		cl.fastcgi = newFastCGIBackend(l.FastCGI)
	}

	// Static.
	cl.root = l.Root
	cl.index = l.Index
	if cl.index == nil && l.Root != "" {
		cl.index = []string{"index.html"}
	}

	// Websocket paths (nil = all).
	if l.Websocket != nil && l.Websocket.Enabled {
		// presence flag; paths checked at request time
	}
	return cl, nil
}

// pickLocation resolves the location for a path (named first, then default).
func (s *site) pickLocation(path string) *compiledLoc {
	for _, l := range s.locs {
		if l.pattern.match(path) {
			return l
		}
	}
	return s.def
}

// followProtocol reports whether the request arrived on a follow_protocol listener.
func (s *site) followProtocol(addr string) bool {
	return s.httpFollow[addr]
}

func listenPort(addr string, def int) int {
	if i := strings.LastIndexByte(addr, ':'); i >= 0 {
		var p int
		if _, err := fmt.Sscanf(addr[i+1:], "%d", &p); err == nil && p > 0 {
			return p
		}
	}
	return def
}
