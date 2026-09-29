// Package storex opens SQLite (pure-Go driver) and runs idempotent migrations.
// All DDL for the whole core lives in Migrations below — single place to change schema.
package storex

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite" // registers driver "sqlite"
)

// Migrations are executed in order at boot (IF NOT EXISTS => idempotent).
// Append new tables at the END; never edit existing statements (old DBs depend on them).
var Migrations = []string{
	`CREATE TABLE IF NOT EXISTS admins (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		username TEXT NOT NULL UNIQUE,
		password_hash TEXT NOT NULL,
		role TEXT NOT NULL DEFAULT 'operator',
		status INTEGER NOT NULL DEFAULT 1,
		created_at TEXT NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS tenants (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		username TEXT NOT NULL UNIQUE,
		password_hash TEXT NOT NULL,
		email TEXT NOT NULL DEFAULT '',
		status INTEGER NOT NULL DEFAULT 1,
		max_sites INTEGER NOT NULL DEFAULT 10,
		traffic_quota_mb INTEGER NOT NULL DEFAULT 0,
		balance INTEGER NOT NULL DEFAULT 0,
		created_at TEXT NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS nodes (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		token TEXT NOT NULL UNIQUE,
		tenant_id INTEGER NOT NULL DEFAULT 0,
		status INTEGER NOT NULL DEFAULT 0,
		driver TEXT NOT NULL DEFAULT '',
		host TEXT NOT NULL DEFAULT '',
		ip TEXT NOT NULL DEFAULT '',
		last_seen TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS sites (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		tenant_id INTEGER NOT NULL,
		name TEXT NOT NULL,
		domain TEXT NOT NULL UNIQUE,
		origin_proto TEXT NOT NULL DEFAULT 'http',
		origin_host TEXT NOT NULL,
		origin_port INTEGER NOT NULL DEFAULT 80,
		cache_ttl INTEGER NOT NULL DEFAULT 0,
		waf_enabled INTEGER NOT NULL DEFAULT 0,
		https INTEGER NOT NULL DEFAULT 0,
		status INTEGER NOT NULL DEFAULT 1,
		created_at TEXT NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS site_nodes (
		site_id INTEGER NOT NULL,
		node_id INTEGER NOT NULL,
		PRIMARY KEY (site_id, node_id)
	)`,
	`CREATE TABLE IF NOT EXISTS waf_rules (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		tenant_id INTEGER NOT NULL,
		name TEXT NOT NULL,
		type TEXT NOT NULL,
		value TEXT NOT NULL,
		action TEXT NOT NULL DEFAULT 'block',
		enabled INTEGER NOT NULL DEFAULT 1,
		created_at TEXT NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS certs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		tenant_id INTEGER NOT NULL,
		domain TEXT NOT NULL UNIQUE,
		leaf_pem TEXT NOT NULL,
		key_pem TEXT NOT NULL,
		expires_at TEXT,
		created_at TEXT NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS usage_daily (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		tenant_id INTEGER NOT NULL,
		node_id INTEGER NOT NULL,
		site_id INTEGER NOT NULL,
		day TEXT NOT NULL,
		requests INTEGER NOT NULL DEFAULT 0,
		bytes INTEGER NOT NULL DEFAULT 0,
		cache_hits INTEGER NOT NULL DEFAULT 0,
		cache_misses INTEGER NOT NULL DEFAULT 0,
		UNIQUE (tenant_id, node_id, site_id, day)
	)`,
	`CREATE TABLE IF NOT EXISTS settings (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS oplogs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		actor_type TEXT NOT NULL,
		actor_id INTEGER NOT NULL,
		action TEXT NOT NULL,
		target TEXT NOT NULL DEFAULT '',
		detail TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS outbox (
		version INTEGER PRIMARY KEY AUTOINCREMENT,
		node_id INTEGER NOT NULL DEFAULT 0,
		type TEXT NOT NULL,
		payload TEXT NOT NULL,
		created_at TEXT NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS tasks (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		type TEXT NOT NULL,
		target TEXT NOT NULL DEFAULT '',
		payload TEXT NOT NULL DEFAULT '',
		status TEXT NOT NULL DEFAULT 'pending',
		error TEXT NOT NULL DEFAULT '',
		attempts INTEGER NOT NULL DEFAULT 0,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL
	)`,
	`CREATE INDEX IF NOT EXISTS idx_tasks_status ON tasks (status, id)`,
	`CREATE TABLE IF NOT EXISTS access_logs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		node_id INTEGER NOT NULL,
		site_id INTEGER NOT NULL,
		ts TEXT NOT NULL,
		ip TEXT NOT NULL DEFAULT '',
		method TEXT NOT NULL DEFAULT '',
		host TEXT NOT NULL DEFAULT '',
		path TEXT NOT NULL DEFAULT '',
		status INTEGER NOT NULL DEFAULT 0,
		bytes INTEGER NOT NULL DEFAULT 0,
		cache TEXT NOT NULL DEFAULT '',
		ua TEXT NOT NULL DEFAULT '',
		referer TEXT NOT NULL DEFAULT '',
		latency_ms INTEGER NOT NULL DEFAULT 0
	)`,
	`CREATE INDEX IF NOT EXISTS idx_access_logs_site ON access_logs (site_id, id DESC)`,
	`CREATE INDEX IF NOT EXISTS idx_access_logs_ts ON access_logs (ts)`,
	`CREATE TABLE IF NOT EXISTS dns_records (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		tenant_id INTEGER NOT NULL,
		domain TEXT NOT NULL,
		name TEXT NOT NULL,
		type TEXT NOT NULL,
		value TEXT NOT NULL,
		priority INTEGER NOT NULL DEFAULT 0,
		ttl INTEGER NOT NULL DEFAULT 600,
		enabled INTEGER NOT NULL DEFAULT 1,
		remark TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL
	)`,
	`CREATE INDEX IF NOT EXISTS idx_dns_records_domain ON dns_records (domain, id DESC)`,
}

// codeMigrations adds columns to existing tables (SQLite ALTER is not
// idempotent, so each one is guarded by a PRAGMA table_info check).
func codeMigrations(db *sql.DB) error {
	type alter struct{ table, column, def string }
	alters := []alter{
		{"sites", "config", "TEXT NOT NULL DEFAULT '{}'"},
		{"sites", "updated_at", "TEXT NOT NULL DEFAULT ''"},
		{"certs", "name", "TEXT NOT NULL DEFAULT ''"},
		{"certs", "type", "TEXT NOT NULL DEFAULT 'ca'"},
		{"certs", "issuer", "TEXT NOT NULL DEFAULT ''"},
		{"nodes", "version", "TEXT NOT NULL DEFAULT ''"},
	}
	for _, a := range alters {
		rows, err := db.Query(`PRAGMA table_info(` + a.table + `)`)
		if err != nil {
			return err
		}
		exists := map[string]bool{}
		for rows.Next() {
			var cid int
			var name, typ string
			var notNull int
			var dflt any
			var pk int
			if err := rows.Scan(&cid, &name, &typ, &notNull, &dflt, &pk); err == nil {
				exists[name] = true
			}
		}
		rows.Close()
		if !exists[a.column] {
			if _, err := db.Exec(`ALTER TABLE ` + a.table + ` ADD COLUMN ` + a.column + ` ` + a.def); err != nil {
				return fmt.Errorf("alter %s.%s: %w", a.table, a.column, err)
			}
		}
	}
	return nil
}

// Open opens the SQLite database and applies migrations.
// A single connection is enforced (SQLite writer serialization, safe for this scale).
func Open(path string) (*sql.DB, error) {
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	pragmas := []string{
		`PRAGMA journal_mode=WAL`,
		`PRAGMA synchronous=NORMAL`,
		`PRAGMA foreign_keys=ON`,
		`PRAGMA busy_timeout=5000`,
	}
	for _, p := range pragmas {
		if _, err := db.Exec(p); err != nil {
			return nil, fmt.Errorf("pragma %s: %w", p, err)
		}
	}
	for _, m := range Migrations {
		if _, err := db.Exec(m); err != nil {
			return nil, fmt.Errorf("migration: %w\nSQL: %s", err, m)
		}
	}
	if err := codeMigrations(db); err != nil {
		return nil, err
	}
	return db, nil
}
