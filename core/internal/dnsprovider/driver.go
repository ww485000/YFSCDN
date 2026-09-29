package dnsprovider

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"edgecdn/core/internal/dns"
)

type Driver interface {
	Upsert(ctx context.Context, p Provider, r dns.Record, upstreamID string) (string, error)
	Delete(ctx context.Context, p Provider, r dns.Record, upstreamID string) error
}

func DriverFor(p Provider) Driver {
	switch normalizeType(p.Type) {
	case "cloudflare":
		return cloudflareDriver{}
	case "manual", "mock":
		return noopDriver{}
	default:
		return unsupportedDriver{typ: p.Type}
	}
}

func RecordHash(r dns.Record) string {
	raw := fmt.Sprintf("%d|%s|%s|%s|%s|%d|%d|%d", r.TenantID, r.Domain, r.Name, r.Type, r.Value, r.Priority, r.TTL, r.Enabled)
	sum := sha1.Sum([]byte(raw))
	return hex.EncodeToString(sum[:])
}

type noopDriver struct{}

func (noopDriver) Upsert(_ context.Context, _ Provider, r dns.Record, upstreamID string) (string, error) {
	if upstreamID != "" {
		return upstreamID, nil
	}
	return fmt.Sprintf("local:%d", r.ID), nil
}

func (noopDriver) Delete(_ context.Context, _ Provider, _ dns.Record, _ string) error { return nil }

type unsupportedDriver struct{ typ string }

func (d unsupportedDriver) Upsert(context.Context, Provider, dns.Record, string) (string, error) {
	return "", fmt.Errorf("%s adapter is not implemented yet", d.typ)
}

func (d unsupportedDriver) Delete(context.Context, Provider, dns.Record, string) error {
	return fmt.Errorf("%s adapter is not implemented yet", d.typ)
}

type cloudflareDriver struct{}

type cfResponse struct {
	Success bool              `json:"success"`
	Errors  []json.RawMessage `json:"errors"`
	Result  json.RawMessage   `json:"result"`
}

type cfZone struct {
	ID string `json:"id"`
}

type cfRecord struct {
	ID string `json:"id"`
}

func (cloudflareDriver) Upsert(ctx context.Context, p Provider, r dns.Record, upstreamID string) (string, error) {
	if r.Enabled == 0 {
		if upstreamID != "" {
			return "", cloudflareDriver{}.Delete(ctx, p, r, upstreamID)
		}
		return "", nil
	}
	zoneID, err := cloudflareZoneID(ctx, p, r.Domain)
	if err != nil {
		return "", err
	}
	body := map[string]any{
		"type":    r.Type,
		"name":    r.Name,
		"content": r.Value,
		"ttl":     r.TTL,
		"proxied": false,
	}
	if r.Type == "MX" {
		body["priority"] = r.Priority
	}
	var out cfRecord
	method := http.MethodPost
	path := "/zones/" + zoneID + "/dns_records"
	if upstreamID != "" {
		method = http.MethodPut
		path += "/" + upstreamID
	}
	if err := cloudflareJSON(ctx, p, method, path, body, &out); err != nil {
		return "", err
	}
	return out.ID, nil
}

func (cloudflareDriver) Delete(ctx context.Context, p Provider, r dns.Record, upstreamID string) error {
	if upstreamID == "" {
		return nil
	}
	zoneID, err := cloudflareZoneID(ctx, p, r.Domain)
	if err != nil {
		return err
	}
	return cloudflareJSON(ctx, p, http.MethodDelete, "/zones/"+zoneID+"/dns_records/"+upstreamID, nil, nil)
}

func cloudflareZoneID(ctx context.Context, p Provider, domain string) (string, error) {
	var zones []cfZone
	if err := cloudflareJSON(ctx, p, http.MethodGet, "/zones?name="+domain, nil, &zones); err != nil {
		return "", err
	}
	if len(zones) == 0 || zones[0].ID == "" {
		return "", fmt.Errorf("cloudflare zone not found: %s", domain)
	}
	return zones[0].ID, nil
}

func cloudflareJSON(ctx context.Context, p Provider, method, path string, body any, out any) error {
	base := strings.TrimRight(p.APIEndpoint, "/")
	if base == "" {
		base = "https://api.cloudflare.com/client/v4"
	}
	var rd io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, base+path, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(p.SecretKey))
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	var env cfResponse
	if err := json.Unmarshal(raw, &env); err != nil {
		return fmt.Errorf("cloudflare invalid response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 || !env.Success {
		return fmt.Errorf("cloudflare api failed status=%d errors=%s", resp.StatusCode, string(raw))
	}
	if out != nil && len(env.Result) > 0 {
		if err := json.Unmarshal(env.Result, out); err != nil {
			return err
		}
	}
	return nil
}
