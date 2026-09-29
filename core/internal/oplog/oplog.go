// Package oplog: audit log of admin/tenant operations.
package oplog

import (
	"database/sql"
	"time"
)

// Log is one audit entry.
type Log struct {
	ID         int64  `json:"id"`
	ActorType  string `json:"actor_type"` // "admin" | "tenant" | "node" | "system"
	ActorID    int64  `json:"actor_id"`
	Action     string `json:"action"` // e.g. "site.create"
	Target     string `json:"target"`
	Detail     string `json:"detail"`
	CreatedAt  string `json:"created_at"`
}

// Store wraps the oplogs table.
type Store struct{ db *sql.DB }

func New(db *sql.DB) *Store { return &Store{db: db} }

// Add records one entry (never fails loudly: best effort).
func (s *Store) Add(actorType string, actorID int64, action, target, detail string) {
	now := time.Now().UTC().Format(time.RFC3339)
	_, _ = s.db.Exec(`INSERT INTO oplogs (actor_type, actor_id, action, target, detail, created_at) VALUES (?,?,?,?,?,?)`,
		actorType, actorID, action, target, detail, now)
}

// List returns paged logs, newest first, optional keyword on action/target.
func (s *Store) List(page, size int, keyword string) ([]Log, int64, error) {
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 200 {
		size = 20
	}
	where := `WHERE 1=1`
	var args []any
	if keyword != "" {
		where += ` AND (action LIKE ? OR target LIKE ? OR detail LIKE ?)`
		like := "%" + keyword + "%"
		args = append(args, like, like, like)
	}
	var total int64
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM oplogs `+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.db.Query(`SELECT id, actor_type, actor_id, action, target, detail, created_at FROM oplogs `+where+
		` ORDER BY id DESC LIMIT ? OFFSET ?`, append(args, size, (page-1)*size)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []Log
	for rows.Next() {
		var l Log
		if err := rows.Scan(&l.ID, &l.ActorType, &l.ActorID, &l.Action, &l.Target, &l.Detail, &l.CreatedAt); err != nil {
			return nil, 0, err
		}
		out = append(out, l)
	}
	return out, total, rows.Err()
}
