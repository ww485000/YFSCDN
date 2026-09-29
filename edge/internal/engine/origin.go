package engine

import (
	"fmt"
	"log"
	"math/rand"
	"net"
	"net/http"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"edgecdn/edge/internal/contract"
)

// originState is one origin with live health status.
type originState struct {
	cfg      contract.Origin
	up       atomic.Bool
	fails    atomic.Int32
}

// originGroup holds the origins of one reverse_proxy block.
type originGroup struct {
	cfg     *contract.ReverseProxy
	states  []*originState
	rr      atomic.Uint64
	hashKey string // header or arg name for hash scheduling
	mu      sync.Mutex
	stopHC  chan struct{}
}

func newOriginGroup(rp *contract.ReverseProxy) (*originGroup, error) {
	if rp == nil || len(rp.Origins) == 0 {
		return nil, fmt.Errorf("no origins")
	}
	og := &originGroup{cfg: rp}
	var sched *contract.Scheduling
	if rp.Scheduling != nil {
		sched = rp.Scheduling
		switch sched.Type {
		case "", "random", "round_robin", "hash", "sticky":
		default:
			return nil, fmt.Errorf("unsupported scheduling %q", sched.Type)
		}
		if sched.Type == "hash" {
			og.hashKey = sched.Key
		}
	}
	for i := range rp.Origins {
		o := &rp.Origins[i]
		if o.Status == "off" {
			continue
		}
		if !strings.Contains(o.Addr, ":") {
			host := o.Addr
			port := 80
			if o.SSL {
				port = 443
			}
			o.Addr = net.JoinHostPort(host, fmt.Sprintf("%d", port))
		}
		st := &originState{cfg: *o}
		st.up.Store(true)
		og.states = append(og.states, st)
	}
	if len(og.states) == 0 {
		return nil, fmt.Errorf("all origins disabled")
	}
	return og, nil
}

func (g *originGroup) pick(r *http.Request) *originState {
	up := make([]*originState, 0, len(g.states))
	for _, st := range g.states {
		if st.up.Load() {
			up = append(up, st)
		}
	}
	if len(up) == 0 {
		// all unhealthy -> fall back to first (fail-open)
		return g.states[0]
	}
	stype := ""
	if g.cfg.Scheduling != nil {
		stype = g.cfg.Scheduling.Type
	}
	switch stype {
	case "round_robin":
		i := g.rr.Add(1) - 1
		return up[i%uint64(len(up))]
	case "hash", "sticky": // sticky ≈ ip-hash (session affinity without cookies)
		v := hashValue(r, g.hashKey)
		if v == "" {
			return up[rand.Intn(len(up))]
		}
		return up[fnvHash(v)%uint64(len(up))]
	default: // random
		return up[rand.Intn(len(up))]
	}
}

func hashValue(r *http.Request, key string) string {
	if key == "" {
		// default hash key = client IP
		return clientIP(r)
	}
	if strings.EqualFold(key, "ip") {
		return clientIP(r)
	}
	if i := strings.IndexByte(key, ':'); i > 0 {
		switch strings.ToLower(key[:i]) {
		case "cookie":
			if c, err := r.Cookie(key[i+1:]); err == nil {
				return c.Value
			}
			return ""
		case "header":
			return r.Header.Get(key[i+1:])
		}
	}
	return key // plain string key = constant hash source
}

func fnvHash(s string) uint64 {
	var h uint64 = 1469598103934665603
	for i := 0; i < len(s); i++ {
		h ^= uint64(s[i])
		h *= 1099511628211
	}
	return h
}

// startHealthCheck polls the origins in the background.
func (g *originGroup) startHealthCheck() {
	hc := g.cfg.HealthCheck
	if hc == nil {
		return
	}
	interval := time.Duration(hc.Interval) * time.Second
	if interval < time.Second {
		interval = time.Second
	}
	timeout := time.Duration(hc.Timeout) * time.Second
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	path := hc.Path
	if path == "" {
		path = "/"
	}
	failNeed := hc.Failures
	if failNeed < 1 {
		failNeed = 2
	}
	okNeed := hc.Successes
	if okNeed < 1 {
		okNeed = 1
	}

	g.mu.Lock()
	if g.stopHC != nil {
		g.mu.Unlock()
		return
	}
	g.stopHC = make(chan struct{})
	stop := g.stopHC
	g.mu.Unlock()

	client := &http.Client{Timeout: timeout}
	probe := func(st *originState) bool {
		scheme := "http"
		if st.cfg.SSL {
			scheme = "https"
		}
		u := scheme + "://" + st.cfg.Addr + path
		req, err := http.NewRequest(http.MethodGet, u, nil)
		if err != nil {
			return false
		}
		req.Header.Set("Host", st.cfg.Addr)
		resp, err := client.Do(req)
		if err != nil {
			return false
		}
		defer resp.Body.Close()
		return resp.StatusCode < 500
	}

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		oks := map[int]int{}
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				for i, st := range g.states {
					if st.cfg.Status == "off" {
						continue
					}
					if probe(st) {
						n := oks[i] + 1
						oks[i] = n
						st.fails.Store(0)
						if !st.up.Load() && n >= okNeed {
							st.up.Store(true)
							log.Printf("[engine] origin %s is up", st.cfg.Addr)
						}
					} else {
						oks[i] = 0
						f := st.fails.Add(1)
						if st.up.Load() && int(f) >= failNeed {
							st.up.Store(false)
							log.Printf("[engine] origin %s marked down (%d fails)", st.cfg.Addr, f)
						}
					}
				}
			}
		}
	}()
}

// StopHealthCheck cancels the health-check loop (on site removal).
func (g *originGroup) StopHealthCheck() {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.stopHC != nil {
		close(g.stopHC)
		g.stopHC = nil
	}
}

// String renders the origin list (debug).
func (g *originGroup) String() string {
	var b []string
	for _, st := range g.states {
		b = append(b, st.cfg.Addr)
	}
	sort.Strings(b)
	return strings.Join(b, ",")
}

// clientIP extracts the client IP (X-Forwarded-For first, then RemoteAddr).
func clientIP(r *http.Request) string {
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		if i := strings.IndexByte(fwd, ','); i >= 0 {
			fwd = fwd[:i]
		}
		return strings.TrimSpace(fwd)
	}
	if h, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return h
	}
	return r.RemoteAddr
}
