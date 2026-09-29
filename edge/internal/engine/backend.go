package engine

import (
	"context"
	"io"
	"mime"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

// proxy forwards the request to the picked origin.
func (cl *compiledLoc) proxy(fw *finalWriter, r *http.Request) {
	o := cl.origins.pick(r)
	scheme := "http"
	if o.cfg.SSL {
		scheme = "https"
	}
	target := &url.URL{Scheme: scheme, Host: o.cfg.Addr}

	ctx := r.Context()
	if cl.loc.ReverseProxy != nil && cl.loc.ReverseProxy.TimeoutSec > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(cl.loc.ReverseProxy.TimeoutSec)*time.Second)
		defer cancel()
	}

	proxy := &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			req.URL.Scheme = target.Scheme
			req.URL.Host = target.Host
			req.Host = target.Host
			if cl.headers != nil {
				cl.headers.applyUp(req)
			}
		},
		Transport: proxyTransport,
		ModifyResponse: func(resp *http.Response) error {
			// GoEdge 404-page semantics: replace error bodies with the site's
			// custom page (status code is preserved).
			if cl.pages == nil || resp.StatusCode < 400 {
				return nil
			}
			page, ok := cl.pages[resp.StatusCode]
			if !ok || page == nil || page.Content == "" {
				return nil
			}
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
			_ = resp.Body.Close()
			body := page.Content
			resp.Body = io.NopCloser(strings.NewReader(body))
			resp.ContentLength = int64(len(body))
			resp.Header.Set("Content-Type", "text/html; charset=utf-8")
			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, req *http.Request, err error) {
			status := http.StatusBadGateway
			if ctx.Err() == context.DeadlineExceeded {
				status = http.StatusGatewayTimeout
			}
			_ = err
			serveError(w, req, status, "upstream error", cl.pages)
		},
	}
	proxy.ServeHTTP(fw, r.WithContext(ctx))
}

// staticServe serves files from cl.root (index support, traversal guard).
func (cl *compiledLoc) staticServe(fw *finalWriter, r *http.Request) {
	root := cl.root
	upath := path.Clean("/" + strings.TrimPrefix(r.URL.Path, "/"))
	fp := filepath.Join(root, filepath.FromSlash(upath))

	// path traversal guard
	rel, err := filepath.Rel(root, fp)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		serveError(fw, r, http.StatusForbidden, "forbidden", cl.pages)
		return
	}

	fi, err := os.Stat(fp)
	if err != nil {
		serveError(fw, r, http.StatusNotFound, "not found", cl.pages)
		return
	}
	if fi.IsDir() {
		served := false
		for _, idx := range cl.index {
			ip := filepath.Join(fp, idx)
			if ifi, err := os.Stat(ip); err == nil && !ifi.IsDir() {
				fp = ip
				fw.inner.Header().Set("Content-Type", mime.TypeByExtension(".html"))
				served = true
				break
			}
		}
		if !served {
			serveError(fw, r, http.StatusNotFound, "not found", cl.pages)
			return
		}
	}
	f, err := os.Open(fp)
	if err != nil {
		serveError(fw, r, http.StatusNotFound, "not found", cl.pages)
		return
	}
	defer f.Close()
	name := filepath.Base(fp)
	http.ServeContent(fw, r, name, fi.ModTime(), f)
}
