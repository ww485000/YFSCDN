package engine

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"time"

	"edgecdn/edge/internal/contract"
)

// authZ compiles a location auth (basic or subrequest).
type authZ struct {
	realm string
	basic map[string]string // user -> sha1(pw) hex
	subURL string
	client *http.Client
}

func compileAuth(a *contract.Auth) *authZ {
	if a == nil || !a.Enabled {
		return nil
	}
	az := &authZ{realm: a.Realm, basic: map[string]string{}}
	if az.realm == "" {
		az.realm = "restricted"
	}
	if a.Basic != nil {
		for _, u := range a.Basic.Users {
			az.basic[u.User] = normalizePassword(u.Password)
		}
	}
	if a.SubRequest != nil && a.SubRequest.URL != "" {
		az.subURL = a.SubRequest.URL
		timeout := time.Duration(a.SubRequest.Timeout) * time.Second
		if timeout <= 0 {
			timeout = 5 * time.Second
		}
		az.client = &http.Client{Timeout: timeout}
	}
	return az
}

// normalizePassword accepts plain text or "sha1:<hex>" and returns sha1 hex.
func normalizePassword(pw string) string {
	if i := strings.IndexByte(pw, ':'); i == 4 && strings.EqualFold(pw[:4], "sha1") {
		if _, err := hex.DecodeString(pw[5:]); err == nil {
			return strings.ToLower(pw[5:])
		}
	}
	return fmt.Sprintf("%x", sha1.Sum([]byte(pw)))
}

// check returns (status, reason) when rejected.
func (az *authZ) check(r *http.Request) (int, string) {
	if az.subURL != "" {
		return az.subCheck(r)
	}
	if len(az.basic) == 0 {
		return 0, ""
	}
	user, pass, ok := r.BasicAuth()
	if !ok {
		return http.StatusUnauthorized, "no credentials"
	}
	if az.basic[user] == fmt.Sprintf("%x", sha1.Sum([]byte(pass))) {
		return 0, ""
	}
	return http.StatusUnauthorized, "bad credentials"
}

func (az *authZ) subCheck(r *http.Request) (int, string) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, az.subURL, nil)
	if err != nil {
		return http.StatusUnauthorized, "subrequest bad url"
	}
	req.Header.Set("X-Original-URL", r.URL.String())
	req.Header.Set("X-Original-IP", clientIP(r))
	req.Header.Set("X-Original-Method", r.Method)
	req.Header.Set("X-Original-User-Agent", r.UserAgent())
	resp, err := az.client.Do(req)
	if err != nil {
		// fail open when the auth service is unreachable
		return 0, ""
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return 0, ""
	}
	return http.StatusUnauthorized, "subrequest denied"
}

// realmHeader builds the WWW-Authenticate header value.
func (az *authZ) realmHeader() string {
	return `Basic realm="` + az.realm + `"`
}
