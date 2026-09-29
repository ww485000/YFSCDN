// Package driver defines the data-plane interface.
// Site specs use the shared contract package (mirrored from core — see
// docs/api-contract.md; the two copies must stay in sync).
package driver

import "edgecdn/edge/internal/contract"

// Driver is the data-plane backend (gopxy = pure Go engine, nginx = nginx instance).
type Driver interface {
	Name() string
	Version() string
	Start() error
	ApplySite(spec contract.Spec) error
	RemoveSite(id int64) error
	// DrainLogs returns pending access-log entries (up to ~1000) for upload.
	DrainLogs() []map[string]any
	Stop() error
}
