package user

import (
	"strconv"

	"edgecdn/core/internal/oplog"
	"edgecdn/core/internal/webx"
)

type Handler struct {
	svc   *Store
	oplog *oplog.Store
}

// RegisterAdmin mounts operator-account routes.
func RegisterAdmin(r *webx.Router, svc *Store, oplog *oplog.Store) {
	h := &Handler{svc: svc, oplog: oplog}
	r.GET("/api/v1/admin/admins", h.list)
	r.POST("/api/v1/admin/admins", h.create)
	r.PUT("/api/v1/admin/admins/:id", h.update)
	r.DELETE("/api/v1/admin/admins/:id", h.delete)
}

func (h *Handler) list(c *webx.Context) {
	out, err := h.svc.List()
	if err != nil {
		webx.Fail(c.W, 500, 500, err.Error())
		return
	}
	webx.OK(c.W, out)
}

func (h *Handler) create(c *webx.Context) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Role     string `json:"role"`
	}
	if err := webx.BindJSON(c, &req); err != nil {
		webx.Fail(c.W, 400, 400, "bad json body")
		return
	}
	a, err := h.svc.Create(req.Username, req.Password, req.Role)
	if err != nil {
		webx.Fail(c.W, 400, 400, err.Error())
		return
	}
	h.oplog.Add(c.User.UserName(), c.User.UserID(), "admin.create", a.Username, a.Role)
	webx.OK(c.W, a)
}

func (h *Handler) update(c *webx.Context) {
	id, err := webx.ParamInt64(c, "id")
	if err != nil {
		webx.Fail(c.W, 400, 400, err.Error())
		return
	}
	var req struct {
		Role     string `json:"role"`
		Status   int    `json:"status"`
		Password string `json:"password"`
	}
	if err := webx.BindJSON(c, &req); err != nil {
		webx.Fail(c.W, 400, 400, "bad json body")
		return
	}
	if req.Password != "" {
		if err := h.svc.SetPassword(id, req.Password); err != nil {
			webx.Fail(c.W, 400, 400, err.Error())
			return
		}
	}
	if req.Role != "" || req.Status > 0 {
		if err := h.svc.Update(id, req.Role, req.Status); err != nil {
			webx.Fail(c.W, 400, 400, err.Error())
			return
		}
	}
	h.oplog.Add(c.User.UserName(), c.User.UserID(), "admin.update", strconv.FormatInt(id, 10), "")
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
	h.oplog.Add(c.User.UserName(), c.User.UserID(), "admin.delete", strconv.FormatInt(id, 10), "")
	webx.OK(c.W, map[string]int64{"id": id})
}
