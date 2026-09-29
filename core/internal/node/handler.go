package node

import (
	"database/sql"
	"encoding/json"
	"strconv"
	"time"

	"edgecdn/core/internal/contract"
	"edgecdn/core/internal/logs"
	"edgecdn/core/internal/outbox"
	"edgecdn/core/internal/setting"
	"edgecdn/core/internal/site"
	"edgecdn/core/internal/usage"
	"edgecdn/core/internal/webx"
)

// Admin handles operator-side node routes.
type Admin struct{ svc *Store }

// RegisterAdmin mounts node management routes.
func RegisterAdmin(r *webx.Router, svc *Store) {
	h := &Admin{svc: svc}
	r.GET("/api/v1/admin/nodes", h.list)
	r.POST("/api/v1/admin/nodes", h.create)
	r.PUT("/api/v1/admin/nodes/:id", h.update)
	r.DELETE("/api/v1/admin/nodes/:id", h.delete)
}

func (h *Admin) list(c *webx.Context) {
	out, total, err := h.svc.List(webx.QueryInt64(c, "tenant_id", 0), webx.QueryStr(c, "keyword"),
		webx.QueryInt(c, "page", 1), webx.QueryInt(c, "size", 20))
	if err != nil {
		webx.Fail(c.W, 500, 500, err.Error())
		return
	}
	webx.Page(c.W, out, total)
}

func (h *Admin) create(c *webx.Context) {
	var req struct {
		Name     string `json:"name"`
		TenantID int64  `json:"tenant_id"`
	}
	if err := webx.BindJSON(c, &req); err != nil {
		webx.Fail(c.W, 400, 400, "bad json body")
		return
	}
	n, err := h.svc.Create(req.Name, req.TenantID)
	if err != nil {
		webx.Fail(c.W, 400, 400, err.Error())
		return
	}
	webx.OK(c.W, n)
}

func (h *Admin) update(c *webx.Context) {
	id, err := webx.ParamInt64(c, "id")
	if err != nil {
		webx.Fail(c.W, 400, 400, err.Error())
		return
	}
	var req struct {
		Name     string `json:"name"`
		TenantID int64  `json:"tenant_id"`
	}
	if err := webx.BindJSON(c, &req); err != nil {
		webx.Fail(c.W, 400, 400, "bad json body")
		return
	}
	if req.Name == "" {
		cur, _ := h.svc.Get(id)
		req.Name = cur.Name
	}
	if err := h.svc.Update(id, req.Name, req.TenantID); err != nil {
		webx.Fail(c.W, 400, 400, err.Error())
		return
	}
	webx.OK(c.W, map[string]int64{"id": id})
}

func (h *Admin) delete(c *webx.Context) {
	id, err := webx.ParamInt64(c, "id")
	if err != nil {
		webx.Fail(c.W, 400, 400, err.Error())
		return
	}
	if err := h.svc.Delete(id); err != nil {
		webx.Fail(c.W, 400, 400, err.Error())
		return
	}
	webx.OK(c.W, map[string]int64{"id": id})
}

// RegisterUser mounts the tenant-scoped read-only node list.
func RegisterUser(r *webx.Router, svc *Store) {
	r.GET("/api/v1/user/nodes", func(c *webx.Context) {
		out, total, err := svc.List(c.User.TenantID(), "", 1, 200) // TenantID() is int64
		if err != nil {
			webx.Fail(c.W, 500, 500, err.Error())
			return
		}
		webx.Page(c.W, out, total)
	})
}

// Edge handles node-side API (token auth, no JWT).
type Edge struct {
	svc    *Store
	outbox *outbox.Outbox
	usage  *usage.Store
	set    *setting.Store
	db     *sql.DB
	sites  *site.Store
	logs   *logs.Store
}

// RegisterEdge mounts the edge-node API.
func RegisterEdge(r *webx.Router, svc *Store, ob *outbox.Outbox, u *usage.Store, set *setting.Store, db *sql.DB, sites *site.Store, logs *logs.Store) {
	h := &Edge{svc: svc, outbox: ob, usage: u, set: set, db: db, sites: sites, logs: logs}
	r.POST("/api/v1/edge/register", h.register)
	r.GET("/api/v1/edge/poll", h.poll)
	r.POST("/api/v1/edge/heartbeat", h.heartbeat)
	r.GET("/api/v1/edge/full", h.full)
	r.POST("/api/v1/edge/logs", h.uploadLogs)
}

// full returns every active site assigned to the node (edge restart recovery).
// Sites without any node binding are broadcast to all nodes.
// NOTE: single-connection DB — collect IDs first, close rows, THEN run the
// per-site nested queries (a query inside an open rows loop deadlocks).
func (h *Edge) full(c *webx.Context) {
	n, err := h.svc.GetByToken(c.R.Header.Get("X-Node-Token"))
	if err != nil {
		n, err = h.svc.GetByToken(webx.QueryStr(c, "token"))
	}
	if err != nil {
		webx.Fail(c.W, 401, 401, "unknown node token")
		return
	}
	var ids []int64
	{
		rows, err := h.db.Query(`SELECT id FROM sites WHERE status = 'normal' AND
			(id IN (SELECT site_id FROM site_nodes WHERE node_id = ?)
			 OR id NOT IN (SELECT site_id FROM site_nodes))`, n.ID)
		if err != nil {
			webx.Fail(c.W, 500, 500, err.Error())
			return
		}
		for rows.Next() {
			var id int64
			if rows.Scan(&id) == nil {
				ids = append(ids, id)
			}
		}
		rows.Close()
	}
	specs := []contract.Spec{}
	for _, id := range ids {
		st, err := h.sites.Get(id)
		if err != nil {
			continue
		}
		spec, err := h.sites.BuildSpec(st)
		if err != nil {
			continue
		}
		specs = append(specs, spec)
	}
	webx.OK(c.W, map[string]any{"sites": specs})
}

func (h *Edge) register(c *webx.Context) {
	var req struct {
		Token   string `json:"token"`
		Host    string `json:"host"`
		IP      string `json:"ip"`
		Driver  string `json:"driver"`
		Version string `json:"version"`
	}
	if err := webx.BindJSON(c, &req); err != nil {
		webx.Fail(c.W, 400, 400, "bad json body")
		return
	}
	n, err := h.svc.Register(req.Token, req.Host, req.IP, req.Driver)
	if err != nil {
		webx.Fail(c.W, 401, 401, err.Error())
		return
	}
	h.svc.Touch(n.ID, req.Host, req.IP, req.Driver, req.Version)
	webx.OK(c.W, map[string]any{"node_id": n.ID, "name": n.Name})
}

// pollEvent is the wire format of one outbox event (payload stays a JSON object).
type pollEvent struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

// poll implements long polling: waits up to the configured timeout for new events.
func (h *Edge) poll(c *webx.Context) {
	n, err := h.svc.GetByToken(c.R.Header.Get("X-Node-Token"))
	if err == nil {
		h.svc.Touch(n.ID, "", "", n.Driver, n.Version)
	}
	if err != nil {
		// also accept ?token= (some proxies strip headers on GET)
		n, err = h.svc.GetByToken(webx.QueryStr(c, "token"))
	}
	if err != nil {
		webx.Fail(c.W, 401, 401, "unknown node token")
		return
	}
	var cursor int64
	if v := webx.QueryStr(c, "cursor"); v != "" {
		cursor, _ = strconv.ParseInt(v, 10, 64)
	}
	timeout := time.Duration(h.set.GetInt("edge.poll_timeout_sec", 30)) * time.Second
	deadline := time.Now().Add(timeout)
	for {
		evs, err := h.outbox.Poll(n.ID, cursor, 200)
		if err != nil {
			webx.Fail(c.W, 500, 500, err.Error())
			return
		}
		if len(evs) > 0 {
			out := make([]pollEvent, 0, len(evs))
			max := cursor
			for _, e := range evs {
				if e.Version > max {
					max = e.Version
				}
				out = append(out, pollEvent{Type: e.Type, Payload: json.RawMessage(e.Payload)})
			}
			webx.OK(c.W, map[string]any{"cursor": max, "events": out})
			return
		}
		if time.Now().After(deadline) {
			webx.OK(c.W, map[string]any{"cursor": cursor, "events": []any{}})
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
}

// heartbeatReport is one per-site counter batch.
type heartbeatReport struct {
	SiteID     int64 `json:"site_id"`
	Requests   int64 `json:"requests"`
	Bytes      int64 `json:"bytes"`
	CacheHits  int64 `json:"cache_hits"`
	CacheMisses int64 `json:"cache_misses"`
}

func (h *Edge) heartbeat(c *webx.Context) {
	n, err := h.svc.GetByToken(c.R.Header.Get("X-Node-Token"))
	if err != nil {
		webx.Fail(c.W, 401, 401, "unknown node token")
		return
	}
	var req struct {
		Host    string            `json:"host"`
		IP      string            `json:"ip"`
		Driver  string            `json:"driver"`
		Version string            `json:"version"`
		Reports []heartbeatReport `json:"reports"`
	}
	if err := webx.BindJSON(c, &req); err != nil {
		webx.Fail(c.W, 400, 400, "bad json body")
		return
	}
	ver := req.Version
	if ver == "" {
		ver = n.Version
	}
	h.svc.Touch(n.ID, req.Host, req.IP, req.Driver, ver)
	for _, rep := range req.Reports {
		if rep.SiteID <= 0 || rep.Requests == 0 && rep.Bytes == 0 {
			continue
		}
		var tenantID int64
		if err := h.db.QueryRow(`SELECT tenant_id FROM sites WHERE id = ?`, rep.SiteID).Scan(&tenantID); err != nil {
			continue // site deleted; skip
		}
		_ = h.usage.Record(tenantID, n.ID, rep.SiteID, rep.Requests, rep.Bytes, rep.CacheHits, rep.CacheMisses)
	}
	webx.OK(c.W, map[string]any{"ok": true, "node_id": n.ID})
}

// uploadLogs accepts a batch of access logs from a node.
func (h *Edge) uploadLogs(c *webx.Context) {
	n, err := h.svc.GetByToken(c.R.Header.Get("X-Node-Token"))
	if err != nil {
		webx.Fail(c.W, 401, 401, "unknown node token")
		return
	}
	var req struct {
		Logs []logs.Log `json:"logs"`
	}
	if err := webx.BindJSON(c, &req); err != nil {
		webx.Fail(c.W, 400, 400, "bad json body")
		return
	}
	if len(req.Logs) > 2000 {
		req.Logs = req.Logs[:2000]
	}
	if err := h.logs.InsertBatch(n.ID, req.Logs); err != nil {
		webx.Fail(c.W, 500, 500, err.Error())
		return
	}
	webx.OK(c.W, map[string]any{"ok": true, "stored": len(req.Logs)})
}
