// Package stats: per-site counters reported to core on heartbeat.
package stats

import "sync"

// Report is one heartbeat entry.
type Report struct {
	SiteID      int64 `json:"site_id"`
	Requests    int64 `json:"requests"`
	Bytes       int64 `json:"bytes"`
	CacheHits   int64 `json:"cache_hits"`
	CacheMisses int64 `json:"cache_misses"`
}

// Stats accumulates counters and resets on Snapshot.
type Stats struct {
	mu sync.Mutex
	m  map[int64]*Report
}

func New() *Stats { return &Stats{m: map[int64]*Report{}} }

// Add records one response.
func (s *Stats) Add(siteID int64, respBytes int64, cacheHit bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := s.m[siteID]
	if r == nil {
		r = &Report{SiteID: siteID}
		s.m[siteID] = r
	}
	r.Requests++
	r.Bytes += respBytes
	if cacheHit {
		r.CacheHits++
	} else {
		r.CacheMisses++
	}
}

// Snapshot returns accumulated reports and resets the counters.
func (s *Stats) Snapshot() []Report {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Report, 0, len(s.m))
	for id, r := range s.m {
		out = append(out, *r)
		delete(s.m, id)
	}
	return out
}
