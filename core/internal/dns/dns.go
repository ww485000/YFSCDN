// Package dns: DNS record management (GoEdge "DNS 解析" module).
//
// Records are stored per tenant/zone and can be pushed to an upstream DNS
// provider. The dns_resolve/dns_clean task types call dnsprovider drivers;
// providers that do not yet have a real adapter degrade with an explicit task
// error instead of blocking local DNS record management.
package dns

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// Supported record types.
var Types = []string{"A", "AAAA", "CNAME", "MX", "TXT", "NS", "SRV"}

// Record is one DNS record row.
type Record struct {
	ID        int64  `json:"id"`
	TenantID  int64  `json:"tenant_id"`
	Domain    string `json:"domain"` // zone (apex domain)
	Name      string `json:"name"`   // full record name (FQDN)
	Type      string `json:"type"`
	Value     string `json:"value"`
	Priority  int    `json:"priority"` // MX only
	TTL       int    `json:"ttl"`
	Enabled   int    `json:"enabled"`
	Line      string `json:"line"`      // provider line/route: default, telecom, unicom, etc.
	Weight    int    `json:"weight"`    // provider load-balance weight, if supported
	Proxied   int    `json:"proxied"`   // Cloudflare proxy switch
	SyncMode  string `json:"sync_mode"` // auto | manual
	Remark    string `json:"remark"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// Store wraps the dns_records table.
type Store struct{ db *sql.DB }

func New(db *sql.DB) *Store { return &Store{db: db} }

const cols = `id, tenant_id, domain, name, type, value, priority, ttl, enabled, line, weight, proxied, sync_mode, remark, created_at, updated_at`

func scan(r interface{ Scan(...any) error }) (Record, error) {
	var c Record
	err := r.Scan(&c.ID, &c.TenantID, &c.Domain, &c.Name, &c.Type, &c.Value, &c.Priority, &c.TTL, &c.Enabled, &c.Line, &c.Weight, &c.Proxied, &c.SyncMode, &c.Remark, &c.CreatedAt, &c.UpdatedAt)
	return c, err
}

func normalize(t string) string { return strings.ToUpper(strings.TrimSpace(t)) }

func normalizeLine(line string) string {
	line = strings.TrimSpace(line)
	if line == "" {
		return "default"
	}
	return line
}

func normalizeSyncMode(mode string) string {
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode == "" {
		return "auto"
	}
	switch mode {
	case "auto", "manual":
		return mode
	default:
		return "auto"
	}
}

func (c Record) validate() error {
	if strings.TrimSpace(c.Domain) == "" {
		return fmt.Errorf("domain is required")
	}
	t := normalize(c.Type)
	ok := false
	for _, x := range Types {
		if x == t {
			ok = true
			break
		}
	}
	if !ok {
		return fmt.Errorf("unsupported record type %q (allowed: %s)", c.Type, strings.Join(Types, "/"))
	}
	if strings.TrimSpace(c.Value) == "" {
		return fmt.Errorf("value is required")
	}
	if t == "MX" && c.Priority < 0 {
		return fmt.Errorf("MX priority must be >= 0")
	}
	if c.Weight < 0 {
		return fmt.Errorf("weight must be >= 0")
	}
	return nil
}

func (s *Store) insert(tenantID int64, c *Record) (int64, error) {
	if err := c.validate(); err != nil {
		return 0, err
	}
	c.Type = normalize(c.Type)
	c.Name = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(c.Name)), ".")
	if c.Name == "" {
		c.Name = c.Domain
	}
	c.Domain = strings.ToLower(strings.TrimSpace(c.Domain))
	c.Line = normalizeLine(c.Line)
	c.SyncMode = normalizeSyncMode(c.SyncMode)
	c.Remark = strings.TrimSpace(c.Remark)
	if c.TTL <= 0 {
		c.TTL = 600
	}
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := s.db.Exec(`INSERT INTO dns_records (tenant_id, domain, name, type, value, priority, ttl, enabled, line, weight, proxied, sync_mode, remark, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		tenantID, c.Domain, c.Name, c.Type, c.Value, c.Priority, c.TTL, c.Enabled, c.Line, c.Weight, c.Proxied, c.SyncMode, c.Remark, now, now)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// Create adds a record (admin passes tenant id; user flow passes its own).
func (s *Store) Create(tenantID int64, c *Record) (int64, error) { return s.insert(tenantID, c) }

// Update changes a record (admin only; tenant id is not changeable).
func (s *Store) Update(c *Record) error {
	var tid int64
	err := s.db.QueryRow(`SELECT tenant_id FROM dns_records WHERE id = ?`, c.ID).Scan(&tid)
	if err == sql.ErrNoRows {
		return fmt.Errorf("record not found")
	} else if err != nil {
		return err
	}
	c.TenantID = tid
	if err := c.validate(); err != nil {
		return err
	}
	c.Type = normalize(c.Type)
	c.Name = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(c.Name)), ".")
	c.Domain = strings.ToLower(strings.TrimSpace(c.Domain))
	c.Line = normalizeLine(c.Line)
	c.SyncMode = normalizeSyncMode(c.SyncMode)
	if c.TTL <= 0 {
		c.TTL = 600
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err = s.db.Exec(`UPDATE dns_records SET domain=?, name=?, type=?, value=?, priority=?, ttl=?, enabled=?, line=?, weight=?, proxied=?, sync_mode=?, remark=?, updated_at=? WHERE id=?`,
		c.Domain, c.Name, c.Type, c.Value, c.Priority, c.TTL, c.Enabled, c.Line, c.Weight, c.Proxied, c.SyncMode, c.Remark, now, c.ID)
	return err
}

// Delete removes a record (returns the tenant id it belonged to).
func (s *Store) Delete(id int64) (int64, error) {
	var tid int64
	err := s.db.QueryRow(`SELECT tenant_id FROM dns_records WHERE id = ?`, id).Scan(&tid)
	if err == sql.ErrNoRows {
		return 0, fmt.Errorf("record not found")
	} else if err != nil {
		return 0, err
	}
	if _, err := s.db.Exec(`DELETE FROM dns_records WHERE id = ?`, id); err != nil {
		return 0, err
	}
	return tid, nil
}

// Get fetches one record.
func (s *Store) Get(id int64) (Record, error) {
	return scan(s.db.QueryRow(`SELECT `+cols+` FROM dns_records WHERE id = ?`, id))
}

// List returns records (tenantID 0 = all, admin only), newest first.
func (s *Store) List(tenantID int64, domain string, page, size int) ([]Record, int64, error) {
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
	if domain != "" {
		where += ` AND domain = ?`
		args = append(args, strings.ToLower(domain))
	}
	var total int64
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM dns_records `+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.db.Query(`SELECT `+cols+` FROM dns_records `+where+` ORDER BY id DESC LIMIT ? OFFSET ?`,
		append(args, size, (page-1)*size)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []Record
	for rows.Next() {
		if c, err := scan(rows); err == nil {
			out = append(out, c)
		}
	}
	return out, total, rows.Err()
}

// Zones lists distinct domains (for the zone selector), optionally per tenant.
func (s *Store) Zones(tenantID int64) ([]string, error) {
	where := `WHERE 1=1`
	var args []any
	if tenantID > 0 {
		where += ` AND tenant_id = ?`
		args = append(args, tenantID)
	}
	rows, err := s.db.Query(`SELECT domain, COUNT(*) FROM dns_records `+where+` GROUP BY domain ORDER BY domain`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var d string
		var n int64
		if rows.Scan(&d, &n) == nil {
			out = append(out, d)
		}
	}
	return out, rows.Err()
}

// TenantOf returns the owner tenant id of a record.
func (s *Store) TenantOf(id int64) (int64, error) {
	var tid int64
	err := s.db.QueryRow(`SELECT tenant_id FROM dns_records WHERE id = ?`, id).Scan(&tid)
	if err == sql.ErrNoRows {
		return 0, fmt.Errorf("record not found")
	}
	return tid, err
}

// ListTenant returns all records for one tenant, optionally filtered by zone.
func (s *Store) ListTenant(tenantID int64, domain string) ([]Record, error) {
	out, _, err := s.List(tenantID, domain, 1, 200)
	return out, err
}
