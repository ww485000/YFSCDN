// Package outbox is the event queue between core and edge nodes.
// core appends events on every site change; edges long-poll for events after their cursor.
package outbox

import (
	"database/sql"
	"log"
	"time"
)

// Event is one outbox row. NodeID=0 means broadcast to all nodes.
type Event struct {
	Version int64  `json:"version"`
	NodeID  int64  `json:"-"`
	Type    string `json:"type"`
	Payload string `json:"payload"`
}

// Outbox wraps the outbox table.
type Outbox struct{ db *sql.DB }

func New(db *sql.DB) *Outbox { return &Outbox{db: db} }

// Append adds an event for the given nodes (empty => broadcast).
func (o *Outbox) Append(nodeIDs []int64, typ, payload string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	if len(nodeIDs) == 0 {
		if _, err := o.db.Exec(`INSERT INTO outbox (node_id, type, payload, created_at) VALUES (0,?,?,?)`, typ, payload, now); err != nil {
			return err
		}
		log.Printf("[outbox] append broadcast %s", typ)
		return nil
	}
	for _, nid := range nodeIDs {
		if _, err := o.db.Exec(`INSERT INTO outbox (node_id, type, payload, created_at) VALUES (?,?,?,?)`, nid, typ, payload, now); err != nil {
			return err
		}
	}
	log.Printf("[outbox] append %s for %d node(s)", typ, len(nodeIDs))
	return nil
}

// Poll returns events for the node with version > cursor (ordered).
func (o *Outbox) Poll(nodeID, cursor int64, limit int) ([]Event, error) {
	rows, err := o.db.Query(
		`SELECT version, node_id, type, payload FROM outbox
		 WHERE version > ? AND (node_id = 0 OR node_id = ?)
		 ORDER BY version ASC LIMIT ?`, cursor, nodeID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var evs []Event
	for rows.Next() {
		var e Event
		if err := rows.Scan(&e.Version, &e.NodeID, &e.Type, &e.Payload); err != nil {
			return nil, err
		}
		evs = append(evs, e)
	}
	return evs, rows.Err()
}

// MaxVersion returns the current largest version (0 if empty).
func (o *Outbox) MaxVersion() (int64, error) {
	var v int64
	err := o.db.QueryRow(`SELECT COALESCE(MAX(version),0) FROM outbox`).Scan(&v)
	return v, err
}

// GC deletes old events (call periodically; keeps the table small on local deployments).
func (o *Outbox) GC(olderThan time.Duration) {
	cutoff := time.Now().UTC().Add(-olderThan).Format(time.RFC3339)
	if _, err := o.db.Exec(`DELETE FROM outbox WHERE created_at < ?`, cutoff); err == nil {
		log.Printf("[outbox] gc events older than %s", olderThan)
	}
}
