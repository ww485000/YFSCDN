package usage

import "edgecdn/core/internal/webx"

// Register mounts usage routes for both scopes.
func Register(r *webx.Router, svc *Store) {
	r.GET("/api/v1/admin/usage/summary", func(c *webx.Context) {
		sm, err := svc.Summary(webx.QueryInt64(c, "tenant_id", 0), webx.QueryStr(c, "from"), webx.QueryStr(c, "to"))
		if err != nil {
			webx.Fail(c.W, 500, 500, err.Error())
			return
		}
		webx.OK(c.W, sm)
	})
	r.GET("/api/v1/admin/usage/daily", func(c *webx.Context) {
		out, err := svc.List(webx.QueryInt64(c, "tenant_id", 0), webx.QueryStr(c, "from"), webx.QueryStr(c, "to"))
		if err != nil {
			webx.Fail(c.W, 500, 500, err.Error())
			return
		}
		webx.OK(c.W, out)
	})
	r.GET("/api/v1/user/usage/summary", func(c *webx.Context) {
		sm, err := svc.Summary(c.User.TenantID(), webx.QueryStr(c, "from"), webx.QueryStr(c, "to"))
		if err != nil {
			webx.Fail(c.W, 500, 500, err.Error())
			return
		}
		webx.OK(c.W, sm)
	})
	r.GET("/api/v1/user/usage/daily", func(c *webx.Context) {
		out, err := svc.List(c.User.TenantID(), webx.QueryStr(c, "from"), webx.QueryStr(c, "to"))
		if err != nil {
			webx.Fail(c.W, 500, 500, err.Error())
			return
		}
		webx.OK(c.W, out)
	})
}
