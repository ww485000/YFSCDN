// Package authx: minimal HS256 JWT (stdlib only) + password hashing.
package authx

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Claims are the JWT payload. (Field names differ from method names on purpose.)
type Claims struct {
	Sub      string `json:"sub"`      // numeric user id
	Name     string `json:"name"`
	RoleName string `json:"role"`     // "admin" | "tenant"
	Tenant   int64  `json:"tid"`      // tenant id (0 for admins)
	Iat      int64  `json:"iat"`
	Exp      int64  `json:"exp"`
}

// AuthInfo interface (webx.AuthInfo) implementation.
func (c *Claims) UserID() int64 {
	var id int64
	fmt.Sscanf(c.Sub, "%d", &id)
	return id
}
func (c *Claims) UserName() string { return c.Name }
func (c *Claims) Role() string     { return c.RoleName }
func (c *Claims) TenantID() int64  { return c.Tenant }

// JWT signs/verifies HS256 tokens.
type JWT struct{ secret []byte }

func NewJWT(secret string) *JWT { return &JWT{secret: []byte(secret)} }

// Issue creates a token valid for d.
func (j *JWT) Issue(c Claims, d time.Duration) (string, error) {
	now := time.Now()
	c.Iat = now.Unix()
	c.Exp = now.Add(d).Unix()
	payload, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	body := base64.RawURLEncoding.EncodeToString(payload)
	sig := j.sign(header + "." + body)
	return header + "." + body + "." + sig, nil
}

// Parse verifies signature and expiry, returning claims.
func (j *JWT) Parse(token string) (*Claims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("bad token format")
	}
	want := j.sign(parts[0] + "." + parts[1])
	if !hmac.Equal([]byte(want), []byte(parts[2])) {
		return nil, fmt.Errorf("bad signature")
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, err
	}
	var c Claims
	if err := json.Unmarshal(raw, &c); err != nil {
		return nil, err
	}
	if time.Now().Unix() > c.Exp {
		return nil, fmt.Errorf("token expired")
	}
	return &c, nil
}

func (j *JWT) sign(data string) string {
	mac := hmac.New(sha256.New, j.secret)
	mac.Write([]byte(data))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
