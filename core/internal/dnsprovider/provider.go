// Package dnsprovider manages upstream DNS service providers.
// Real cloud adapters plug in behind ProviderDriver; the CRUD/storage layer is
// already production-shaped so adding DNSPod/Cloudflare does not change API/UI.
package dnsprovider

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

type Provider struct {
	ID          int64  `json:"id"`
	TenantID    int64  `json:"tenant_id"`
	Name        string `json:"name"`
	Type        string `json:"type"`
	AccessKey   string `json:"access_key,omitempty"`
	SecretKey   string `json:"secret_key,omitempty"`
	APIEndpoint string `json:"api_endpoint"`
	Status      int    `json:"status"`
	Remark      string `json:"remark"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

type Store struct{ db *sql.DB }

func New(db *sql.DB) *Store { return &Store{db: db} }

const cols = `id, tenant_id, name, type, access_key, secret_key, api_endpoint, status, remark, created_at, updated_at`

func scan(r interface{ Scan(...any) error }) (Provider, error) {
	var p Provider
	err := r.Scan(&p.ID, &p.TenantID, &p.Name, &p.Type, &p.AccessKey, &p.SecretKey, &p.APIEndpoint, &p.Status, &p.Remark, &p.CreatedAt, &p.UpdatedAt)
	p.SecretKey = ""
	return p, err
}

func scanWithSecret(r interface{ Scan(...any) error }) (Provider, error) {
	var p Provider
	err := r.Scan(&p.ID, &p.TenantID, &p.Name, &p.Type, &p.AccessKey, &p.SecretKey, &p.APIEndpoint, &p.Status, &p.Remark, &p.CreatedAt, &p.UpdatedAt)
	return p, err
}

func normalizeType(t string) string {
	t = strings.ToLower(strings.TrimSpace(t))
	if t == "" {
		return "manual"
	}
	return t
}

func (p *Provider) validate() error {
	p.Name = strings.TrimSpace(p.Name)
	p.Type = normalizeType(p.Type)
	p.APIEndpoint = strings.TrimSpace(p.APIEndpoint)
	p.Remark = strings.TrimSpace(p.Remark)
	if p.Name == "" {
		return fmt.Errorf("name is required")
	}
	switch p.Type {
	case "manual", "mock", "dnspod", "cloudflare", "aliyun":
	default:
		return fmt.Errorf("unsupported provider type %q", p.Type)
	}
	if p.Status != 0 {
		p.Status = 1
	}
	return nil
}

func (s *Store) Create(p *Provider) (int64, error) {
	if err := p.validate(); err != nil {
		return 0, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := s.db.Exec(`INSERT INTO dns_providers (tenant_id, name, type, access_key, secret_key, api_endpoint, status, remark, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?)`,
		p.TenantID, p.Name, p.Type, strings.TrimSpace(p.AccessKey), strings.TrimSpace(p.SecretKey), p.APIEndpoint, p.Status, p.Remark, now, now)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) Update(p *Provider) error {
	if err := p.validate(); err != nil {
		return err
	}
	var oldSecret string
	err := s.db.QueryRow(`SELECT secret_key FROM dns_providers WHERE id = ?`, p.ID).Scan(&oldSecret)
	if err == sql.ErrNoRows {
		return fmt.Errorf("provider not found")
	}
	if err != nil {
		return err
	}
	secret := strings.TrimSpace(p.SecretKey)
	if secret == "" {
		secret = oldSecret
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err = s.db.Exec(`UPDATE dns_providers SET tenant_id=?, name=?, type=?, access_key=?, secret_key=?, api_endpoint=?, status=?, remark=?, updated_at=? WHERE id=?`,
		p.TenantID, p.Name, p.Type, strings.TrimSpace(p.AccessKey), secret, p.APIEndpoint, p.Status, p.Remark, now, p.ID)
	return err
}

func (s *Store) Delete(id int64) error {
	res, err := s.db.Exec(`DELETE FROM dns_providers WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("provider not found")
	}
	return nil
}

func (s *Store) Get(id int64) (Provider, error) {
	return scan(s.db.QueryRow(`SELECT `+cols+` FROM dns_providers WHERE id = ?`, id))
}

func (s *Store) GetWithSecret(id int64) (Provider, error) {
	return scanWithSecret(s.db.QueryRow(`SELECT `+cols+` FROM dns_providers WHERE id = ?`, id))
}

func (s *Store) List(tenantID int64, page, size int) ([]Provider, int64, error) {
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
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM dns_providers `+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.db.Query(`SELECT `+cols+` FROM dns_providers `+where+` ORDER BY id DESC LIMIT ? OFFSET ?`, append(args, size, (page-1)*size)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []Provider
	for rows.Next() {
		if p, err := scan(rows); err == nil {
			out = append(out, p)
		}
	}
	return out, total, rows.Err()
}

func (s *Store) TenantOf(id int64) (int64, error) {
	var tid int64
	err := s.db.QueryRow(`SELECT tenant_id FROM dns_providers WHERE id = ?`, id).Scan(&tid)
	if err == sql.ErrNoRows {
		return 0, fmt.Errorf("provider not found")
	}
	return tid, err
}

func (s *Store) ActiveByTenant(tenantID int64) ([]Provider, error) {
	rows, err := s.db.Query(`SELECT `+cols+` FROM dns_providers WHERE tenant_id = ? AND status = 1 ORDER BY id ASC`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Provider
	for rows.Next() {
		p, err := scanWithSecret(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) GetSync(providerID, recordID int64) (string, error) {
	var upstreamID string
	err := s.db.QueryRow(`SELECT upstream_id FROM dns_record_sync WHERE provider_id = ? AND record_id = ?`, providerID, recordID).Scan(&upstreamID)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return upstreamID, err
}

func (s *Store) SaveSync(providerID, recordID int64, upstreamID, hash, errMsg string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(`INSERT INTO dns_record_sync (provider_id, record_id, upstream_id, last_hash, last_error, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?)
		ON CONFLICT(provider_id, record_id) DO UPDATE SET upstream_id=excluded.upstream_id, last_hash=excluded.last_hash, last_error=excluded.last_error, updated_at=excluded.updated_at`,
		providerID, recordID, upstreamID, hash, errMsg, now, now)
	return err
}

func (s *Store) DeleteSync(providerID, recordID int64) error {
	_, err := s.db.Exec(`DELETE FROM dns_record_sync WHERE provider_id = ? AND record_id = ?`, providerID, recordID)
	return err
}

func (s *Store) DeleteRecordSyncs(recordID int64) error {
	_, err := s.db.Exec(`DELETE FROM dns_record_sync WHERE record_id = ?`, recordID)
	return err
}
