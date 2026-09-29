package auth

import "edgecdn/core/internal/webx"

// Register mounts auth routes (public).
func Register(r *webx.Router, svc *Service) {
	r.POST("/api/v1/auth/login", func(c *webx.Context) {
		var req LoginReq
		if err := webx.BindJSON(c, &req); err != nil {
			webx.Fail(c.W, 400, 400, "bad json body")
			return
		}
		tok, info, err := svc.Login(req)
		if err != nil {
			webx.Fail(c.W, 401, 401, err.Error())
			return
		}
		webx.OK(c.W, map[string]any{"token": tok, "user": info})
	})
	r.GET("/api/v1/auth/me", func(c *webx.Context) {
		if c.User == nil {
			webx.Fail(c.W, 401, 401, "not authenticated")
			return
		}
		webx.OK(c.W, map[string]any{
			"user_id":   c.User.UserID(),
			"name":      c.User.UserName(),
			"role":      c.User.Role(),
			"tenant_id": c.User.TenantID(),
		})
	})
}
