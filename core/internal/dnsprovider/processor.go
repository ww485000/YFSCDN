package dnsprovider

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"edgecdn/core/internal/dns"
	"edgecdn/core/internal/task"
)

type Processor struct {
	providers *Store
	records   *dns.Store
}

func NewProcessor(providers *Store, records *dns.Store) *Processor {
	return &Processor{providers: providers, records: records}
}

func (p *Processor) Resolve(t *task.Task) error {
	switch {
	case strings.HasPrefix(t.Target, "provider:"):
		id, err := parseID(strings.TrimPrefix(t.Target, "provider:"))
		if err != nil {
			return err
		}
		return p.syncProvider(id)
	case strings.HasPrefix(t.Target, "record:"):
		id, err := parseID(strings.TrimPrefix(t.Target, "record:"))
		if err != nil {
			return err
		}
		return p.syncRecord(id)
	default:
		return fmt.Errorf("unsupported dns resolve target %q", t.Target)
	}
}

func (p *Processor) Clean(t *task.Task) error {
	if !strings.HasPrefix(t.Target, "record:") {
		return fmt.Errorf("unsupported dns clean target %q", t.Target)
	}
	id, err := parseID(strings.TrimPrefix(t.Target, "record:"))
	if err != nil {
		return err
	}
	rec, err := p.records.Get(id)
	if err != nil {
		if t.Payload == "" || json.Unmarshal([]byte(t.Payload), &rec) != nil || rec.ID == 0 {
			return p.providers.DeleteRecordSyncs(id)
		}
	}
	providers, err := p.providers.ActiveByTenant(rec.TenantID)
	if err != nil {
		return err
	}
	var lastErr error
	for _, provider := range providers {
		upstreamID, _ := p.providers.GetSync(provider.ID, rec.ID)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		err := DriverFor(provider).Delete(ctx, provider, rec, upstreamID)
		cancel()
		if err != nil {
			_ = p.providers.SaveSync(provider.ID, rec.ID, upstreamID, RecordHash(rec), err.Error())
			lastErr = err
			continue
		}
		_ = p.providers.DeleteSync(provider.ID, rec.ID)
	}
	return lastErr
}

func (p *Processor) syncProvider(providerID int64) error {
	provider, err := p.providers.GetWithSecret(providerID)
	if err != nil {
		return err
	}
	if provider.Status == 0 {
		return nil
	}
	records, err := p.records.ListTenant(provider.TenantID, "")
	if err != nil {
		return err
	}
	var lastErr error
	for _, rec := range records {
		if err := p.upsert(provider, rec); err != nil {
			lastErr = err
		}
	}
	return lastErr
}

func (p *Processor) syncRecord(recordID int64) error {
	rec, err := p.records.Get(recordID)
	if err != nil {
		return err
	}
	providers, err := p.providers.ActiveByTenant(rec.TenantID)
	if err != nil {
		return err
	}
	var lastErr error
	for _, provider := range providers {
		if err := p.upsert(provider, rec); err != nil {
			lastErr = err
		}
	}
	return lastErr
}

func (p *Processor) upsert(provider Provider, rec dns.Record) error {
	if rec.SyncMode == "manual" {
		return nil
	}
	upstreamID, _ := p.providers.GetSync(provider.ID, rec.ID)
	hash := RecordHash(rec)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	nextID, err := DriverFor(provider).Upsert(ctx, provider, rec, upstreamID)
	cancel()
	if err != nil {
		_ = p.providers.SaveSync(provider.ID, rec.ID, upstreamID, hash, err.Error())
		return err
	}
	if nextID == "" {
		return p.providers.DeleteSync(provider.ID, rec.ID)
	}
	return p.providers.SaveSync(provider.ID, rec.ID, nextID, hash, "")
}

func parseID(s string) (int64, error) {
	var id int64
	for _, ch := range s {
		if ch < '0' || ch > '9' {
			return 0, fmt.Errorf("invalid id %q", s)
		}
		id = id*10 + int64(ch-'0')
	}
	if id <= 0 {
		return 0, fmt.Errorf("invalid id %q", s)
	}
	return id, nil
}
