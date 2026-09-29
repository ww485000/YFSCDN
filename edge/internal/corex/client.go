// Package corex: edge <-> core communication (register, long poll, full sync,
// heartbeat + stats, access-log upload).
// Protocol contract: docs/api-contract.md "节点端 API".
package corex

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"edgecdn/edge/internal/cfg"
	"edgecdn/edge/internal/contract"
	"edgecdn/edge/internal/driver"
	"edgecdn/edge/internal/stats"
)

// Client keeps the edge in sync with core.
type Client struct {
	cfg     *cfg.Config
	drv     driver.Driver
	stats   *stats.Stats
	httpc   *http.Client
	pollC   *http.Client // longer timeout for long polling
	mu      sync.Mutex
	cursor  int64
	nodeID  int64
}

// New builds the client.
func New(c *cfg.Config, drv driver.Driver, st *stats.Stats) *Client {
	cl := &Client{
		cfg:     c,
		drv:     drv,
		stats:   st,
		httpc:   &http.Client{Timeout: 10 * time.Second},
		pollC:   &http.Client{Timeout: 40 * time.Second},
	}
	cl.loadCursor()
	return cl
}

// cursorFile persists the long-poll cursor across restarts so a node resumes
// where it left off instead of replaying the whole outbox history.
func (c *Client) cursorFile() string {
	return filepath.Join(c.cfg.DataDir, "poll-cursor")
}

func (c *Client) loadCursor() {
	b, err := os.ReadFile(c.cursorFile())
	if err != nil {
		return
	}
	if n, err := strconv.ParseInt(string(bytes.TrimSpace(b)), 10, 64); err == nil && n > 0 {
		c.cursor = n
		log.Printf("[corex] resumed poll cursor at %d", n)
	}
}

func (c *Client) saveCursor() {
	if c.cursor <= 0 {
		return
	}
	if err := os.MkdirAll(c.cfg.DataDir, 0o755); err == nil {
		_ = os.WriteFile(c.cursorFile(), []byte(strconv.FormatInt(c.cursor, 10)), 0o644)
	}
}

// Run blocks: registers, full-syncs, then runs poll + heartbeat loops.
func (c *Client) Run(ctx context.Context) error {
	if err := c.register(); err != nil {
		return fmt.Errorf("register: %w", err)
	}
	if err := c.fullSync(); err != nil {
		log.Printf("[corex] full sync failed (will rely on poll events): %v", err)
	}
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		c.pollLoop(ctx)
	}()
	go func() {
		defer wg.Done()
		c.heartbeatLoop(ctx)
	}()
	<-ctx.Done()
	wg.Wait()
	// final log drain
	if logs := c.drv.DrainLogs(); len(logs) > 0 {
		_ = c.postLogs(logs)
	}
	return nil
}

func (c *Client) register() error {
	body, _ := json.Marshal(map[string]any{
		"token":   c.cfg.NodeToken,
		"host":    c.cfg.Host,
		"ip":      c.cfg.Host,
		"driver":  c.drv.Name(),
		"version": c.drv.Version(),
	})
	var lastErr error
	for i := 0; i < 30; i++ {
		lastErr = c.post("/api/v1/edge/register", body, nil)
		if lastErr == nil {
			log.Printf("[corex] registered with core %s as driver %s v%s", c.cfg.CoreURL, c.drv.Name(), c.drv.Version())
			return nil
		}
		if isAuthErr(lastErr) {
			return fmt.Errorf("core rejected node token (check node_token matches the node created in core): %w", lastErr)
		}
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("core unreachable after retries: %w", lastErr)
}

// fullSync pulls every site assigned to this node (recovery after restart).
func (c *Client) fullSync() error {
	var resp struct {
		Sites []contract.Spec `json:"sites"`
	}
	if err := c.get("/api/v1/edge/full", &resp); err != nil {
		return err
	}
	for i := range resp.Sites {
		if resp.Sites[i].Site.ID <= 0 {
			continue // legacy payload without a site id
		}
		if err := c.apply(resp.Sites[i]); err != nil {
			log.Printf("[corex] full sync apply site %d failed: %v", resp.Sites[i].Site.ID, err)
		}
	}
	log.Printf("[corex] full sync: %d site(s) loaded", len(resp.Sites))
	return nil
}

// pollLoop long-polls for events; on failure retries with backoff.
func (c *Client) pollLoop(ctx context.Context) {
	backoff := 2 * time.Second
	for ctx.Err() == nil {
		c.mu.Lock()
		cursor := c.cursor
		c.mu.Unlock()

		var resp struct {
			Cursor int64 `json:"cursor"`
			Events []struct {
				Type    string          `json:"type"`
				Payload json.RawMessage `json:"payload"`
			} `json:"events"`
		}
		if err := c.poll(cursor, &resp); err != nil {
			log.Printf("[corex] poll error: %v (retry in %s)", err, backoff)
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
			if backoff < 30*time.Second {
				backoff *= 2
			}
			continue
		}
		backoff = 2 * time.Second
		if resp.Cursor > 0 {
			c.mu.Lock()
			if resp.Cursor > c.cursor {
				c.cursor = resp.Cursor
			}
			c.mu.Unlock()
			c.saveCursor()
		}
		for _, e := range resp.Events {
			switch e.Type {
			case "site_upsert":
				var spec contract.Spec
				if err := json.Unmarshal(e.Payload, &spec); err != nil {
					log.Printf("[corex] bad site_upsert payload: %v", err)
					continue
				}
				if spec.Site.ID <= 0 {
					continue // legacy payload without a site id
				}
				if err := c.apply(spec); err != nil {
					log.Printf("[corex] apply site %d failed: %v", spec.Site.ID, err)
				}
			case "site_delete":
				var p struct {
					ID int64 `json:"id"`
				}
				if err := json.Unmarshal(e.Payload, &p); err == nil {
					_ = c.drv.RemoveSite(p.ID)
				}
			default:
				log.Printf("[corex] unknown event type %q (edge may be older than core)", e.Type)
			}
		}
	}
}

// heartbeatLoop reports traffic counters + pending access logs periodically.
func (c *Client) heartbeatLoop(ctx context.Context) {
	interval := time.Duration(c.cfg.HeartbeatSec) * time.Second
	if interval < 5*time.Second {
		interval = 30 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			reports := c.stats.Snapshot()
			body, _ := json.Marshal(map[string]any{
				"driver":  c.drv.Name(),
				"version": c.drv.Version(),
				"reports": reports,
			})
			if err := c.post("/api/v1/edge/heartbeat", body, nil); err != nil {
				log.Printf("[corex] heartbeat failed: %v", err)
			}
			if logs := c.drv.DrainLogs(); len(logs) > 0 {
				if err := c.postLogs(logs); err != nil {
					log.Printf("[corex] log upload failed: %v", err)
				}
			}
		}
	}
}

func (c *Client) postLogs(entries []map[string]any) error {
	body, _ := json.Marshal(map[string]any{"logs": entries})
	return c.post("/api/v1/edge/logs", body, nil)
}

func (c *Client) apply(spec contract.Spec) error {
	return c.drv.ApplySite(spec)
}

func (c *Client) poll(cursor int64, out any) error {
	u := fmt.Sprintf("%s/api/v1/edge/poll?cursor=%d&token=%s",
		c.cfg.CoreURL, cursor, url.QueryEscape(c.cfg.NodeToken))
	return c.do(c.pollC, http.MethodGet, u, nil, out)
}

func (c *Client) get(path string, out any) error {
	return c.do(c.httpc, http.MethodGet, c.cfg.CoreURL+path, nil, out)
}

func (c *Client) post(path string, body []byte, out any) error {
	return c.do(c.httpc, http.MethodPost, c.cfg.CoreURL+path, body, out)
}

// do performs one core request; core answers {code,message,data}.
func (c *Client) do(client *http.Client, method, u string, body []byte, out any) error {
	var rd io.Reader
	if body != nil {
		rd = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, u, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Node-Token", c.NodeToken())
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	var env struct {
		Code    int    `json:"code"`
		Message string  `json:"message"`
		Data    any     `json:"data"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return fmt.Errorf("bad response (%s): %s", u, truncate(string(raw), 200))
	}
	if env.Code != 0 || resp.StatusCode >= 400 {
		return &apiError{status: resp.StatusCode, code: env.Code, msg: env.Message}
	}
	if out != nil {
		db, _ := json.Marshal(env.Data)
		if err := json.Unmarshal(db, out); err != nil {
			return fmt.Errorf("decode data: %w", err)
		}
	}
	return nil
}

// NodeToken exposes the token (used by do; also lets tests spy on it).
func (c *Client) NodeToken() string { return c.cfg.NodeToken }

type apiError struct {
	status int
	code   int
	msg    string
}

func (e *apiError) Error() string {
	return fmt.Sprintf("core api %d/%d: %s", e.status, e.code, e.msg)
}

func isAuthErr(err error) bool {
	ae, ok := err.(*apiError)
	return ok && (ae.status == 401 || ae.code == 401)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
