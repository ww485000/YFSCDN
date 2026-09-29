package site

import (
	"fmt"

	"edgecdn/core/internal/logs"
	"edgecdn/core/internal/tenant"
	"edgecdn/core/internal/usage"
	"edgecdn/core/internal/webx"
)

// Admin handles operator-side site routes (cross-tenant).
type Admin struct {
	svc     *Store
	tenants *tenant.Store
	usage   *usage.Store
	logs    *logs.Store
}

// RegisterAdmin mounts operator site routes.
func RegisterAdmin(r *webx.Router, svc *Store, tenants *tenant.Store, u *usage.Store, l *logs.Store) {
	h := &Admin{svc: svc, tenants: tenants, usage: u, logs: l}
	r.GET("/api/v1/admin/sites", h.list)
	r.POST("/api/v1/admin/sites", h.create)
	r.GET("/api/v1/admin/sites/:id", h.get)
	r.PUT("/api/v1/admin/sites/:id", h.update)
	r.DELETE("/api/v1/admin/sites/:id", h.delete)
	r.POST("/api/v1/admin/sites/:id/status", h.status)
	r.GET("/api/v1/admin/sites/:id/usage", h.usageSeries)
	r.GET("/api/v1/admin/sites/:id/stats", h.stats)
	r.GET("/api/v1/admin/sites/:id/logs", h.accessLogs)
	r.GET("/api/v1/admin/sites/:id/logs/status-dist", h.statusDist)
}

func (h *Admin) list(c *webx.Context) {
	out, total, err := h.svc.List(webx.QueryInt64(c, "tenant_id", 0), webx.QueryStr(c, "keyword"),
		webx.QueryInt(c, "page", 1), webx.QueryInt(c, "size", 20))
	if err != nil {
		webx.Fail(c.W, 500, 500, err.Error())
		return
	}
	webx.Page(c.W, out, total)
}

func (h *Admin) get(c *webx.Context) {
	id, err := webx.ParamInt64(c, "id")
	if err != nil {
		webx.Fail(c.W, 400, 400, err.Error())
		return
	}
	st, err := h.svc.Get(id)
	if err != nil {
		webx.Fail(c.W, 404, 404, err.Error())
		return
	}
	webx.OK(c.W, st)
}

func (h *Admin) create(c *webx.Context) {
	var req struct {
		TenantID int64 `json:"tenant_id"`
		CreateReq
	}
	if err := webx.BindJSON(c, &req); err != nil {
		webx.Fail(c.W, 400, 400, "bad json body")
		return
	}
	if req.TenantID <= 0 {
		webx.Fail(c.W, 400, 400, "tenant_id required")
		return
	}
	t, err := h.tenants.Get(req.TenantID)
	if err != nil {
		webx.Fail(c.W, 404, 404, err.Error())
		return
	}
	st, err := h.svc.Create(t.ID, c.User.UserName(), c.User.UserID(), req.CreateReq, t.MaxSites)
	if err != nil {
		webx.Fail(c.W, 400, 400, err.Error())
		return
	}
	webx.OK(c.W, st)
}

func (h *Admin) update(c *webx.Context) {
	id, err := webx.ParamInt64(c, "id")
	if err != nil {
		webx.Fail(c.W, 400, 400, err.Error())
		return
	}
	var req UpdateReq
	if err := webx.BindJSON(c, &req); err != nil {
		webx.Fail(c.W, 400, 400, "bad json body")
		return
	}
	st, err := h.svc.Update(id, c.User.UserName(), c.User.UserID(), req)
	if err != nil {
		webx.Fail(c.W, 400, 400, err.Error())
		return
	}
	webx.OK(c.W, st)
}

func (h *Admin) delete(c *webx.Context) {
	id, err := webx.ParamInt64(c, "id")
	if err != nil {
		webx.Fail(c.W, 400, 400, err.Error())
		return
	}
	if err := h.svc.Delete(id, c.User.UserName(), c.User.UserID()); err != nil {
		webx.Fail(c.W, 400, 400, err.Error())
		return
	}
	webx.OK(c.W, map[string]int64{"id": id})
}

func (h *Admin) status(c *webx.Context) {
	id, err := webx.ParamInt64(c, "id")
	if err != nil {
		webx.Fail(c.W, 400, 400, err.Error())
		return
	}
	var req struct {
		Status string `json:"status"`
	}
	if err := webx.BindJSON(c, &req); err != nil {
		webx.Fail(c.W, 400, 400, "bad json body")
		return
	}
	st, err := h.svc.SetStatus(id, req.Status, c.User.UserName(), c.User.UserID())
	if err != nil {
		webx.Fail(c.W, 400, 400, err.Error())
		return
	}
	webx.OK(c.W, st)
}

// usageSeries: per-day counters for the site (days<=365).
func (h *Admin) usageSeries(c *webx.Context) {
	id, err := webx.ParamInt64(c, "id")
	if err != nil {
		webx.Fail(c.W, 400, 400, err.Error())
		return
	}
	days := webx.QueryInt(c, "days", 7)
	if days < 1 || days > 365 {
		days = 7
	}
	out, err := h.usage.Series(0, id, days)
	if err != nil {
		webx.Fail(c.W, 500, 500, err.Error())
		return
	}
	webx.OK(c.W, out)
}

// stats: window + today totals and cache hit rate.
func (h *Admin) stats(c *webx.Context) {
	id, err := webx.ParamInt64(c, "id")
	if err != nil {
		webx.Fail(c.W, 400, 400, err.Error())
		return
	}
	days := webx.QueryInt(c, "days", 30)
	totals, err := h.usage.SiteTotals(0, days)
	if err != nil {
		webx.Fail(c.W, 500, 500, err.Error())
		return
	}
	out := map[string]any{"days": days, "requests": 0, "bytes": 0, "cache_hits": 0, "cache_misses": 0, "cache_hit_rate": 0}
	for i := range totals {
		if totals[i].SiteID == id {
			out["requests"] = totals[i].Requests
			out["bytes"] = totals[i].Bytes
			out["cache_hits"] = totals[i].CacheHits
			out["cache_misses"] = totals[i].CacheMisses
			if totals[i].CacheHits+totals[i].CacheMisses > 0 {
				out["cache_hit_rate"] = float64(totals[i].CacheHits) / float64(totals[i].CacheHits+totals[i].CacheMisses)
			}
			break
		}
	}
	webx.OK(c.W, out)
}

// accessLogs: paged access logs for the site.
func (h *Admin) accessLogs(c *webx.Context) {
	id, err := webx.ParamInt64(c, "id")
	if err != nil {
		webx.Fail(c.W, 400, 400, err.Error())
		return
	}
	out, total, err := h.logs.List(id, webx.QueryStr(c, "ip"), webx.QueryStr(c, "path"),
		webx.QueryInt(c, "status", 0), webx.QueryInt(c, "page", 1), webx.QueryInt(c, "size", 50))
	if err != nil {
		webx.Fail(c.W, 500, 500, err.Error())
		return
	}
	webx.Page(c.W, out, total)
}

// statusDist: status-code distribution over recent rows.
func (h *Admin) statusDist(c *webx.Context) {
	id, err := webx.ParamInt64(c, "id")
	if err != nil {
		webx.Fail(c.W, 400, 400, err.Error())
		return
	}
	out, err := h.logs.StatusDist(id, 5000)
	if err != nil {
		webx.Fail(c.W, 500, 500, err.Error())
		return
	}
	webx.OK(c.W, out)
}

// User handles tenant-scoped site routes.
type User struct {
	svc     *Store
	tenants *tenant.Store
	usage   *usage.Store
	logs    *logs.Store
}

// RegisterUser mounts tenant portal site routes (scoped by JWT tenant_id).
func RegisterUser(r *webx.Router, svc *Store, tenants *tenant.Store, u *usage.Store, l *logs.Store) {
	h := &User{svc: svc, tenants: tenants, usage: u, logs: l}
	r.GET("/api/v1/user/sites", h.list)
	r.POST("/api/v1/user/sites", h.create)
	r.GET("/api/v1/user/sites/:id", h.get)
	r.PUT("/api/v1/user/sites/:id", h.update)
	r.DELETE("/api/v1/user/sites/:id", h.delete)
	r.POST("/api/v1/user/sites/:id/status", h.status)
	r.GET("/api/v1/user/sites/:id/usage", h.usageSeries)
	r.GET("/api/v1/user/sites/:id/stats", h.stats)
	r.GET("/api/v1/user/sites/:id/logs", h.accessLogs)
}

func (h *User) owned(c *webx.Context, id int64) (Site, error) {
	st, err := h.svc.Get(id)
	if err != nil {
		return Site{}, err
	}
	if st.TenantID != c.User.TenantID() {
		return Site{}, fmt.Errorf("not your site")
	}
	return st, nil
}

func (h *User) list(c *webx.Context) {
	out, total, err := h.svc.List(c.User.TenantID(), webx.QueryStr(c, "keyword"),
		webx.QueryInt(c, "page", 1), webx.QueryInt(c, "size", 20))
	if err != nil {
		webx.Fail(c.W, 500, 500, err.Error())
		return
	}
	webx.Page(c.W, out, total)
}

func (h *User) get(c *webx.Context) {
	id, err := webx.ParamInt64(c, "id")
	if err != nil {
		webx.Fail(c.W, 400, 400, err.Error())
		return
	}
	st, err := h.owned(c, id)
	if err != nil {
		webx.Fail(c.W, 404, 404, err.Error())
		return
	}
	webx.OK(c.W, st)
}

func (h *User) create(c *webx.Context) {
	var req CreateReq
	if err := webx.BindJSON(c, &req); err != nil {
		webx.Fail(c.W, 400, 400, "bad json body")
		return
	}
	t, err := h.tenants.Get(c.User.TenantID())
	if err != nil {
		webx.Fail(c.W, 403, 403, "tenant not found or disabled")
		return
	}
	st, err := h.svc.Create(t.ID, "tenant", c.User.UserID(), req, t.MaxSites)
	if err != nil {
		webx.Fail(c.W, 400, 400, err.Error())
		return
	}
	webx.OK(c.W, st)
}

func (h *User) update(c *webx.Context) {
	id, err := webx.ParamInt64(c, "id")
	if err != nil {
		webx.Fail(c.W, 400, 400, err.Error())
		return
	}
	if _, err := h.owned(c, id); err != nil {
		webx.Fail(c.W, 404, 404, err.Error())
		return
	}
	var req UpdateReq
	if err := webx.BindJSON(c, &req); err != nil {
		webx.Fail(c.W, 400, 400, "bad json body")
		return
	}
	st, err := h.svc.Update(id, "tenant", c.User.UserID(), req)
	if err != nil {
		webx.Fail(c.W, 400, 400, err.Error())
		return
	}
	webx.OK(c.W, st)
}

func (h *User) delete(c *webx.Context) {
	id, err := webx.ParamInt64(c, "id")
	if err != nil {
		webx.Fail(c.W, 400, 400, err.Error())
		return
	}
	if _, err := h.owned(c, id); err != nil {
		webx.Fail(c.W, 404, 404, err.Error())
		return
	}
	if err := h.svc.Delete(id, "tenant", c.User.UserID()); err != nil {
		webx.Fail(c.W, 400, 400, err.Error())
		return
	}
	webx.OK(c.W, map[string]int64{"id": id})
}

func (h *User) status(c *webx.Context) {
	id, err := webx.ParamInt64(c, "id")
	if err != nil {
		webx.Fail(c.W, 400, 400, err.Error())
		return
	}
	if _, err := h.owned(c, id); err != nil {
		webx.Fail(c.W, 404, 404, err.Error())
		return
	}
	var req struct {
		Status string `json:"status"`
	}
	if err := webx.BindJSON(c, &req); err != nil {
		webx.Fail(c.W, 400, 400, "bad json body")
		return
	}
	st, err := h.svc.SetStatus(id, req.Status, "tenant", c.User.UserID())
	if err != nil {
		webx.Fail(c.W, 400, 400, err.Error())
		return
	}
	webx.OK(c.W, st)
}

func (h *User) usageSeries(c *webx.Context) {
	id, err := webx.ParamInt64(c, "id")
	if err != nil {
		webx.Fail(c.W, 400, 400, err.Error())
		return
	}
	if _, err := h.owned(c, id); err != nil {
		webx.Fail(c.W, 404, 404, err.Error())
		return
	}
	days := webx.QueryInt(c, "days", 7)
	if days < 1 || days > 365 {
		days = 7
	}
	out, err := h.usage.Series(c.User.TenantID(), id, days)
	if err != nil {
		webx.Fail(c.W, 500, 500, err.Error())
		return
	}
	webx.OK(c.W, out)
}

func (h *User) stats(c *webx.Context) {
	id, err := webx.ParamInt64(c, "id")
	if err != nil {
		webx.Fail(c.W, 400, 400, err.Error())
		return
	}
	if _, err := h.owned(c, id); err != nil {
		webx.Fail(c.W, 404, 404, err.Error())
		return
	}
	days := webx.QueryInt(c, "days", 30)
	totals, err := h.usage.SiteTotals(c.User.TenantID(), days)
	if err != nil {
		webx.Fail(c.W, 500, 500, err.Error())
		return
	}
	out := map[string]any{"days": days, "requests": 0, "bytes": 0, "cache_hits": 0, "cache_misses": 0, "cache_hit_rate": 0}
	for i := range totals {
		if totals[i].SiteID == id {
			out["requests"] = totals[i].Requests
			out["bytes"] = totals[i].Bytes
			out["cache_hits"] = totals[i].CacheHits
			out["cache_misses"] = totals[i].CacheMisses
			if totals[i].CacheHits+totals[i].CacheMisses > 0 {
				out["cache_hit_rate"] = float64(totals[i].CacheHits) / float64(totals[i].CacheHits+totals[i].CacheMisses)
			}
			break
		}
	}
	webx.OK(c.W, out)
}

func (h *User) accessLogs(c *webx.Context) {
	id, err := webx.ParamInt64(c, "id")
	if err != nil {
		webx.Fail(c.W, 400, 400, err.Error())
		return
	}
	if _, err := h.owned(c, id); err != nil {
		webx.Fail(c.W, 404, 404, err.Error())
		return
	}
	out, total, err := h.logs.List(id, webx.QueryStr(c, "ip"), webx.QueryStr(c, "path"),
		webx.QueryInt(c, "status", 0), webx.QueryInt(c, "page", 1), webx.QueryInt(c, "size", 50))
	if err != nil {
		webx.Fail(c.W, 500, 500, err.Error())
		return
	}
	webx.Page(c.W, out, total)
}
