package tenant

import (
	"strconv"

	"edgecdn/core/internal/authx"
	"edgecdn/core/internal/oplog"
	"edgecdn/core/internal/webx"
)

type Handler struct {
	svc   *Store
	oplog *oplog.Store
}

// RegisterAdmin mounts tenant management routes.
func RegisterAdmin(r *webx.Router, svc *Store, oplog *oplog.Store) {
	h := &Handler{svc: svc, oplog: oplog}
	r.GET("/api/v1/admin/tenants", h.list)
	r.POST("/api/v1/admin/tenants", h.create)
	r.PUT("/api/v1/admin/tenants/:id", h.update)
	r.DELETE("/api/v1/admin/tenants/:id", h.delete)
}

func (h *Handler) list(c *webx.Context) {
	out, total, err := h.svc.List(webx.QueryStr(c, "keyword"), webx.QueryInt(c, "page", 1), webx.QueryInt(c, "size", 20))
	if err != nil {
		webx.Fail(c.W, 500, 500, err.Error())
		return
	}
	webx.Page(c.W, out, total)
}

func (h *Handler) create(c *webx.Context) {
	var req struct {
		Name     string `json:"name"`
		Username string `json:"username"`
		Password string `json:"password"`
		Email    string `json:"email"`
	}
	if err := webx.BindJSON(c, &req); err != nil {
		webx.Fail(c.W, 400, 400, "bad json body")
		return
	}
	t, err := h.svc.Create(req.Name, req.Username, req.Password, req.Email)
	if err != nil {
		webx.Fail(c.W, 400, 400, err.Error())
		return
	}
	h.oplog.Add(c.User.UserName(), c.User.UserID(), "tenant.create", t.Username, t.Name)
	webx.OK(c.W, t)
}

func (h *Handler) update(c *webx.Context) {
	id, err := webx.ParamInt64(c, "id")
	if err != nil {
		webx.Fail(c.W, 400, 400, err.Error())
		return
	}
	var req struct {
		Name           string `json:"name"`
		Email          string `json:"email"`
		MaxSites       int    `json:"max_sites"`
		TrafficQuotaMB int64  `json:"traffic_quota_mb"`
		Status         int    `json:"status"`
		Password       string `json:"password"`
	}
	if err := webx.BindJSON(c, &req); err != nil {
		webx.Fail(c.W, 400, 400, "bad json body")
		return
	}
	cur, err := h.svc.Get(id)
	if err != nil {
		webx.Fail(c.W, 404, 404, err.Error())
		return
	}
	if req.Name == "" {
		req.Name = cur.Name
	}
	if req.Email == "" {
		req.Email = cur.Email
	}
	if req.MaxSites <= 0 {
		req.MaxSites = cur.MaxSites
	}
	if req.TrafficQuotaMB < 0 {
		req.TrafficQuotaMB = cur.TrafficQuotaMB
	}
	if req.Status == 0 {
		req.Status = 1
	}
	if err := h.svc.Update(id, req.Name, req.Email, req.MaxSites, req.TrafficQuotaMB, req.Status, req.Password); err != nil {
		webx.Fail(c.W, 400, 400, err.Error())
		return
	}
	h.oplog.Add(c.User.UserName(), c.User.UserID(), "tenant.update", cur.Username, "")
	webx.OK(c.W, map[string]int64{"id": id})
}

func (h *Handler) delete(c *webx.Context) {
	id, err := webx.ParamInt64(c, "id")
	if err != nil {
		webx.Fail(c.W, 400, 400, err.Error())
		return
	}
	if err := h.svc.Delete(id); err != nil {
		webx.Fail(c.W, 400, 400, err.Error())
		return
	}
	h.oplog.Add(c.User.UserName(), c.User.UserID(), "tenant.delete", strconv.FormatInt(id, 10), "")
	webx.OK(c.W, map[string]int64{"id": id})
}

// RegisterUser mounts the tenant-portal self-service route (change own password).
func RegisterUser(r *webx.Router, svc *Store) {
	r.POST("/api/v1/user/password", func(c *webx.Context) {
		var req struct {
			OldPassword string `json:"old_password"`
			NewPassword string `json:"new_password"`
		}
		if err := webx.BindJSON(c, &req); err != nil {
			webx.Fail(c.W, 400, 400, "bad json body")
			return
		}
		t, err := svc.Get(c.User.TenantID())
		if err != nil {
			webx.Fail(c.W, 404, 404, err.Error())
			return
		}
		if !authx.CheckPassword(t.Password(), req.OldPassword) {
			webx.Fail(c.W, 400, 400, "old password incorrect")
			return
		}
		if err := svc.Update(t.ID, t.Name, t.Email, t.MaxSites, t.TrafficQuotaMB, t.Status, req.NewPassword); err != nil {
			webx.Fail(c.W, 400, 400, err.Error())
			return
		}
		webx.OK(c.W, map[string]string{"ok": "1"})
	})
}
