// Package node: edge node registry. A node authenticates with a unique token.
package node

import (
	"database/sql"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"edgecdn/core/internal/oplog"
)

// Node is one edge instance.
type Node struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Token     string `json:"token"`
	TenantID  int64  `json:"tenant_id"` // 0 = platform shared node
	Status    int    `json:"status"`    // 1=online 0=offline
	Driver    string `json:"driver"`
	Host      string `json:"host"`
	IP        string `json:"ip"`
	Version   string `json:"version"`
	LastSeen  string `json:"last_seen"`
	CreatedAt string `json:"created_at"`
}

// Store wraps the nodes table + node business rules.
type Store struct {
	db    *sql.DB
	oplog *oplog.Store
}

func New(db *sql.DB, oplog *oplog.Store) *Store { return &Store{db: db, oplog: oplog} }

// List returns paged nodes; tenantID 0 = all, >0 filters shared or owned.
func (s *Store) List(tenantID int64, keyword string, page, size int) ([]Node, int64, error) {
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 200 {
		size = 20
	}
	where := `WHERE 1=1`
	var args []any
	if tenantID > 0 {
		where += ` AND (tenant_id = ? OR tenant_id = 0)`
		args = append(args, tenantID)
	}
	if keyword != "" {
		where += ` AND (name LIKE ? OR host LIKE ? OR ip LIKE ?)`
		like := "%" + keyword + "%"
		args = append(args, like, like, like)
	}
	var total int64
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM nodes `+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.db.Query(`SELECT id, name, token, tenant_id, status, driver, host, ip, version, last_seen, created_at
		FROM nodes `+where+` ORDER BY id DESC LIMIT ? OFFSET ?`,
		append(args, size, (page-1)*size)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []Node
	for rows.Next() {
		var n Node
		var lastSeen sql.NullString
		if err := rows.Scan(&n.ID, &n.Name, &n.Token, &n.TenantID, &n.Status,
			&n.Driver, &n.Host, &n.IP, &n.Version, &lastSeen, &n.CreatedAt); err != nil {
			return nil, 0, err
		}
		n.LastSeen = lastSeen.String
		out = append(out, n)
	}
	return out, total, rows.Err()
}

// Get fetches by id.
func (s *Store) Get(id int64) (Node, error) {
	var n Node
	err := s.db.QueryRow(`SELECT id, name, token, tenant_id, status, driver, host, ip, version, last_seen, created_at
		FROM nodes WHERE id = ?`, id).
		Scan(&n.ID, &n.Name, &n.Token, &n.TenantID, &n.Status, &n.Driver, &n.Host, &n.IP, &n.Version, &n.LastSeen, &n.CreatedAt)
	if err == sql.ErrNoRows {
		return n, fmt.Errorf("node not found")
	}
	return n, err
}

// GetByToken authenticates edge traffic.
func (s *Store) GetByToken(token string) (Node, error) {
	var n Node
	err := s.db.QueryRow(`SELECT id, name, token, tenant_id, status, driver, host, ip, version, last_seen, created_at
		FROM nodes WHERE token = ?`, token).
		Scan(&n.ID, &n.Name, &n.Token, &n.TenantID, &n.Status, &n.Driver, &n.Host, &n.IP, &n.Version, &n.LastSeen, &n.CreatedAt)
	if err == sql.ErrNoRows {
		return n, fmt.Errorf("unknown node token")
	}
	return n, err
}

// Create inserts a node with a fresh random token.
func (s *Store) Create(name string, tenantID int64) (Node, error) {
	if name == "" {
		return Node{}, fmt.Errorf("name required")
	}
	token := newToken()
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := s.db.Exec(`INSERT INTO nodes (name, token, tenant_id, status, created_at) VALUES (?,?,?,0,?)`,
		name, token, tenantID, now)
	if err != nil {
		return Node{}, err
	}
	id, _ := res.LastInsertId()
	n, err := s.Get(id)
	if err != nil {
		return Node{}, err
	}
	s.oplog.Add("admin", 0, "node.create", n.Name, fmt.Sprintf("tenant_id=%d", tenantID))
	return n, nil
}

// Update renames / re-assigns a node (token never changes).
func (s *Store) Update(id int64, name string, tenantID int64) error {
	if _, err := s.Get(id); err != nil {
		return err
	}
	_, err := s.db.Exec(`UPDATE nodes SET name=?, tenant_id=? WHERE id=?`, name, tenantID, id)
	return err
}

// Delete removes a node.
func (s *Store) Delete(id int64) error {
	if _, err := s.Get(id); err != nil {
		return err
	}
	_, err := s.db.Exec(`DELETE FROM nodes WHERE id=?`, id)
	if err == nil {
		_, _ = s.db.Exec(`DELETE FROM site_nodes WHERE node_id=?`, id)
	}
	return err
}

// Register handles edge bootstrap: validate token, mark online.
func (s *Store) Register(token, host, ip, driver string) (Node, error) {
	n, err := s.GetByToken(token)
	if err != nil {
		return Node{}, err
	}
	s.Touch(n.ID, host, ip, driver, "")
	s.oplog.Add("node", n.ID, "node.register", n.Name, host+" "+ip+" "+driver)
	return s.Get(n.ID)
}

// Touch updates liveness metadata (called by register + heartbeat).
func (s *Store) Touch(id int64, host, ip, driver, version string) {
	now := time.Now().UTC().Format(time.RFC3339)
	if host == "" {
		host = "-"
	}
	if ip == "" {
		ip = "-"
	}
	_, _ = s.db.Exec(`UPDATE nodes SET status=1, host=?, ip=?, driver=?, version=?, last_seen=? WHERE id=?`,
		host, ip, driver, version, now, id)
}

// MarkOffline sets nodes offline when last_seen is older than cutoff.
func (s *Store) MarkOffline(cutoff time.Duration) {
	c := time.Now().UTC().Add(-cutoff).Format(time.RFC3339)
	if res, err := s.db.Exec(`UPDATE nodes SET status=0 WHERE status=1 AND last_seen < ?`, c); err == nil {
		if n, _ := res.RowsAffected(); n > 0 {
			s.oplog.Add("system", 0, "node.offline", "", fmt.Sprintf("%d node(s) marked offline", n))
		}
	}
}

func newToken() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand never fails on supported platforms
		panic(err)
	}
	return hex.EncodeToString(b)
}
