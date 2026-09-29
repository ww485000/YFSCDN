package waf

import (
	"edgecdn/core/internal/oplog"
	"edgecdn/core/internal/webx"
)

// Admin handles operator-side WAF routes (cross-tenant).
type Admin struct {
	svc   *Store
	oplog *oplog.Store
}

// RegisterAdmin mounts operator WAF routes.
func RegisterAdmin(r *webx.Router, svc *Store, oplog *oplog.Store) {
	h := &Admin{svc: svc, oplog: oplog}
	r.GET("/api/v1/admin/waf-rules", h.list)
	r.POST("/api/v1/admin/waf-rules", h.create)
	r.PUT("/api/v1/admin/waf-rules/:id", h.update)
	r.DELETE("/api/v1/admin/waf-rules/:id", h.delete)
}

func (h *Admin) list(c *webx.Context) {
	out, total, err := h.svc.List(webx.QueryInt64(c, "tenant_id", 0), webx.QueryInt(c, "page", 1), webx.QueryInt(c, "size", 50))
	if err != nil {
		webx.Fail(c.W, 500, 500, err.Error())
		return
	}
	webx.Page(c.W, out, total)
}

func (h *Admin) create(c *webx.Context) {
	var req struct {
		TenantID int64  `json:"tenant_id"`
		Name     string `json:"name"`
		Type     string `json:"type"`
		Value    string `json:"value"`
		Action   string `json:"action"`
	}
	if err := webx.BindJSON(c, &req); err != nil {
		webx.Fail(c.W, 400, 400, "bad json body")
		return
	}
	if req.TenantID <= 0 {
		webx.Fail(c.W, 400, 400, "tenant_id required")
		return
	}
	rule, err := h.svc.Create(req.TenantID, req.Name, req.Type, req.Value, req.Action)
	if err != nil {
		webx.Fail(c.W, 400, 400, err.Error())
		return
	}
	h.oplog.Add(c.User.UserName(), c.User.UserID(), "waf.create", rule.Name, rule.Type+" "+rule.Value)
	webx.OK(c.W, rule)
}

func (h *Admin) update(c *webx.Context) {
	id, err := webx.ParamInt64(c, "id")
	if err != nil {
		webx.Fail(c.W, 400, 400, err.Error())
		return
	}
	var req struct {
		Name    string `json:"name"`
		Type    string `json:"type"`
		Value   string `json:"value"`
		Action  string `json:"action"`
		Enabled int    `json:"enabled"`
	}
	if err := webx.BindJSON(c, &req); err != nil {
		webx.Fail(c.W, 400, 400, "bad json body")
		return
	}
	if err := h.svc.Update(id, req.Name, req.Type, req.Value, req.Action, req.Enabled); err != nil {
		webx.Fail(c.W, 400, 400, err.Error())
		return
	}
	h.oplog.Add(c.User.UserName(), c.User.UserID(), "waf.update", "", "")
	webx.OK(c.W, map[string]int64{"id": id})
}

func (h *Admin) delete(c *webx.Context) {
	id, err := webx.ParamInt64(c, "id")
	if err != nil {
		webx.Fail(c.W, 400, 400, err.Error())
		return
	}
	if err := h.svc.Delete(id); err != nil {
		webx.Fail(c.W, 400, 400, err.Error())
		return
	}
	h.oplog.Add(c.User.UserName(), c.User.UserID(), "waf.delete", "", "")
	webx.OK(c.W, map[string]int64{"id": id})
}

// User handles tenant-scoped WAF routes.
type User struct {
	svc   *Store
	oplog *oplog.Store
}

// RegisterUser mounts tenant portal WAF routes.
func RegisterUser(r *webx.Router, svc *Store, oplog *oplog.Store) {
	h := &User{svc: svc, oplog: oplog}
	r.GET("/api/v1/user/waf-rules", h.list)
	r.POST("/api/v1/user/waf-rules", h.create)
	r.PUT("/api/v1/user/waf-rules/:id", h.update)
	r.DELETE("/api/v1/user/waf-rules/:id", h.delete)
}

func (h *User) list(c *webx.Context) {
	out, total, err := h.svc.List(c.User.TenantID(), 1, 200)
	if err != nil {
		webx.Fail(c.W, 500, 500, err.Error())
		return
	}
	webx.Page(c.W, out, total)
}

func (h *User) create(c *webx.Context) {
	var req struct {
		Name   string `json:"name"`
		Type   string `json:"type"`
		Value  string `json:"value"`
		Action string `json:"action"`
	}
	if err := webx.BindJSON(c, &req); err != nil {
		webx.Fail(c.W, 400, 400, "bad json body")
		return
	}
	rule, err := h.svc.Create(c.User.TenantID(), req.Name, req.Type, req.Value, req.Action)
	if err != nil {
		webx.Fail(c.W, 400, 400, err.Error())
		return
	}
	h.oplog.Add("tenant", c.User.UserID(), "waf.create", rule.Name, rule.Type+" "+rule.Value)
	webx.OK(c.W, rule)
}

func (h *User) update(c *webx.Context) {
	id, err := webx.ParamInt64(c, "id")
	if err != nil {
		webx.Fail(c.W, 400, 400, err.Error())
		return
	}
	rule, err := h.svc.Get(id)
	if err != nil {
		webx.Fail(c.W, 404, 404, err.Error())
		return
	}
	if rule.TenantID != c.User.TenantID() {
		webx.Fail(c.W, 403, 403, "not your rule")
		return
	}
	var req struct {
		Name    string `json:"name"`
		Type    string `json:"type"`
		Value   string `json:"value"`
		Action  string `json:"action"`
		Enabled int    `json:"enabled"`
	}
	if err := webx.BindJSON(c, &req); err != nil {
		webx.Fail(c.W, 400, 400, "bad json body")
		return
	}
	if err := h.svc.Update(id, req.Name, req.Type, req.Value, req.Action, req.Enabled); err != nil {
		webx.Fail(c.W, 400, 400, err.Error())
		return
	}
	h.oplog.Add("tenant", c.User.UserID(), "waf.update", "", "")
	webx.OK(c.W, map[string]int64{"id": id})
}

func (h *User) delete(c *webx.Context) {
	id, err := webx.ParamInt64(c, "id")
	if err != nil {
		webx.Fail(c.W, 400, 400, err.Error())
		return
	}
	rule, err := h.svc.Get(id)
	if err != nil {
		webx.Fail(c.W, 404, 404, err.Error())
		return
	}
	if rule.TenantID != c.User.TenantID() {
		webx.Fail(c.W, 403, 403, "not your rule")
		return
	}
	if err := h.svc.Delete(id); err != nil {
		webx.Fail(c.W, 400, 400, err.Error())
		return
	}
	h.oplog.Add("tenant", c.User.UserID(), "waf.delete", "", "")
	webx.OK(c.W, map[string]int64{"id": id})
}
