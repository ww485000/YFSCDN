package dnsprovider

import (
	"edgecdn/core/internal/task"
	"edgecdn/core/internal/webx"
)

func RegisterAdmin(r *webx.Router, s *Store, tasks *task.Store) {
	r.GET("/api/v1/admin/dns/providers", func(c *webx.Context) {
		out, total, err := s.List(webx.QueryInt64(c, "tenant_id", 0), webx.QueryInt(c, "page", 1), webx.QueryInt(c, "size", 20))
		if err != nil {
			webx.Fail(c.W, 500, 500, err.Error())
			return
		}
		webx.Page(c.W, out, total)
	})
	r.GET("/api/v1/admin/dns/syncs", func(c *webx.Context) {
		out, total, err := s.ListSyncs(webx.QueryInt64(c, "tenant_id", 0), webx.QueryStr(c, "status"), webx.QueryInt(c, "page", 1), webx.QueryInt(c, "size", 20))
		if err != nil {
			webx.Fail(c.W, 500, 500, err.Error())
			return
		}
		webx.Page(c.W, out, total)
	})
	r.POST("/api/v1/admin/dns/syncs/:id/retry", func(c *webx.Context) {
		retrySync(c, s, tasks, 0)
	})
	r.POST("/api/v1/admin/dns/providers", func(c *webx.Context) {
		var p Provider
		if err := webx.BindJSON(c, &p); err != nil {
			webx.Fail(c.W, 400, 400, err.Error())
			return
		}
		if p.TenantID == 0 {
			webx.Fail(c.W, 400, 400, "tenant_id is required")
			return
		}
		id, err := s.Create(&p)
		if err != nil {
			webx.Fail(c.W, 400, 400, err.Error())
			return
		}
		out, _ := s.Get(id)
		webx.OK(c.W, out)
	})
	r.PUT("/api/v1/admin/dns/providers/:id", func(c *webx.Context) {
		id, err := webx.ParamInt64(c, "id")
		if err != nil {
			webx.Fail(c.W, 400, 400, err.Error())
			return
		}
		var p Provider
		if err := webx.BindJSON(c, &p); err != nil {
			webx.Fail(c.W, 400, 400, err.Error())
			return
		}
		if p.TenantID == 0 {
			webx.Fail(c.W, 400, 400, "tenant_id is required")
			return
		}
		p.ID = id
		if err := s.Update(&p); err != nil {
			webx.Fail(c.W, 400, 400, err.Error())
			return
		}
		webx.OK(c.W, nil)
	})
	r.DELETE("/api/v1/admin/dns/providers/:id", func(c *webx.Context) {
		id, err := webx.ParamInt64(c, "id")
		if err != nil {
			webx.Fail(c.W, 400, 400, err.Error())
			return
		}
		if err := s.Delete(id); err != nil {
			webx.Fail(c.W, 400, 400, err.Error())
			return
		}
		webx.OK(c.W, nil)
	})
	r.POST("/api/v1/admin/dns/providers/:id/sync", func(c *webx.Context) {
		id, err := webx.ParamInt64(c, "id")
		if err != nil {
			webx.Fail(c.W, 400, 400, err.Error())
			return
		}
		if _, err := s.Get(id); err != nil {
			webx.Fail(c.W, 404, 404, err.Error())
			return
		}
		taskID, err := tasks.Enqueue(task.TypeDNSResolve, "provider:"+itoa(id), "")
		if err != nil {
			webx.Fail(c.W, 500, 500, err.Error())
			return
		}
		webx.OK(c.W, map[string]any{"task_id": taskID})
	})
}

func RegisterUser(r *webx.Router, s *Store, tasks *task.Store) {
	r.GET("/api/v1/user/dns/providers", func(c *webx.Context) {
		out, total, err := s.List(c.User.TenantID(), webx.QueryInt(c, "page", 1), webx.QueryInt(c, "size", 20))
		if err != nil {
			webx.Fail(c.W, 500, 500, err.Error())
			return
		}
		webx.Page(c.W, out, total)
	})
	r.GET("/api/v1/user/dns/syncs", func(c *webx.Context) {
		out, total, err := s.ListSyncs(c.User.TenantID(), webx.QueryStr(c, "status"), webx.QueryInt(c, "page", 1), webx.QueryInt(c, "size", 20))
		if err != nil {
			webx.Fail(c.W, 500, 500, err.Error())
			return
		}
		webx.Page(c.W, out, total)
	})
	r.POST("/api/v1/user/dns/syncs/:id/retry", func(c *webx.Context) {
		retrySync(c, s, tasks, c.User.TenantID())
	})
	r.POST("/api/v1/user/dns/providers", func(c *webx.Context) {
		var p Provider
		if err := webx.BindJSON(c, &p); err != nil {
			webx.Fail(c.W, 400, 400, err.Error())
			return
		}
		p.TenantID = c.User.TenantID()
		id, err := s.Create(&p)
		if err != nil {
			webx.Fail(c.W, 400, 400, err.Error())
			return
		}
		out, _ := s.Get(id)
		webx.OK(c.W, out)
	})
	r.PUT("/api/v1/user/dns/providers/:id", func(c *webx.Context) {
		id, err := webx.ParamInt64(c, "id")
		if err != nil {
			webx.Fail(c.W, 400, 400, err.Error())
			return
		}
		if tid, _ := s.TenantOf(id); tid != c.User.TenantID() {
			webx.Fail(c.W, 404, 404, "provider not found")
			return
		}
		var p Provider
		if err := webx.BindJSON(c, &p); err != nil {
			webx.Fail(c.W, 400, 400, err.Error())
			return
		}
		p.ID = id
		p.TenantID = c.User.TenantID()
		if err := s.Update(&p); err != nil {
			webx.Fail(c.W, 400, 400, err.Error())
			return
		}
		webx.OK(c.W, nil)
	})
	r.DELETE("/api/v1/user/dns/providers/:id", func(c *webx.Context) {
		id, err := webx.ParamInt64(c, "id")
		if err != nil {
			webx.Fail(c.W, 400, 400, err.Error())
			return
		}
		if tid, _ := s.TenantOf(id); tid != c.User.TenantID() {
			webx.Fail(c.W, 404, 404, "provider not found")
			return
		}
		if err := s.Delete(id); err != nil {
			webx.Fail(c.W, 400, 400, err.Error())
			return
		}
		webx.OK(c.W, nil)
	})
	r.POST("/api/v1/user/dns/providers/:id/sync", func(c *webx.Context) {
		id, err := webx.ParamInt64(c, "id")
		if err != nil {
			webx.Fail(c.W, 400, 400, err.Error())
			return
		}
		if tid, _ := s.TenantOf(id); tid != c.User.TenantID() {
			webx.Fail(c.W, 404, 404, "provider not found")
			return
		}
		taskID, err := tasks.Enqueue(task.TypeDNSResolve, "provider:"+itoa(id), "")
		if err != nil {
			webx.Fail(c.W, 500, 500, err.Error())
			return
		}
		webx.OK(c.W, map[string]any{"task_id": taskID})
	})
}

func itoa(v int64) string {
	if v == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	n := v
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

func retrySync(c *webx.Context, s *Store, tasks *task.Store, tenantID int64) {
	id, err := webx.ParamInt64(c, "id")
	if err != nil {
		webx.Fail(c.W, 400, 400, err.Error())
		return
	}
	row, err := s.GetSyncView(id)
	if err != nil {
		webx.Fail(c.W, 404, 404, err.Error())
		return
	}
	if tenantID > 0 && row.TenantID != tenantID {
		webx.Fail(c.W, 404, 404, "sync not found")
		return
	}
	taskID, err := tasks.Enqueue(task.TypeDNSResolve, "record:"+itoa(row.RecordID), "")
	if err != nil {
		webx.Fail(c.W, 500, 500, err.Error())
		return
	}
	webx.OK(c.W, map[string]any{"task_id": taskID})
}
