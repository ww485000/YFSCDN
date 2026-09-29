package task

import "edgecdn/core/internal/webx"

// RegisterAdmin mounts the task listing route.
func RegisterAdmin(r *webx.Router, s *Store) {
	r.GET("/api/v1/admin/tasks", func(c *webx.Context) {
		out, total, err := s.List(webx.QueryStr(c, "type"), webx.QueryStr(c, "status"),
			webx.QueryInt(c, "page", 1), webx.QueryInt(c, "size", 20))
		if err != nil {
			webx.Fail(c.W, 500, 500, err.Error())
			return
		}
		webx.Page(c.W, out, total)
	})
}
