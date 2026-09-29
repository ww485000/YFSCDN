// Package setting: platform KV parameters. See docs/api-contract.md "设置项".
package setting

import (
	"database/sql"
	"fmt"
)

// Defaults must match docs/api-contract.md.
var Defaults = map[string]string{
	"platform.name":            "EdgeCDN",
	"tenant.default_max_sites": "10",
	"tenant.default_traffic_mb": "10240",
	"cdn.rate_per_mb":          "0",
	"edge.poll_timeout_sec":    "30",
}

// Store wraps the settings table.
type Store struct{ db *sql.DB }

func New(db *sql.DB) *Store { return &Store{db: db} }

// EnsureDefaults inserts missing keys (idempotent, call at boot).
func (s *Store) EnsureDefaults() error {
	for k, v := range Defaults {
		if _, err := s.db.Exec(`INSERT INTO settings (key,value) VALUES (?,?) ON CONFLICT(key) DO NOTHING`, k, v); err != nil {
			return err
		}
	}
	return nil
}

// Get returns the value or def when the key is missing.
func (s *Store) Get(key, def string) string {
	var v string
	if err := s.db.QueryRow(`SELECT value FROM settings WHERE key = ?`, key).Scan(&v); err == nil {
		return v
	}
	return def
}

// GetInt parses a numeric setting (falls back to def).
func (s *Store) GetInt(key string, def int) int {
	v := s.Get(key, fmt.Sprintf("%d", def))
	var n int
	if _, err := fmt.Sscanf(v, "%d", &n); err != nil {
		return def
	}
	return n
}

// Set upserts one key.
func (s *Store) Set(key, value string) error {
	_, err := s.db.Exec(`INSERT INTO settings (key,value) VALUES (?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value)
	return err
}

// All returns every key/value pair.
func (s *Store) All() (map[string]string, error) {
	rows, err := s.db.Query(`SELECT key, value FROM settings ORDER BY key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	m := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		m[k] = v
	}
	return m, rows.Err()
}

// PutAll replaces the whole map (admin settings update).
func (s *Store) PutAll(m map[string]string) error {
	for k, v := range m {
		if err := s.Set(k, v); err != nil {
			return err
		}
	}
	return nil
}
