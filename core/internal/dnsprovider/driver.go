package dnsprovider

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
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
	case "dnspod":
		return dnspodDriver{}
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

type dnspodDriver struct{}

type tcResponse struct {
	Response struct {
		Error *struct {
			Code    string `json:"Code"`
			Message string `json:"Message"`
		} `json:"Error,omitempty"`
		RecordID  any    `json:"RecordId,omitempty"`
		RequestID string `json:"RequestId"`
	} `json:"Response"`
}

func (dnspodDriver) Upsert(ctx context.Context, p Provider, r dns.Record, upstreamID string) (string, error) {
	if r.Enabled == 0 {
		if upstreamID != "" {
			return "", dnspodDriver{}.Delete(ctx, p, r, upstreamID)
		}
		return "", nil
	}
	body := map[string]any{
		"Domain":     r.Domain,
		"SubDomain":  dnspodSubDomain(r),
		"RecordType": r.Type,
		"RecordLine": "默认",
		"Value":      r.Value,
		"TTL":        r.TTL,
	}
	action := "CreateRecord"
	if upstreamID != "" {
		action = "ModifyRecord"
		body["RecordId"] = upstreamID
	}
	if r.Type == "MX" {
		body["MX"] = r.Priority
	}
	resp, err := tencentJSON(ctx, p, action, body)
	if err != nil {
		return "", err
	}
	if upstreamID != "" {
		return upstreamID, nil
	}
	id := fmt.Sprint(resp.Response.RecordID)
	if id == "" || id == "<nil>" {
		return "", fmt.Errorf("dnspod create response missing RecordId")
	}
	return id, nil
}

func (dnspodDriver) Delete(ctx context.Context, p Provider, r dns.Record, upstreamID string) error {
	if upstreamID == "" {
		return nil
	}
	_, err := tencentJSON(ctx, p, "DeleteRecord", map[string]any{
		"Domain":   r.Domain,
		"RecordId": upstreamID,
	})
	return err
}

func dnspodSubDomain(r dns.Record) string {
	name := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(r.Name)), ".")
	domain := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(r.Domain)), ".")
	if name == "" || name == domain {
		return "@"
	}
	suffix := "." + domain
	if strings.HasSuffix(name, suffix) {
		return strings.TrimSuffix(name, suffix)
	}
	return name
}

func tencentJSON(ctx context.Context, p Provider, action string, body any) (tcResponse, error) {
	var out tcResponse
	host := strings.TrimPrefix(strings.TrimPrefix(strings.TrimRight(p.APIEndpoint, "/"), "https://"), "http://")
	if host == "" {
		host = "dnspod.tencentcloudapi.com"
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return out, err
	}
	ts := time.Now().Unix()
	date := time.Unix(ts, 0).UTC().Format("2006-01-02")
	service := "dnspod"
	algorithm := "TC3-HMAC-SHA256"
	hashedPayload := sha256Hex(payload)
	canonicalHeaders := "content-type:application/json; charset=utf-8\nhost:" + host + "\nx-tc-action:" + strings.ToLower(action) + "\n"
	signedHeaders := "content-type;host;x-tc-action"
	canonicalRequest := "POST\n/\n\n" + canonicalHeaders + "\n" + signedHeaders + "\n" + hashedPayload
	credentialScope := date + "/" + service + "/tc3_request"
	stringToSign := algorithm + "\n" + fmt.Sprint(ts) + "\n" + credentialScope + "\n" + sha256Hex([]byte(canonicalRequest))
	kDate := hmacSHA256([]byte("TC3"+p.SecretKey), date)
	kService := hmacSHA256(kDate, service)
	kSigning := hmacSHA256(kService, "tc3_request")
	signature := hex.EncodeToString(hmacSHA256(kSigning, stringToSign))
	auth := algorithm + " Credential=" + p.AccessKey + "/" + credentialScope + ", SignedHeaders=" + signedHeaders + ", Signature=" + signature
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://"+host, bytes.NewReader(payload))
	if err != nil {
		return out, err
	}
	req.Header.Set("Authorization", auth)
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Host", host)
	req.Header.Set("X-TC-Action", action)
	req.Header.Set("X-TC-Version", "2021-03-23")
	req.Header.Set("X-TC-Timestamp", fmt.Sprint(ts))
	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return out, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return out, err
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return out, fmt.Errorf("dnspod invalid response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 || out.Response.Error != nil {
		if out.Response.Error != nil {
			return out, fmt.Errorf("dnspod api failed %s: %s", out.Response.Error.Code, out.Response.Error.Message)
		}
		return out, fmt.Errorf("dnspod api failed status=%d body=%s", resp.StatusCode, string(raw))
	}
	return out, nil
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func hmacSHA256(key []byte, parts ...any) []byte {
	h := hmac.New(sha256.New, key)
	for _, p := range parts {
		switch v := p.(type) {
		case string:
			_, _ = h.Write([]byte(v))
		case []byte:
			_, _ = h.Write(v)
		}
	}
	return h.Sum(nil)
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
