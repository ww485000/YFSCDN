package engine

import (
	"bytes"
	"io"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"edgecdn/edge/internal/contract"
	fcgi "github.com/iwind/gofcgi/pkg/fcgi"
)

// fastcgiBackend serves a location via FastCGI (php-fpm etc.).
// A shared connection pool is kept per backend address (gofcgi).
type fastcgiBackend struct {
	addr    string
	network string // tcp | unix
	index   string
	params  map[string]string
	pool    *fcgi.Pool
}

func newFastCGIBackend(f *contract.FastCGI) *fastcgiBackend {
	network := "tcp"
	if strings.HasPrefix(f.Addr, "/") {
		network = "unix"
	}
	idx := f.Index
	if idx == "" {
		idx = "index.php"
	}
	return &fastcgiBackend{
		addr:   f.Addr,
		network: network,
		index:  idx,
		params: f.Params,
		pool:   fcgi.SharedPool(network, f.Addr, 8),
	}
}

func (fb *fastcgiBackend) serve(w http.ResponseWriter, r *http.Request) {
	client, err := fb.pool.Client()
	if err != nil {
		log.Printf("[engine] fastcgi %s: no client: %v", fb.addr, err)
		serveError(w, r, http.StatusBadGateway, "fastcgi backend unavailable", nil)
		return
	}

	req := fcgi.NewRequest()
	req.SetTimeout(30 * time.Second)
	req.SetParams(fb.buildParams(r))
	if r.Body != nil && r.ContentLength > 0 {
		body, err := io.ReadAll(io.LimitReader(r.Body, 64<<20))
		if err != nil {
			serveError(w, r, http.StatusBadGateway, "bad request body", nil)
			return
		}
		req.SetBody(bytes.NewReader(body), uint32(len(body)))
	}

	resp, stderr, err := client.Call(req)
	if err != nil {
		log.Printf("[engine] fastcgi %s: %v (stderr: %s)", fb.addr, err, string(stderr))
		serveError(w, r, http.StatusBadGateway, "fastcgi error", nil)
		return
	}
	defer resp.Body.Close()
	copyHeader(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

// buildParams composes standard CGI + FastCGI params (nginx fastcgi_params).
func (fb *fastcgiBackend) buildParams(r *http.Request) map[string]string {
	host := r.Host
	serverName := host
	serverPort := "80"
	if h, p, err := net.SplitHostPort(host); err == nil {
		serverName, serverPort = h, p
	}
	remoteAddr := r.RemoteAddr
	remoteIP, remotePort := remoteAddr, "0"
	if h, p, err := net.SplitHostPort(remoteAddr); err == nil {
		remoteIP, remotePort = h, p
	}
	remoteIP = clientIP(r)

	path := r.URL.Path
	isPHP := strings.HasSuffix(path, ".php")
	scriptName := path
	scriptFile := path
	if !isPHP {
		scriptName = "/" + fb.index
		scriptFile = scriptName
	}

	p := map[string]string{
		"SERVER_SOFTWARE":   "edgecdn/edge",
		"GATEWAY_INTERFACE": "CGI/1.1",
		"SERVER_PROTOCOL":   r.Proto,
		"SCRIPT_NAME":       scriptName,
		"SCRIPT_FILENAME":   scriptFile,
		"REQUEST_URI":       r.RequestURI,
		"REQUEST_METHOD":    r.Method,
		"QUERY_STRING":      r.URL.RawQuery,
		"DOCUMENT_ROOT":     "/",
		"SERVER_NAME":       serverName,
		"SERVER_ADDR":       serverName,
		"SERVER_PORT":       serverPort,
		"REMOTE_ADDR":       remoteIP,
		"REMOTE_PORT":       remotePort,
		"REDIRECT_STATUS":   "200",
	}
	if r.TLS != nil {
		p["HTTPS"] = "on"
	}
	if r.ContentLength > 0 {
		p["CONTENT_LENGTH"] = strconv.FormatInt(r.ContentLength, 10)
		if ct := r.Header.Get("Content-Type"); ct != "" {
			p["CONTENT_TYPE"] = ct
		}
	}
	// HTTP_* request headers
	rep := strings.NewReplacer("-", "_")
	for k, vv := range r.Header {
		switch strings.ToLower(k) {
		case "content-type", "content-length", "host":
			continue
		}
		p["HTTP_"+strings.ToUpper(rep.Replace(k))] = strings.Join(vv, ", ")
	}
	// user-configured extra params win
	for k, v := range fb.params {
		p[k] = v
	}
	return p
}

// copyHeader copies response headers (skipping hop-by-hop ones).
func copyHeader(dst, src http.Header) {
	for k, vv := range src {
		switch strings.ToLower(k) {
		case "connection", "keep-alive", "proxy-authenticate", "proxy-authorization",
			"te", "trailer", "transfer-encoding", "upgrade", "content-length":
			continue
		}
		for _, v := range vv {
			dst.Add(k, v)
		}
	}
}
