package parity

import "edgecdn/core/internal/webx"

func RegisterAdmin(r *webx.Router) {
	r.GET("/api/v1/admin/goedge-parity", func(c *webx.Context) {
		webx.OK(c.W, MatrixData())
	})
}
