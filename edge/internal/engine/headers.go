package engine

import (
	"net/http"
	"strings"

	"edgecdn/edge/internal/contract"
)

// headerPolicy compiles downstream/upstream header ops + CORS.
type headerPolicy struct {
	down   []contract.HeaderOp
	up     []contract.HeaderOp
	cors   *contract.CORS
}

func compileHeaders(h *contract.Headers) *headerPolicy {
	if h == nil {
		return nil
	}
	return &headerPolicy{down: h.Downstream, up: h.Upstream, cors: h.CORS}
}

// applyUp mutates the proxied request's headers.
func (hp *headerPolicy) applyUp(r *http.Request) {
	if hp == nil {
		return
	}
	for i := range hp.up {
		op := &hp.up[i]
		switch op.Op {
		case "add", "set":
			if op.Name != "" {
				r.Header.Set(op.Name, op.Value)
			}
		case "delete":
			if op.Name != "" {
				r.Header.Del(op.Name)
			}
		}
	}
}

// applyDown mutates the response headers before they reach the client.
func (hp *headerPolicy) applyDown(h http.Header) {
	if hp == nil {
		return
	}
	for i := range hp.down {
		op := &hp.down[i]
		if op.Name == "" {
			continue
		}
		switch op.Op {
		case "add":
			h.Add(op.Name, op.Value)
		case "set":
			h.Set(op.Name, op.Value)
		case "delete":
			h.Del(op.Name)
		}
	}
}

// applyCORS adds CORS headers to the response and handles preflight.
// Returns true when the request was a handled preflight (caller must stop).
func (hp *headerPolicy) applyCORS(w http.ResponseWriter, r *http.Request) bool {
	if hp == nil || hp.cors == nil || !hp.cors.Enabled {
		return false
	}
	c := hp.cors
	origin := r.Header.Get("Origin")
	if origin == "" {
		return false
	}
	ao := resolveOrigin(c.AllowOrigins, origin)
	if ao == "" {
		return false
	}
	h := w.Header()
	h.Set("Access-Control-Allow-Origin", ao)
	if ao != "*" {
		h.Set("Vary", "Origin")
	}
	if len(c.AllowMethods) > 0 {
		h.Set("Access-Control-Allow-Methods", strings.Join(c.AllowMethods, ", "))
	}
	if len(c.AllowHeaders) > 0 {
		h.Set("Access-Control-Allow-Headers", strings.Join(c.AllowHeaders, ", "))
	}
	if c.MaxAge > 0 {
		h.Set("Access-Control-Max-Age", itoa(c.MaxAge))
	}
	if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
		w.WriteHeader(http.StatusNoContent)
		return true
	}
	return false
}

func resolveOrigin(allow []string, origin string) string {
	for _, a := range allow {
		a = strings.TrimSpace(a)
		if a == "" {
			continue
		}
		if a == "*" || a == origin {
			if a == "*" {
				return "*"
			}
			return a
		}
		// wildcard suffix match: *.example.com vs sub.example.com
		if strings.HasPrefix(a, "*.") {
			suffix := a[1:] // ".example.com"
			if strings.HasSuffix(origin, suffix) {
				return origin
			}
		}
	}
	return ""
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
