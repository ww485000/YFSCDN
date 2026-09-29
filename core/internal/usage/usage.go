// Package usage: per-tenant daily traffic aggregation (billing source).
package usage

import (
	"database/sql"
	"fmt"
	"time"
)

// Daily is one (tenant, node, site, day) aggregate row.
type Daily struct {
	ID           int64  `json:"id"`
	TenantID     int64  `json:"tenant_id"`
	NodeID       int64  `json:"node_id"`
	SiteID       int64  `json:"site_id"`
	Day          string `json:"day"`
	Requests     int64  `json:"requests"`
	Bytes        int64  `json:"bytes"`
	CacheHits    int64  `json:"cache_hits"`
	CacheMisses  int64  `json:"cache_misses"`
}

// Summary is an aggregated window result.
type Summary struct {
	Requests    int64 `json:"requests"`
	Bytes       int64 `json:"bytes"`
	CacheHits   int64 `json:"cache_hits"`
	CacheMisses int64 `json:"cache_misses"`
	Days        int64 `json:"days"`
}

// Store wraps usage_daily.
type Store struct{ db *sql.DB }

func New(db *sql.DB) *Store { return &Store{db: db} }

// Record upserts one heartbeat report (atomic per-day counters).
func (s *Store) Record(tenantID, nodeID, siteID, requests, bytes, cacheHits, cacheMisses int64) error {
	day := time.Now().UTC().Format("2006-01-02")
	_, err := s.db.Exec(`INSERT INTO usage_daily (tenant_id, node_id, site_id, day, requests, bytes, cache_hits, cache_misses)
		VALUES (?,?,?,?,?,?,?,?)
		ON CONFLICT(tenant_id, node_id, site_id, day) DO UPDATE SET
			requests = requests + excluded.requests,
			bytes = bytes + excluded.bytes,
			cache_hits = cache_hits + excluded.cache_hits,
			cache_misses = cache_misses + excluded.cache_misses`,
		tenantID, nodeID, siteID, day, requests, bytes, cacheHits, cacheMisses)
	return err
}

// List returns daily rows in range; tenantID 0 = all.
func (s *Store) List(tenantID int64, from, to string) ([]Daily, error) {
	where := `WHERE 1=1`
	var args []any
	if tenantID > 0 {
		where += ` AND tenant_id = ?`
		args = append(args, tenantID)
	}
	if from != "" {
		where += ` AND day >= ?`
		args = append(args, from)
	}
	if to != "" {
		where += ` AND day <= ?`
		args = append(args, to)
	}
	rows, err := s.db.Query(`SELECT id, tenant_id, node_id, site_id, day, requests, bytes, cache_hits, cache_misses
		FROM usage_daily `+where+` ORDER BY day DESC, id DESC LIMIT 500`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Daily
	for rows.Next() {
		var d Daily
		if err := rows.Scan(&d.ID, &d.TenantID, &d.NodeID, &d.SiteID, &d.Day,
			&d.Requests, &d.Bytes, &d.CacheHits, &d.CacheMisses); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// Summary aggregates a range; tenantID 0 = all.
func (s *Store) Summary(tenantID int64, from, to string) (Summary, error) {
	var sm Summary
	where := `WHERE 1=1`
	var args []any
	if tenantID > 0 {
		where += ` AND tenant_id = ?`
		args = append(args, tenantID)
	}
	if from != "" {
		where += ` AND day >= ?`
		args = append(args, from)
	}
	if to != "" {
		where += ` AND day <= ?`
		args = append(args, to)
	}
	err := s.db.QueryRow(`SELECT COALESCE(SUM(requests),0), COALESCE(SUM(bytes),0),
		COALESCE(SUM(cache_hits),0), COALESCE(SUM(cache_misses),0), COUNT(DISTINCT day)
		FROM usage_daily `+where, args...).
		Scan(&sm.Requests, &sm.Bytes, &sm.CacheHits, &sm.CacheMisses, &sm.Days)
	return sm, err
}

// DayPoint is one daily aggregate (all sites/nodes, or filtered).
type DayPoint struct {
	Day         string `json:"day"`
	Requests    int64  `json:"requests"`
	Bytes       int64  `json:"bytes"`
	CacheHits   int64  `json:"cache_hits"`
	CacheMisses int64  `json:"cache_misses"`
}

// Series returns per-day points over the last `days` days (ending today),
// optionally scoped to a tenant and/or a single site.
func (s *Store) Series(tenantID, siteID int64, days int) ([]DayPoint, error) {
	if days < 1 || days > 365 {
		days = 14
	}
	from := time.Now().UTC().AddDate(0, 0, -(days - 1)).Format("2006-01-02")
	where := `WHERE day >= ?`
	args := []any{from}
	if tenantID > 0 {
		where += ` AND tenant_id = ?`
		args = append(args, tenantID)
	}
	if siteID > 0 {
		where += ` AND site_id = ?`
		args = append(args, siteID)
	}
	rows, err := s.db.Query(`SELECT day, COALESCE(SUM(requests),0), COALESCE(SUM(bytes),0),
		COALESCE(SUM(cache_hits),0), COALESCE(SUM(cache_misses),0)
		FROM usage_daily `+where+` GROUP BY day ORDER BY day ASC`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []DayPoint{}
	for rows.Next() {
		var d DayPoint
		if err := rows.Scan(&d.Day, &d.Requests, &d.Bytes, &d.CacheHits, &d.CacheMisses); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// SiteTotal is one site's aggregate over a window.
type SiteTotal struct {
	SiteID      int64 `json:"site_id"`
	Requests    int64 `json:"requests"`
	Bytes       int64 `json:"bytes"`
	CacheHits   int64 `json:"cache_hits"`
	CacheMisses int64 `json:"cache_misses"`
}

// SiteTotals aggregates per-site counters over the last `days` days.
func (s *Store) SiteTotals(tenantID int64, days int) ([]SiteTotal, error) {
	if days < 1 || days > 365 {
		days = 7
	}
	from := time.Now().UTC().AddDate(0, 0, -(days - 1)).Format("2006-01-02")
	where := `WHERE day >= ?`
	args := []any{from}
	if tenantID > 0 {
		where += ` AND tenant_id = ?`
		args = append(args, tenantID)
	}
	rows, err := s.db.Query(`SELECT site_id, COALESCE(SUM(requests),0), COALESCE(SUM(bytes),0),
		COALESCE(SUM(cache_hits),0), COALESCE(SUM(cache_misses),0)
		FROM usage_daily `+where+` GROUP BY site_id ORDER BY bytes DESC LIMIT 200`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SiteTotal
	for rows.Next() {
		var t SiteTotal
		if err := rows.Scan(&t.SiteID, &t.Requests, &t.Bytes, &t.CacheHits, &t.CacheMisses); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// TodayTotal returns platform-wide today counters (dashboard).
func (s *Store) TodayTotal() (requests, bytes int64, err error) {
	day := time.Now().UTC().Format("2006-01-02")
	err = s.db.QueryRow(`SELECT COALESCE(SUM(requests),0), COALESCE(SUM(bytes),0) FROM usage_daily WHERE day = ?`, day).
		Scan(&requests, &bytes)
	if err != nil {
		return 0, 0, fmt.Errorf("today total: %w", err)
	}
	return requests, bytes, nil
}
