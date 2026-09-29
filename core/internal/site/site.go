// Package site: CDN site CRUD with the full GoEdge-style site config
// (contract.SiteConfig). The config JSON is the single source of truth;
// legacy flat columns are denormalized for listings and compatibility.
package site

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"edgecdn/core/internal/cert"
	"edgecdn/core/internal/contract"
	"edgecdn/core/internal/oplog"
	"edgecdn/core/internal/outbox"
	"edgecdn/core/internal/waf"
)

// Event types emitted to edges (see docs/api-contract.md).
const (
	EventSiteUpsert = "site_upsert"
	EventSiteDelete = "site_delete"
)

// Site is one CDN site row. Config is nil in list results.
type Site struct {
	ID        int64                  `json:"id"`
	TenantID  int64                  `json:"tenant_id"`
	Name      string                 `json:"name"`
	Domain    string                 `json:"domain"` // denormalized primary domain
	Status    string                 `json:"status"`
	Config    *contract.SiteConfig   `json:"config,omitempty"`
	NodeIDs   []int64                `json:"node_ids"`
	CreatedAt string                 `json:"created_at"`
	UpdatedAt string                 `json:"updated_at"`
}

// CreateReq is the site create body.
type CreateReq struct {
	Name    string                 `json:"name"`
	NodeIDs []int64                `json:"node_ids"`
	Config  *contract.SiteConfig   `json:"config"`
}

// UpdateReq is the site update body; nil fields keep current values.
// NodeIDs: nil = keep bindings, non-nil = replace (empty = broadcast).
type UpdateReq struct {
	NodeIDs *[]int64               `json:"node_ids"`
	Config  *contract.SiteConfig   `json:"config"`
	Status  string                 `json:"status"`
}

// Store wraps sites + site_nodes + push logic.
type Store struct {
	db     *sql.DB
	waf    *waf.Store
	cert   *cert.Service
	outbox *outbox.Outbox
	oplog  *oplog.Store
}

func New(db *sql.DB, waf *waf.Store, cert *cert.Service, outbox *outbox.Outbox, oplog *oplog.Store) *Store {
	return &Store{db: db, waf: waf, cert: cert, outbox: outbox, oplog: oplog}
}

const metaCols = `id, tenant_id, name, domain, status, created_at, updated_at`

// List returns paged site metadata; tenantID 0 = all (admin).
func (s *Store) List(tenantID int64, keyword string, page, size int) ([]Site, int64, error) {
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 200 {
		size = 20
	}
	where := `WHERE 1=1`
	var args []any
	if tenantID > 0 {
		where += ` AND s.tenant_id = ?`
		args = append(args, tenantID)
	}
	if keyword != "" {
		where += ` AND (s.name LIKE ? OR s.domain LIKE ?)`
		like := "%" + keyword + "%"
		args = append(args, like, like)
	}
	var total int64
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM sites s `+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.db.Query(`SELECT s.`+metaCols+` FROM sites s `+where+` ORDER BY s.id DESC LIMIT ? OFFSET ?`,
		append(args, size, (page-1)*size)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []Site
	for rows.Next() {
		var st Site
		if err := scanMeta(rows, &st); err != nil {
			return nil, 0, err
		}
		out = append(out, st)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	rows.Close()
	for i := range out {
		out[i].NodeIDs = s.nodeIDsFor(out[i].ID)
	}
	return out, total, nil
}

// Get fetches a site with its full config and node bindings.
func (s *Store) Get(id int64) (Site, error) {
	var st Site
	var cfgJSON string
	err := s.db.QueryRow(`SELECT `+metaCols+`, config FROM sites WHERE id = ?`, id).
		Scan(&st.ID, &st.TenantID, &st.Name, &st.Domain, &st.Status, &st.CreatedAt, &st.UpdatedAt, &cfgJSON)
	if err == sql.ErrNoRows {
		return st, fmt.Errorf("site not found")
	}
	if err != nil {
		return st, err
	}
	st.Status = normalizeStatus(st.Status)
	st.Config = s.parseConfig(&st, cfgJSON)
	st.NodeIDs = s.nodeIDsFor(id)
	return st, nil
}

// GetTenantless fetches a site by id without tenant check (edge full sync).
func (s *Store) GetTenantless(id int64) (Site, error) { return s.Get(id) }

// CountForTenant counts a tenant's sites (quota check).
func (s *Store) CountForTenant(tenantID int64) (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM sites WHERE tenant_id = ?`, tenantID).Scan(&n)
	return n, err
}

// Create inserts a site from a full config document.
func (s *Store) Create(tenantID int64, actor string, actorID int64, req CreateReq, maxSites int) (Site, error) {
	if req.Config == nil {
		return Site{}, fmt.Errorf("config required")
	}
	cfg := *req.Config
	cfg.ID = 0
	if req.Name != "" {
		cfg.Name = req.Name
	}
	cfg.EnsureDefaults()
	if err := cfg.Validate(); err != nil {
		return Site{}, err
	}
	count, err := s.CountForTenant(tenantID)
	if err != nil {
		return Site{}, err
	}
	if count >= maxSites {
		return Site{}, fmt.Errorf("site quota exceeded (%d/%d)", count, maxSites)
	}
	if err := s.domainConflict(tenantID, 0, cfg.PrimaryDomain()); err != nil {
		return Site{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	cfgJSON, err := json.Marshal(&cfg)
	if err != nil {
		return Site{}, err
	}
	lp, lh, lport, lttl, lwaf, lhttps := legacyCols(&cfg)
	res, err := s.db.Exec(`INSERT INTO sites (tenant_id, name, domain, origin_proto, origin_host, origin_port,
		cache_ttl, waf_enabled, https, status, config, created_at, updated_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		tenantID, cfg.Name, cfg.PrimaryDomain(), lp, lh, lport, lttl, lwaf, lhttps,
		cfg.Status, string(cfgJSON), now, now)
	if err != nil {
		return Site{}, fmt.Errorf("insert failed (domain may exist): %w", err)
	}
	id, _ := res.LastInsertId()
	cfg.ID = id
	s.setNodes(id, req.NodeIDs)

	st := Site{ID: id, TenantID: tenantID, Name: cfg.Name, Domain: cfg.PrimaryDomain(),
		Status: cfg.Status, Config: &cfg, NodeIDs: req.NodeIDs, CreatedAt: now, UpdatedAt: now}
	if err := s.ensureCerts(&cfg, tenantID); err != nil {
		return st, err
	}
	s.oplog.Add(actor, actorID, "site.create", cfg.PrimaryDomain(), fmt.Sprintf("id=%d domains=%v nodes=%v", id, cfg.Domains(), req.NodeIDs))
	if err := s.Emit(st); err != nil {
		return st, err
	}
	return st, nil
}

// Update replaces a site's config (full document), optionally rebinds nodes/status.
func (s *Store) Update(id int64, actor string, actorID int64, req UpdateReq) (Site, error) {
	cur, err := s.Get(id)
	if err != nil {
		return Site{}, err
	}
	var cfg *contract.SiteConfig
	if req.Config != nil {
		cp := *req.Config // copy: req points into the decoded JSON
		cfg = &cp
		cfg.ID = id
	} else {
		cfg = cur.Config
	}
	if cfg == nil {
		return Site{}, fmt.Errorf("site %d has no config", id)
	}
	if req.Status != "" {
		if req.Status != "normal" && req.Status != "off" {
			return Site{}, fmt.Errorf("invalid status %q", req.Status)
		}
		cfg.Status = req.Status
	}
	cfg.EnsureDefaults()
	if err := cfg.Validate(); err != nil {
		return Site{}, err
	}
	if err := s.domainConflict(cur.TenantID, id, cfg.PrimaryDomain()); err != nil {
		return Site{}, err
	}
	cfgJSON, err := json.Marshal(cfg)
	if err != nil {
		return Site{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	lp, lh, lport, lttl, lwaf, lhttps := legacyCols(cfg)
	_, err = s.db.Exec(`UPDATE sites SET name=?, domain=?, origin_proto=?, origin_host=?, origin_port=?,
		cache_ttl=?, waf_enabled=?, https=?, status=?, config=?, updated_at=? WHERE id=?`,
		cfg.Name, cfg.PrimaryDomain(), lp, lh, lport, lttl, lwaf, lhttps,
		cfg.Status, string(cfgJSON), now, id)
	if err != nil {
		return Site{}, fmt.Errorf("update failed (domain may conflict): %w", err)
	}
	if req.NodeIDs != nil {
		s.setNodes(id, *req.NodeIDs)
	}
	if err := s.ensureCerts(cfg, cur.TenantID); err != nil {
		return Site{}, err
	}
	s.oplog.Add(actor, actorID, "site.update", cfg.PrimaryDomain(),
		fmt.Sprintf("id=%d status=%s nodes_changed=%v", id, cfg.Status, req.NodeIDs != nil))
	st, err := s.Get(id)
	if err != nil {
		return Site{}, err
	}
	return st, s.Emit(st)
}

// SetStatus toggles a site on/off and re-emits.
func (s *Store) SetStatus(id int64, status string, actor string, actorID int64) (Site, error) {
	if status != "normal" && status != "off" {
		return Site{}, fmt.Errorf("invalid status %q", status)
	}
	st, err := s.Get(id)
	if err != nil {
		return Site{}, err
	}
	_, err = s.db.Exec(`UPDATE sites SET status=?, updated_at=? WHERE id=?`,
		status, time.Now().UTC().Format(time.RFC3339), id)
	if err != nil {
		return Site{}, err
	}
	s.oplog.Add(actor, actorID, "site.status", st.Domain, fmt.Sprintf("id=%d status=%s", id, status))
	st, err = s.Get(id)
	if err != nil {
		return Site{}, err
	}
	return st, s.Emit(st)
}

// Delete removes a site and tells its nodes.
func (s *Store) Delete(id int64, actor string, actorID int64) error {
	st, err := s.Get(id)
	if err != nil {
		return err
	}
	if _, err := s.db.Exec(`DELETE FROM sites WHERE id=?`, id); err != nil {
		return err
	}
	_, _ = s.db.Exec(`DELETE FROM site_nodes WHERE site_id=?`, id)
	payload, _ := json.Marshal(map[string]int64{"id": id})
	if err := s.outbox.Append(st.NodeIDs, EventSiteDelete, string(payload)); err != nil {
		return err
	}
	s.oplog.Add(actor, actorID, "site.delete", st.Domain, fmt.Sprintf("id=%d", id))
	return nil
}

// Emit pushes the full site spec to all bound nodes (broadcast when unbound).
func (s *Store) Emit(st Site) error {
	spec, err := s.BuildSpec(st)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(spec)
	if err != nil {
		return err
	}
	return s.outbox.Append(st.NodeIDs, EventSiteUpsert, string(payload))
}

// EmitForTenant re-pushes every active site of a tenant (WAF rules changed).
// Implements waf.SiteEmitter.
func (s *Store) EmitForTenant(tenantID int64) error {
	sites, _, err := s.List(tenantID, "", 1, 10000)
	if err != nil {
		return err
	}
	for _, st := range sites {
		if st.Status == "normal" {
			full, err := s.Get(st.ID)
			if err != nil {
				continue
			}
			if err := s.Emit(full); err != nil {
				return err
			}
		}
	}
	return nil
}

// EmitByDomain re-pushes every active site that uses the given domain
// (primary domain or bound in server_names). Called after cert changes.
func (s *Store) EmitByDomain(domain string) error {
	where := `WHERE status = 'normal' AND domain = ?`
	args := []any{domain}
	if !strings.ContainsAny(domain, `%_`) {
		where += ` OR config LIKE ?`
		args = append(args, `%"name":"`+domain+`"%`)
	}
	rows, err := s.db.Query(`SELECT id FROM sites `+where, args...)
	if err != nil {
		return err
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	rows.Close()
	for _, id := range ids {
		st, err := s.Get(id)
		if err != nil {
			continue
		}
		if err := s.Emit(st); err != nil {
			return err
		}
	}
	return nil
}

// BuildSpec assembles the edge-facing spec (config + certs + global WAF rules).
func (s *Store) BuildSpec(st Site) (contract.Spec, error) {
	if st.Config == nil {
		return contract.Spec{}, fmt.Errorf("site %d has no config", st.ID)
	}
	spec := contract.Spec{Site: *st.Config, GlobalWAFEnabled: true}
	spec.Site.ID = st.ID // stamp the DB id (config docs are stored with ID=0)
	// Certs: one per active domain (CA-issued, uploaded, or ACME).
	for _, d := range st.Config.Domains() {
		if c, err := s.cert.GetByDomain(d); err == nil {
			spec.Certs = append(spec.Certs, contract.DomainCert{
				Domain: d, Cert: c.LeafPEM, Key: c.KeyPEM, ExpiresAt: c.ExpiresAt,
			})
		}
	}
	// Tenant-global WAF rules.
	if rules, err := s.waf.EnabledForTenant(st.TenantID); err == nil {
		for _, r := range rules {
			spec.GlobalRules = append(spec.GlobalRules, contract.WAFRule{
				Name: r.Name, Status: "normal", Target: ruleTarget(r.Type),
				Operator: ruleOperator(r.Type), Value: r.Value, Action: r.Action, Level: 3,
			})
		}
	}
	return spec, nil
}

// ensureCerts issues CA certs for https-listened domains that have none.
func (s *Store) ensureCerts(cfg *contract.SiteConfig, tenantID int64) error {
	hasHTTPS := false
	for _, ln := range cfg.Listeners {
		if ln.Protocol == "https" {
			hasHTTPS = true
			break
		}
	}
	if !hasHTTPS {
		return nil
	}
	for _, d := range cfg.Domains() {
		if _, err := s.cert.GetByDomain(d); err == nil {
			continue
		}
		if _, err := s.cert.IssueLeaf(d, tenantID); err != nil {
			return fmt.Errorf("cert issue for %s: %w", d, err)
		}
	}
	return nil
}

// legacyCols derives the denormalized legacy columns from a config document.
func legacyCols(cfg *contract.SiteConfig) (proto, host string, port, ttl, wafInt, httpsInt int) {
	proto, host, port = "http", "127.0.0.1", 80
	loc := cfg.Location
	if loc != nil && loc.ReverseProxy != nil && len(loc.ReverseProxy.Origins) > 0 {
		o := loc.ReverseProxy.Origins[0]
		if h, p, ok := splitAddr(o.Addr); ok {
			host, port, proto = h, p, "http"
			if o.SSL {
				proto = "https"
			}
		}
	}
	if loc != nil && loc.Cache != nil && loc.Cache.Enabled {
		ttl = loc.Cache.TTL
	}
	if loc != nil && loc.WAF != nil && loc.WAF.Enabled {
		wafInt = 1
	}
	for _, ln := range cfg.Listeners {
		if ln.Protocol == "https" {
			httpsInt = 1
			break
		}
	}
	return
}

func splitAddr(addr string) (string, int, bool) {
	i := strings.LastIndexByte(addr, ':')
	if i < 0 {
		return "", 0, false
	}
	var p int
	if _, err := fmt.Sscanf(addr[i+1:], "%d", &p); err != nil || p < 1 || p > 65535 {
		return "", 0, false
	}
	return addr[:i], p, true
}

func (s *Store) domainConflict(tenantID, selfID int64, domain string) error {
	var other int64
	err := s.db.QueryRow(`SELECT id FROM sites WHERE domain = ? AND id != ? AND tenant_id != ?`,
		domain, selfID, tenantID).Scan(&other)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	return fmt.Errorf("domain %q already used by site %d", domain, other)
}

// parseConfig decodes the config column; legacy rows are backfilled.
func (s *Store) parseConfig(st *Site, cfgJSON string) *contract.SiteConfig {
	if strings.TrimSpace(cfgJSON) == "" || cfgJSON == "{}" || cfgJSON == "null" {
		return s.legacyToConfig(st)
	}
	var cfg contract.SiteConfig
	if err := json.Unmarshal([]byte(cfgJSON), &cfg); err != nil {
		// Corrupt config: fall back to legacy so the site keeps working.
		return s.legacyToConfig(st)
	}
	cfg.EnsureDefaults()
	return &cfg
}

// legacyToConfig rebuilds a SiteConfig from the old flat columns.
func (s *Store) legacyToConfig(st *Site) *contract.SiteConfig {
	var legacy struct {
		OriginProto string
		OriginHost  string
		OriginPort  int
		CacheTTL    int
		WAFEnabled  int
		HTTPS       int
	}
	_ = s.db.QueryRow(`SELECT origin_proto, origin_host, origin_port, cache_ttl, waf_enabled, https FROM sites WHERE id = ?`, st.ID).
		Scan(&legacy.OriginProto, &legacy.OriginHost, &legacy.OriginPort, &legacy.CacheTTL, &legacy.WAFEnabled, &legacy.HTTPS)
	if legacy.OriginHost == "" {
		legacy.OriginHost = "127.0.0.1"
		legacy.OriginPort = 8080
	}
	if legacy.OriginProto == "" {
		legacy.OriginProto = "http"
	}
	https := legacy.HTTPS == 1
	domain := st.Domain
	if domain == "" {
		domain = "localhost"
	}
	cfg := &contract.SiteConfig{
		ID:     st.ID,
		Name:   st.Name,
		Status: st.Status,
		ServerNames: []contract.ServerName{{Name: domain, IsDefault: true, Status: "normal"}},
		Listeners: []contract.Listener{{
			Protocol: "http", Listen: "127.0.0.1:18080",
			FollowProtocol: false,
		}},
		Location: &contract.Location{
			ReverseProxy: &contract.ReverseProxy{
				Origins: []contract.Origin{{
					Name: "origin", Addr: fmt.Sprintf("%s:%d", legacy.OriginHost, legacy.OriginPort),
					Status: "normal", SSL: legacy.OriginProto == "https", Insecure: true,
				}},
			},
			Cache: &contract.Cache{Enabled: legacy.CacheTTL > 0, Storage: "file", TTL: legacy.CacheTTL, OnlyGet: true},
			WAF:   &contract.WAF{Enabled: legacy.WAFEnabled == 1},
			AccessLog: &contract.AccessLog{Enabled: true},
			Stat:      &contract.Stat{Enabled: true},
		},
	}
	if https {
		cfg.Listeners = append(cfg.Listeners, contract.Listener{Protocol: "https", Listen: "127.0.0.1:18443"})
		cfg.TLS = &contract.TLSConfig{}
	}
	cfg.EnsureDefaults()
	return cfg
}

// MigrateLegacy repairs rows created by the pre-contract schema:
//  1. normalize integer statuses ("1"/"" -> normal, "0" -> off);
//  2. persist a rebuilt config document for sites stored with empty/null
//     config JSON so full-sync (which requires status='normal' + a config)
//     can deliver them to the edges.
// Single-connection DB: collect IDs first, close rows, then do nested reads.
func (s *Store) MigrateLegacy() error {
	if _, err := s.db.Exec(`UPDATE sites SET status='normal' WHERE status IN ('1','')`); err != nil {
		return err
	}
	if _, err := s.db.Exec(`UPDATE sites SET status='off' WHERE status = '0'`); err != nil {
		return err
	}
	rows, err := s.db.Query(`SELECT id FROM sites WHERE config IS NULL OR TRIM(config) IN ('', '{}', 'null')`)
	if err != nil {
		return err
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	rows.Close()
	for _, id := range ids {
		st, err := s.Get(id)
		if err != nil || st.Config == nil {
			continue
		}
		cfgJSON, err := json.Marshal(st.Config)
		if err != nil {
			continue
		}
		if _, err := s.db.Exec(`UPDATE sites SET config = ? WHERE id = ?`, string(cfgJSON), id); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) nodeIDsFor(siteID int64) []int64 {
	rows, err := s.db.Query(`SELECT node_id FROM site_nodes WHERE site_id = ? ORDER BY node_id`, siteID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := []int64{}
	for rows.Next() {
		var id int64
		if rows.Scan(&id) == nil {
			out = append(out, id)
		}
	}
	return out
}

func (s *Store) setNodes(siteID int64, nodeIDs []int64) {
	_, _ = s.db.Exec(`DELETE FROM site_nodes WHERE site_id = ?`, siteID)
	for _, nid := range nodeIDs {
		_, _ = s.db.Exec(`INSERT OR IGNORE INTO site_nodes (site_id, node_id) VALUES (?,?)`, siteID, nid)
	}
}

func scanMeta(rows *sql.Rows, st *Site) error {
	if err := rows.Scan(&st.ID, &st.TenantID, &st.Name, &st.Domain, &st.Status, &st.CreatedAt, &st.UpdatedAt); err != nil {
		return err
	}
	st.Status = normalizeStatus(st.Status)
	return nil
}

// normalizeStatus maps legacy integer statuses ("1"/"0") onto the
// GoEdge-style string statuses.
func normalizeStatus(s string) string {
	switch s {
	case "normal", "off":
		return s
	case "1", "":
		return "normal"
	case "0":
		return "off"
	default:
		return "normal"
	}
}

// ruleTarget maps a legacy WAF rule type to a contract WAF target.
func ruleTarget(typ string) string {
	switch typ {
	case "ip_blacklist", "ip_whitelist":
		return "ip"
	case "ua_blacklist":
		return "user_agent"
	case "path_blacklist":
		return "url"
	default:
		return "url"
	}
}

// ruleOperator maps a legacy WAF rule type to a contract operator.
func ruleOperator(typ string) string {
	switch typ {
	case "ip_whitelist":
		return "not_equals" // expressed via IP list on the edge side
	case "rate_limit":
		return "regex"
	default:
		return "contains"
	}
}
