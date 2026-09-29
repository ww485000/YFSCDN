// Package auth: login for both roles (operator admin + tenant user).
package auth

import (
	"fmt"
	"strconv"
	"time"

	"edgecdn/core/internal/authx"
	"edgecdn/core/internal/tenant"
	"edgecdn/core/internal/user"
)

// Service authenticates and issues JWTs.
type Service struct {
	jwt     *authx.JWT
	admins  *user.Store
	tenants *tenant.Store
}

func NewService(jwt *authx.JWT, admins *user.Store, tenants *tenant.Store) *Service {
	return &Service{jwt: jwt, admins: admins, tenants: tenants}
}

// LoginReq is the login body. Scope: "admin" (operator) or "user" (tenant portal).
type LoginReq struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Scope    string `json:"scope"`
}

// Login verifies credentials for the given scope and returns (token, user info).
func (s *Service) Login(req LoginReq) (string, map[string]any, error) {
	switch req.Scope {
	case "admin":
		a, err := s.admins.GetByUsername(req.Username)
		if err != nil || !authx.CheckPassword(a.Password(), req.Password) {
			return "", nil, fmt.Errorf("username or password incorrect")
		}
		if a.Status != 1 {
			return "", nil, fmt.Errorf("account disabled")
		}
		tok, err := s.jwt.Issue(authx.Claims{Sub: strconv.FormatInt(a.ID, 10), Name: a.Username, RoleName: "admin"}, 12*time.Hour)
		if err != nil {
			return "", nil, err
		}
		return tok, map[string]any{"id": a.ID, "username": a.Username, "role": a.Role}, nil

	case "user":
		t, err := s.tenants.GetByUsername(req.Username)
		if err != nil || !authx.CheckPassword(t.Password(), req.Password) {
			return "", nil, fmt.Errorf("username or password incorrect")
		}
		if t.Status != 1 {
			return "", nil, fmt.Errorf("account disabled, contact operator")
		}
		tok, err := s.jwt.Issue(authx.Claims{
			Sub: strconv.FormatInt(t.ID, 10), Name: t.Name, RoleName: "tenant", Tenant: t.ID,
		}, 12*time.Hour)
		if err != nil {
			return "", nil, err
		}
		return tok, map[string]any{"id": t.ID, "name": t.Name, "username": t.Username, "role": "tenant"}, nil

	default:
		return "", nil, fmt.Errorf("scope must be \"admin\" or \"user\"")
	}
}
