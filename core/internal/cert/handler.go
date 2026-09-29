package cert

import (
	"edgecdn/core/internal/oplog"
	"edgecdn/core/internal/webx"
)

// SiteEmitter re-pushes affected sites after a cert change.
type SiteEmitter interface {
	EmitByDomain(domain string) error
}

// Register mounts cert management routes.
func Register(r *webx.Router, svc *Service, emit SiteEmitter, oplog *oplog.Store) {
	r.GET("/api/v1/admin/certs", func(c *webx.Context) {
		out, err := svc.List(webx.QueryInt64(c, "tenant_id", 0))
		if err != nil {
			webx.Fail(c.W, 500, 500, err.Error())
			return
		}
		webx.OK(c.W, out)
	})
	// admin: tenant_id comes from the body; user: from the JWT.
	r.POST("/api/v1/admin/certs/upload", hUpload(svc, emit, oplog, true))
	r.POST("/api/v1/admin/certs/:id/reissue", hReissue(svc, emit, oplog))
	r.DELETE("/api/v1/admin/certs/:id", hDelete(svc, emit, oplog))

	r.GET("/api/v1/user/certs", func(c *webx.Context) {
		out, err := svc.List(c.User.TenantID())
		if err != nil {
			webx.Fail(c.W, 500, 500, err.Error())
			return
		}
		webx.OK(c.W, out)
	})
	r.POST("/api/v1/user/certs/upload", hUpload(svc, emit, oplog, false))
}

func hUpload(svc *Service, emit SiteEmitter, oplog *oplog.Store, fromBody bool) webx.Handler {
	return func(c *webx.Context) {
		var req struct {
			TenantID int64  `json:"tenant_id"`
			Domain   string `json:"domain"`
			Name     string `json:"name"`
			Cert     string `json:"cert"`
			Key      string `json:"key"`
		}
		if err := webx.BindJSON(c, &req); err != nil {
			webx.Fail(c.W, 400, 400, "bad json body")
			return
		}
		if req.Cert == "" || req.Key == "" {
			webx.Fail(c.W, 400, 400, "cert and key (PEM) required")
			return
		}
		var tenantID int64
		if fromBody {
			if req.TenantID <= 0 {
				webx.Fail(c.W, 400, 400, "tenant_id required")
				return
			}
			tenantID = req.TenantID
		} else {
			tenantID = c.User.TenantID()
		}
		ct, err := svc.UploadCustom(tenantID, req.Domain, req.Name, req.Cert, req.Key)
		if err != nil {
			webx.Fail(c.W, 400, 400, err.Error())
			return
		}
		if oplog != nil {
			oplog.Add(c.User.UserName(), c.User.UserID(), "cert.upload", ct.Domain, "type=custom expires="+ct.ExpiresAt)
		}
		if emit != nil {
			_ = emit.EmitByDomain(ct.Domain)
		}
		webx.OK(c.W, ct)
	}
}

func hReissue(svc *Service, emit SiteEmitter, oplog *oplog.Store) webx.Handler {
	return func(c *webx.Context) {
		id, err := webx.ParamInt64(c, "id")
		if err != nil {
			webx.Fail(c.W, 400, 400, err.Error())
			return
		}
		cur, err := svc.Get(id)
		if err != nil {
			webx.Fail(c.W, 404, 404, err.Error())
			return
		}
		ct, err := svc.Reissue(id)
		if err != nil {
			webx.Fail(c.W, 400, 400, err.Error())
			return
		}
		if oplog != nil {
			oplog.Add(c.User.UserName(), c.User.UserID(), "cert.reissue", cur.Domain, "type=ca")
		}
		if emit != nil {
			_ = emit.EmitByDomain(ct.Domain)
		}
		webx.OK(c.W, ct)
	}
}

func hDelete(svc *Service, emit SiteEmitter, oplog *oplog.Store) webx.Handler {
	return func(c *webx.Context) {
		id, err := webx.ParamInt64(c, "id")
		if err != nil {
			webx.Fail(c.W, 400, 400, err.Error())
			return
		}
		cur, err := svc.Get(id)
		if err != nil {
			webx.Fail(c.W, 404, 404, err.Error())
			return
		}
		if err := svc.Delete(id); err != nil {
			webx.Fail(c.W, 400, 400, err.Error())
			return
		}
		if oplog != nil {
			oplog.Add(c.User.UserName(), c.User.UserID(), "cert.delete", cur.Domain, "id="+itoa(id))
		}
		if emit != nil {
			_ = emit.EmitByDomain(cur.Domain)
		}
		webx.OK(c.W, map[string]int64{"id": id})
	}
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
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
