// Package engine is the pure-Go data plane: protocol listeners (HTTP/HTTPS/TCP/UDP),
// SNI + Host site routing, and the full GoEdge feature pipeline per site/location.
package engine

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"log"
	"math/big"
	"net"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"edgecdn/edge/internal/contract"
	"edgecdn/edge/internal/stats"
)

// Engine is the data-plane core. It owns listeners and the applied sites.
type Engine struct {
	dataDir     string
	defaultHTTP string // fallback HTTP listen addr (edge config)
	defaultTLS  string // fallback HTTPS listen addr (edge config)
	stats       *stats.Stats
	logWriter   *AccessLogWriter

	mu      sync.RWMutex
	sites   map[int64]*site
	byHost  map[string]*site
	certs   map[string]*tls.Certificate // sni -> cert
	fallbackCert *tls.Certificate       // self-signed, served for unknown SNI
	httpSrv map[string]*httpServer
	tcp     map[string]*rawProxy
	udp     map[string]*rawProxy
}

type httpServer struct {
	addr    string
	tls     bool
	srv     *http.Server
	engine  *Engine
	closing bool
}

type ctxKey int

const listenerAddrKey ctxKey = 1

// New creates an engine (not started).
func New(dataDir, defaultHTTP, defaultTLS string, st *stats.Stats, logs *AccessLogWriter) *Engine {
	e := &Engine{
		dataDir:     dataDir,
		defaultHTTP: defaultHTTP,
		defaultTLS:  defaultTLS,
		stats:       st,
		logWriter:   logs,
		sites:       map[int64]*site{},
		byHost:      map[string]*site{},
		certs:       map[string]*tls.Certificate{},
		httpSrv:     map[string]*httpServer{},
		tcp:         map[string]*rawProxy{},
		udp:         map[string]*rawProxy{},
	}
	e.fallbackCert = newFallbackCert()
	return e
}

// Start launches the fallback HTTP/HTTPS listeners so the edge answers
// even before the first site arrives.
func (e *Engine) Start() error {
	if e.defaultHTTP != "" && e.defaultHTTP != "off" {
		if err := e.ensureHTTP(e.defaultHTTP, false); err != nil {
			return err
		}
	}
	if e.defaultTLS != "" && e.defaultTLS != "off" {
		if err := e.ensureHTTP(e.defaultTLS, true); err != nil {
			return err
		}
	}
	return nil
}

// Stop closes everything.
func (e *Engine) Stop() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, hs := range e.httpSrv {
		if hs.srv != nil {
			_ = hs.srv.Close()
		}
	}
	e.httpSrv = map[string]*httpServer{}
	for k, rp := range e.tcp {
		rp.close()
		delete(e.tcp, k)
	}
	for k, rp := range e.udp {
		rp.close()
		delete(e.udp, k)
	}
	return nil
}

// ensureHTTP starts (or reuses) an HTTP/HTTPS listener at addr.
func (e *Engine) ensureHTTP(addr string, isTLS bool) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if _, ok := e.httpSrv[addr]; ok {
		return nil
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", addr, err)
	}
	hs := &httpServer{addr: addr, tls: isTLS, engine: e}
	if isTLS {
		hs.srv = &http.Server{
			Addr:    addr,
			Handler: e.handlerFor(addr),
			TLSConfig: &tls.Config{
				GetCertificate: e.getCertificate,
				MinVersion:     tls.VersionTLS12,
			},
		}
		go func() {
			log.Printf("[engine] https listening on %s", addr)
			if err := hs.srv.ServeTLS(ln, "", ""); err != nil && err != http.ErrServerClosed {
				log.Printf("[engine] https %s stopped: %v", addr, err)
			}
		}()
	} else {
		hs.srv = &http.Server{Addr: addr, Handler: e.handlerFor(addr)}
		go func() {
			log.Printf("[engine] http listening on %s", addr)
			if err := hs.srv.Serve(ln); err != nil && err != http.ErrServerClosed {
				log.Printf("[engine] http %s stopped: %v", addr, err)
			}
		}()
	}
	e.httpSrv[addr] = hs
	return nil
}

// handlerFor wraps the request with its listener address (for follow_protocol).
func (e *Engine) handlerFor(addr string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(context.WithValue(r.Context(), listenerAddrKey, addr))
		e.ServeHTTP(w, r)
	})
}

// getCertificate picks the SNI cert (exact, then wildcard, then self-signed fallback).
func (e *Engine) getCertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	host := strings.ToLower(hello.ServerName)
	e.mu.RLock()
	defer e.mu.RUnlock()
	if host != "" {
		if c, ok := e.certs[host]; ok {
			return c, nil
		}
		if i := strings.IndexByte(host, '.'); i > 0 {
			if c, ok := e.certs["*."+host[i+1:]]; ok {
				return c, nil
			}
		}
	}
	// Unknown SNI (or none): serve the self-signed fallback so the TLS
	// listener always answers; the edge still routes by Host/SNI.
	if e.fallbackCert != nil {
		return e.fallbackCert, nil
	}
	return nil, fmt.Errorf("no certificate for %q", host)
}

// newFallbackCert builds a throwaway self-signed cert (edge default CA).
func newFallbackCert() *tls.Certificate {
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "edgecdn-fallback"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(3650 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return nil
	}
	return &tls.Certificate{
		Certificate: [][]byte{der},
		PrivateKey:  key,
	}
}

// route finds the site for a request (SNI first, then Host).
func (e *Engine) route(r *http.Request) *site {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if r.TLS != nil && r.TLS.ServerName != "" {
		if s, ok := e.byHost[strings.ToLower(r.TLS.ServerName)]; ok {
			return s
		}
	}
	host := r.Host
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	if s, ok := e.byHost[strings.ToLower(host)]; ok {
		return s
	}
	return nil
}

// ServeHTTP routes one HTTP(S) request to its site pipeline.
func (e *Engine) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s := e.route(r)
	if s == nil {
		http.Error(w, "no site configured for host "+r.Host, http.StatusNotFound)
		return
	}
	s.handle(w, r)
}

// schedOf dereferences the optional scheduling pointer.
func schedOf(s *contract.Scheduling) contract.Scheduling {
	if s == nil {
		return contract.Scheduling{}
	}
	return *s
}

// ApplySpec installs (or replaces) a site and (re)starts its listeners.
func (e *Engine) ApplySpec(spec contract.Spec) error {
	cfg := spec.Site
	cfg.EnsureDefaults()

	// Compile listeners plan.
	plan := listenerPlan{}
	hasHTTPS := false
	for _, ln := range cfg.Listeners {
		switch ln.Protocol {
		case "http", "https":
			plan.http = append(plan.http, ln)
			if ln.Protocol == "https" {
				hasHTTPS = true
			}
		case "tcp":
			plan.raw = append(plan.raw, rawPlan{proto: "tcp", listen: ln.Listen, siteID: cfg.ID, origins: ln.Origins, sched: schedOf(ln.Scheduling)})
		case "udp":
			plan.raw = append(plan.raw, rawPlan{proto: "udp", listen: ln.Listen, siteID: cfg.ID, origins: ln.Origins, sched: schedOf(ln.Scheduling)})
		}
	}
	_ = hasHTTPS

	// Build the site object.
	st, err := compileSite(spec, e.dataDir, e.stats, e.logWriter)
	if err != nil {
		return err
	}

	e.mu.Lock()
	// Drop old registrations.
	if old := e.sites[cfg.ID]; old != nil {
		for _, d := range old.domains() {
			if e.byHost[d] == old {
				delete(e.byHost, d)
			}
		}
		delete(e.sites, cfg.ID)
	}
	// Register.
	e.sites[cfg.ID] = st
	for _, d := range st.domains() {
		e.byHost[d] = st
	}
	for _, dc := range spec.Certs {
		if dc.Cert == "" || dc.Key == "" {
			continue
		}
		if c, err := tls.X509KeyPair([]byte(dc.Cert), []byte(dc.Key)); err == nil {
			dom := strings.ToLower(dc.Domain)
			if _, ok := e.certs[dom]; !ok {
				e.certs[dom] = &c
			}
		} else {
			log.Printf("[engine] site %d: bad cert for %s: %v", cfg.ID, dc.Domain, err)
		}
	}
	if cfg.TLS != nil && cfg.TLS.Cert != "" && cfg.TLS.Key != "" {
		if c, err := tls.X509KeyPair([]byte(cfg.TLS.Cert), []byte(cfg.TLS.Key)); err == nil {
			for _, d := range st.domains() {
				if _, ok := e.certs[d]; !ok {
					e.certs[d] = &c
				}
			}
		}
	}
	e.mu.Unlock()

	// Start HTTP(S) listeners (idempotent; error if addr taken by OS/other process).
	for _, ln := range cfg.Listeners {
		if ln.Protocol == "http" || ln.Protocol == "https" {
			if err := e.ensureHTTP(ln.Listen, ln.Protocol == "https"); err != nil {
				log.Printf("[engine] site %d: listener %s %s: %v (site active for other addrs)", cfg.ID, ln.Protocol, ln.Listen, err)
			}
		}
	}
	// TCP/UDP raw listeners.
	for _, rp := range plan.raw {
		if err := e.applyRaw(rp); err != nil {
			log.Printf("[engine] site %d: %s listener %s: %v", cfg.ID, rp.proto, rp.listen, err)
		}
	}
	log.Printf("[engine] applied site %d %s (listeners=%d locations=%d)", cfg.ID, cfg.PrimaryDomain(), len(cfg.Listeners), 1+len(cfg.Locations))
	return nil
}

// RemoveSite drops a site and its exclusive raw listeners.
func (e *Engine) RemoveSite(id int64) error {
	e.mu.Lock()
	st := e.sites[id]
	if st != nil {
		for _, d := range st.domains() {
			if e.byHost[d] == st {
				delete(e.byHost, d)
			}
			// certs are shared; leave them (harmless) or drop if no other site uses.
		}
		delete(e.sites, id)
	}
	// Close raw listeners whose owner is this site.
	for k, rp := range e.tcp {
		if rp.siteID == id {
			go rp.close()
			delete(e.tcp, k)
		}
	}
	for k, rp := range e.udp {
		if rp.siteID == id {
			go rp.close()
			delete(e.udp, k)
		}
	}
	e.mu.Unlock()
	log.Printf("[engine] removed site %d", id)
	return nil
}

// applyRaw starts or re-points a raw (tcp/udp) listener.
func (e *Engine) applyRaw(rp rawPlan) error {
	key := rp.proto + "|" + strings.ToLower(rp.listen)
	e.mu.Lock()
	defer e.mu.Unlock()
	old := e.tcp[rp.proto+"|"+strings.ToLower(rp.listen)]
	if rp.proto == "udp" {
		old = e.udp[rp.proto+"|"+strings.ToLower(rp.listen)]
	}
	if old != nil && old.siteID != rp.siteID {
		return fmt.Errorf("addr %s already used by another site", rp.listen)
	}
	if old != nil {
		old.update(rp.origins, rp.sched)
		return nil
	}
	lst, err := newRawProxy(rp.proto, rp.listen, rp.siteID, rp.origins, rp.sched)
	if err != nil {
		return err
	}
	if err := lst.start(); err != nil {
		return err
	}
	if rp.proto == "udp" {
		e.udp[key] = lst
	} else {
		e.tcp[key] = lst
	}
	log.Printf("[engine] %s proxy %s -> %v", rp.proto, rp.listen, rp.origins)
	return nil
}

// DomainsForSite lists a site's domains (for tests/debug).
func (e *Engine) DomainsForSite(id int64) []string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if st := e.sites[id]; st != nil {
		out := st.domains()
		sort.Strings(out)
		return out
	}
	return nil
}

type listenerPlan struct {
	http []contract.Listener
	raw  []rawPlan
}
