package engine

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"edgecdn/edge/internal/contract"
)

// cacheStore serves/stores responses per location.
type cacheStore struct {
	cfg      *contract.Cache
	dir      string
	ttl      time.Duration
	staleTTL time.Duration
	onlyGet  bool
	// memory store (storage = "memory")
	mu      sync.Mutex
	mem     map[string]memEntry
	memSize int64
	maxSize int64
}

type memEntry struct {
	body    []byte
	headers http.Header
	status  int
	stored  time.Time
}

func newCacheStore(dataDir string, c *contract.Cache) (*cacheStore, error) {
	cs := &cacheStore{cfg: c, onlyGet: c.OnlyGet}
	ttl := time.Duration(c.TTL) * time.Second
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	cs.ttl = ttl
	if c.Stale != nil {
		cs.staleTTL = time.Duration(c.Stale.TTL) * time.Second
	}
	maxBytes, _ := contract.ParseSize(c.MaxSize)
	if maxBytes <= 0 {
		if c.Storage == "memory" {
			maxBytes = 256 << 20
		} else {
			maxBytes = 10 << 30
		}
	}
	cs.maxSize = maxBytes

	if c.Storage == "memory" {
		cs.mem = map[string]memEntry{}
		return cs, nil
	}
	// file storage
	d := filepath.Join(dataDir, "cache")
	if err := os.MkdirAll(d, 0o755); err != nil {
		return nil, err
	}
	cs.dir = d
	go cs.evictLoop()
	return cs, nil
}

// key builds the cache key (custom query/header composition via cfg.Key).
func (cs *cacheStore) key(r *http.Request) string {
	var b strings.Builder
	b.WriteString(r.Host)
	b.WriteString(r.URL.Path)
	q := r.URL.Query()
	if cs.cfg.Key != nil {
		if len(cs.cfg.Key.IgnoreQuery) > 0 {
			for _, ig := range cs.cfg.Key.IgnoreQuery {
				q.Del(ig)
			}
		} else if len(cs.cfg.Key.IncludeQuery) > 0 {
			keep := map[string]bool{}
			for _, k := range cs.cfg.Key.IncludeQuery {
				keep[k] = true
			}
			for k := range q {
				if !keep[k] {
					q.Del(k)
				}
			}
		}
	}
	if e := q.Encode(); e != "" {
		b.WriteByte('?')
		b.WriteString(e)
	}
	if cs.cfg.Key != nil {
		for _, h := range cs.cfg.Key.IncludeHeaders {
			b.WriteString("\x1e")
			b.WriteString(h)
			b.WriteString(":")
			b.WriteString(r.Header.Get(h))
		}
	}
	sum := sha256.Sum256([]byte(b.String()))
	return fmt.Sprintf("%x", sum)
}

type cachedEntry struct {
	body    []byte
	headers http.Header
	status  int
	stale   bool
}

// get returns a cached entry or nil.
func (cs *cacheStore) get(r *http.Request) *cachedEntry {
	k := cs.key(r)
	now := time.Now()
	staleOK := cs.cfg.Stale != nil && cs.cfg.Stale.Enabled
	if cs.mem != nil {
		cs.mu.Lock()
		e, ok := cs.mem[k]
		cs.mu.Unlock()
		if !ok {
			return nil
		}
		if now.Sub(e.stored) < cs.ttl {
			return &cachedEntry{body: e.body, headers: e.headers, status: e.status}
		}
		if staleOK && now.Sub(e.stored) < cs.ttl+cs.staleTTL {
			return &cachedEntry{body: e.body, headers: e.headers, status: e.status, stale: true}
		}
		return nil
	}
	if cs.dir == "" {
		return nil
	}
	meta, body, ok := cs.readMeta(k)
	if !ok {
		return nil
	}
	age := now.Sub(meta.Stored)
	if age < cs.ttl {
		return &cachedEntry{body: body, headers: meta.Header(), status: meta.Status}
	}
	if staleOK && age < cs.ttl+cs.staleTTL {
		return &cachedEntry{body: body, headers: meta.Header(), status: meta.Status, stale: true}
	}
	return nil
}

// put stores a response (skips when NoCacheHeaders present).
func (cs *cacheStore) put(r *http.Request, status int, headers http.Header, body []byte) {
	if status != http.StatusOK || len(body) == 0 {
		return
	}
	if len(body) > 10<<20 { // 10MB cap per entry
		return
	}
	// no-cache headers (default: Set-Cookie)
	noCache := cs.cfg.NoCacheHeaders
	if len(noCache) == 0 {
		noCache = []string{"Set-Cookie"}
	}
	for _, h := range noCache {
		if headers.Get(h) != "" {
			return
		}
	}
	if cs.mem != nil {
		cs.mu.Lock()
		defer cs.mu.Unlock()
		if cs.memSize+int64(len(body)) > cs.maxSize {
			cs.evictMemory()
		}
		cs.mem[cs.key(r)] = memEntry{body: body, headers: cloneHeader(headers), status: status, stored: time.Now()}
		cs.memSize += int64(len(body))
		return
	}
	k := cs.key(r)
	meta := fileMeta{Status: status, Stored: time.Now()}
	for k2, v := range headers {
		for _, vv := range v {
			meta.Headers = append(meta.Headers, k2+"\x1f"+vv)
		}
	}
	j, _ := json.Marshal(meta)
	_ = os.WriteFile(cs.fileBody(k), body, 0o644)
	_ = os.WriteFile(cs.fileMeta(k), j, 0o644)
}

type fileMeta struct {
	Status  int       `json:"status"`
	Headers []string  `json:"headers"`
	Stored  time.Time `json:"stored"`
}

func (cs *cacheStore) fileBody(k string) string { return filepath.Join(cs.dir, k+".body") }
func (cs *cacheStore) fileMeta(k string) string { return filepath.Join(cs.dir, k+".meta") }

func (cs *cacheStore) readMeta(k string) (*fileMeta, []byte, bool) {
	j, err := os.ReadFile(cs.fileMeta(k))
	if err != nil {
		return nil, nil, false
	}
	var m fileMeta
	if err := json.Unmarshal(j, &m); err != nil {
		return nil, nil, false
	}
	body, err := os.ReadFile(cs.fileBody(k))
	if err != nil {
		return nil, nil, false
	}
	return &m, body, true
}

func (m *fileMeta) Header() http.Header {
	h := http.Header{}
	for _, kv := range m.Headers {
		i := strings.IndexByte(kv, 0x1f)
		if i <= 0 {
			continue
		}
		h.Add(kv[:i], kv[i+1:])
	}
	return h
}

func (cs *cacheStore) evictMemory() {
	type kk struct {
		k   string
		t   time.Time
		sz  int
	}
	var all []kk
	for k, e := range cs.mem {
		all = append(all, kk{k: k, t: e.stored, sz: len(e.body)})
	}
	// oldest first
	for i := 0; i < len(all); i++ {
		for j := i + 1; j < len(all); j++ {
			if all[j].t.Before(all[i].t) {
				all[i], all[j] = all[j], all[i]
			}
		}
	}
	drop := len(all) / 2
	if drop < 1 && len(all) > 0 {
		drop = 1
	}
	for i := 0; i < drop && i < len(all); i++ {
		cs.memSize -= int64(all[i].sz)
		delete(cs.mem, all[i].k)
	}
}

func (cs *cacheStore) evictLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		if cs.dir == "" {
			return
		}
		entries, err := os.ReadDir(cs.dir)
		if err != nil {
			continue
		}
		// delete expired metas
		for _, en := range entries {
			if !strings.HasSuffix(en.Name(), ".meta") {
				continue
			}
			p := filepath.Join(cs.dir, en.Name())
			fi, err := en.Info()
			if err != nil {
				continue
			}
			if time.Now().Sub(fi.ModTime()) > cs.ttl+cs.staleTTL {
				os.Remove(p)
				os.Remove(strings.TrimSuffix(p, ".meta") + ".body")
			}
		}
	}
}

func cloneHeader(h http.Header) http.Header {
	out := http.Header{}
	for k, v := range h {
		out[k] = append([]string(nil), v...)
	}
	return out
}

// drainBody copies an upstream body into memory (bounded).
func drainBody(r io.Reader, max int64) ([]byte, error) {
	return io.ReadAll(io.LimitReader(r, max))
}
