// Package user: operator (admin-side) accounts. Role: superadmin | operator.
package user

import (
	"database/sql"
	"fmt"
	"time"

	"edgecdn/core/internal/authx"
)

// Admin is an operator account.
type Admin struct {
	ID           int64  `json:"id"`
	Username     string `json:"username"`
	Role         string `json:"role"`
	Status       int    `json:"status"`
	CreatedAt    string `json:"created_at"`
	passwordHash string // never serialized
}

// Password returns the bcrypt hash (login only).
func (a *Admin) Password() string { return a.passwordHash }

// Store wraps the admins table.
type Store struct{ db *sql.DB }

func New(db *sql.DB) *Store { return &Store{db: db} }

// SeedAdmin inserts the first admin when the table is empty (boot only).
func (s *Store) SeedAdmin(username, password string) error {
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM admins`).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	hash, err := authx.HashPassword(password)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`INSERT INTO admins (username, password_hash, role, status, created_at) VALUES (?,?,?,?,?)`,
		username, hash, "superadmin", 1, time.Now().UTC().Format(time.RFC3339))
	return err
}

// List returns all admin accounts.
func (s *Store) List() ([]Admin, error) {
	rows, err := s.db.Query(`SELECT id, username, role, status, created_at FROM admins ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Admin
	for rows.Next() {
		var a Admin
		if err := rows.Scan(&a.ID, &a.Username, &a.Role, &a.Status, &a.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// Get fetches by id.
func (s *Store) Get(id int64) (Admin, error) {
	var a Admin
	err := s.db.QueryRow(`SELECT id, username, role, status, created_at FROM admins WHERE id = ?`, id).
		Scan(&a.ID, &a.Username, &a.Role, &a.Status, &a.CreatedAt)
	if err == sql.ErrNoRows {
		return a, fmt.Errorf("admin not found")
	}
	return a, err
}

// GetByUsername is used at login.
func (s *Store) GetByUsername(username string) (Admin, error) {
	var a Admin
	err := s.db.QueryRow(`SELECT id, username, password_hash, role, status FROM admins WHERE username = ?`, username).
		Scan(&a.ID, &a.Username, &a.passwordHash, &a.Role, &a.Status)
	if err == sql.ErrNoRows {
		return a, fmt.Errorf("admin not found")
	}
	return a, err
}

// Create inserts a new admin (username unique, role validated).
func (s *Store) Create(username, password, role string) (Admin, error) {
	if role != "superadmin" && role != "operator" {
		return Admin{}, fmt.Errorf("invalid role")
	}
	hash, err := authx.HashPassword(password)
	if err != nil {
		return Admin{}, err
	}
	res, err := s.db.Exec(`INSERT INTO admins (username, password_hash, role, status, created_at) VALUES (?,?,?,?,?)`,
		username, hash, role, 1, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return Admin{}, fmt.Errorf("username already exists or db error: %w", err)
	}
	id, _ := res.LastInsertId()
	return s.Get(id)
}

// Update changes role/status (never the last active superadmin).
func (s *Store) Update(id int64, role string, status int) error {
	if role != "superadmin" && role != "operator" {
		return fmt.Errorf("invalid role")
	}
	cur, err := s.Get(id)
	if err != nil {
		return err
	}
	if cur.Role == "superadmin" && (role != "superadmin" || status != 1) {
		var n int
		if err := s.db.QueryRow(`SELECT COUNT(*) FROM admins WHERE role='superadmin' AND status=1`).Scan(&n); err != nil {
			return err
		}
		if n <= 1 {
			return fmt.Errorf("cannot demote the last active superadmin")
		}
	}
	_, err = s.db.Exec(`UPDATE admins SET role=?, status=? WHERE id=?`, role, status, id)
	return err
}

// SetPassword resets an admin password.
func (s *Store) SetPassword(id int64, newPw string) error {
	hash, err := authx.HashPassword(newPw)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`UPDATE admins SET password_hash=? WHERE id=?`, hash, id)
	return err
}

// Delete removes an admin (protected for the last active superadmin).
func (s *Store) Delete(id int64) error {
	a, err := s.Get(id)
	if err != nil {
		return err
	}
	if a.Role == "superadmin" {
		var n int
		if err := s.db.QueryRow(`SELECT COUNT(*) FROM admins WHERE role='superadmin' AND status=1`).Scan(&n); err != nil {
			return err
		}
		if n <= 1 {
			return fmt.Errorf("cannot delete the last superadmin")
		}
	}
	_, err = s.db.Exec(`DELETE FROM admins WHERE id=?`, id)
	return err
}
