package oplog

import "edgecdn/core/internal/webx"

// Register mounts the oplog listing route (admin only).
func Register(r *webx.Router, svc *Store) {
	r.GET("/api/v1/admin/oplogs", func(c *webx.Context) {
		out, total, err := svc.List(webx.QueryInt(c, "page", 1), webx.QueryInt(c, "size", 20), webx.QueryStr(c, "keyword"))
		if err != nil {
			webx.Fail(c.W, 500, 500, err.Error())
			return
		}
		webx.Page(c.W, out, total)
	})
}
