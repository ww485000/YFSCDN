// Package cfg loads core configuration from a JSON file with env overrides.
package cfg

import (
	"encoding/json"
	"fmt"
	"os"
)

// Config holds all core settings.
type Config struct {
	ListenAddr string `json:"listen_addr"` // HTTP listen address, e.g. ":8080"
	DataDir    string `json:"data_dir"`    // SQLite file directory
	JWTSecret  string `json:"jwt_secret"`
	StaticDir  string `json:"static_dir"` // optional dir with built web/dist to serve at /
	CORSOrigin string `json:"cors_origin"`
}

// Load reads config from path (may not exist -> defaults), then env overrides.
func Load(path string) (*Config, error) {
	c := &Config{
		ListenAddr: ":8080",
		DataDir:    "core/data",
		JWTSecret:  "edgecdn-dev-secret-change-me",
		CORSOrigin: "*",
	}
	if path != "" {
		b, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				return c, nil
			}
			return nil, err
		}
		if err := json.Unmarshal(b, c); err != nil {
			return nil, fmt.Errorf("parse config %s: %w", path, err)
		}
	}
	if v := os.Getenv("EDGECDN_LISTEN"); v != "" {
		c.ListenAddr = v
	}
	if v := os.Getenv("EDGECDN_DATA"); v != "" {
		c.DataDir = v
	}
	if v := os.Getenv("EDGECDN_JWT"); v != "" {
		c.JWTSecret = v
	}
	if v := os.Getenv("EDGECDN_STATIC"); v != "" {
		c.StaticDir = v
	}
	return c, nil
}
