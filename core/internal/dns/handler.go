package dns

import (
	"encoding/json"

	"edgecdn/core/internal/task"
	"edgecdn/core/internal/webx"
)

// RegisterAdmin mounts admin DNS routes (cross-tenant).
func RegisterAdmin(r *webx.Router, s *Store, tasks *task.Store) {
	r.GET("/api/v1/admin/dns/records", func(c *webx.Context) {
		out, total, err := s.List(0, webx.QueryStr(c, "domain"), webx.QueryInt(c, "page", 1), webx.QueryInt(c, "size", 20))
		if err != nil {
			webx.Fail(c.W, 500, 500, err.Error())
			return
		}
		webx.Page(c.W, out, total)
	})
	r.GET("/api/v1/admin/dns/zones", func(c *webx.Context) {
		z, err := s.Zones(0)
		if err != nil {
			webx.Fail(c.W, 500, 500, err.Error())
			return
		}
		webx.OK(c.W, z)
	})
	r.POST("/api/v1/admin/dns/records", func(c *webx.Context) {
		var rec Record
		if err := webx.BindJSON(c, &rec); err != nil {
			webx.Fail(c.W, 400, 400, err.Error())
			return
		}
		if rec.TenantID == 0 {
			webx.Fail(c.W, 400, 400, "tenant_id is required")
			return
		}
		id, err := s.Create(rec.TenantID, &rec)
		if err != nil {
			webx.Fail(c.W, 400, 400, err.Error())
			return
		}
		g, _ := s.Get(id)
		enqueueDNS(tasks, task.TypeDNSResolve, "record:"+itoa(id), g)
		webx.OK(c.W, g)
	})
	r.PUT("/api/v1/admin/dns/records/:id", func(c *webx.Context) {
		id, err := webx.ParamInt64(c, "id")
		if err != nil {
			webx.Fail(c.W, 400, 400, err.Error())
			return
		}
		var rec Record
		if err := webx.BindJSON(c, &rec); err != nil {
			webx.Fail(c.W, 400, 400, err.Error())
			return
		}
		rec.ID = id
		if err := s.Update(&rec); err != nil {
			webx.Fail(c.W, 400, 400, err.Error())
			return
		}
		g, _ := s.Get(id)
		enqueueDNS(tasks, task.TypeDNSResolve, "record:"+itoa(id), g)
		webx.OK(c.W, nil)
	})
	r.DELETE("/api/v1/admin/dns/records/:id", func(c *webx.Context) {
		id, err := webx.ParamInt64(c, "id")
		if err != nil {
			webx.Fail(c.W, 400, 400, err.Error())
			return
		}
		if _, err := s.Delete(id); err != nil {
			webx.Fail(c.W, 400, 400, err.Error())
			return
		}
		enqueueDNS(tasks, task.TypeDNSClean, "record:"+itoa(id), map[string]any{"id": id})
		webx.OK(c.W, nil)
	})
}

// RegisterUser mounts tenant DNS routes (own tenant only).
func RegisterUser(r *webx.Router, s *Store, tasks *task.Store) {
	r.GET("/api/v1/user/dns/records", func(c *webx.Context) {
		out, total, err := s.List(c.User.TenantID(), webx.QueryStr(c, "domain"), webx.QueryInt(c, "page", 1), webx.QueryInt(c, "size", 20))
		if err != nil {
			webx.Fail(c.W, 500, 500, err.Error())
			return
		}
		webx.Page(c.W, out, total)
	})
	r.GET("/api/v1/user/dns/zones", func(c *webx.Context) {
		z, err := s.Zones(c.User.TenantID())
		if err != nil {
			webx.Fail(c.W, 500, 500, err.Error())
			return
		}
		webx.OK(c.W, z)
	})
	r.POST("/api/v1/user/dns/records", func(c *webx.Context) {
		var rec Record
		if err := webx.BindJSON(c, &rec); err != nil {
			webx.Fail(c.W, 400, 400, err.Error())
			return
		}
		id, err := s.Create(c.User.TenantID(), &rec)
		if err != nil {
			webx.Fail(c.W, 400, 400, err.Error())
			return
		}
		g, _ := s.Get(id)
		enqueueDNS(tasks, task.TypeDNSResolve, "record:"+itoa(id), g)
		webx.OK(c.W, g)
	})
	r.PUT("/api/v1/user/dns/records/:id", func(c *webx.Context) {
		id, err := webx.ParamInt64(c, "id")
		if err != nil {
			webx.Fail(c.W, 400, 400, err.Error())
			return
		}
		if tid, _ := s.TenantOf(id); tid != c.User.TenantID() {
			webx.Fail(c.W, 404, 404, "record not found")
			return
		}
		var rec Record
		if err := webx.BindJSON(c, &rec); err != nil {
			webx.Fail(c.W, 400, 400, err.Error())
			return
		}
		rec.ID = id
		if err := s.Update(&rec); err != nil {
			webx.Fail(c.W, 400, 400, err.Error())
			return
		}
		g, _ := s.Get(id)
		enqueueDNS(tasks, task.TypeDNSResolve, "record:"+itoa(id), g)
		webx.OK(c.W, nil)
	})
	r.DELETE("/api/v1/user/dns/records/:id", func(c *webx.Context) {
		id, err := webx.ParamInt64(c, "id")
		if err != nil {
			webx.Fail(c.W, 400, 400, err.Error())
			return
		}
		if tid, _ := s.TenantOf(id); tid != c.User.TenantID() {
			webx.Fail(c.W, 404, 404, "record not found")
			return
		}
		if _, err := s.Delete(id); err != nil {
			webx.Fail(c.W, 400, 400, err.Error())
			return
		}
		enqueueDNS(tasks, task.TypeDNSClean, "record:"+itoa(id), map[string]any{"id": id, "tenant_id": c.User.TenantID()})
		webx.OK(c.W, nil)
	})
}

func enqueueDNS(tasks *task.Store, typ, target string, payload any) {
	if tasks == nil {
		return
	}
	raw, _ := json.Marshal(payload)
	_, _ = tasks.Enqueue(typ, target, string(raw))
}

func itoa(v int64) string {
	if v == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	n := v
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
