// Package cfg loads edge node configuration (JSON file + env overrides).
package cfg

import (
	"encoding/json"
	"fmt"
	"os"
)

// Config holds all edge settings.
type Config struct {
	CoreURL   string `json:"core_url"`   // e.g. http://127.0.0.1:8080
	NodeToken string `json:"node_token"` // token from the core's node record
	Name      string `json:"name"`
	Driver    string `json:"driver"`    // "gopxy" (default) | "nginx"
	ListenAddr string `json:"listen_addr"` // HTTP listen, default ":80"
	TLSAddr   string `json:"tls_addr"`  // HTTPS listen, default ":443"
	DataDir   string `json:"data_dir"`  // cache + nginx conf root, default "edge/data"
	NginxBin  string `json:"nginx_bin"` // nginx executable (nginx driver only)
	Host      string `json:"host"`      // self-reported address for core
	HeartbeatSec int `json:"heartbeat_sec"`
}

// Load reads config (file optional) then env overrides.
func Load(path string) (*Config, error) {
	c := &Config{
		CoreURL:   "http://127.0.0.1:8080",
		Driver:    "gopxy",
		ListenAddr: ":80",
		TLSAddr:   ":443",
		DataDir:   "edge/data",
		HeartbeatSec: 30,
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
	if v := os.Getenv("EDGECDN_EDGE_CORE"); v != "" {
		c.CoreURL = v
	}
	if v := os.Getenv("EDGECDN_EDGE_TOKEN"); v != "" {
		c.NodeToken = v
	}
	if v := os.Getenv("EDGECDN_EDGE_DRIVER"); v != "" {
		c.Driver = v
	}
	if v := os.Getenv("EDGECDN_EDGE_DATA"); v != "" {
		c.DataDir = v
	}
	if v := os.Getenv("EDGECDN_EDGE_HOST"); v != "" {
		c.Host = v
	}
	if c.NodeToken == "" {
		return nil, fmt.Errorf("node_token is required (edge config or EDGECDN_EDGE_TOKEN)")
	}
	return c, nil
}
