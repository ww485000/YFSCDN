// Package task is the background job queue: cert renewal checks, ACME
// renewals, DNS resolve/clean. core processes tasks on a ticker; the admin
// UI lists them (GoEdge "tasks" page).
package task

import (
	"database/sql"
	"log"
	"strings"
	"time"
)

// Task statuses.
const (
	StatusPending = "pending"
	StatusRunning = "running"
	StatusDone    = "done"
	StatusFailed  = "failed"
)

// Task types.
const (
	TypeCertCheck  = "cert_check"  // re-issue expiring CA leaf certs
	TypeACMERenew  = "acme_renew"  // renew a cert via ACME (provider DNS-01 / http-01)
	TypeDNSResolve = "dns_resolve" // create DNS records for a domain
	TypeDNSClean   = "dns_clean"   // remove DNS records for a domain
	TypeSiteResync = "site_resync" // force re-push a site to its nodes
)

// Task is one queue row.
type Task struct {
	ID        int64  `json:"id"`
	Type      string `json:"type"`
	Target    string `json:"target"`
	Payload   string `json:"payload"`
	Status    string `json:"status"`
	Error     string `json:"error"`
	Attempts  int    `json:"attempts"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// Handler processes one task; implement per type in app wiring.
type Handler func(t *Task) error

// Store wraps the tasks table.
type Store struct {
	db       *sql.DB
	handlers map[string]Handler
}

func New(db *sql.DB) *Store {
	return &Store{db: db, handlers: map[string]Handler{}}
}

// SetHandler registers a processor for a task type.
func (s *Store) SetHandler(typ string, h Handler) { s.handlers[typ] = h }

// Enqueue adds a pending task (no-op if an identical pending/running task exists).
func (s *Store) Enqueue(typ, target, payload string) (int64, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	var id int64
	err := s.db.QueryRow(`SELECT id FROM tasks WHERE type = ? AND target = ? AND status IN ('pending','running') LIMIT 1`,
		typ, target).Scan(&id)
	if err == nil {
		return id, nil // dedupe
	}
	if err != sql.ErrNoRows {
		return 0, err
	}
	res, err := s.db.Exec(`INSERT INTO tasks (type, target, payload, status, created_at, updated_at) VALUES (?,?,?,?,?,?)`,
		typ, target, payload, StatusPending, now, now)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// Tick processes one batch of pending tasks (call periodically).
func (s *Store) Tick(limit int) int {
	if limit <= 0 || limit > 50 {
		limit = 10
	}
	rows, err := s.db.Query(`SELECT id, type, target, payload FROM tasks WHERE status = 'pending' ORDER BY id ASC LIMIT ?`, limit)
	if err != nil {
		return 0
	}
	var batch []Task
	for rows.Next() {
		var t Task
		if rows.Scan(&t.ID, &t.Type, &t.Target, &t.Payload) == nil {
			batch = append(batch, t)
		}
	}
	rows.Close()
	done := 0
	for _, t := range batch {
		now := time.Now().UTC().Format(time.RFC3339)
		if _, err := s.db.Exec(`UPDATE tasks SET status = 'running', updated_at = ? WHERE id = ?`, now, t.ID); err != nil {
			continue
		}
		h, ok := s.handlers[t.Type]
		if !ok {
			s.finish(t.ID, StatusFailed, "no handler for type "+t.Type, false)
			continue
		}
		if err := h(&t); err != nil {
			log.Printf("[task] %s target=%s failed: %v", t.Type, t.Target, err)
			retry := t.Attempts < 3
			s.finish(t.ID, StatusFailed, err.Error(), retry)
			continue
		}
		s.finish(t.ID, StatusDone, "", false)
		done++
	}
	return done
}

func (s *Store) finish(id int64, status, errMsg string, retry bool) {
	now := time.Now().UTC().Format(time.RFC3339)
	if retry {
		_, _ = s.db.Exec(`UPDATE tasks SET status = 'pending', error = ?, attempts = attempts + 1, updated_at = ? WHERE id = ?`,
			errMsg, now, id)
		return
	}
	_, _ = s.db.Exec(`UPDATE tasks SET status = ?, error = ?, attempts = attempts + 1, updated_at = ? WHERE id = ?`,
		status, errMsg, now, id)
}

// List returns recent tasks (newest first), optionally filtered.
func (s *Store) List(typ, status string, page, size int) ([]Task, int64, error) {
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 200 {
		size = 20
	}
	where := `WHERE 1=1`
	var args []any
	if typ != "" {
		where += ` AND type = ?`
		args = append(args, typ)
	}
	if status != "" {
		where += ` AND status = ?`
		args = append(args, status)
	}
	var total int64
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM tasks `+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.db.Query(`SELECT id, type, target, payload, status, error, attempts, created_at, updated_at
		FROM tasks `+where+` ORDER BY id DESC LIMIT ? OFFSET ?`,
		append(args, size, (page-1)*size)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []Task
	for rows.Next() {
		var t Task
		if err := rows.Scan(&t.ID, &t.Type, &t.Target, &t.Payload, &t.Status, &t.Error, &t.Attempts, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, 0, err
		}
		out = append(out, t)
	}
	return out, total, rows.Err()
}

// GC drops old finished tasks.
func (s *Store) GC(olderThan time.Duration) {
	cutoff := time.Now().UTC().Add(-olderThan).Format(time.RFC3339)
	_, _ = s.db.Exec(`DELETE FROM tasks WHERE status IN ('done','failed') AND updated_at < ?`, cutoff)
}

// ParseTarget extracts a domain from a task target ("cert:example.com").
func ParseTarget(target string) string {
	if i := strings.IndexByte(target, ':'); i >= 0 {
		return target[i+1:]
	}
	return target
}
