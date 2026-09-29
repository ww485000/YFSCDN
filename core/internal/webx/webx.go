// Package webx is a tiny HTTP router + response helpers built on net/http.
// Pattern syntax: "/api/v1/admin/sites/:id" — ":name" captures one path segment.
package webx

import (
	"log"
	"net/http"
	"strings"
)

// AuthInfo is implemented by auth claims; keeps webx free of authx imports.
type AuthInfo interface {
	UserID() int64
	UserName() string
	Role() string // "admin" | "tenant"
	TenantID() int64
}

// Handler processes a request.
type Handler func(c *Context)

// Context is passed to handlers.
type Context struct {
	R      *http.Request
	W      http.ResponseWriter
	Params map[string]string
	User   AuthInfo // nil for unauthenticated public requests
}

type route struct {
	parts   []string
	params  []string
	handler Handler
}

// ScopeGuard maps a path to the required role class: "admin" or "tenant".
type ScopeGuard func(path string) (want string, ok bool)

// Router matches requests by method + path pattern.
type Router struct {
	routes     map[string][]*route
	auth       func(token string) (AuthInfo, error)
	scopeGuard ScopeGuard
	prefix     []string // public path prefixes (no token required)
}

func New() *Router {
	return &Router{routes: map[string][]*route{}}
}

// SetAuth installs the token resolver; nil disables auth checks.
func (rt *Router) SetAuth(f func(token string) (AuthInfo, error)) { rt.auth = f }

// SetScopeGuard installs a role-class guard (e.g. /api/v1/admin/* requires an admin).
func (rt *Router) SetScopeGuard(g ScopeGuard) { rt.scopeGuard = g }

// Public marks path prefixes as not requiring a token.
func (rt *Router) Public(prefixes ...string) { rt.prefix = append(rt.prefix, prefixes...) }

func (rt *Router) Handle(method, pattern string, h Handler) {
	parts := splitPath(pattern)
	params := []string{}
	for _, p := range parts {
		if strings.HasPrefix(p, ":") {
			params = append(params, p[1:])
		}
	}
	rt.routes[method] = append(rt.routes[method], &route{parts: parts, params: params, handler: h})
}

func (rt *Router) GET(p string, h Handler)    { rt.Handle(http.MethodGet, p, h) }
func (rt *Router) POST(p string, h Handler)   { rt.Handle(http.MethodPost, p, h) }
func (rt *Router) PUT(p string, h Handler)    { rt.Handle(http.MethodPut, p, h) }
func (rt *Router) DELETE(p string, h Handler) { rt.Handle(http.MethodDelete, p, h) }

func (rt *Router) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// recover from panics
	defer func() {
		if e := recover(); e != nil {
			log.Printf("[webx] panic on %s %s: %v", r.Method, r.URL.Path, e)
			Fail(w, http.StatusInternalServerError, 500, "internal error")
		}
	}()
	// CORS
	co := "*"
	if r.Header.Get("Origin") != "" {
		w.Header().Set("Access-Control-Allow-Origin", co)
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,PUT,DELETE,OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type,Authorization,X-Node-Token")
	}
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	ctx := &Context{R: r, W: w, Params: map[string]string{}}
	// token resolution
	if rt.auth != nil {
		tok := bearerToken(r)
		if tok != "" {
			info, err := rt.auth(tok)
			if err != nil {
				Fail(w, http.StatusUnauthorized, 401, "invalid or expired token")
				return
			}
			ctx.User = info
		} else if !rt.isPublic(r.URL.Path) {
			Fail(w, http.StatusUnauthorized, 401, "missing token")
			return
		}
		// role-class guard (JWT role: "admin" = operator, "tenant" = tenant user)
		if rt.scopeGuard != nil && ctx.User != nil {
			if want, ok := rt.scopeGuard(r.URL.Path); ok && want != "" {
				if want == "admin" && ctx.User.Role() == "tenant" {
					Fail(w, http.StatusForbidden, 403, "tenant token cannot call admin API")
					return
				}
				if want == "tenant" && ctx.User.Role() != "tenant" {
					Fail(w, http.StatusForbidden, 403, "admin token cannot call user API")
					return
				}
			}
		}
	}

	for _, rt2 := range rt.routes[r.Method] {
		if params, ok := match(rt2.parts, splitPath(r.URL.Path)); ok {
			ctx.Params = params
			rt2.handler(ctx)
			return
		}
	}
	Fail(w, http.StatusNotFound, 404, "not found: "+r.Method+" "+r.URL.Path)
}

func (rt *Router) isPublic(p string) bool {
	for _, pre := range rt.prefix {
		if p == pre || strings.HasPrefix(p, pre+"/") || strings.HasPrefix(p, pre) {
			return true
		}
	}
	return false
}

func match(parts, segs []string) (map[string]string, bool) {
	if len(parts) != len(segs) {
		return nil, false
	}
	params := map[string]string{}
	for i, p := range parts {
		if strings.HasPrefix(p, ":") {
			if segs[i] == "" {
				return nil, false
			}
			params[p[1:]] = segs[i]
		} else if p != segs[i] {
			return nil, false
		}
	}
	return params, true
}

func splitPath(p string) []string {
	p = strings.Trim(p, "/")
	if p == "" {
		return []string{}
	}
	return strings.Split(p, "/")
}

func bearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	const pre = "Bearer "
	if len(h) > len(pre) && strings.EqualFold(h[:len(pre)], pre) {
		return h[len(pre):]
	}
	return ""
}
