package gopxy

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"time"
)

// Cache is a tiny disk cache: one meta JSON + one body file per key.
type Cache struct {
	dir string
}

// NewCache creates the cache dir.
func NewCache(dataDir string) (*Cache, error) {
	dir := filepath.Join(dataDir, "cache")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	c := &Cache{dir: dir}
	go c.gcLoop()
	return c, nil
}

type meta struct {
	Status   int               `json:"status"`
	Headers  map[string]string `json:"headers"`
	StoredAt time.Time         `json:"stored_at"`
	Size     int64             `json:"size"`
}

// Get returns cached body + headers when fresh (age <= ttlSec).
func (c *Cache) Get(key string, ttlSec int) ([]byte, map[string]string, bool) {
	if ttlSec <= 0 {
		return nil, nil, false
	}
	m, ok := c.readMeta(key)
	if !ok {
		return nil, nil, false
	}
	if time.Since(m.StoredAt) > time.Duration(ttlSec)*time.Second {
		return nil, nil, false
	}
	body, err := os.ReadFile(c.bodyPath(key))
	if err != nil {
		return nil, nil, false
	}
	return body, m.Headers, true
}

// Put stores a fresh response.
func (c *Cache) Put(key string, status int, headers map[string]string, body []byte) error {
	m := meta{Status: status, Headers: headers, StoredAt: time.Now(), Size: int64(len(body))}
	mb, _ := json.Marshal(m)
	if err := os.WriteFile(c.metaPath(key), mb, 0o644); err != nil {
		return err
	}
	return os.WriteFile(c.bodyPath(key), body, 0o644)
}

// GC removes entries older than 24h.
func (c *Cache) GC() error {
	entries, err := os.ReadDir(c.dir)
	if err != nil {
		return err
	}
	now := time.Now()
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		info, err := os.Stat(filepath.Join(c.dir, e.Name()))
		if err != nil {
			continue
		}
		key := e.Name()[:len(e.Name())-len(".json")]
		if now.Sub(info.ModTime()) > 24*time.Hour {
			_ = os.Remove(c.metaPath(key))
			_ = os.Remove(c.bodyPath(key))
		}
	}
	return nil
}

func (c *Cache) gcLoop() {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		if err := c.GC(); err != nil {
			log.Printf("[cache] gc: %v", err)
		}
	}
}

// cacheKey is hex-only (sha1), so it is filesystem-safe by construction.
func (c *Cache) metaPath(key string) string {
	return filepath.Join(c.dir, key+".json")
}

func (c *Cache) bodyPath(key string) string {
	return filepath.Join(c.dir, key+".bin")
}

func (c *Cache) readMeta(key string) (meta, bool) {
	var m meta
	b, err := os.ReadFile(c.metaPath(key))
	if err != nil {
		return m, false
	}
	if err := json.Unmarshal(b, &m); err != nil {
		return m, false
	}
	return m, true
}
