package engine

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

var proxyTransport = &http.Transport{
	MaxIdleConns:        256,
	MaxIdleConnsPerHost: 32,
	IdleConnTimeout:     90 * time.Second,
	TLSHandshakeTimeout: 10 * time.Second,
}

// handle is the entry for one routed request.
func (s *site) handle(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	loc := s.pickLocation(r.URL.Path)
	ip := clientIP(r)

	// ---- host redirect (server_name redirect_to) ----
	if s.hostRedirect(w, r) {
		s.logEntry(r, start, http.StatusMovedPermanently, 0, "BYPASS")
		return
	}

	// ---- site traffic limits ----
	if s.traffic != nil {
		if s.traffic.overBytes() {
			serveError(w, r, http.StatusServiceUnavailable, "traffic limit exceeded", loc.pages)
			s.logEntry(r, start, http.StatusServiceUnavailable, 0, "BYPASS")
			return
		}
		if !s.traffic.enter() {
			serveError(w, r, http.StatusTooManyRequests, "concurrent connections limit", loc.pages)
			s.logEntry(r, start, http.StatusTooManyRequests, 0, "BYPASS")
			return
		}
		defer s.traffic.leave()
	}

	// ---- access control (IP / referer / UA) ----
	if msg := loc.access.check(r); msg != "" {
		serveError(w, r, http.StatusForbidden, msg, loc.pages)
		s.logEntry(r, start, http.StatusForbidden, 0, "BYPASS")
		return
	}

	// ---- WAF ----
	if s.waf != nil {
		if v := s.waf.Check(r); v != nil {
			switch v.action {
			case "block":
				serveError(w, r, http.StatusForbidden, "blocked by WAF", loc.pages)
				s.logEntry(r, start, http.StatusForbidden, 0, "BYPASS")
				return
			case "js":
				serveJSChallenge(w, r, loc)
				s.logEntry(r, start, http.StatusForbidden, 0, "BYPASS")
				return
			case "captcha":
				serveCaptcha(w, r, loc)
				s.logEntry(r, start, http.StatusForbidden, 0, "BYPASS")
				return
			default: // log
			}
		}
	}

	// ---- CC limit ----
	if loc.cc != nil && !loc.cc.allow(ip) {
		serveError(w, r, http.StatusTooManyRequests, "rate limit", loc.pages)
		s.logEntry(r, start, http.StatusTooManyRequests, 0, "BYPASS")
		return
	}

	// ---- auth ----
	if loc.authZ != nil {
		if status, reason := loc.authZ.check(r); status != 0 {
			w.Header().Set("WWW-Authenticate", loc.authZ.realmHeader())
			serveError(w, r, status, reason, loc.pages)
			s.logEntry(r, start, status, 0, "BYPASS")
			return
		}
	}

	// ---- CORS preflight ----
	if loc.headers != nil && loc.headers.applyCORS(w, r) {
		s.logEntry(r, start, http.StatusNoContent, 0, "BYPASS")
		return
	}

	// ---- rewrites ----
	args := &rewriteArgs{query: r.URL.Query(), path: r.URL.Path, host: r.Host}
	for _, rw := range loc.rewrites {
		kind, value := rw.eval(r, args)
		switch kind {
		case kindReturn:
			status := rw.returnStatus()
			w.Header().Set("Location", value)
			if status >= 200 && status < 300 {
				// non-redirect return: value is the body
				w.Header().Set("Content-Type", "text/plain; charset=utf-8")
				w.WriteHeader(status)
				_, _ = w.Write([]byte(value))
			} else {
				w.WriteHeader(status)
			}
			s.logEntry(r, start, status, 0, "BYPASS")
			return
		case kindRewrite:
			r.URL.Path = value
		}
	}

	// ---- https redirect / follow protocol ----
	if r.TLS == nil {
		if loc.loc != nil && loc.loc.RedirectHTTPS != nil && loc.loc.RedirectHTTPS.Enabled {
			status := loc.loc.RedirectHTTPS.Status
			if status != http.StatusFound {
				status = http.StatusMovedPermanently
			}
			if !s.redirectHTTPS(w, r, status) {
				s.logEntry(r, start, status, 0, "BYPASS")
				return
			}
		} else if addr, _ := r.Context().Value(listenerAddrKey).(string); addr != "" && s.followProtocol(addr) {
			if !s.redirectHTTPS(w, r, http.StatusMovedPermanently) {
				s.logEntry(r, start, http.StatusMovedPermanently, 0, "BYPASS")
				return
			}
		}
	}

	// ---- response writer (transforms + caching) ----
	fw := newFinalWriter(w, s, loc, r)
	defer fw.finalize()

	// ---- cache lookup (GET, no-cache bypass) ----
	cacheFlag := "BYPASS"
	if loc.cache != nil && (r.Method == http.MethodGet || !loc.cache.onlyGet) &&
		r.Header.Get("Cache-Control") != "no-cache" {
		if e := loc.cache.get(r); e != nil {
			flag := "HIT"
			if e.stale {
				flag = "STALE"
			}
			fw.serveCached(e, flag)
			s.trafficAdd(fw.bytes)
			if s.stats != nil {
				s.stats.Add(s.id, fw.bytes, flag == "HIT")
			}
			s.logEntry(r, start, fw.code, fw.bytes, flag)
			return
		}
	}

	// ---- source dispatch ----
	switch {
	case isWebsocketUpgrade(r) && loc.origins != nil:
		loc.proxy(fw, r)
	case loc.fastcgi != nil:
		loc.fastcgi.serve(fw, r)
	case loc.root != "":
		loc.staticServe(fw, r)
	case loc.origins != nil:
		loc.proxy(fw, r)
	default:
		serveError(fw, r, http.StatusNotFound, "no backend configured", loc.pages)
	}

	// ---- stats + access log ----
	switch {
	case fw.cacheStatus == cacheStale:
		cacheFlag = "STALE"
	case fw.cacheStatus == cacheHit:
		cacheFlag = "HIT"
	case fw.cacheStatus == cacheMiss:
		cacheFlag = "MISS"
	}
	s.trafficAdd(fw.bytes)
	if s.stats != nil {
		s.stats.Add(s.id, fw.bytes, cacheFlag == "HIT")
	}
	s.logEntry(r, start, fw.code, fw.bytes, cacheFlag)
}

// trafficAdd feeds the site traffic limiter.
func (s *site) trafficAdd(n int64) {
	if s.traffic != nil {
		s.traffic.addBytes(n)
	}
}

func (s *site) logEntry(r *http.Request, start time.Time, status int, bytes int64, cache string) {
	if s == nil || s.logWriter == nil || !s.accessLog {
		return
	}
	ua := r.UserAgent()
	if len(ua) > 256 {
		ua = ua[:256]
	}
	ref := r.Referer()
	if len(ref) > 256 {
		ref = ref[:256]
	}
	s.logWriter.Record(AccessLogEntry{
		SiteID:  s.id,
		IP:      clientIP(r),
		Host:    r.Host,
		Method:  r.Method,
		Path:    r.URL.Path,
		Proto:   r.Proto,
		Status:  status,
		Bytes:   bytes,
		Referer: ref,
		UA:      ua,
		Latency: time.Since(start).Milliseconds(),
		Cache:   cache,
	})
}

func isWebsocketUpgrade(r *http.Request) bool {
	return strings.EqualFold(r.Header.Get("Upgrade"), "websocket") &&
		strings.Contains(strings.ToLower(r.Header.Get("Connection")), "upgrade")
}

// redirectHTTPS issues a 301/302 to the https listener. Returns false when it did.
func (s *site) redirectHTTPS(w http.ResponseWriter, r *http.Request, status int) bool {
	port := s.httpsPort
	if port <= 0 {
		port = 443
	}
	host := r.Host
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	target := "https://" + host
	if port != 443 {
		target += ":" + itoa(port)
	}
	target += r.URL.RequestURI()
	w.Header().Set("Location", target)
	w.WriteHeader(status)
	return false
}

// hostRedirect checks for a server-name RedirectTo (GoEdge host redirect) and
// issues a 301 if configured for this host. Returns true when redirected.
func (s *site) hostRedirect(w http.ResponseWriter, r *http.Request) bool {
	host := r.Host
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	host = strings.ToLower(host)
	for _, sn := range s.cfg.ServerNames {
		if strings.ToLower(sn.Name) != host {
			continue
		}
		if sn.RedirectTo == "" {
			return false
		}
		target := sn.RedirectTo
		if !strings.HasPrefix(target, "http://") && !strings.HasPrefix(target, "https://") {
			target = "https://" + target
		}
		target += r.URL.RequestURI()
		w.Header().Set("Location", target)
		w.WriteHeader(http.StatusMovedPermanently)
		return true
	}
	return false
}

// serveJSChallenge serves a minimal JS challenge page (WAF "js" action).
func serveJSChallenge(w http.ResponseWriter, r *http.Request, loc *compiledLoc) {
	body := `<!doctype html><html><head><meta charset="utf-8"><title>Checking…</title>
<script>
var t = Date.now();
document.cookie = "edgecdn_js=" + (t & 0x7fffffff) + ";path=/;max-age=30";
location.reload();
</script></head><body>Checking your browser…</body></html>`
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusForbidden)
	_, _ = w.Write([]byte(body))
}

// serveCaptcha serves a placeholder captcha page (WAF "captcha" action).
func serveCaptcha(w http.ResponseWriter, r *http.Request, loc *compiledLoc) {
	body := `<!doctype html><html><head><meta charset="utf-8"><title>Captcha</title></head>
<body><h1>Security Check</h1><p>Please verify you are not a robot.</p>
<p>(captcha engine placeholder)</p></body></html>`
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusForbidden)
	_, _ = w.Write([]byte(body))
}

// ---------- finalWriter ----------

const (
	wraw = iota
	wbuffer
	wcompress
)

const (
	cacheNone = iota
	cacheHit
	cacheMiss
	cacheStale
)

// finalWriter applies response transforms (cache/webp/charset/minify/compress).
type finalWriter struct {
	inner http.ResponseWriter
	s     *site
	loc   *compiledLoc
	r     *http.Request

	code int
	sent bool
	bytes int64
	mode  int
	// buffer mode
	buf     []byte
	contentLength int64
	done    bool
	// compress mode
	cw      io.Writer
	// cache
	cacheStatus int
	cacheReq    *http.Request
	cacheHeaders http.Header

	// HSTS etc. applied once
	hstsApplied bool
}

func newFinalWriter(w http.ResponseWriter, s *site, loc *compiledLoc, r *http.Request) *finalWriter {
	fw := &finalWriter{inner: w, s: s, loc: loc, r: r, code: http.StatusOK, cacheStatus: cacheNone}
	return fw
}

// WriteHeader is called by the backend before the body.
func (fw *finalWriter) WriteHeader(code int) {
	if fw.sent {
		return
	}
	fw.code = code
	h := fw.inner.Header()

	// downstream header ops
	fw.loc.headers.applyDown(h)
	// HSTS (HSTS is a pointer; nil means not configured)
	if fw.s.cfg.TLS != nil && fw.s.cfg.TLS.HSTS != nil && fw.s.cfg.TLS.HSTS.Enabled && fw.r.TLS != nil {
		v := "max-age=" + itoa(fw.s.cfg.TLS.HSTS.MaxAge)
		if fw.s.cfg.TLS.HSTS.IncludeSubDomains {
			v += "; includeSubDomains"
		}
		h.Set("Strict-Transport-Security", v)
	}
	// expires
	if fw.loc.expires > 0 && code/100 == 2 {
		h.Set("Expires", expiresNow(fw.loc.expires))
	}
	// CORS headers (non-preflight)
	if fw.loc.headers != nil && fw.r.Header.Get("Origin") != "" {
		fw.loc.headers.applyCORS(fw.inner, fw.r)
	}

	if code == http.StatusSwitchingProtocols {
		fw.mode = wraw
		fw.sent = true
		fw.inner.WriteHeader(code)
		return
	}

	ct := h.Get("Content-Type")
	// cache capture (raw + buffered paths)
	if fw.loc.cache != nil && fw.r.Method == http.MethodGet && code == http.StatusOK {
		fw.cacheStatus = cacheMiss
		fw.cacheReq = fw.r
		fw.cacheHeaders = h
	}
	// buffered transform path?
	if fw.needsBufferTransform(ct, code) {
		fw.mode = wbuffer
		fw.contentLength = int64(0)
		if v := h.Get("Content-Length"); v != "" {
			fmt.Sscanf(v, "%d", &fw.contentLength)
		}
		return // headers deferred until finalize
	}
	// compress stream path?
	if fw.loc.compress != nil && fw.loc.compress.minBytes() > 0 && code/100 == 2 && h.Get("Content-Encoding") == "" {
		cw, ok := fw.loc.compress.startWriter(fw.inner, fw.r)
		if ok {
			fw.mode = wcompress
			fw.cw = cw
		}
	}
	fw.sent = true
	fw.inner.WriteHeader(code)
}

func (fw *finalWriter) needsBufferTransform(ct string, code int) bool {
	if code/100 != 2 || fw.r.Method != http.MethodGet {
		return false
	}
	// webp (site-level converter, only when client accepts it)
	if wc := fw.s.webp; wc != nil && wc.acceptsWebP(fw.r) && wc.matches(ct) {
		return true
	}
	// charset
	if fw.loc.charset != nil && fw.loc.charset.matches(ct) {
		return true
	}
	// html minify
	if fw.loc.minify != nil && fw.loc.minify.HTML && strings.Contains(ct, "html") {
		return true
	}
	return false
}

// Write routes body bytes by mode.
func (fw *finalWriter) Write(b []byte) (int, error) {
	fw.bytes += int64(len(b))
	switch fw.mode {
	case wbuffer:
		fw.buf = append(fw.buf, b...)
		if int64(len(fw.buf)) > maxTransformBytes {
			fw.abandonTransform()
			return fw.inner.Write(b)
		}
		if fw.contentLength > 0 && int64(len(fw.buf)) >= fw.contentLength {
			fw.applyTransform()
		}
		return len(b), nil
	case wcompress:
		return fw.cw.Write(b)
	default:
		n, err := fw.inner.Write(b)
		if fw.cacheStatus == cacheMiss {
			fw.buf = append(fw.buf, b[:n]...)
			if int64(len(fw.buf)) > maxTransformBytes {
				fw.cacheStatus = cacheNone // too big to cache
			}
		}
		return n, err
	}
}

// abandonTransform switches buffer mode to raw (body too big).
func (fw *finalWriter) abandonTransform() {
	fw.mode = wraw
	if !fw.sent {
		fw.sent = true
		fw.inner.WriteHeader(fw.code)
	}
	// already-buffered bytes must go out before further writes
	if len(fw.buf) > 0 {
		_, _ = fw.inner.Write(fw.buf)
	}
	fw.buf = nil
	fw.cacheStatus = cacheNone
}

// applyTransform finalizes the buffered body (webp / charset / minify / cache).
func (fw *finalWriter) applyTransform() {
	if fw.done {
		return
	}
	fw.done = true
	body := fw.buf
	h := fw.inner.Header()

	// cache stores the original (untransformed) body
	if fw.loc.cache != nil && fw.cacheStatus == cacheMiss {
		fw.loc.cache.put(fw.cacheReq, fw.code, fw.cacheHeaders, body)
	}

	// webp
	if wc := fw.s.webp; wc != nil && wc.acceptsWebP(fw.r) && wc.matches(h.Get("Content-Type")) {
		if out, ct, e := wc.convert(body); e == nil {
			body = out
			h.Set("Content-Type", ct)
		}
	}
	// charset
	if fw.loc.charset != nil && fw.loc.charset.matches(h.Get("Content-Type")) {
		if out, cs, e := fw.loc.charset.convert(body, h.Get("Content-Type")); e == nil {
			body = out
			h.Set("Content-Type", setContentTypeCharset(h.Get("Content-Type"), cs))
		}
	}
	// html minify
	if fw.loc.minify != nil && fw.loc.minify.HTML && strings.Contains(h.Get("Content-Type"), "html") {
		body = minifyHTML(fw.loc.minify, body)
	}

	h.Set("Content-Length", itoa(len(body)))
	h.Del("Transfer-Encoding")
	h.Set("X-EdgeCDN-Cache", cacheFlag(fw.cacheStatus))
	fw.sent = true
	fw.inner.WriteHeader(fw.code)
	_, _ = fw.inner.Write(body)
	fw.mode = wraw
	fw.buf = nil
}

// serveCached replays a cached entry through the transform pipeline.
func (fw *finalWriter) serveCached(e *cachedEntry, flag string) {
	fw.code = e.status
	if flag == "STALE" {
		fw.cacheStatus = cacheStale
	} else {
		fw.cacheStatus = cacheHit
	}
	h := fw.inner.Header()
	for k, vv := range e.headers {
		if strings.EqualFold(k, "Content-Length") || strings.EqualFold(k, "Transfer-Encoding") {
			continue
		}
		for _, v := range vv {
			h.Add(k, v)
		}
	}
	h.Set("X-EdgeCDN-Cache", flag)
	if fw.needsBufferTransform(h.Get("Content-Type"), fw.code) {
		fw.mode = wbuffer
		fw.buf = append([]byte(nil), e.body...)
		fw.applyTransform() // cacheStatus stays HIT/STALE → header + re-store
		return
	}
	fw.sent = true
	fw.inner.WriteHeader(fw.code)
	if fw.loc.compress != nil {
		if cw, ok := fw.loc.compress.startWriter(fw.inner, fw.r); ok {
			_, _ = cw.Write(e.body)
			flushCompressor(cw)
		} else {
			_, _ = fw.inner.Write(e.body)
		}
	} else {
		_, _ = fw.inner.Write(e.body)
	}
	fw.bytes += int64(len(e.body))
}

// Flush: end-of-body signal for the buffered path.
func (fw *finalWriter) Flush() {
	if fw.mode == wbuffer && !fw.done {
		fw.applyTransform()
		return
	}
	if f, ok := fw.inner.(http.Flusher); ok {
		f.Flush()
	}
}

// finalize runs at request end: flush compressors / pending cache / leftovers.
func (fw *finalWriter) finalize() {
	if fw.mode == wbuffer && !fw.done {
		if len(fw.buf) == 0 && !fw.sent {
			// headers were never needed (empty body)
			fw.inner.WriteHeader(fw.code)
		} else {
			fw.applyTransform()
		}
		return
	}
	if fw.mode == wcompress && fw.cw != nil {
		flushCompressor(fw.cw)
	}
	if fw.mode == wraw && fw.cacheStatus == cacheMiss && fw.cacheReq != nil {
		fw.loc.cache.put(fw.cacheReq, fw.code, fw.cacheHeaders, fw.buf)
		fw.cacheStatus = cacheHit
		fw.buf = nil
	}
}

// Hijack support (websocket) pass-through.
func (fw *finalWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if h, ok := fw.inner.(http.Hijacker); ok {
		return h.Hijack()
	}
	return nil, nil, errNotHijacker
}

// Header delegates to the underlying writer.
func (fw *finalWriter) Header() http.Header { return fw.inner.Header() }

// setContentTypeCharset replaces (or appends) the charset parameter.
func setContentTypeCharset(ct, cs string) string {
	if i := strings.Index(ct, ";"); i >= 0 {
		base := strings.TrimSpace(ct[:i])
		return base + "; charset=" + cs
	}
	return ct + "; charset=" + cs
}

// cacheFlag maps the cache state to the X-EdgeCDN-Cache value.
func cacheFlag(st int) string {
	switch st {
	case cacheHit:
		return "HIT"
	case cacheStale:
		return "STALE"
	case cacheMiss:
		return "MISS"
	}
	return "BYPASS"
}
