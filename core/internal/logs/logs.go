// Package logs stores edge access logs (uploaded by nodes) and serves queries.
package logs

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// Log is one access-log line.
type Log struct {
	ID        int64  `json:"id"`
	NodeID    int64  `json:"node_id"`
	SiteID    int64  `json:"site_id"`
	TS        string `json:"ts"`
	IP        string `json:"ip"`
	Method    string `json:"method"`
	Host      string `json:"host"`
	Path      string `json:"path"`
	Status    int    `json:"status"`
	Bytes     int64  `json:"bytes"`
	Cache     string `json:"cache"` // HIT|MISS|BYPASS|STALE|BLOCKED|""
	UA        string `json:"ua"`
	Referer   string `json:"referer"`
	LatencyMs int64  `json:"latency_ms"`
}

// Store wraps the access_logs table.
type Store struct{ db *sql.DB }

func New(db *sql.DB) *Store { return &Store{db: db} }

// MaxKept caps stored rows (pruned oldest-first beyond this).
const MaxKept = 100000

// InsertBatch appends one node's log batch.
func (s *Store) InsertBatch(nodeID int64, batch []Log) error {
	if len(batch) == 0 {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, l := range batch {
		if _, err := tx.Exec(`INSERT INTO access_logs (node_id, site_id, ts, ip, method, host, path, status, bytes, cache, ua, referer, latency_ms)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			nodeID, l.SiteID, l.TS, l.IP, l.Method, l.Host, l.Path, l.Status, l.Bytes, l.Cache, l.UA, l.Referer, l.LatencyMs); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	s.Prune(MaxKept)
	return nil
}

// List returns paged logs for a site with optional filters.
func (s *Store) List(siteID int64, ip, path string, status int, page, size int) ([]Log, int64, error) {
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 500 {
		size = 50
	}
	where := `WHERE site_id = ?`
	args := []any{siteID}
	if ip != "" {
		where += ` AND ip LIKE ?`
		args = append(args, "%"+ip+"%")
	}
	if path != "" {
		where += ` AND path LIKE ?`
		args = append(args, "%"+path+"%")
	}
	if status > 0 {
		where += ` AND status = ?`
		args = append(args, status)
	}
	var total int64
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM access_logs `+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.db.Query(`SELECT id, node_id, site_id, ts, ip, method, host, path, status, bytes, cache, ua, referer, latency_ms
		FROM access_logs `+where+` ORDER BY id DESC LIMIT ? OFFSET ?`,
		append(args, size, (page-1)*size)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []Log
	for rows.Next() {
		var l Log
		if err := rows.Scan(&l.ID, &l.NodeID, &l.SiteID, &l.TS, &l.IP, &l.Method, &l.Host, &l.Path,
			&l.Status, &l.Bytes, &l.Cache, &l.UA, &l.Referer, &l.LatencyMs); err != nil {
			return nil, 0, err
		}
		out = append(out, l)
	}
	return out, total, rows.Err()
}

// Prune deletes the oldest rows beyond keep.
func (s *Store) Prune(keep int) {
	var total int64
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM access_logs`).Scan(&total); err != nil || total <= int64(keep) {
		return
	}
	excess := total - int64(keep)
	cutoffID := fmt.Sprintf("(SELECT id FROM access_logs ORDER BY id DESC LIMIT 1 OFFSET %d)", keep)
	if _, err := s.db.Exec(`DELETE FROM access_logs WHERE id <= `+cutoffID); err != nil {
		_ = excess
	}
}

// TodayBySite returns today's per-site counters (dashboard/usage page).
func (s *Store) TodayBySite(tenantID int64) (map[int64]struct{ req, byt int64 }, error) {
	day := time.Now().UTC().Format("2006-01-02")
	q := `SELECT site_id, COUNT(*), COALESCE(SUM(bytes),0) FROM access_logs WHERE ts >= ?`
	var args []any
	if tenantID > 0 {
		q += ` AND site_id IN (SELECT id FROM sites WHERE tenant_id = ?)`
		args = append(args, day, tenantID)
	} else {
		args = append(args, day)
	}
	q += ` GROUP BY site_id`
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]struct{ req, byt int64 }{}
	for rows.Next() {
		var sid int64
		var req, byt int64
		if rows.Scan(&sid, &req, &byt) == nil {
			out[sid] = struct{ req, byt int64 }{req, byt}
		}
	}
	return out, rows.Err()
}

// StatusDist returns status-code buckets for a site (last N rows).
func (s *Store) StatusDist(siteID int64, limit int) (map[string]int64, error) {
	if limit <= 0 || limit > 5000 {
		limit = 5000
	}
	rows, err := s.db.Query(`SELECT status, COUNT(*) FROM (
		SELECT status FROM access_logs WHERE site_id = ? ORDER BY id DESC LIMIT ?) GROUP BY status`, siteID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int64{}
	var keys []string
	for rows.Next() {
		var st int
		var n int64
		if rows.Scan(&st, &n) == nil {
			k := fmt.Sprintf("%d", st)
			out[k] = n
			keys = append(keys, k)
		}
	}
	_ = strings.Join(keys, ",")
	return out, rows.Err()
}
