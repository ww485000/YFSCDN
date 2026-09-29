// Package cert: certificate management.
//   - Local/demo mode: core issues leaf certs from its own CA (type "ca").
//   - Uploaded custom certs (type "custom") are stored as-is.
//   - ACME certs (type "acme") are managed by the ACME module (see internal/acme).
// All types are looked up by domain and injected into site specs for the edge.
package cert

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"database/sql"
	"encoding/pem"
	"fmt"
	"math/big"
	"strings"
	"time"

	"edgecdn/core/internal/setting"
)

const (
	keyCertCA = "cert_ca_cert"
	keyKeyCA  = "cert_ca_key"
	caYears   = 10
	leafDays  = 365
)

// Types.
const (
	TypeCA      = "ca"      // issued by the local EdgeCDN CA
	TypeCustom  = "custom"  // uploaded by an admin
	TypeACME    = "acme"    // obtained via ACME (Let's Encrypt etc.)
)

// Cert is one stored certificate.
type Cert struct {
	ID        int64  `json:"id"`
	TenantID  int64  `json:"tenant_id"`
	Domain    string `json:"domain"`
	Name      string `json:"name"`
	Type      string `json:"type"`
	Issuer    string `json:"issuer"`
	LeafPEM   string `json:"leaf_pem"`
	KeyPEM    string `json:"key_pem"`
	ExpiresAt string `json:"expires_at"`
	CreatedAt string `json:"created_at"`
}

// Public returns the cert without the private key (for list views).
func (c Cert) Public() Cert {
	c.KeyPEM = ""
	return c
}

// Service manages the CA and the cert store.
type Service struct {
	db  *sql.DB
	set *setting.Store
}

func New(db *sql.DB, set *setting.Store) *Service { return &Service{db: db, set: set} }

const certCols = `id, tenant_id, domain, name, type, issuer, leaf_pem, key_pem, expires_at, created_at`

// EnsureCA creates the self-signed CA when missing (idempotent, call at boot).
func (s *Service) EnsureCA() error {
	caPEM := s.set.Get(keyCertCA, "")
	keyPEM := s.set.Get(keyKeyCA, "")
	if caPEM != "" && keyPEM != "" {
		return nil
	}
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return err
	}
	serial, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "EdgeCDN Local CA", Organization: []string{"EdgeCDN"}},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().AddDate(caYears, 0, 0),
		IsCA:         true,
		KeyUsage:     x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return err
	}
	caPEM = string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
	keyPEM = string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}))
	if err := s.set.Set(keyCertCA, caPEM); err != nil {
		return err
	}
	return s.set.Set(keyKeyCA, keyPEM)
}

// IssueLeaf issues (or re-issues) a CA-signed leaf cert for domain and stores it.
func (s *Service) IssueLeaf(domain string, tenantID int64) (Cert, error) {
	if err := s.EnsureCA(); err != nil {
		return Cert{}, err
	}
	caBlock, _ := pem.Decode([]byte(s.set.Get(keyCertCA, "")))
	keyBlock, _ := pem.Decode([]byte(s.set.Get(keyKeyCA, "")))
	if caBlock == nil || keyBlock == nil {
		return Cert{}, fmt.Errorf("CA missing, call EnsureCA first")
	}
	caCert, err := x509.ParseCertificate(caBlock.Bytes)
	if err != nil {
		return Cert{}, err
	}
	caKey, err := x509.ParsePKCS1PrivateKey(keyBlock.Bytes)
	if err != nil {
		return Cert{}, fmt.Errorf("parse CA key: %w", err)
	}

	leafKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return Cert{}, err
	}
	serial, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	now := time.Now()
	dnsNames := []string{domain}
	if strings.HasPrefix(domain, "*.") {
		dnsNames = append(dnsNames, strings.TrimPrefix(domain, "*."))
	}
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: domain},
		NotBefore:    now.Add(-time.Hour),
		NotAfter:     now.AddDate(0, 0, leafDays),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     dnsNames,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, caCert, &leafKey.PublicKey, caKey)
	if err != nil {
		return Cert{}, err
	}
	leafPEM := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
	keyPEM := string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(leafKey)}))
	expires := now.AddDate(0, 0, leafDays).UTC().Format(time.RFC3339)
	created := now.UTC().Format(time.RFC3339)

	var id int64
	err = s.db.QueryRow(`SELECT id FROM certs WHERE domain = ?`, domain).Scan(&id)
	if err == sql.ErrNoRows {
		res, err := s.db.Exec(`INSERT INTO certs (tenant_id, domain, name, type, issuer, leaf_pem, key_pem, expires_at, created_at)
			VALUES (?,?,?,?,?,?,?,?,?)`,
			tenantID, domain, domain, TypeCA, "EdgeCDN Local CA", leafPEM, keyPEM, expires, created)
		if err != nil {
			return Cert{}, err
		}
		id, _ = res.LastInsertId()
	} else if err != nil {
		return Cert{}, err
	} else {
		// Re-issue: keep type if it was custom/acme? No — an explicit IssueLeaf
		// means "issue a CA leaf"; only overwrite CA-typed rows.
		var typ string
		_ = s.db.QueryRow(`SELECT type FROM certs WHERE id = ?`, id).Scan(&typ)
		if typ == TypeCA {
			if _, err := s.db.Exec(`UPDATE certs SET tenant_id=?, name=?, leaf_pem=?, key_pem=?, expires_at=?, created_at=? WHERE id=?`,
				tenantID, domain, leafPEM, keyPEM, expires, created, id); err != nil {
				return Cert{}, err
			}
		} else {
			return Cert{}, fmt.Errorf("domain %s has a %s cert; use upload/ACME flow to change it", domain, typ)
		}
	}
	return s.Get(id)
}

// UploadCustom stores an uploaded cert/key pair for a domain.
// The domain parameter may be empty; then the first SAN of the cert is used.
func (s *Service) UploadCustom(tenantID int64, domain, name, certPEM, keyPEM string) (Cert, error) {
	block, _ := pem.Decode([]byte(certPEM))
	if block == nil {
		return Cert{}, fmt.Errorf("no PEM certificate found in input")
	}
	xc, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return Cert{}, fmt.Errorf("parse certificate: %w", err)
	}
	if _, err := pem.Decode([]byte(keyPEM)); err != nil {
		return Cert{}, fmt.Errorf("no PEM private key found in input")
	}
	if domain == "" {
		if len(xc.DNSNames) > 0 {
			domain = xc.DNSNames[0]
		} else {
			domain = xc.Subject.CommonName
		}
	}
	if name == "" {
		name = domain
	}
	// The key PEM must contain at least one PEM block; keep the original text.
	var issuer string
	if i := xc.Issuer.CommonName; i != "" {
		issuer = i
	}
	created := time.Now().UTC().Format(time.RFC3339)
	var id int64
	err = s.db.QueryRow(`SELECT id FROM certs WHERE domain = ?`, domain).Scan(&id)
	if err == sql.ErrNoRows {
		res, err := s.db.Exec(`INSERT INTO certs (tenant_id, domain, name, type, issuer, leaf_pem, key_pem, expires_at, created_at)
			VALUES (?,?,?,?,?,?,?,?,?)`,
			tenantID, domain, name, TypeCustom, issuer, strings.TrimSpace(certPEM)+"\n", strings.TrimSpace(keyPEM)+"\n",
			xc.NotAfter.UTC().Format(time.RFC3339), created)
		if err != nil {
			return Cert{}, err
		}
		id, _ = res.LastInsertId()
	} else if err != nil {
		return Cert{}, err
	} else {
		if _, err := s.db.Exec(`UPDATE certs SET tenant_id=?, name=?, type=?, issuer=?, leaf_pem=?, key_pem=?, expires_at=?, created_at=? WHERE id=?`,
			tenantID, name, TypeCustom, issuer, strings.TrimSpace(certPEM)+"\n", strings.TrimSpace(keyPEM)+"\n",
			xc.NotAfter.UTC().Format(time.RFC3339), created, id); err != nil {
			return Cert{}, err
		}
	}
	return s.Get(id)
}

// SetACME stores/refreshes an ACME-obtained cert for a domain.
func (s *Service) SetACME(tenantID int64, domain, certPEM, keyPEM, issuer string) (Cert, error) {
	block, _ := pem.Decode([]byte(certPEM))
	var expires string
	if block != nil {
		if xc, err := x509.ParseCertificate(block.Bytes); err == nil {
			expires = xc.NotAfter.UTC().Format(time.RFC3339)
		}
	}
	created := time.Now().UTC().Format(time.RFC3339)
	var id int64
	err := s.db.QueryRow(`SELECT id FROM certs WHERE domain = ?`, domain).Scan(&id)
	if err == sql.ErrNoRows {
		res, err := s.db.Exec(`INSERT INTO certs (tenant_id, domain, name, type, issuer, leaf_pem, key_pem, expires_at, created_at)
			VALUES (?,?,?,?,?,?,?,?,?)`,
			tenantID, domain, domain, TypeACME, issuer, certPEM, keyPEM, expires, created)
		if err != nil {
			return Cert{}, err
		}
		id, _ = res.LastInsertId()
	} else if err != nil {
		return Cert{}, err
	} else {
		if _, err := s.db.Exec(`UPDATE certs SET tenant_id=?, type=?, issuer=?, leaf_pem=?, key_pem=?, expires_at=?, created_at=? WHERE id=?`,
			tenantID, TypeACME, issuer, certPEM, keyPEM, expires, created, id); err != nil {
			return Cert{}, err
		}
	}
	return s.Get(id)
}

// Reissue re-issues the CA leaf for a CA-typed cert.
func (s *Service) Reissue(id int64) (Cert, error) {
	c, err := s.Get(id)
	if err != nil {
		return Cert{}, err
	}
	if c.Type != TypeCA {
		return Cert{}, fmt.Errorf("cert %d is %s-typed; only CA certs can be re-issued here", id, c.Type)
	}
	return s.IssueLeaf(c.Domain, c.TenantID)
}

// Get fetches by id.
func (s *Service) Get(id int64) (Cert, error) {
	var c Cert
	err := s.db.QueryRow(`SELECT `+certCols+` FROM certs WHERE id = ?`, id).
		Scan(&c.ID, &c.TenantID, &c.Domain, &c.Name, &c.Type, &c.Issuer, &c.LeafPEM, &c.KeyPEM, &c.ExpiresAt, &c.CreatedAt)
	if err == sql.ErrNoRows {
		return c, fmt.Errorf("cert not found")
	}
	return c, err
}

// GetByDomain looks up a stored cert.
func (s *Service) GetByDomain(domain string) (Cert, error) {
	var c Cert
	err := s.db.QueryRow(`SELECT `+certCols+` FROM certs WHERE domain = ?`, domain).
		Scan(&c.ID, &c.TenantID, &c.Domain, &c.Name, &c.Type, &c.Issuer, &c.LeafPEM, &c.KeyPEM, &c.ExpiresAt, &c.CreatedAt)
	if err == sql.ErrNoRows {
		return c, fmt.Errorf("cert not found for domain")
	}
	return c, err
}

// List returns certs for a tenant (0 = all), without private keys.
func (s *Service) List(tenantID int64) ([]Cert, error) {
	where := `WHERE 1=1`
	var args []any
	if tenantID > 0 {
		where = `WHERE tenant_id = ?`
		args = append(args, tenantID)
	}
	rows, err := s.db.Query(`SELECT `+certCols+` FROM certs `+where+` ORDER BY id DESC`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Cert
	for rows.Next() {
		var c Cert
		if err := rows.Scan(&c.ID, &c.TenantID, &c.Domain, &c.Name, &c.Type, &c.Issuer, &c.LeafPEM, &c.KeyPEM, &c.ExpiresAt, &c.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, c.Public())
	}
	return out, rows.Err()
}

// Delete removes a cert row.
func (s *Service) Delete(id int64) error {
	if _, err := s.db.Exec(`DELETE FROM certs WHERE id = ?`, id); err != nil {
		return err
	}
	return nil
}

// ExpiringSoon lists CA certs expiring within window (for the task worker).
func (s *Service) ExpiringSoon(window time.Duration) ([]Cert, error) {
	until := time.Now().UTC().Add(window).UTC().Format(time.RFC3339)
	rows, err := s.db.Query(`SELECT `+certCols+` FROM certs WHERE type = 'ca' AND expires_at != '' AND expires_at < ? ORDER BY expires_at ASC`, until)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Cert
	for rows.Next() {
		var c Cert
		if err := rows.Scan(&c.ID, &c.TenantID, &c.Domain, &c.Name, &c.Type, &c.Issuer, &c.LeafPEM, &c.KeyPEM, &c.ExpiresAt, &c.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, c.Public())
	}
	return out, rows.Err()
}
