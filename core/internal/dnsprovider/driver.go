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
	"net/url"
	"sort"
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
	case "aliyun":
		return aliyunDriver{}
	case "manual", "mock":
		return noopDriver{}
	default:
		return unsupportedDriver{typ: p.Type}
	}
}

func RecordHash(r dns.Record) string {
	raw := fmt.Sprintf("%d|%s|%s|%s|%s|%d|%d|%d|%s|%d|%d|%s", r.TenantID, r.Domain, r.Name, r.Type, r.Value, r.Priority, r.TTL, r.Enabled, r.Line, r.Weight, r.Proxied, r.SyncMode)
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

type aliyunDriver struct{}

type aliyunResponse struct {
	RecordID string `json:"RecordId"`
	Code     string `json:"Code"`
	Message  string `json:"Message"`
}

func (aliyunDriver) Upsert(ctx context.Context, p Provider, r dns.Record, upstreamID string) (string, error) {
	if r.Enabled == 0 {
		if upstreamID != "" {
			return "", aliyunDriver{}.Delete(ctx, p, r, upstreamID)
		}
		return "", nil
	}
	params := map[string]string{
		"DomainName": r.Domain,
		"RR":         aliyunRR(r),
		"Type":       r.Type,
		"Value":      r.Value,
		"TTL":        fmt.Sprint(r.TTL),
	}
	if line := aliyunLine(r.Line); line != "" {
		params["Line"] = line
	}
	if r.Weight > 0 {
		params["Weight"] = fmt.Sprint(r.Weight)
	}
	action := "AddDomainRecord"
	if upstreamID != "" {
		action = "UpdateDomainRecord"
		params["RecordId"] = upstreamID
	}
	if r.Type == "MX" {
		params["Priority"] = fmt.Sprint(r.Priority)
	}
	var out aliyunResponse
	if err := aliyunRPC(ctx, p, action, params, &out); err != nil {
		return "", err
	}
	if upstreamID != "" {
		return upstreamID, nil
	}
	if out.RecordID == "" {
		return "", fmt.Errorf("aliyun create response missing RecordId")
	}
	return out.RecordID, nil
}

func (aliyunDriver) Delete(ctx context.Context, p Provider, _ dns.Record, upstreamID string) error {
	if upstreamID == "" {
		return nil
	}
	return aliyunRPC(ctx, p, "DeleteDomainRecord", map[string]string{"RecordId": upstreamID}, nil)
}

func aliyunLine(line string) string {
	line = strings.TrimSpace(line)
	if line == "" || strings.EqualFold(line, "default") {
		return ""
	}
	return line
}

func aliyunRR(r dns.Record) string {
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

func aliyunRPC(ctx context.Context, p Provider, action string, params map[string]string, out any) error {
	endpoint := strings.TrimRight(p.APIEndpoint, "/")
	if endpoint == "" {
		endpoint = "https://alidns.aliyuncs.com/"
	}
	q := map[string]string{
		"Action":           action,
		"Version":          "2015-01-09",
		"Format":           "JSON",
		"AccessKeyId":      strings.TrimSpace(p.AccessKey),
		"SignatureMethod":  "HMAC-SHA1",
		"Timestamp":        time.Now().UTC().Format("2006-01-02T15:04:05Z"),
		"SignatureVersion": "1.0",
		"SignatureNonce":   fmt.Sprintf("%d", time.Now().UnixNano()),
	}
	for k, v := range params {
		q[k] = v
	}
	q["Signature"] = aliyunSignature("GET", q, strings.TrimSpace(p.SecretKey))
	values := url.Values{}
	for k, v := range q {
		values.Set(k, v)
	}
	sep := "?"
	if strings.Contains(endpoint, "?") {
		sep = "&"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+sep+values.Encode(), nil)
	if err != nil {
		return err
	}
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
	var env aliyunResponse
	if err := json.Unmarshal(raw, &env); err != nil {
		return fmt.Errorf("aliyun invalid response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 || env.Code != "" {
		if env.Code != "" {
			return fmt.Errorf("aliyun api failed %s: %s", env.Code, env.Message)
		}
		return fmt.Errorf("aliyun api failed status=%d body=%s", resp.StatusCode, string(raw))
	}
	if out != nil {
		if err := json.Unmarshal(raw, out); err != nil {
			return err
		}
	}
	return nil
}

func aliyunSignature(method string, params map[string]string, secret string) string {
	keys := make([]string, 0, len(params))
	for k := range params {
		if k != "Signature" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	pairs := make([]string, 0, len(keys))
	for _, k := range keys {
		pairs = append(pairs, aliyunPercent(k)+"="+aliyunPercent(params[k]))
	}
	stringToSign := method + "&" + aliyunPercent("/") + "&" + aliyunPercent(strings.Join(pairs, "&"))
	sum := hmacSHA1([]byte(secret+"&"), stringToSign)
	return base64Encode(sum)
}

func aliyunPercent(s string) string {
	escaped := url.QueryEscape(s)
	escaped = strings.ReplaceAll(escaped, "+", "%20")
	escaped = strings.ReplaceAll(escaped, "*", "%2A")
	escaped = strings.ReplaceAll(escaped, "%7E", "~")
	return escaped
}

func hmacSHA1(key []byte, parts ...string) []byte {
	h := hmac.New(sha1.New, key)
	for _, p := range parts {
		_, _ = h.Write([]byte(p))
	}
	return h.Sum(nil)
}

func base64Encode(b []byte) string {
	const table = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	if len(b) == 0 {
		return ""
	}
	out := make([]byte, 0, ((len(b)+2)/3)*4)
	for i := 0; i < len(b); i += 3 {
		var n uint32
		remain := len(b) - i
		n |= uint32(b[i]) << 16
		if remain > 1 {
			n |= uint32(b[i+1]) << 8
		}
		if remain > 2 {
			n |= uint32(b[i+2])
		}
		out = append(out, table[(n>>18)&63], table[(n>>12)&63])
		if remain > 1 {
			out = append(out, table[(n>>6)&63])
		} else {
			out = append(out, '=')
		}
		if remain > 2 {
			out = append(out, table[n&63])
		} else {
			out = append(out, '=')
		}
	}
	return string(out)
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
		"RecordLine": dnspodLine(r.Line),
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

func dnspodLine(line string) string {
	line = strings.TrimSpace(line)
	if line == "" || strings.EqualFold(line, "default") {
		return "默认"
	}
	return line
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
		"proxied": r.Proxied == 1 && (r.Type == "A" || r.Type == "AAAA" || r.Type == "CNAME"),
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
