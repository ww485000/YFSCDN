package setting

import (
	"edgecdn/core/internal/oplog"
	"edgecdn/core/internal/webx"
)

// Register mounts platform settings routes (admin only).
func Register(r *webx.Router, svc *Store, oplog *oplog.Store) {
	r.GET("/api/v1/admin/settings", func(c *webx.Context) {
		m, err := svc.All()
		if err != nil {
			webx.Fail(c.W, 500, 500, err.Error())
			return
		}
		webx.OK(c.W, m)
	})
	r.PUT("/api/v1/admin/settings", func(c *webx.Context) {
		var m map[string]string
		if err := webx.BindJSON(c, &m); err != nil {
			webx.Fail(c.W, 400, 400, "body must be a JSON object of key/value strings")
			return
		}
		if err := svc.PutAll(m); err != nil {
			webx.Fail(c.W, 400, 400, err.Error())
			return
		}
		oplog.Add(c.User.UserName(), c.User.UserID(), "settings.update", "", "")
		webx.OK(c.W, m)
	})
}
