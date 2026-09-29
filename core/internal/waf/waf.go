// Package waf: WAF rule CRUD. Rules are NOT pushed separately —
// on any change the owning tenant's sites are re-emitted via site_upsert
// (edge always applies the full spec, which includes the rule list).
// Rule semantics MUST match docs/api-contract.md and edge/internal/wafx.
package waf

import (
	"database/sql"
	"fmt"
	"time"
)

// Rule is one WAF rule.
type Rule struct {
	ID        int64  `json:"id"`
	TenantID  int64  `json:"tenant_id"`
	Name      string `json:"name"`
	Type      string `json:"type"`   // ip_blacklist|ip_whitelist|ua_blacklist|path_blacklist|rate_limit
	Value     string `json:"value"`
	Action    string `json:"action"` // block | log
	Enabled   int    `json:"enabled"`
	CreatedAt string `json:"created_at"`
}

var validTypes = map[string]bool{
	"ip_blacklist":   true,
	"ip_whitelist":   true,
	"ua_blacklist":   true,
	"path_blacklist": true,
	"rate_limit":     true,
}

// SiteEmitter re-pushes a tenant's site specs after a rule change.
// Implemented by site.Service — kept as an interface to avoid an import cycle.
type SiteEmitter interface {
	EmitForTenant(tenantID int64) error
}

// Store wraps the waf_rules table + rule business rules.
type Store struct {
	db      *sql.DB
	emitter SiteEmitter // optional (nil = no re-push)
}

func New(db *sql.DB) *Store { return &Store{db: db} }

// SetEmitter wires the site emitter (call from app after site service exists).
func (s *Store) SetEmitter(e SiteEmitter) { s.emitter = e }

// List returns paged rules for a tenant (0 = all).
func (s *Store) List(tenantID int64, page, size int) ([]Rule, int64, error) {
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 200 {
		size = 20
	}
	where := `WHERE 1=1`
	var args []any
	if tenantID > 0 {
		where += ` AND tenant_id = ?`
		args = append(args, tenantID)
	}
	var total int64
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM waf_rules `+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.db.Query(`SELECT id, tenant_id, name, type, value, action, enabled, created_at
		FROM waf_rules `+where+` ORDER BY id DESC LIMIT ? OFFSET ?`,
		append(args, size, (page-1)*size)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []Rule
	for rows.Next() {
		var r Rule
		if err := rows.Scan(&r.ID, &r.TenantID, &r.Name, &r.Type, &r.Value, &r.Action, &r.Enabled, &r.CreatedAt); err != nil {
			return nil, 0, err
		}
		out = append(out, r)
	}
	return out, total, rows.Err()
}

// Get fetches by id.
func (s *Store) Get(id int64) (Rule, error) {
	var r Rule
	err := s.db.QueryRow(`SELECT id, tenant_id, name, type, value, action, enabled, created_at FROM waf_rules WHERE id = ?`, id).
		Scan(&r.ID, &r.TenantID, &r.Name, &r.Type, &r.Value, &r.Action, &r.Enabled, &r.CreatedAt)
	if err == sql.ErrNoRows {
		return r, fmt.Errorf("waf rule not found")
	}
	return r, err
}

// EnabledForTenant returns all enabled rules of a tenant (for SiteSpec build).
func (s *Store) EnabledForTenant(tenantID int64) ([]Rule, error) {
	rows, err := s.db.Query(`SELECT id, tenant_id, name, type, value, action, enabled, created_at
		FROM waf_rules WHERE tenant_id = ? AND enabled = 1 ORDER BY id`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Rule
	for rows.Next() {
		var r Rule
		if err := rows.Scan(&r.ID, &r.TenantID, &r.Name, &r.Type, &r.Value, &r.Action, &r.Enabled, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// Create inserts a rule and re-pushes affected sites.
func (s *Store) Create(tenantID int64, name, typ, value, action string) (Rule, error) {
	if name == "" || !validTypes[typ] || value == "" {
		return Rule{}, fmt.Errorf("name/value required, type must be one of the 5 WAF types")
	}
	if action != "block" && action != "log" {
		action = "block"
	}
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := s.db.Exec(`INSERT INTO waf_rules (tenant_id, name, type, value, action, enabled, created_at) VALUES (?,?,?,?,?,1,?)`,
		tenantID, name, typ, value, action, now)
	if err != nil {
		return Rule{}, err
	}
	id, _ := res.LastInsertId()
	r, err := s.Get(id)
	if err != nil {
		return Rule{}, err
	}
	return r, s.push(tenantID)
}

// Update modifies a rule and re-pushes affected sites.
func (s *Store) Update(id int64, name, typ, value, action string, enabled int) error {
	r, err := s.Get(id)
	if err != nil {
		return err
	}
	if typ != "" && !validTypes[typ] {
		return fmt.Errorf("invalid type")
	}
	if name == "" {
		name = r.Name
	}
	if typ == "" {
		typ = r.Type
	}
	if value == "" {
		value = r.Value
	}
	if action != "block" && action != "log" {
		action = r.Action
	}
	if enabled != 0 && enabled != 1 {
		enabled = 1
	}
	_, err = s.db.Exec(`UPDATE waf_rules SET name=?, type=?, value=?, action=?, enabled=? WHERE id=?`,
		name, typ, value, action, enabled, id)
	if err != nil {
		return err
	}
	return s.push(r.TenantID)
}

// Delete removes a rule and re-pushes affected sites.
func (s *Store) Delete(id int64) error {
	r, err := s.Get(id)
	if err != nil {
		return err
	}
	if _, err := s.db.Exec(`DELETE FROM waf_rules WHERE id=?`, id); err != nil {
		return err
	}
	return s.push(r.TenantID)
}

// push re-emits all sites of the tenant so edges pick up the new rules.
func (s *Store) push(tenantID int64) error {
	if s.emitter == nil {
		return nil
	}
	return s.emitter.EmitForTenant(tenantID)
}
