// Package gopxy: pure-Go data plane (default driver).
// A thin wrapper around the engine package (full GoEdge feature set:
// listeners, rewrites, WAF, cache, compression, webp, auth, limits, tcp/udp).
package gopxy

import (
	"path/filepath"

	"edgecdn/edge/internal/contract"
	"edgecdn/edge/internal/engine"
	"edgecdn/edge/internal/stats"
)

// Version is the edge protocol/engine version reported to core.
const Version = "2.0.0"

// Proxy is the gopxy driver.
type Proxy struct {
	listenAddr string
	tlsAddr    string
	dataDir    string
	eng        *engine.Engine
	logs       *engine.AccessLogWriter
}

// New creates the driver (not started).
func New(listenAddr, tlsAddr, dataDir string, st *stats.Stats) (*Proxy, error) {
	logs := engine.NewAccessLogWriter(filepath.Join(dataDir, "logs"))
	eng := engine.New(dataDir, listenAddr, tlsAddr, st, logs)
	return &Proxy{
		listenAddr: listenAddr,
		tlsAddr:    tlsAddr,
		dataDir:    dataDir,
		eng:        eng,
		logs:       logs,
	}, nil
}

func (p *Proxy) Name() string    { return "gopxy" }
func (p *Proxy) Version() string { return Version }

// Start launches the fallback HTTP/HTTPS listeners.
func (p *Proxy) Start() error { return p.eng.Start() }

// ApplySite (re)applies a full site spec.
func (p *Proxy) ApplySite(spec contract.Spec) error { return p.eng.ApplySpec(spec) }

// RemoveSite drops a site.
func (p *Proxy) RemoveSite(id int64) error { return p.eng.RemoveSite(id) }

// DrainLogs returns pending access-log entries for upload.
func (p *Proxy) DrainLogs() []map[string]any { return p.logs.Take(1000) }

// Stop shuts the engine down.
func (p *Proxy) Stop() error {
	_ = p.eng.Stop()
	p.logs.Close()
	return nil
}
