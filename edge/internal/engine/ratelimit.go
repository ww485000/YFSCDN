package engine

import (
	"bufio"
	"errors"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"edgecdn/edge/internal/contract"
)

// ccLimiter is a fixed-window request limiter per client IP.
type ccLimiter struct {
	interval time.Duration
	limit    int
	mu       sync.Mutex
	buckets  map[string]*ccBucket
	stop     chan struct{}
}

type ccBucket struct {
	windowStart time.Time
	count       int
}

func newCClimiter(r *contract.RequestLimit) *ccLimiter {
	interval := time.Duration(r.Interval) * time.Second
	if interval <= 0 {
		interval = time.Minute
	}
	limit := r.Limit
	if limit <= 0 {
		limit = 1000
	}
	cl := &ccLimiter{interval: interval, limit: limit, buckets: map[string]*ccBucket{}}
	cl.stop = make(chan struct{})
	go cl.gc()
	return cl
}

func (cl *ccLimiter) gc() {
	ticker := time.NewTicker(cl.interval * 2)
	defer ticker.Stop()
	for {
		select {
		case <-cl.stop:
			return
		case <-ticker.C:
			cl.mu.Lock()
			now := time.Now()
			for k, b := range cl.buckets {
				if now.Sub(b.windowStart) > cl.interval*2 {
					delete(cl.buckets, k)
				}
			}
			cl.mu.Unlock()
		}
	}
}

// allow decides if the request passes; true = allowed.
func (cl *ccLimiter) allow(ip string) bool {
	cl.mu.Lock()
	defer cl.mu.Unlock()
	now := time.Now()
	b := cl.buckets[ip]
	if b == nil || now.Sub(b.windowStart) >= cl.interval {
		cl.buckets[ip] = &ccBucket{windowStart: now, count: 1}
		return true
	}
	b.count++
	return b.count <= cl.limit
}

func (cl *ccLimiter) stopGC() {
	select {
	case <-cl.stop:
	default:
		close(cl.stop)
	}
}

// trafficLimiter caps total bytes + concurrent connections for a site.
type trafficLimiter struct {
	maxBytes int64
	maxConn  int64
	bytes    atomic.Int64
	conns    atomic.Int64
}

func newTrafficLimiter(maxBytes int64, maxConn int) *trafficLimiter {
	return &trafficLimiter{maxBytes: maxBytes, maxConn: int64(maxConn)}
}

// enter reserves one connection; false when over the limit.
func (tl *trafficLimiter) enter() bool {
	if tl.maxConn > 0 {
		if tl.conns.Add(1) > tl.maxConn {
			tl.conns.Add(-1)
			return false
		}
	} else {
		tl.conns.Add(1)
	}
	return true
}

func (tl *trafficLimiter) leave() {
	tl.conns.Add(-1)
}

// overBytes reports when the site's total traffic exceeded the cap.
func (tl *trafficLimiter) overBytes() bool {
	return tl.maxBytes > 0 && tl.bytes.Load() >= tl.maxBytes
}

func (tl *trafficLimiter) addBytes(n int64) {
	if n > 0 {
		tl.bytes.Add(n)
	}
}

// statusWriter wraps ResponseWriter to count bytes written.
type statusWriter struct {
	http.ResponseWriter
	status int
	bytes  int64
}

func newStatusWriter(w http.ResponseWriter) *statusWriter {
	return &statusWriter{ResponseWriter: w, status: http.StatusOK}
}

func (sw *statusWriter) WriteHeader(code int) {
	if sw.status == 0 {
		sw.status = code
	}
	sw.ResponseWriter.WriteHeader(code)
}

func (sw *statusWriter) Write(b []byte) (int, error) {
	n, err := sw.ResponseWriter.Write(b)
	sw.bytes += int64(n)
	return n, err
}

// Flush support (pass-through when available).
func (sw *statusWriter) Flush() {
	if f, ok := sw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Hijack support (websocket) pass-through.
func (sw *statusWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if h, ok := sw.ResponseWriter.(http.Hijacker); ok {
		return h.Hijack()
	}
	return nil, nil, errNotHijacker
}

var errNotHijacker = errors.New("response writer does not support hijacking")
