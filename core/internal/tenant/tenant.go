// Package tenant: multi-tenant accounts (the "users" of this CDN).
// Each tenant owns sites/waf-rules/certs/usage, isolated by tenant_id.
package tenant

import (
	"database/sql"
	"fmt"
	"time"

	"edgecdn/core/internal/authx"
	"edgecdn/core/internal/setting"
)

// Tenant is one customer.
type Tenant struct {
	ID             int64  `json:"id"`
	Name           string `json:"name"`
	Username       string `json:"username"`
	Email          string `json:"email"`
	Status         int    `json:"status"` // 1=enabled 0=disabled
	MaxSites       int    `json:"max_sites"`
	TrafficQuotaMB int64  `json:"traffic_quota_mb"` // 0=unlimited
	Balance        int64  `json:"balance"`          // virtual currency
	CreatedAt      string `json:"created_at"`
	passwordHash   string // never serialized
}

// Password returns the bcrypt hash (login only).
func (t *Tenant) Password() string { return t.passwordHash }

// Store wraps the tenants table + tenant business rules.
type Store struct {
	db  *sql.DB
	set *setting.Store
}

func New(db *sql.DB, set *setting.Store) *Store { return &Store{db: db, set: set} }

// List returns paged tenants (0 = all).
func (s *Store) List(keyword string, page, size int) ([]Tenant, int64, error) {
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 200 {
		size = 20
	}
	where := `WHERE 1=1`
	var args []any
	if keyword != "" {
		where += ` AND (name LIKE ? OR username LIKE ? OR email LIKE ?)`
		like := "%" + keyword + "%"
		args = append(args, like, like, like)
	}
	var total int64
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM tenants `+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.db.Query(`SELECT id, name, username, email, status, max_sites, traffic_quota_mb, balance, created_at
		FROM tenants `+where+` ORDER BY id DESC LIMIT ? OFFSET ?`,
		append(args, size, (page-1)*size)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []Tenant
	for rows.Next() {
		var t Tenant
		if err := rows.Scan(&t.ID, &t.Name, &t.Username, &t.Email, &t.Status,
			&t.MaxSites, &t.TrafficQuotaMB, &t.Balance, &t.CreatedAt); err != nil {
			return nil, 0, err
		}
		out = append(out, t)
	}
	return out, total, rows.Err()
}

// Get fetches by id.
func (s *Store) Get(id int64) (Tenant, error) {
	var t Tenant
	err := s.db.QueryRow(`SELECT id, name, username, email, status, max_sites, traffic_quota_mb, balance, created_at
		FROM tenants WHERE id = ?`, id).
		Scan(&t.ID, &t.Name, &t.Username, &t.Email, &t.Status, &t.MaxSites, &t.TrafficQuotaMB, &t.Balance, &t.CreatedAt)
	if err == sql.ErrNoRows {
		return t, fmt.Errorf("tenant not found")
	}
	return t, err
}

// GetByUsername is used at login.
func (s *Store) GetByUsername(username string) (Tenant, error) {
	var t Tenant
	err := s.db.QueryRow(`SELECT id, name, username, password_hash, email, status, max_sites, traffic_quota_mb, balance
		FROM tenants WHERE username = ?`, username).
		Scan(&t.ID, &t.Name, &t.Username, &t.passwordHash, &t.Email, &t.Status, &t.MaxSites, &t.TrafficQuotaMB, &t.Balance)
	if err == sql.ErrNoRows {
		return t, fmt.Errorf("tenant not found")
	}
	return t, err
}

// Create inserts a tenant with settings-driven defaults.
func (s *Store) Create(name, username, password, email string) (Tenant, error) {
	if name == "" || username == "" || password == "" {
		return Tenant{}, fmt.Errorf("name/username/password required")
	}
	if len(password) < 6 {
		return Tenant{}, fmt.Errorf("password too short (min 6)")
	}
	hash, err := authx.HashPassword(password)
	if err != nil {
		return Tenant{}, err
	}
	maxSites := s.set.GetInt("tenant.default_max_sites", 10)
	quotaMB := int64(s.set.GetInt("tenant.default_traffic_mb", 10240))
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := s.db.Exec(`INSERT INTO tenants (name, username, password_hash, email, status, max_sites, traffic_quota_mb, balance, created_at)
		VALUES (?,?,?,?,1,?,?,0,?)`, name, username, hash, email, maxSites, quotaMB, now)
	if err != nil {
		return Tenant{}, fmt.Errorf("username already exists or db error: %w", err)
	}
	id, _ := res.LastInsertId()
	return s.Get(id)
}

// Update modifies tenant fields (password optional: empty = keep).
func (s *Store) Update(id int64, name, email string, maxSites int, quotaMB int64, status int, newPw string) error {
	if _, err := s.Get(id); err != nil {
		return err
	}
	if newPw != "" {
		if len(newPw) < 6 {
			return fmt.Errorf("password too short (min 6)")
		}
		hash, err := authx.HashPassword(newPw)
		if err != nil {
			return err
		}
		if _, err := s.db.Exec(`UPDATE tenants SET name=?, email=?, max_sites=?, traffic_quota_mb=?, status=?, password_hash=? WHERE id=?`,
			name, email, maxSites, quotaMB, status, hash, id); err != nil {
			return err
		}
		return nil
	}
	_, err := s.db.Exec(`UPDATE tenants SET name=?, email=?, max_sites=?, traffic_quota_mb=?, status=? WHERE id=?`,
		name, email, maxSites, quotaMB, status, id)
	return err
}

// Delete removes a tenant (blocked while it still has sites).
func (s *Store) Delete(id int64) error {
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM sites WHERE tenant_id = ?`, id).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return fmt.Errorf("tenant still has %d site(s), delete them first", n)
	}
	_, err := s.db.Exec(`DELETE FROM tenants WHERE id=?`, id)
	return err
}

// CountSites returns the tenant's site count (quota check).
func (s *Store) CountSites(tenantID int64) (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM sites WHERE tenant_id = ?`, tenantID).Scan(&n)
	return n, err
}
